// Package controller 把各模块拼装成本地端逻辑，并提供与旧版 pywebview
// js_api 完全一致的方法（供 HTTP /api/* 调用）。对应旧版 WebAppController。
package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/account"
	"bailan-relay/internal/applog"
	"bailan-relay/internal/certstore"
	"bailan-relay/internal/config"
	"bailan-relay/internal/devin"
	"bailan-relay/internal/license"
	"bailan-relay/internal/netdns"
	"bailan-relay/internal/proxybypass"
	"bailan-relay/internal/proxysrv"
)

// Notice 是推送给前端的一次性提示。
type Notice struct {
	Sequence     int     `json:"sequence"`
	Message      string  `json:"message"`
	Tone         string  `json:"tone"`
	Action       *string `json:"action"`
	AutoNavigate bool    `json:"auto_navigate"`
}

// Controller 持有运行期状态。
type Controller struct {
	log      *applog.Logger
	ca       *certstore.CA
	resolver *netdns.Resolver
	proxy    *proxysrv.Server
	license  *license.Manager
	account  *account.Manager

	statusMu     sync.RWMutex
	status       map[string]any
	proxyState   string
	proxyMessage string
	noticeSeq    int

	opMu        sync.Mutex
	refreshMu   sync.Mutex
	relayClient *http.Client
	modelClient *http.Client

	modelsMu         sync.RWMutex
	models           []config.Model
	modelsAt         time.Time
	announcement     *clientAnnouncement
	update           *clientUpdate
	updateMu         sync.Mutex
	updateProgressMu sync.RWMutex
	updateProgress   updateProgressView

	shutdownOnce sync.Once
	shutdownCh   chan struct{}
	onExit       func()

	// 客户版「关闭窗口即退出」：pagehide 触发 closing 计时器，心跳可取消。
	closeMu    sync.Mutex
	closeTimer *time.Timer
}

// New 创建控制器并启动后台状态监控。
func New(log *applog.Logger, ca *certstore.CA, resolver *netdns.Resolver, proxy *proxysrv.Server) *Controller {
	c := &Controller{
		log:          log,
		ca:           ca,
		resolver:     resolver,
		proxy:        proxy,
		proxyState:   "idle",
		proxyMessage: "正在检测本地状态",
		// 探测/拉取后台配置的出站走硬化拨号（netdns 真实 IP + TUN 绕过），
		// 防止用户开系统代理/虚拟网卡时心跳、健康检查被 fake-ip 或隧道截获。
		relayClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{DialContext: resolver.DialContext(5 * time.Second)},
		},
		modelClient: &http.Client{
			Timeout:   8 * time.Second,
			Transport: &http.Transport{DialContext: resolver.DialContext(5 * time.Second)},
		},
		shutdownCh: make(chan struct{}),
	}
	c.status = map[string]any{
		"proxy_running":    false,
		"proxy_state":      "idle",
		"proxy_message":    c.proxyMessage,
		"proxy_has_web":    false,
		"relay_ok":         false,
		"relay_detail":     "正在检测",
		"relay_checking":   true,
		"cert_file_exists": nil,
		"cert_installed":   nil,
		"updated_at":       "",
		"summary":          "正在读取运行状态。",
		"activity":         map[string]any{"online": false},
		"notice":           nil,
	}
	go c.monitorStatus()
	go c.syncCatalogLoop()
	return c
}

// syncCatalogLoop 在后台周期同步模型目录，使「所选模型是否已被后台隐藏」的判断
// 不依赖界面是否打开（代理转发时据此拦截已不存在的模型）。
func (c *Controller) syncCatalogLoop() {
	for {
		c.clientBundle(false)
		select {
		case <-c.shutdownCh:
			return
		case <-time.After(15 * time.Second):
		}
	}
}

// SetOnExit 注册退出回调（用于 relaunch/关闭时清理）。
func (c *Controller) SetOnExit(fn func()) { c.onExit = fn }

// SetLicenseManager 注入卡密授权管理器，并把令牌提供者接入转发流程。
func (c *Controller) SetLicenseManager(m *license.Manager) {
	c.license = m
	if m != nil {
		c.proxy.SetTokenProvider(m.HeaderValues)
	}
}

func (c *Controller) licenseStatus() map[string]any {
	if c.license == nil {
		return map[string]any{"activated": false, "valid": false, "message": "授权模块未初始化"}
	}
	return c.license.Status()
}

// cancelPendingClose 取消「关闭窗口即退出」的待执行计时器（收到心跳时调用）。
func (c *Controller) cancelPendingClose() {
	c.closeMu.Lock()
	if c.closeTimer != nil {
		c.closeTimer.Stop()
		c.closeTimer = nil
	}
	c.closeMu.Unlock()
}

func (c *Controller) statusSnapshot() map[string]any {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()
	raw, _ := json.Marshal(c.status)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	out = exportStatusForEdition(out)
	out["license"] = c.licenseStatus()
	out["relay_reconnect"] = c.proxy.ReconnectStatus()
	return out
}

func (c *Controller) setProxyState(state, message string, running *bool) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.proxyState = state
	c.proxyMessage = message
	c.status["proxy_state"] = state
	c.status["proxy_message"] = message
	c.status["proxy_has_web"] = false
	if running != nil {
		c.status["proxy_running"] = *running
	}
	c.status["updated_at"] = time.Now().Format("15:04:05")
}

func (c *Controller) setNotice(message, tone string, action *string, autoNav bool) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.noticeSeq++
	c.status["notice"] = Notice{
		Sequence:     c.noticeSeq,
		Message:      message,
		Tone:         tone,
		Action:       action,
		AutoNavigate: autoNav,
	}
}

func (c *Controller) summary(cfg config.Config, detail string) string {
	if config.IsCustomer() {
		model := config.ModelLabel(config.NormalizeModelID(cfg.OverrideModel))
		if c.proxy.Running() {
			return "本地加速已开启 · 模型 " + model
		}
		return "本地加速未开启 · 模型 " + model
	}
	url := cfg.RelayServerURL
	if url == "" {
		url = "未配置"
	}
	domains := strings.Join(cfg.HijackDomains(), "、")
	if domains == "" {
		domains = "—"
	}
	model := config.NormalizeModelID(cfg.OverrideModel)
	return fmt.Sprintf("中继 %s · hosts %s · 模型 %s · %s", url, domains, model, detail)
}

func (c *Controller) checkRelayServer() (bool, string) {
	relayURL := config.Load().RelayServerURL
	if relayURL == "" {
		return false, "未配置"
	}
	health := strings.TrimRight(relayURL, "/") + "/health"
	start := time.Now()
	resp, err := c.relayClient.Get(health)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	elapsed := time.Since(start).Milliseconds()
	return resp.StatusCode == 200, fmt.Sprintf("HTTP %d · %dms", resp.StatusCode, elapsed)
}

func (c *Controller) fetchRelayHealth() map[string]any {
	relayURL := config.Load().RelayServerURL
	if relayURL == "" {
		return nil
	}
	health := strings.TrimRight(relayURL, "/") + "/health"
	resp, err := c.relayClient.Get(health)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	var data map[string]any
	if json.Unmarshal(body, &data) != nil {
		return nil
	}
	return data
}

func (c *Controller) refreshStatusData(includeCertificate bool) map[string]any {
	if !c.refreshMu.TryLock() {
		return c.statusSnapshot()
	}
	defer c.refreshMu.Unlock()

	c.statusMu.Lock()
	c.status["relay_checking"] = true
	c.statusMu.Unlock()

	running := c.proxy.Running()
	relayOK, relayDetail := c.checkRelayServer()
	cfg := config.Load()
	var activity map[string]any
	if relayOK {
		activity = c.fetchRelayHealth()
	}
	var certExists, certInstalled *bool
	if includeCertificate {
		fe := certstore.FileExists()
		ci, _ := certstore.IsInstalled()
		certExists = &fe
		certInstalled = &ci
	}

	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	if c.proxyState != "starting" && c.proxyState != "stopping" {
		if running {
			c.proxyState = "running"
			if c.proxyMessage == "正在检测本地状态" || c.proxyMessage == "代理已停止" || c.proxyMessage == "等待启动" {
				c.proxyMessage = "应用级拦截正在运行"
			}
		} else if c.proxyState == "running" {
			c.proxyState = "idle"
			c.proxyMessage = "代理进程已停止"
		} else if c.proxyState != "error" {
			c.proxyState = "idle"
			if c.proxyMessage == "正在检测本地状态" {
				c.proxyMessage = "等待启动"
			}
		}
	}
	act := map[string]any{"online": activity != nil}
	for k, v := range activity {
		act[k] = v
	}
	c.status["proxy_running"] = running
	c.status["proxy_state"] = c.proxyState
	c.status["proxy_message"] = c.proxyMessage
	c.status["proxy_has_web"] = false
	c.status["relay_ok"] = relayOK
	c.status["relay_detail"] = relayDetail
	c.status["relay_checking"] = false
	c.status["updated_at"] = time.Now().Format("15:04:05")
	c.status["summary"] = c.summary(cfg, relayDetail)
	c.status["activity"] = act
	if includeCertificate {
		c.status["cert_file_exists"] = certExists
		c.status["cert_installed"] = certInstalled
	}
	raw, _ := json.Marshal(c.status)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	out = exportStatusForEdition(out)
	out["license"] = c.licenseStatus()
	out["relay_reconnect"] = c.proxy.ReconnectStatus()
	return out
}

func (c *Controller) monitorStatus() {
	var certificateDue time.Time
	var bypassDue time.Time
	for {
		select {
		case <-c.shutdownCh:
			return
		default:
		}
		includeCert := time.Now().After(certificateDue)
		c.refreshStatusData(includeCert)
		if includeCert {
			certificateDue = time.Now().Add(30 * time.Second)
		}
		// 守护代理绕过列表：Clash 等工具开关系统代理时会重写 ProxyOverride，
		// 把目标域名冲掉——拦截运行期间周期性补回。
		if c.proxy.Running() && time.Now().After(bypassDue) {
			bypassDue = time.Now().Add(15 * time.Second)
			if changed, err := proxybypass.Apply(config.Load().HijackDomains()); err == nil && changed {
				c.log.Log("[Manager] 代理绕过条目被改写，已重新补回")
			}
		}
		select {
		case <-c.shutdownCh:
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (c *Controller) publicConfig(cfg config.Config) map[string]any {
	found := devin.DiscoverExecutable(cfg.DevinExecutable)
	executable := cfg.DevinExecutable
	if found != "" {
		executable = found
		if executable != cfg.DevinExecutable {
			cfg = config.Save(func(x *config.Config) { x.DevinExecutable = executable })
		}
	}
	m := map[string]any{
		"relay_server_url":       cfg.RelayServerURL,
		"devin_executable":       executable,
		"relay_auth_token":       cfg.RelayAuthToken,
		"dns_servers":            config.NormalizeDNSServers(cfg.DNSServers),
		"stub_paths":             strSlice(cfg.StubPaths),
		"target_domains":         cfg.HijackDomains(),
		"target_paths":           strSlice(cfg.TargetPaths),
		"override_model":         config.NormalizeModelID(cfg.OverrideModel),
		"show_mitmweb":           false,
		"restart_devin_on_start": cfg.RestartDevinOnStart,
		"license_api_url":        cfg.LicenseAPIURL,
	}
	if config.IsCustomer() {
		// 客户版：不向前端返回任何核心配置。
		m["relay_server_url"] = ""
		m["relay_auth_token"] = ""
		m["dns_servers"] = []string{}
		m["stub_paths"] = []string{}
		m["target_domains"] = []string{}
		m["target_paths"] = []string{}
		m["license_api_url"] = ""
	}
	return m
}

// detailForCustomer 把中继连通性映射成不含地址的通用文案。
func detailForCustomer(ok bool) string {
	if ok {
		return "已连接"
	}
	return "未连接"
}

// exportStatusForEdition 在客户版下对状态快照做脱敏：
// 去掉中继连通性细节里的地址/错误原文，去掉活动里的请求路径等实现细节。
func exportStatusForEdition(out map[string]any) map[string]any {
	if !config.IsCustomer() || out == nil {
		return out
	}
	checking, _ := out["relay_checking"].(bool)
	ok, _ := out["relay_ok"].(bool)
	switch {
	case checking:
		out["relay_detail"] = "检测中"
	default:
		out["relay_detail"] = detailForCustomer(ok)
	}
	if act, isMap := out["activity"].(map[string]any); isMap {
		safe := map[string]any{"online": act["online"]}
		for _, k := range []string{"queue_count", "jwt_available", "last_request_time", "is_processing", "modify_count"} {
			if v, has := act[k]; has {
				safe[k] = v
			}
		}
		out["activity"] = safe
	}
	return out
}

// Shutdown 停止后台任务、关闭拦截服务并恢复 hosts。
func (c *Controller) Shutdown() {
	c.shutdownOnce.Do(func() {
		close(c.shutdownCh)
		c.proxy.Stop()
		if c.onExit != nil {
			c.onExit()
		}
	})
}

func strSlice(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
