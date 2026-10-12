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

	mu         sync.RWMutex
	domains    map[string]bool
	paths      map[string]bool
	stub       []string
	relayBase  string
	relayToken string
	model      string

	relayClient  *http.Client
	originClient *http.Client

	// tokenProvider 返回转发时携带的授权信息（令牌 / 卡密 / 设备）。
	tokenProvider func() (token, cardKey, deviceID string)

	runMu   sync.Mutex
	servers []*http.Server
	running bool

	reconnect reconnectState
	upload    uploader
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
	s.mu.Unlock()
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

func (s *Server) snapshot() (map[string]bool, map[string]bool, []string, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.domains, s.paths, s.stub, s.relayBase, s.relayToken
}
