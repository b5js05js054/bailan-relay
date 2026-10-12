//go:build windows

// Package proxybypass 把目标域名写入 WinINET「代理绕过列表」
// （HKCU\...\Internet Settings\ProxyOverride），让遵守系统代理的程序
// （Electron/Chromium、部分语言栈 HTTP 客户端）对这些域名恢复直连——
// 直连后 hosts 劫持与 --host-resolver-rules 才能生效。停止拦截时移除我们追加的条目。
//
// 局限：PAC 自动脚本 / WPAD 自动检测模式下 ProxyOverride 不生效（Chromium 不支持），
// 那种场景只能用 PACMode() 检测后提示用户改用规则模式。
package proxybypass

import (
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	settingsKey        = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	valueProxyOverride = "ProxyOverride"
	valueProxyEnable   = "ProxyEnable"
	valueAutoConfigURL = "AutoConfigURL"
)

// InternetSetOption 通知常量：写完注册表后广播，让运行中的程序重新读取代理配置。
const (
	optionRefresh         = 37 // INTERNET_OPTION_REFRESH
	optionSettingsChanged = 39 // INTERNET_OPTION_SETTINGS_CHANGED
	optionProxyChanged    = 95 // INTERNET_OPTION_PROXY_SETTINGS_CHANGED
)

var procInternetSetOption = syscall.NewLazyDLL("wininet.dll").NewProc("InternetSetOptionW")

// Apply 把每个域名以 "d" 与 "*.d" 两种形态追加进 ProxyOverride（幂等，保留原有条目）。
// 返回是否发生了写入。
func Apply(domains []string) (bool, error) {
	return mutate(domains, true)
}

// Restore 从 ProxyOverride 移除我们追加的条目，保留用户自己的条目。
func Restore(domains []string) (bool, error) {
	return mutate(domains, false)
}

// Enabled 返回系统代理开关（ProxyEnable）是否打开。
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, settingsKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(valueProxyEnable)
	return err == nil && v != 0
}

// PACMode 返回是否配置了 PAC 自动脚本（该模式下绕过列表不生效）。
func PACMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, settingsKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valueAutoConfigURL)
	return err == nil && strings.TrimSpace(v) != ""
}

// mutate 读出现有 ProxyOverride（分号分隔），按 add=true 补全 / add=false 移除目标条目。
func mutate(domains []string, add bool) (bool, error) {
	want := map[string]bool{}
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		want[d] = true
		want["*."+d] = true
	}
	if len(want) == 0 {
		return false, nil
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, settingsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	raw, _, _ := k.GetStringValue(valueProxyOverride)

	var kept []string
	seen := map[string]bool{}
	removed := false
	for _, part := range strings.Split(raw, ";") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		lp := strings.ToLower(p)
		if want[lp] {
			if !add {
				removed = true
				continue
			}
		}
		if seen[lp] {
			continue // 顺手去重
		}
		seen[lp] = true
		kept = append(kept, p)
	}

	changed := removed
	if add {
		for d := range want {
			if !seen[d] {
				kept = append(kept, d)
				seen[d] = true
				changed = true
			}
		}
	}
	if !changed {
		return false, nil
	}
	if err := k.SetStringValue(valueProxyOverride, strings.Join(kept, ";")); err != nil {
		return false, err
	}
	notify()
	return true, nil
}

// notify 通知 WinINET 客户端代理配置已变更（best-effort，失败不影响）。
func notify() {
	for _, opt := range []uintptr{optionSettingsChanged, optionProxyChanged, optionRefresh} {
		_, _, _ = procInternetSetOption.Call(0, opt, 0, 0)
	}
}
