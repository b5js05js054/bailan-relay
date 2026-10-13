// Package config 负责本地端的配置加载/保存、默认值、模型列表以及
// 域名与模型 ID 的规范化处理。对应旧版 Python app.py 中的 config 相关逻辑。
package config

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// AppTitle 应用标题。
const AppTitle = "Devin_hub"

// UIVersion 上报给前端的界面版本号。
const UIVersion = "3.0"

// AppVersion 客户端程序版本号，用于远程更新检查。
// 可通过链接期注入：-ldflags "-X bailan-relay/internal/config.AppVersion=1.0.1"
var AppVersion = "1.2.5"

// Edition 区分构建版本：
//   - "full"     完整版（默认，开发/自用，展示全部信息）
//   - "customer" 客户版（隐藏中继地址、目标域名、转发路径、DNS、Token 等核心信息）
//
// 客户版通过链接期注入：
//
//	-ldflags "-X bailan-relay/internal/config.Edition=customer"
var Edition = "full"

// IsCustomer 返回当前是否为客户版构建。
func IsCustomer() bool { return strings.EqualFold(strings.TrimSpace(Edition), "customer") }

// 本地 HTTPS 拦截服务监听端口（hosts 把目标域名指到 127.0.0.1:443）。
const ProxyListenPort = 443

// CertCN 返回本地根证书的 Common Name（安装/卸载/校验都以它为准）。
// 客户版与完整版使用不同 CN，保证两版在同一台机器上各自独立安装、互不覆盖，
// 也让证书页能正确判断“本版本自己的 CA 是否已被系统信任”。
func CertCN() string {
	if IsCustomer() {
		return "BaiLan Devin Relay Client CA"
	}
	return "BaiLan Devin Relay CA"
}

// hosts 段落标记，写入与清理都以这两行为界。
const (
	HostsBegin = "# BEGIN BaiLan Proxy Manager"
	HostsEnd   = "# END BaiLan Proxy Manager"
)

// Model 是可选上游模型（id + 展示名 + 是否免费）。
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Free  bool   `json:"free"`
	// 以下由后台模型目录下发，仅用于展示：Ratio 为倍率（0=免费），PriceIn/PriceOut 为每 100 万 tokens 的价格。
	Ratio    float64 `json:"ratio"`
	PriceIn  float64 `json:"price_in"`
	PriceOut float64 `json:"price_out"`
	// BillingMode = "request" 时按次计费，PriceRequest 为每次价格；TierThreshold/TierRatio 为长上下文阶梯。
	BillingMode   string  `json:"billing_mode,omitempty"`
	PriceRequest  float64 `json:"price_request,omitempty"`
	TierThreshold int     `json:"tier_threshold,omitempty"`
	TierRatio     float64 `json:"tier_ratio,omitempty"`
	// 模型能力：0 / false = 未知。
	ContextWindow    int  `json:"context_window,omitempty"`
	MaxOutput        int  `json:"max_output,omitempty"`
	SupportsThinking bool `json:"supports_thinking,omitempty"`
}

// SelectableModels 是内置兜底模型列表（授权后台不可达时使用）。
// 运行期可通过 SetSelectableModels 替换为后台下发的模型目录。
var SelectableModels = []Model{
	{ID: "swe-2-medium", Label: "SWE-2 · Medium", Free: true},
	{ID: "swe-2-high", Label: "SWE-2 · High", Free: true},
	{ID: "swe-2-max", Label: "SWE-2 · Max", Free: true},
	{ID: "glm-5-2", Label: "GLM-5.2", Free: true},
}

// DefaultOverrideModel 默认替换目标模型。
const DefaultOverrideModel = "swe-2-high"

var (
	modelMu       sync.RWMutex
	runtimeModels = append([]Model(nil), SelectableModels...)
	// catalogSynced 表示 runtimeModels 来自授权后台下发的目录（而非内置兜底）。
	// 只有同步成功后，才能据此判断某个模型「在后台已被隐藏/删除」。
	catalogSynced  bool
	modelLabels    = map[string]string{}
	labelToID      = map[string]string{}
	modelIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)
)

func init() {
	rebuildModelMapsLocked()
}

func rebuildModelMapsLocked() {
	modelLabels = make(map[string]string, len(runtimeModels))
	labelToID = make(map[string]string, len(runtimeModels))
	for _, m := range runtimeModels {
		if m.ID == "" {
			continue
		}
		if m.Label == "" {
			m.Label = m.ID
		}
		modelLabels[m.ID] = m.Label
		labelToID[m.Label] = m.ID
	}
}

// Models 返回当前生效的可选模型列表。
func Models() []Model {
	modelMu.RLock()
	defer modelMu.RUnlock()
	if len(runtimeModels) == 0 {
		return append([]Model(nil), SelectableModels...)
	}
	return append([]Model(nil), runtimeModels...)
}

// SetSelectableModels 更新运行期模型目录（来自授权后台）。空列表会回退为内置默认。
func SetSelectableModels(models []Model) {
	clean := make([]Model, 0, len(models))
	seen := map[string]bool{}
	for _, m := range models {
		id := strings.ToLower(strings.TrimSpace(m.ID))
		if !modelIDPattern.MatchString(id) || seen[id] {
			continue
		}
		seen[id] = true
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = id
		}
		m.ID, m.Label = id, label
		clean = append(clean, m)
	}
	modelMu.Lock()
	defer modelMu.Unlock()
	if len(clean) == 0 {
		runtimeModels = append([]Model(nil), SelectableModels...)
		catalogSynced = false
	} else {
		runtimeModels = clean
		catalogSynced = true
	}
	rebuildModelMapsLocked()
}

// CatalogSynced 返回当前模型目录是否来自授权后台。
func CatalogSynced() bool {
	modelMu.RLock()
	defer modelMu.RUnlock()
	return catalogSynced
}

// ModelInCatalog 判断模型是否仍在后台下发的目录里（即后台未隐藏/删除）。
// 目录尚未从后台同步成功（离线、后台不可达、旧版后台）时一律返回 true，
// 避免因为拉不到目录而误拦用户。
func ModelInCatalog(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	modelMu.RLock()
	defer modelMu.RUnlock()
	if !catalogSynced {
		return true
	}
	_, ok := modelLabels[id]
	return ok
}

// DefaultTargetDomains 内置目标域名（Devin/Windsurf/Codeium API）。
var DefaultTargetDomains = []string{
	"server.self-serve.windsurf.com",
	"server.codeium.com",
	"server-beta.codeium.com",
	"server-staging.codeium.com",
	"api.codeium.com",
	"inference.codeium.com",
	"southcentral-lb.codeium.com",
}

// DefaultTargetPaths 默认转发到中继的 API 路径。
var DefaultTargetPaths = []string{
	"/exa.api_server_pb.ApiServerService/GetChatMessage",
}

// DefaultStubPaths 默认本地空应答关键字。策略接口必须回源，不设置默认项。
var DefaultStubPaths = []string{}

var reservedPassthroughKeywords = map[string]bool{
	"getuserjwt":         true,
	"getcliteamsettings": true,
}

// NormalizeStubPaths 清洗本地空应答关键字并移除必须回源的策略接口。
// 输入：用户配置的关键字；输出：去空、去重且不会影响团队策略获取的列表。
func NormalizeStubPaths(raw []string) []string {
	result := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		value := strings.TrimSpace(item)
		key := strings.ToLower(value)
		if key == "" || seen[key] || reservedPassthroughKeywords[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

// DefaultDNSServers 默认解析 DNS。
var DefaultDNSServers = []string{"223.5.5.5", "119.29.29.29", "8.8.8.8", "1.1.1.1"}

// Config 是持久化到 config.json 的完整配置。
type Config struct {
	RelayServerURL      string   `json:"relay_server_url"`
	InterceptProcess    []string `json:"intercept_process"`
	DevinExecutable     string   `json:"devin_executable"`
	TargetDomains       []string `json:"target_domains"`
	TargetPaths         []string `json:"target_paths"`
	RelayAuthToken      string   `json:"relay_auth_token"`
	StubPaths           []string `json:"stub_paths"`
	DNSServers          []string `json:"dns_servers"`
	OverrideModel       string   `json:"override_model"`
	ShowMitmweb         bool     `json:"show_mitmweb"`
	RestartDevinOnStart bool     `json:"restart_devin_on_start"`
	MitmwebToken        string   `json:"mitmweb_token,omitempty"`
	// LicenseAPIURL 卡密授权管理系统地址（激活/心跳）。
	LicenseAPIURL string `json:"license_api_url"`
}

// Default 返回一份默认配置（首次运行时写入磁盘）。
func Default() Config {
	return Config{
		RelayServerURL:      "http://devin.exic.us.ci:5001",
		InterceptProcess:    append([]string(nil), DefaultInterceptProcesses...),
		DevinExecutable:     defaultDevinExecutable(),
		TargetDomains:       append([]string(nil), DefaultTargetDomains...),
		TargetPaths:         append([]string(nil), DefaultTargetPaths...),
		RelayAuthToken:      "",
		StubPaths:           append([]string(nil), DefaultStubPaths...),
		DNSServers:          append([]string(nil), DefaultDNSServers...),
		OverrideModel:       DefaultOverrideModel,
		ShowMitmweb:         false,
		RestartDevinOnStart: true,
		LicenseAPIURL:       DefaultLicenseAPIURL,
	}
}

// DefaultLicenseAPIURL 卡密授权管理系统默认地址（与中继同主机、独立端口）。
const DefaultLicenseAPIURL = "http://devin.exic.us.ci:8000"

// LicenseStatePath 返回本地授权状态文件路径。
func LicenseStatePath() string { return filepath.Join(AppDir(), "license.json") }

// UserStatePath 返回本地账号会话文件路径。
func UserStatePath() string { return filepath.Join(AppDir(), "user.json") }

var fileMu sync.RWMutex

// appDirName 客户版使用独立数据目录，避免与完整版共用同一份 config.json 互相覆盖。
func appDirName() string {
	if IsCustomer() {
		return "BaiLan Devin Relay Client"
	}
	return "BaiLan Proxy Manager"
}

// ConfigPath 返回 config.json 的绝对路径。
func ConfigPath() string { return filepath.Join(AppDir(), "config.json") }

// LogDir 返回日志目录。
func LogDir() string { return filepath.Join(AppDir(), "logs") }

// CertDir 返回本地 CA 证书目录。
func CertDir() string { return filepath.Join(AppDir(), "ca") }

// CertFile 返回 CA 证书（.cer）路径。
func CertFile() string { return filepath.Join(CertDir(), "bailan-ca-cert.cer") }

// CertKeyFile 返回 CA 私钥路径。
func CertKeyFile() string { return filepath.Join(CertDir(), "bailan-ca-key.pem") }

var domainRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

// SanitizeDomain 去掉误带的引号/URL 前缀/端口，只保留合法主机名。
func SanitizeDomain(value string) string {
	text := strings.ToLower(strings.TrimSpace(value))
	text = strings.Trim(text, " \t\"'`[](){}<>")
	if i := strings.Index(text, "://"); i >= 0 {
		text = text[i+3:]
	}
	text = strings.SplitN(text, "/", 2)[0]
	text = strings.SplitN(text, "?", 2)[0]
	text = strings.SplitN(text, "#", 2)[0]
	if strings.HasPrefix(text, "[") && strings.Contains(text, "]") {
		text = strings.SplitN(text[1:], "]", 2)[0]
	} else {
		text = strings.SplitN(text, ":", 2)[0]
	}
	text = strings.Trim(text, ".")
	if text == "" || !domainRe.MatchString(text) {
		return ""
	}
	return text
}

// NormalizeTargetDomains 清洗域名列表并去重，为空时回落默认域名。
func NormalizeTargetDomains(raw []string) []string {
	var names []string
	seen := map[string]bool{}
	for _, item := range raw {
		name := SanitizeDomain(item)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		return names
	}
	var out []string
	for _, item := range DefaultTargetDomains {
		if name := SanitizeDomain(item); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// NormalizeModelID 把任意输入规范成合法模型 ID（支持传入展示名）。
// 已注册模型优先；未注册但形态合法的 ID 也允许透传，以便后台新增模型后无需升级客户端。
func NormalizeModelID(value string) string {
	raw := strings.TrimSpace(value)
	lower := strings.ToLower(raw)
	modelMu.RLock()
	if _, ok := modelLabels[raw]; ok {
		modelMu.RUnlock()
		return raw
	}
	if _, ok := modelLabels[lower]; ok {
		modelMu.RUnlock()
		return lower
	}
	if id, ok := labelToID[raw]; ok {
		modelMu.RUnlock()
		return id
	}
	modelMu.RUnlock()
	if modelIDPattern.MatchString(lower) {
		return lower
	}
	return DefaultOverrideModel
}

// ModelLabel 返回模型的展示名。
func ModelLabel(modelID string) string {
	id := NormalizeModelID(modelID)
	modelMu.RLock()
	if label, ok := modelLabels[id]; ok {
		modelMu.RUnlock()
		return label
	}
	modelMu.RUnlock()
	return id
}
