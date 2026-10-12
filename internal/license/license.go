// Package license 负责客户端卡密授权：设备指纹、向卡密管理系统激活/心跳续期、
// 本地持久化授权令牌，并向转发流程提供当前令牌。
//
// 校验的最终裁决在中继服务器（用共享密钥验签 + 吊销列表），本地仅负责持有与上报。
package license

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/applog"
	"bailan-relay/internal/netdns"
	"bailan-relay/internal/sysutil"
)

// State 是持久化到磁盘的授权状态。
type State struct {
	CardKey      string     `json:"card_key"`
	DeviceID     string     `json:"device_id"`
	DeviceName   string     `json:"device_name"`
	Token        string     `json:"token"`
	TokenExp     time.Time  `json:"token_exp"`
	CardExp      *time.Time `json:"card_exp"`
	DurationDays int        `json:"duration_days"`
	Activated    bool       `json:"activated"`
	LastMessage  string     `json:"last_message"`

	// 余额计费：由后台随激活/心跳/余额查询下发，仅用于展示（扣费裁决在后台）。
	Balance        float64 `json:"balance"`
	BillingEnabled bool    `json:"billing_enabled"`
	Currency       string  `json:"currency"`
}

// Manager 管理授权生命周期。
type Manager struct {
	apiURL     string
	statePath  string
	deviceName string
	log        *applog.Logger

	mu     sync.RWMutex
	state  State
	client *http.Client

	// refreshMu 串行化续期，避免多个并发请求同时打爆心跳接口。
	refreshMu     sync.Mutex
	lastRefreshAt time.Time
	lastRefreshOK bool

	stopOnce sync.Once
	stopCh   chan struct{}
	started  bool

	// userToken 返回当前账号令牌；激活/心跳必须携带，后台据此把卡密绑定到账号。
	userToken func() string
}

// SetUserTokenProvider 注入账号令牌提供者。
func (m *Manager) SetUserTokenProvider(fn func() string) { m.userToken = fn }

func (m *Manager) accountToken() string {
	if m.userToken == nil {
		return ""
	}
	return m.userToken()
}

type tokenResponse struct {
	OK             bool       `json:"ok"`
	Token          string     `json:"token"`
	TokenExpiresAt time.Time  `json:"token_expires_at"`
	CardExpiresAt  *time.Time `json:"card_expires_at"`
	DurationDays   int        `json:"duration_days"`
	Message        string     `json:"message"`

	Balance        float64 `json:"balance"`
	BillingEnabled bool    `json:"billing_enabled"`
	Currency       string  `json:"currency"`
}

// applyBalanceLocked 把后台下发的余额信息写入状态（调用方须持有写锁）。
func (m *Manager) applyBalanceLocked(balance float64, enabled bool, currency string) {
	m.state.Balance = balance
	m.state.BillingEnabled = enabled
	if currency != "" {
		m.state.Currency = currency
	}
}

// New 创建管理器：加载已保存的授权、计算设备指纹。
// resolver 提供硬化拨号（真实 IP + TUN 绕过），防止代理/隧道环境下心跳失联。
func New(apiURL, statePath string, resolver *netdns.Resolver, log *applog.Logger) *Manager {
	m := &Manager{
		apiURL:     strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		statePath:  statePath,
		deviceName: hostname(),
		log:        log,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{DialContext: resolver.DialContext(10 * time.Second)},
		},
		stopCh: make(chan struct{}),
	}
	m.load()
	if m.state.DeviceID == "" {
		m.state.DeviceID = deviceFingerprint()
	}
	return m
}

// SetAPIURL 更新管理系统地址（配置变更时调用）。
func (m *Manager) SetAPIURL(url string) {
	m.mu.Lock()
	m.apiURL = strings.TrimRight(strings.TrimSpace(url), "/")
	m.mu.Unlock()
}

func (m *Manager) load() {
	raw, err := os.ReadFile(m.statePath)
	if err != nil {
		return
	}
	var st State
	if json.Unmarshal(raw, &st) == nil {
		m.state = st
	}
}

func (m *Manager) persistLocked() {
	_ = os.MkdirAll(filepath.Dir(m.statePath), 0o755)
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(m.statePath, append(data, '\n'), 0o600)
}

// Activate 用卡密激活并绑定当前设备。
func (m *Manager) Activate(cardKey string) (bool, string) {
	cardKey = strings.ToUpper(strings.TrimSpace(cardKey))
	if cardKey == "" {
		return false, "请输入卡密"
	}
	m.mu.RLock()
	apiURL, deviceID, deviceName := m.apiURL, m.state.DeviceID, m.deviceName
	m.mu.RUnlock()
	if apiURL == "" {
		return false, "未配置授权服务器地址"
	}
	userToken := m.accountToken()
	if userToken == "" {
		return false, "请先登录账号"
	}

	body := map[string]string{"card_key": cardKey, "device_id": deviceID, "device_name": deviceName}
	resp, err := m.post(apiURL+"/api/license/activate", userToken, body)
	if err != nil {
		return false, err.Error()
	}
	if !resp.OK {
		m.mu.Lock()
		m.state.LastMessage = resp.Message
		m.persistLocked()
		m.mu.Unlock()
		return false, resp.Message
	}

	m.mu.Lock()
	m.state.CardKey = cardKey
	m.state.Token = resp.Token
	m.state.TokenExp = resp.TokenExpiresAt
	m.state.CardExp = resp.CardExpiresAt
	m.state.DurationDays = resp.DurationDays
	m.state.Activated = true
	m.state.LastMessage = resp.Message
	m.applyBalanceLocked(resp.Balance, resp.BillingEnabled, resp.Currency)
	m.persistLocked()
	m.mu.Unlock()

	if m.log != nil {
		m.log.Log("[授权] 卡密激活成功，有效期至 " + expText(resp.CardExpiresAt))
	}
	return true, "激活成功"
}

// Refresh 心跳续期。返回 (是否有效, 消息)。
func (m *Manager) Refresh() (bool, string) {
	m.mu.RLock()
	apiURL, cardKey, deviceID, activated := m.apiURL, m.state.CardKey, m.state.DeviceID, m.state.Activated
	m.mu.RUnlock()
	if !activated || cardKey == "" {
		return false, "尚未激活"
	}
	if apiURL == "" {
		return false, "未配置授权服务器地址"
	}
	// 未登录时不发心跳：避免后台 401 被误判为卡密失效而清掉本地授权。
	userToken := m.accountToken()
	if userToken == "" {
		return m.Valid(), "请先登录账号"
	}
	body := map[string]string{"card_key": cardKey, "device_id": deviceID}
	resp, err := m.post(apiURL+"/api/license/heartbeat", userToken, body)
	if err != nil {
		// 网络问题或账号令牌失效不立即注销，沿用旧令牌（可能仍在有效期内）。
		msg := err.Error()
		m.mu.Lock()
		m.state.LastMessage = msg
		m.persistLocked()
		m.mu.Unlock()
		if m.log != nil {
			m.log.Log("[授权] 心跳失败：" + err.Error())
		}
		return m.Valid(), msg
	}
	if !resp.OK {
		m.mu.Lock()
		m.state.Activated = false
		m.state.Token = ""
		m.state.LastMessage = resp.Message
		m.persistLocked()
		m.mu.Unlock()
		if m.log != nil {
			m.log.Log("[授权] 卡密已失效：" + resp.Message)
		}
		return false, resp.Message
	}
	m.mu.Lock()
	m.state.Token = resp.Token
	m.state.TokenExp = resp.TokenExpiresAt
	m.state.CardExp = resp.CardExpiresAt
	m.state.DurationDays = resp.DurationDays
	m.state.Activated = true
	m.state.LastMessage = "ok"
	m.applyBalanceLocked(resp.Balance, resp.BillingEnabled, resp.Currency)
	m.persistLocked()
	m.mu.Unlock()
	return true, "ok"
}

// EnsureFresh 若令牌即将过期则同步续期，供启动闸门调用。
func (m *Manager) EnsureFresh() (bool, string) {
	m.mu.RLock()
	activated := m.state.Activated
	tokenExp := m.state.TokenExp
	hasToken := m.state.Token != ""
	m.mu.RUnlock()
	if !activated {
		return false, "尚未激活卡密"
	}
	if !hasToken || time.Until(tokenExp) < 5*time.Minute {
		return m.Refresh()
	}
	if !m.Valid() {
		return false, "卡密已过期"
	}
	return true, "ok"
}

// Valid 返回当前授权是否有效（已激活且卡密未过期）。
func (m *Manager) Valid() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.state.Activated {
		return false
	}
	if m.state.CardExp != nil && m.state.CardExp.Before(time.Now()) {
		return false
	}
	return true
}

// HeaderValues 返回转发时要携带的授权头（令牌 / 卡密 / 设备）。
// 令牌已过期/缺失时同步续期后再取新令牌，避免本次请求带着死令牌被中继拒绝；
// 仅是临期（<5 分钟）则异步续期，不阻塞当前请求。
func (m *Manager) HeaderValues() (token, cardKey, deviceID string) {
	m.mu.RLock()
	token, cardKey, deviceID = m.state.Token, m.state.CardKey, m.state.DeviceID
	tokenExp := m.state.TokenExp
	activated := m.state.Activated
	m.mu.RUnlock()
	if !activated {
		return token, cardKey, deviceID
	}
	if token == "" || !time.Now().Before(tokenExp) {
		if m.refreshSerial() {
			m.mu.RLock()
			token = m.state.Token
			m.mu.RUnlock()
		}
		return token, cardKey, deviceID
	}
	if time.Until(tokenExp) < 5*time.Minute {
		go m.refreshSerial()
	}
	return token, cardKey, deviceID
}

// refreshCooldown 一次续期尝试后的静默期：后端故障时防止排队请求
// 在 refreshMu 上串行地各自再卡一个完整超时（N 个请求 = N×10s 雪崩）。
const refreshCooldown = 20 * time.Second

// refreshSerial 串行执行心跳续期：多个并发请求合并为一次真实刷新；
// 距上次尝试不足 refreshCooldown 时直接复用结果，静默期内不发请求。
func (m *Manager) refreshSerial() bool {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if time.Since(m.lastRefreshAt) < refreshCooldown {
		return m.lastRefreshOK
	}
	m.lastRefreshAt = time.Now()
	ok, _ := m.Refresh()
	m.lastRefreshOK = ok
	return ok
}

// Deactivate 清除本地授权（退出登录/换卡）。
func (m *Manager) Deactivate() {
	m.mu.Lock()
	deviceID := m.state.DeviceID
	m.state = State{DeviceID: deviceID}
	m.persistLocked()
	m.mu.Unlock()
}

// Status 返回给前端展示的授权状态。
func (m *Manager) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	valid := m.state.Activated && (m.state.CardExp == nil || m.state.CardExp.After(time.Now()))
	masked := maskCode(m.state.CardKey)
	var cardExp string
	if m.state.CardExp != nil {
		cardExp = m.state.CardExp.Local().Format("2006-01-02 15:04")
	}
	var remainDays int
	if m.state.CardExp != nil {
		remainDays = int(time.Until(*m.state.CardExp).Hours() / 24)
		if remainDays < 0 {
			remainDays = 0
		}
	}
	return map[string]any{
		"activated":     m.state.Activated,
		"valid":         valid,
		"card_key":      masked,
		"device_id":     m.state.DeviceID,
		"device_name":   m.state.DeviceName,
		"card_exp":      cardExp,
		"remain_days":   remainDays,
		"duration_days": m.state.DurationDays,
		"message":       m.state.LastMessage,

		"billing_enabled": m.state.BillingEnabled,
		"balance":         m.state.Balance,
		"balance_text":    balanceText(m.state.Balance, m.state.Currency),
		"currency":        m.state.Currency,
	}
}

func balanceText(balance float64, currency string) string {
	if currency == "" {
		currency = "¥"
	}
	return fmt.Sprintf("%s%.4f", currency, balance)
}

// bearer 返回当前授权令牌与接口地址；未激活时 ok=false。
func (m *Manager) bearer() (apiURL, token string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.state.Activated || m.state.Token == "" || m.apiURL == "" {
		return "", "", false
	}
	return m.apiURL, m.state.Token, true
}

// RefreshBalance 向后台拉取最新余额（高级模型扣费后保持界面读数接近实时）。静默失败。
func (m *Manager) RefreshBalance() {
	apiURL, token, ok := m.bearer()
	if !ok {
		return
	}
	var out tokenResponse
	if err := m.doJSON(http.MethodGet, apiURL+"/api/license/balance", token, nil, &out); err != nil || !out.OK {
		return
	}
	m.mu.Lock()
	m.applyBalanceLocked(out.Balance, out.BillingEnabled, out.Currency)
	m.persistLocked()
	m.mu.Unlock()
}

// Redeem 用充值卡密给当前授权的卡密充值。返回 (是否成功, 提示)。
func (m *Manager) Redeem(topupCode string) (bool, string) {
	topupCode = strings.ToUpper(strings.TrimSpace(topupCode))
	if topupCode == "" {
		return false, "请输入充值码"
	}
	// 令牌过期时先续期，避免充值码被用掉前才发现授权失效。
	m.refreshIfNearExpiry()
	apiURL, token, ok := m.bearer()
	if !ok {
		return false, "请先激活卡密再充值"
	}
	var out struct {
		tokenResponse
		Detail string `json:"detail"`
	}
	err := m.doJSON(http.MethodPost, apiURL+"/api/license/redeem", token, map[string]string{"topup_code": topupCode}, &out)
	if err != nil {
		return false, "充值失败：" + err.Error()
	}
	if out.Detail != "" && !out.OK {
		return false, out.Detail
	}
	if out.OK {
		m.mu.Lock()
		m.applyBalanceLocked(out.Balance, out.BillingEnabled, out.Currency)
		m.persistLocked()
		m.mu.Unlock()
	}
	msg := out.Message
	if msg == "" {
		if out.OK {
			msg = "充值成功"
		} else {
			msg = "充值失败"
		}
	}
	return out.OK, msg
}

// refreshIfNearExpiry 令牌临近过期（<5 分钟）且授权有效时同步续期一次。
func (m *Manager) refreshIfNearExpiry() {
	m.mu.RLock()
	nearExpiry := time.Until(m.state.TokenExp) < 5*time.Minute
	m.mu.RUnlock()
	if nearExpiry && m.Valid() {
		m.refreshSerial()
	}
}

var (
	// ErrNotActivated 本机尚未激活卡密（无可用令牌）。
	ErrNotActivated = errors.New("请先激活卡密后再查看用量")
	// ErrUsageUnsupported 后台版本较旧，没有用量查询接口。
	ErrUsageUnsupported = errors.New("服务端暂不支持用量查询，请联系管理员升级")
)

// Usage 查询当前卡密的用量汇总、按天分模型花费与调用明细，响应 JSON 原样返回。
// days 取 1..90（默认 30），page ≥ 1，pageSize 取 1..100（默认 20）。
func (m *Manager) Usage(days, page, pageSize int) (json.RawMessage, error) {
	if days <= 0 {
		days = 30
	} else if days > 90 {
		days = 90
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 100
	}
	m.refreshIfNearExpiry()
	apiURL, token, ok := m.bearer()
	if !ok {
		return nil, ErrNotActivated
	}
	query := fmt.Sprintf("/api/license/usage?days=%d&page=%d&page_size=%d", days, page, pageSize)
	var out json.RawMessage
	err := m.doJSON(http.MethodGet, apiURL+query, token, nil, &out)
	var se *statusError
	// 令牌被后台判定失效（如刚轮换密钥）时续期后重试一次。
	if errors.As(err, &se) && se.Status == http.StatusUnauthorized && m.refreshSerial() {
		if apiURL, token, ok = m.bearer(); !ok {
			return nil, ErrNotActivated
		}
		out = nil
		err = m.doJSON(http.MethodGet, apiURL+query, token, nil, &out)
	}
	if errors.As(err, &se) && se.Status == http.StatusNotFound && (se.Detail == "" || se.Detail == "Not Found") {
		return nil, ErrUsageUnsupported
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// statusError 是授权服务器返回的非 2xx 响应；Detail 为 FastAPI 的 detail 文本（可能为空）。
type statusError struct {
	Status int
	Detail string
}

func (e *statusError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return fmt.Sprintf("授权服务器返回异常（HTTP %d）", e.Status)
}

// doJSON 带 Bearer 令牌的 JSON 请求；HTTP 非 2xx 时若响应带 detail 则作为错误提示返回。
func (m *Manager) doJSON(method, url, token string, body any, out any) error {
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接授权服务器：%w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		se := &statusError{Status: resp.StatusCode}
		var e struct {
			Detail any `json:"detail"`
		}
		if json.Unmarshal(raw, &e) == nil {
			if s, ok := e.Detail.(string); ok {
				se.Detail = s
			}
		}
		return se
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("授权服务器返回异常（HTTP %d）", resp.StatusCode)
	}
	return nil
}

// Start 启动后台心跳循环，保持令牌新鲜并及时感知吊销。
func (m *Manager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	go func() {
		// 启动时若已激活，先续期一次。
		if m.Valid() {
			m.refreshSerial()
		}
		const okInterval, failInterval = 10 * time.Minute, 2 * time.Minute
		ticker := time.NewTicker(okInterval)
		defer ticker.Stop()
		balanceTicker := time.NewTicker(time.Minute)
		defer balanceTicker.Stop()
		for {
			select {
			case <-m.stopCh:
				return
			case <-balanceTicker.C:
				m.RefreshBalance()
			case <-ticker.C:
				m.mu.RLock()
				activated := m.state.Activated
				m.mu.RUnlock()
				if !activated {
					continue
				}
				// 心跳失败时缩短下次重试间隔，尽快自愈（成功则回到正常节奏）。
				if m.refreshSerial() {
					ticker.Reset(okInterval)
				} else {
					ticker.Reset(failInterval)
				}
			}
		}
	}()
}

// Stop 停止后台循环。
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// post 带账号令牌提交激活/心跳。非 2xx（如账号令牌失效 401）作为错误返回，
// 只有后台明确回 ok=false 才代表卡密本身被拒。
func (m *Manager) post(url, userToken string, body map[string]string) (*tokenResponse, error) {
	var out tokenResponse
	if err := m.doJSON(http.MethodPost, url, userToken, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ===== 设备指纹 =====

func deviceFingerprint() string {
	guid := machineGUID()
	seed := guid + "|" + hostname()
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])[:32]
}

func machineGUID() string {
	out, _, err := sysutil.RunHidden("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "MachineGuid") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					return fields[len(fields)-1]
				}
			}
		}
	}
	return ""
}

func hostname() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown"
}

func maskCode(code string) string {
	if code == "" {
		return ""
	}
	if len(code) <= 4 {
		return code
	}
	return code[:4] + "****" + code[len(code)-4:]
}

func expText(t *time.Time) string {
	if t == nil {
		return "永久"
	}
	return t.Local().Format("2006-01-02 15:04")
}
