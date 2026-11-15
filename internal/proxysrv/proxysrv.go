// Package proxysrv 是本地 HTTPS 拦截服务：hosts 把目标域名指到 127.0.0.1:443，
// 本服务终止 TLS 并直接处理请求（屏蔽遥测 / 本地空应答 / 转发中继 / 透传源站），
// 完全取代旧版依赖的 mitmproxy 中间人。逻辑移植自 proxy_addon.py。
package proxysrv

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/applog"
	"bailan-relay/internal/certstore"
	"bailan-relay/internal/config"
	"bailan-relay/internal/connectframe"
	"bailan-relay/internal/netdns"
)

// 屏蔽关键字：命中即回空 200（仅遥测/分析请求）。
var blockKeywords = []string{"Record", "Analytics"}

// 静默屏蔽（不写日志，避免刷屏）。
var silentBlockKeywords = []string{
	"RecordAnalyticsEvent", "BatchRecordAnalyticsEvents", "RecordEvent", "RecordAsyncTelemetry",
}

// policyPassthroughKeywords 是必须回官方源站的认证/策略接口，优先级高于本地空应答配置。
var policyPassthroughKeywords = []string{"GetUserJwt", "GetCliTeamSettings"}

// 转发中继时需删除的 IDE 指纹 headers。
var headersToRemove = []string{
	"cookie", "origin", "referer",
	"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
	"sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site",
	"x-requested-with", "x-client-info", "sentry-trace", "baggage",
	"expect",
}

var hopByHop = map[string]bool{
	"connection": true, "keep-alive": true, "proxy-authenticate": true,
	"proxy-authorization": true, "te": true, "trailer": true,
	"transfer-encoding": true, "upgrade": true,
}

// Server 是拦截服务。
type Server struct {
	ca       *certstore.CA
	resolver *netdns.Resolver
	log      *applog.Logger

	mu            sync.RWMutex
	domains       map[string]bool
	paths         map[string]bool
	stub          []string
	relayBase     string
	relayToken    string
	model         string
	modelOpts     map[string]config.ModelOption
	sessionModels map[string]sessionChoice
	relayClient   *http.Client
	originClient  *http.Client

	// tokenProvider 返回转发时携带的授权信息（令牌 / 卡密 / 设备）。
	tokenProvider func() (token, cardKey, deviceID string)

	runMu   sync.Mutex
	servers []*http.Server
	running bool

	reconnect reconnectState
	upload    uploader

	catalogMu    sync.Mutex
	catalogStamp string
}

// SetTokenProvider 注入授权令牌提供者（由 license.Manager 提供）。
func (s *Server) SetTokenProvider(fn func() (token, cardKey, deviceID string)) {
	s.mu.Lock()
	s.tokenProvider = fn
	s.mu.Unlock()
}

func (s *Server) licenseHeaders() (token, cardKey, deviceID string) {
	s.mu.RLock()
	fn := s.tokenProvider
	s.mu.RUnlock()
	if fn == nil {
		return "", "", ""
	}
	return fn()
}

// New 创建拦截服务。
func New(ca *certstore.CA, resolver *netdns.Resolver, log *applog.Logger) *Server {
	s := &Server{
		ca:       ca,
		resolver: resolver,
		log:      log,
		domains:  map[string]bool{},
		paths:    map[string]bool{},
	}
	// 显式拨号超时：默认 30s 太久（Windows connectex 约 21s 才放弃），
	// 10s 放弃交给 forwardRelay 重试，瞬时丢包恢复更快。
	// dialRelay 走自建 DNS 解析真实 IP + TUN 绕过（隧道存在时绑物理网卡）。
	s.relayClient = &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			ForceAttemptHTTP2:     true,
			DisableCompression:    true,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 0,
			WriteBufferSize:       4 * 1024,
			ReadBufferSize:        4 * 1024,
			DialContext:           s.dialRelay,
		},
	}
	// 透传源站的 transport：DialContext 把域名解析成真实 IP 再连（绕开 hosts 回环），
	// TLS 与 ALPN(h2/http1.1) 仍由 transport 按真实主机名协商与校验。
	originTransport := &http.Transport{
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 0,
		DialContext:           s.dialOrigin,
	}
	// 认证与团队策略请求与其它透传共用 originClient：ALPN 协商 h2 与源站协议一致。
	// 之前强制独立 HTTP/1.1 会让克隆的 TLS NextProtos 仍协商 h2、而传输栈按 h1 解析，
	// 源站回的 HTTP/2 SETTINGS 帧被当成 malformed response。
	s.originClient = &http.Client{Timeout: 0, Transport: originTransport}
	return s
}

// UpdateConfig 根据 cfg 刷新域名/路径/中继/模型等运行参数（运行中也可调用）。
func (s *Server) UpdateConfig(cfg config.Config) {
	domains := map[string]bool{}
	for _, d := range cfg.HijackDomains() {
		domains[d] = true
	}
	paths := map[string]bool{}
	for _, p := range cfg.TargetPaths {
		if p = strings.TrimSpace(p); p != "" {
			paths[p] = true
		}
	}
	stub := config.NormalizeStubPaths(cfg.StubPaths)
	base := ""
	if u, err := url.Parse(cfg.RelayServerURL); err == nil && u.Host != "" {
		base = u.Scheme + "://" + u.Host
	}
	s.mu.Lock()
	s.domains = domains
	s.paths = paths
	s.stub = stub
	s.relayBase = base
	s.relayToken = cfg.RelayAuthToken
	s.model = config.NormalizeModelID(cfg.OverrideModel)
	s.modelOpts = cloneModelOptions(cfg.ModelOptions)
	s.mu.Unlock()
}

// SetModelOption 更新某个模型的思考强度 / 上下文选择（set_model_option 用）。
func (s *Server) SetModelOption(model string, opt config.ModelOption) {
	s.mu.Lock()
	if s.modelOpts == nil {
		s.modelOpts = map[string]config.ModelOption{}
	}
	if opt == (config.ModelOption{}) {
		delete(s.modelOpts, model)
	} else {
		s.modelOpts[model] = opt
	}
	s.mu.Unlock()
}

// currentModelOption 返回模型当前生效的选择，已不在后台可选范围内的项自动丢弃。
func (s *Server) currentModelOption(model string) config.ModelOption {
	s.mu.RLock()
	opt := s.modelOpts[model]
	s.mu.RUnlock()
	return config.ValidModelOption(model, opt)
}

func cloneModelOptions(src map[string]config.ModelOption) map[string]config.ModelOption {
	out := make(map[string]config.ModelOption, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// SetModel 仅更新替换模型（set_model 用）。
func (s *Server) SetModel(model string) {
	s.mu.Lock()
	s.model = config.NormalizeModelID(model)
	s.mu.Unlock()
}

func (s *Server) currentModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

type sessionChoice struct {
	Model   string
	Effort  string
	Context int
}

// RememberSessionModel 记下某个会话点选的模型。转发时按请求里的会话编号取回。
func (s *Server) RememberSessionModel(sessionID, model, effort string, context int) {
	sessionID = strings.TrimSpace(sessionID)
	model = config.NormalizeModelID(model)
	if sessionID == "" || model == "" {
		return
	}
	s.mu.Lock()
	if s.sessionModels == nil {
		s.sessionModels = map[string]sessionChoice{}
	}
	choice := sessionChoice{Model: model, Effort: effort, Context: context}
	s.sessionModels[sessionID] = choice
	// 3.12.28 的请求里会把斜杠写成下划线：devin-acp_devin-cli_名字。
	if strings.Contains(sessionID, "/") {
		s.sessionModels[strings.ReplaceAll(sessionID, "/", "_")] = choice
	}
	// 请求里通常只有最后一段短名。同一会话再换模型时，短名必须跟着改，否则会一直用第一次选的模型。
	if i := strings.LastIndex(sessionID, "/"); i >= 0 && i < len(sessionID)-1 {
		s.sessionModels[sessionID[i+1:]] = choice
	}
	s.mu.Unlock()
}

func (s *Server) sessionChoice(ids []string) (sessionChoice, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 请求里经常同时出现好几个带横线的字符串。按出现顺序用第一条已经记住的编号，
	// 不能改拿更长的那条，否则另一个会话的完整名字会把当前会话盖掉。
	for _, id := range ids {
		choice, ok := s.sessionModels[id]
		if ok && choice.Model != "" {
			return choice, id, true
		}
	}
	var hitKey string
	var hit sessionChoice
	hits := 0
	for _, id := range ids {
		for key, choice := range s.sessionModels {
			if choice.Model == "" || !strings.HasSuffix(key, "/"+id) {
				continue
			}
			hits++
			hitKey = key
			hit = choice
		}
	}
	if hits == 1 {
		return hit, hitKey, true
	}
	return sessionChoice{}, "", false
}

// sessionChoiceFromBody 在请求体里找已经记住的会话名。
// 3.12.28 的后续消息不再走模型按钮，编号字段也对不上斜杠形式，正文里仍有会话名。
func (s *Server) sessionChoiceFromBody(body []byte) (sessionChoice, string, bool) {
	if len(body) == 0 {
		return sessionChoice{}, "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.sessionModels) == 0 {
		return sessionChoice{}, "", false
	}
	text := string(connectframe.MessagePayload(body))
	type hit struct {
		key    string
		choice sessionChoice
	}
	var hits []hit
	for key, choice := range s.sessionModels {
		if choice.Model == "" || len(key) < 8 || !strings.Contains(key, "-") {
			continue
		}
		forms := []string{key}
		if strings.Contains(key, "/") {
			forms = append(forms, strings.ReplaceAll(key, "/", "_"))
		}
		for _, form := range forms {
			if form != "" && strings.Contains(text, form) {
				hits = append(hits, hit{key: key, choice: choice})
				break
			}
		}
	}
	var kept []hit
	for _, h := range hits {
		covered := false
		for _, other := range hits {
			if other.key != h.key && (strings.HasSuffix(other.key, "/"+h.key) || strings.HasSuffix(other.key, "_"+h.key)) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, h)
		}
	}
	if len(kept) == 0 {
		return sessionChoice{}, "", false
	}
	first := kept[0].choice
	for _, h := range kept[1:] {
		if h.choice.Model != first.Model || h.choice.Effort != first.Effort || h.choice.Context != first.Context {
			return sessionChoice{}, "", false
		}
	}
	return first, kept[0].key, true
}

func (s *Server) snapshot() (map[string]bool, map[string]bool, []string, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.domains, s.paths, s.stub, s.relayBase, s.relayToken
}
