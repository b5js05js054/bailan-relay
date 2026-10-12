package config

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// ensureRuntimeConfig 首次运行时把默认配置写到用户数据目录。
func ensureRuntimeConfig() {
	path := ConfigPath()
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, _ := json.MarshalIndent(sanitizedForDisk(Default()), "", "    ")
	_ = os.WriteFile(path, append(data, '\n'), 0o644)
}

// Load 读取配置，缺省字段以默认值补齐，并顺带清洗目标域名。
func Load() Config {
	fileMu.RLock()
	defer fileMu.RUnlock()
	return loadLocked()
}

func loadLocked() Config {
	ensureRuntimeConfig()
	cfg := Default()
	if raw, err := os.ReadFile(ConfigPath()); err == nil {
		_ = json.Unmarshal(raw, &cfg)
	}
	if IsCustomer() {
		// 客户版：核心字段一律取内置默认值，忽略磁盘上的任何覆盖。
		d := Default()
		cfg.RelayServerURL = d.RelayServerURL
		cfg.RelayAuthToken = d.RelayAuthToken
		cfg.MitmwebToken = ""
		cfg.TargetDomains = append([]string(nil), d.TargetDomains...)
		cfg.TargetPaths = append([]string(nil), d.TargetPaths...)
		cfg.StubPaths = append([]string(nil), d.StubPaths...)
		cfg.DNSServers = append([]string(nil), d.DNSServers...)
		cfg.InterceptProcess = append([]string(nil), d.InterceptProcess...)
		cfg.LicenseAPIURL = d.LicenseAPIURL
	}
	if strings.TrimSpace(cfg.LicenseAPIURL) == "" {
		cfg.LicenseAPIURL = Default().LicenseAPIURL
	}
	cfg.ShowMitmweb = false
	cfg.TargetDomains = NormalizeTargetDomains(cfg.TargetDomains)
	cfg.StubPaths = NormalizeStubPaths(cfg.StubPaths)
	return cfg
}

// sanitizedForDisk 客户版落盘时抹掉核心敏感字段（中继地址/Token/域名/路径/DNS/进程名）。
// 运行期这些值改由编译进二进制的默认配置提供，磁盘上的 config.json 不再包含它们。
func sanitizedForDisk(cfg Config) Config {
	if !IsCustomer() {
		return cfg
	}
	cfg.RelayServerURL = ""
	cfg.RelayAuthToken = ""
	cfg.MitmwebToken = ""
	cfg.TargetDomains = nil
	cfg.TargetPaths = nil
	cfg.StubPaths = nil
	cfg.DNSServers = nil
	cfg.InterceptProcess = nil
	cfg.LicenseAPIURL = ""
	return cfg
}

func writeLocked(cfg Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sanitizedForDisk(cfg), "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Save 把 mutate 应用到当前配置并落盘，返回更新后的配置。
func Save(mutate func(*Config)) Config {
	fileMu.Lock()
	defer fileMu.Unlock()
	cfg := loadLocked()
	if mutate != nil {
		mutate(&cfg)
	}
	cfg.ShowMitmweb = false
	cfg.TargetDomains = NormalizeTargetDomains(cfg.TargetDomains)
	cfg.StubPaths = NormalizeStubPaths(cfg.StubPaths)
	_ = writeLocked(cfg)
	return cfg
}

// TargetDomains 返回规范化后的目标域名。
func (c Config) HijackDomains() []string {
	merged := append(append([]string(nil), c.TargetDomains...), DefaultTargetDomains...)
	return NormalizeTargetDomains(merged)
}

// IsPublicIPv4 排除回环/0.0.0.0，以及本地代理 fake-ip 段 198.18.0.0/15。
func IsPublicIPv4(value string) bool {
	ip := strings.TrimSpace(value)
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return false
	}
	if strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "198.18.") ||
		strings.HasPrefix(ip, "198.19.") || ip == "0.0.0.0" {
		return false
	}
	return true
}

// NormalizeDNSServers 清洗 DNS 列表并补默认项兜底。
func NormalizeDNSServers(raw []string) []string {
	var result []string
	seen := map[string]bool{}
	for _, item := range raw {
		ip := strings.TrimSpace(item)
		if IsPublicIPv4(ip) && !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}
	if len(result) == 0 {
		return append([]string(nil), DefaultDNSServers...)
	}
	return result
}

// EffectiveDNSServers 返回配置 DNS + 默认 DNS（去重）。
func (c Config) EffectiveDNSServers() []string {
	var result []string
	seen := map[string]bool{}
	for _, item := range append(append([]string(nil), c.DNSServers...), DefaultDNSServers...) {
		ip := strings.TrimSpace(item)
		if IsPublicIPv4(ip) && !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}
	return result
}
