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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/applog"
	"bailan-relay/internal/netdns"
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
	CardKey        string     `json:"card_key"`

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

// Activate 用卡密开通效益包。
// 本机还没有钱包时，这张卡密绑定本机、自己成为钱包；已有钱包时，卡密的天数和附带余额并入钱包，
// 钱包里原有的余额不动。所以成功后在用的卡密以后台返回的为准，不一定是输入的这张。
func (m *Manager) Activate(cardKey string) (bool, string) {
	return m.activate(cardKey, true)
}

// activate 提交卡密并保存后台返回的钱包与令牌。
// redeem=false 只把这张卡绑定到本机（充值后自动开通用），不走兑换并入。
func (m *Manager) activate(cardKey string, redeem bool) (bool, string) {
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
	var resp *tokenResponse
	var err error
	if redeem {
		resp, err = m.post(apiURL+"/api/license/pack/redeem", userToken, body)
		// 后台还没有商城接口：退回原来的激活接口，输入的卡密自己就是钱包。
		redeem = !endpointMissing(err)
	}
	if !redeem {
		resp, err = m.post(apiURL+"/api/license/activate", userToken, body)
	}
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
	wallet := strings.ToUpper(strings.TrimSpace(resp.CardKey))
	if wallet == "" {
		wallet = cardKey
	}
	m.commitLicense(wallet, resp)
	if m.log != nil {
		m.log.Log("[授权] 卡密兑换成功，效益包" + expText(resp.CardExpiresAt))
	}
	if msg := strings.TrimSpace(resp.Message); redeem && msg != "" {
		return true, msg
	}
	return true, "激活成功"
}

// endpointMissing 判断错误是不是「后台没有这个接口」（旧版后台），而不是接口自己返回的 404。
func endpointMissing(err error) bool {
	var se *statusError
	return errors.As(err, &se) && se.Status == http.StatusNotFound && (se.Detail == "" || se.Detail == "Not Found")
}

// Restore 本机数据清掉后，按设备指纹找回这台电脑已绑定的卡。
func (m *Manager) Restore() (bool, string) {
	if m.Valid() {
		return true, "已激活"
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
	resp, err := m.post(apiURL+"/api/license/restore", userToken, map[string]string{
		"device_id": deviceID, "device_name": deviceName,
	})
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
	key := strings.ToUpper(strings.TrimSpace(resp.CardKey))
	m.commitLicense(key, resp)
	if m.log != nil {
		m.log.Log("[授权] 已按本机设备恢复钱包，效益包" + expText(resp.CardExpiresAt))
	}
	return true, "已恢复本机授权"
}

// commitLicense 保存后台签发的令牌和钱包状态。cardKey 为空时用后台返回的卡密；两者都没有则沿用本机现有的。
func (m *Manager) commitLicense(cardKey string, resp *tokenResponse) {
	if cardKey == "" {
		cardKey = strings.ToUpper(strings.TrimSpace(resp.CardKey))
	}
	m.mu.Lock()
	if cardKey != "" {
		m.state.CardKey = cardKey
	}
	m.state.Token = resp.Token
	m.state.TokenExp = resp.TokenExpiresAt
	m.state.CardExp = resp.CardExpiresAt
	m.state.DurationDays = resp.DurationDays
	m.state.Activated = true
	m.state.LastMessage = resp.Message
	m.applyBalanceLocked(resp.Balance, resp.BillingEnabled, resp.Currency)
	m.persistLocked()
	m.mu.Unlock()
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

// Valid 返回本机钱包当前是否可用（必须有卡密且激活过）。
func (m *Manager) Valid() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 必须有卡密且激活过
	if m.state.CardKey == "" || !m.state.Activated {
		return false
	}
	// 效益包到期但还有余额时仍可用：高级模型继续从余额扣费
	if m.state.CardExp != nil && m.state.CardExp.Before(time.Now()) {
		return m.state.Balance > 0
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

// Status 返回给前端展示的钱包状态。
// 钱包里两样东西互不影响：pack_active = 效益包在有效期内（可用免费模型），balance = 余额（高级模型扣费）。
// 没开通过效益包（只有余额）时 card_exp 为空，pack_active 与 pack_expired 都是 false。
func (m *Manager) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 必须激活过才显示已激活,前端根据 valid 判断是否真的有效
	activated := m.state.Activated
	valid := activated && (m.state.CardExp == nil || m.state.CardExp.After(time.Now()) || m.state.Balance > 0)
	packActive := activated && m.state.CardExp != nil && m.state.CardExp.After(time.Now())
	packExpired := activated && m.state.CardExp != nil && !packActive
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
		"activated":     activated,
		"valid":         valid,
		"pack_active":   packActive,
		"pack_expired":  packExpired,
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

// userBearer 返回用户账号令牌与接口地址;登录后总是 ok=true。
func (m *Manager) userBearer() (apiURL, token string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	token = m.accountToken()
	if token == "" || m.apiURL == "" {
		return "", "", false
	}
	return m.apiURL, token, true
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
// 不再依赖激活状态:使用用户账号 token,可选指定卡密 code,不填则自动选最近激活的卡密。
func (m *Manager) Redeem(topupCode string) (bool, string) {
	topupCode = strings.ToUpper(strings.TrimSpace(topupCode))
	if topupCode == "" {
		return false, "请输入充值码"
	}
	// 用用户账号 token 而不是卡密 token,这样未激活也能充值
	apiURL, userToken, userOK := m.userBearer()
	if !userOK {
		return false, "请先登录账号"
	}
	// 用当前激活的卡密 code(如果有的话)
	m.mu.RLock()
	cardKey, deviceID, deviceName := m.state.CardKey, m.state.DeviceID, m.deviceName
	m.mu.RUnlock()
	var out struct {
		tokenResponse
		Detail string `json:"detail"`
	}
	body := map[string]string{"topup_code": topupCode, "device_id": deviceID, "device_name": deviceName}
	if cardKey != "" {
		body["card_key"] = cardKey
	}
	err := m.doJSON(http.MethodPost, apiURL+"/api/license/user-redeem", userToken, body, &out)
	if err != nil {
		return false, "充值失败：" + err.Error()
	}
	if out.Detail != "" && !out.OK {
		return false, out.Detail
	}
	if out.OK {
		m.mu.Lock()
		m.applyBalanceLocked(out.Balance, out.BillingEnabled, out.Currency)
		activated := m.state.Activated
		m.persistLocked()
		m.mu.Unlock()
		if !activated && out.CardKey != "" {
			// 余额所在的卡就是钱包，直接绑定到本机即可，不走兑换并入。
			if aok, amsg := m.activate(out.CardKey, false); !aok {
				return true, "充值成功，但钱包绑定本机失败：" + amsg
			}
		}
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
	// ErrNotActivated 本机还没有可用的钱包（没开通效益包、也没充值，无可用令牌）。
	ErrNotActivated = errors.New("请先在商城开通效益包或充值余额")
	// ErrUsageUnsupported 后台版本较旧，没有用量查询接口。
	ErrUsageUnsupported = errors.New("服务端暂不支持用量查询，请联系管理员升级")
	// ErrPoolUnsupported 后台版本较旧，没有号池状态接口。
	ErrPoolUnsupported = errors.New("服务端暂不支持号池状态，请联系管理员升级")
	// ErrShopUnsupported 后台版本较旧，没有商城接口。
	ErrShopUnsupported = errors.New("服务端暂不支持商城，请联系管理员升级")
)

// Shop 查询商城：在售的效益包，以及本机钱包的效益包到期时间和余额。响应 JSON 原样返回。
// 只需要账号登录——还没有钱包的新用户也要能看到商城。
func (m *Manager) Shop() (json.RawMessage, error) {
	apiURL, token, ok := m.userBearer()
	if !ok {
		return nil, errors.New("请先登录账号")
	}
	m.mu.RLock()
	deviceID := m.state.DeviceID
	m.mu.RUnlock()
	var out json.RawMessage
	err := m.doJSON(http.MethodGet, apiURL+"/api/license/shop?device_id="+url.QueryEscape(deviceID), token, nil, &out)
	if endpointMissing(err) {
		return nil, ErrShopUnsupported
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BuyPack 用本机钱包的余额购买效益包。返回 (是否成功, 提示)。
// 成功后保存后台新签发的令牌：效益包到期时间已顺延，余额已扣掉售价。
func (m *Manager) BuyPack(productID int) (bool, string) {
	if productID <= 0 {
		return false, "请选择要购买的效益包"
	}
	apiURL, userToken, ok := m.userBearer()
	if !ok {
		return false, "请先登录账号"
	}
	m.mu.RLock()
	deviceID, deviceName := m.state.DeviceID, m.deviceName
	m.mu.RUnlock()
	var out tokenResponse
	body := map[string]any{"product_id": productID, "device_id": deviceID, "device_name": deviceName}
	err := m.doJSON(http.MethodPost, apiURL+"/api/license/shop/buy", userToken, body, &out)
	if endpointMissing(err) {
		return false, ErrShopUnsupported.Error()
	}
	if err != nil {
		return false, "购买失败：" + err.Error()
	}
	if !out.OK {
		if out.Message == "" {
			return false, "购买失败"
		}
		return false, out.Message
	}
	m.commitLicense("", &out)
	if m.log != nil {
		m.log.Log("[授权] 已用余额购买效益包，效益包" + expText(out.CardExpiresAt))
	}
	if out.Message == "" {
		return true, "效益包已开通"
	}
	return true, out.Message
}

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

// Leaderboard 查询用量排行。metric 为 cost/tokens/calls。
func (m *Manager) Leaderboard(rankRange, dimension, metric string, page int) (json.RawMessage, error) {
	switch rankRange {
	case "day", "month", "all":
	default:
		rankRange = "all"
	}
	switch dimension {
	case "normal", "advanced", "total":
	default:
		dimension = "total"
	}
	switch metric {
	case "cost", "tokens", "calls":
	default:
		metric = "cost"
	}
	if page < 1 {
		page = 1
	}
	apiURL, token, ok := m.userBearer()
	if !ok {
		return nil, errors.New("请先登录账号")
	}
	query := fmt.Sprintf("%s/api/license/leaderboard?range=%s&dimension=%s&metric=%s&page=%d&page_size=50", apiURL, rankRange, dimension, metric, page)
	var out json.RawMessage
	if err := m.doJSON(http.MethodGet, query, token, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetLeaderboard 设置当前账号是否参与花费排行。
func (m *Manager) SetLeaderboard(optIn bool) (json.RawMessage, error) {
	apiURL, token, ok := m.userBearer()
	if !ok {
		return nil, errors.New("请先登录账号")
	}
	var out json.RawMessage
	err := m.doJSON(http.MethodPost, apiURL+"/api/license/leaderboard", token, map[string]bool{"opt_in": optIn}, &out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PoolStatus 查询号池账号状态。授权服务代拉数据，响应 JSON 原样返回。
func (m *Manager) PoolStatus() (json.RawMessage, error) {
	m.refreshIfNearExpiry()
	apiURL, token, ok := m.bearer()
	if !ok {
		return nil, ErrNotActivated
	}
	const path = "/api/pool-status"
	var out json.RawMessage
	err := m.doJSON(http.MethodGet, apiURL+path, token, nil, &out)
	var se *statusError
	if errors.As(err, &se) && se.Status == http.StatusUnauthorized && m.refreshSerial() {
		if apiURL, token, ok = m.bearer(); !ok {
			return nil, ErrNotActivated
		}
		out = nil
		err = m.doJSON(http.MethodGet, apiURL+path, token, nil, &out)
	}
	if errors.As(err, &se) && se.Status == http.StatusNotFound && (se.Detail == "" || se.Detail == "Not Found") {
		return nil, ErrPoolUnsupported
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
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

// expText 效益包到期时间的日志写法，接在「效益包」后面。没开通过效益包（只有余额的钱包）时没有到期时间。
func expText(t *time.Time) string {
	if t == nil {
		return "未开通"
	}
	return "有效期至 " + t.Local().Format("2006-01-02 15:04")
}
