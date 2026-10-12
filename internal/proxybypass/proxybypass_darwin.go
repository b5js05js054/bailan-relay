//go:build darwin

// Package proxybypass 把目标域名写入 macOS 各网络服务的「代理绕过列表」
// （networksetup -setproxybypassdomains），让遵守系统代理的程序对这些域名
// 恢复直连——直连后 hosts 劫持与 --host-resolver-rules 才能生效。
// 停止拦截时移除我们追加的条目，保留用户自己的条目。
//
// 局限：PAC 自动代理模式下绕过列表不生效，只能用 PACMode() 检测后提示用户。
package proxybypass

import (
	"strings"

	"bailan-relay/internal/sysutil"
)

// Apply 把每个域名以 "d" 与 "*.d" 两种形态追加进所有网络服务的绕过列表（幂等）。
// 返回是否发生了写入。
func Apply(domains []string) (bool, error) {
	return mutate(domains, true)
}

// Restore 从绕过列表移除我们追加的条目，保留用户自己的条目。
func Restore(domains []string) (bool, error) {
	return mutate(domains, false)
}

// Enabled 返回是否有任一网络服务开启了 HTTP/HTTPS/SOCKS 代理。
func Enabled() bool {
	out, _, err := sysutil.RunHidden("scutil", "--proxy")
	if err != nil {
		return false
	}
	for _, key := range []string{"HTTPEnable", "HTTPSEnable", "SOCKSEnable"} {
		if strings.Contains(out, key+" : 1") {
			return true
		}
	}
	return false
}

// PACMode 返回是否配置了 PAC 自动代理脚本（该模式下绕过列表不生效）。
func PACMode() bool {
	out, _, err := sysutil.RunHidden("scutil", "--proxy")
	return err == nil && strings.Contains(out, "ProxyAutoConfigEnable : 1")
}

// mutate 逐个网络服务读出现有绕过列表，按 add=true 补全 / add=false 移除目标条目。
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
	services, err := networkServices()
	if err != nil {
		return false, err
	}
	changed := false
	var lastErr error
	for _, svc := range services {
		ok, err := mutateService(svc, want, add)
		if err != nil {
			lastErr = err
			continue
		}
		changed = changed || ok
	}
	if !changed && lastErr != nil {
		return false, lastErr
	}
	return changed, nil
}

// networkServices 列出所有启用中的网络服务名（Wi-Fi、Ethernet 等）。
func networkServices() ([]string, error) {
	out, _, err := sysutil.RunHidden("networksetup", "-listallnetworkservices")
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// 首行为说明文字；以 * 开头的是已停用服务。
		if line == "" || strings.HasPrefix(line, "*") || strings.Contains(line, "asterisk") {
			continue
		}
		services = append(services, line)
	}
	return services, nil
}

// mutateService 修改单个网络服务的绕过列表，返回是否发生了写入。
func mutateService(service string, want map[string]bool, add bool) (bool, error) {
	out, _, err := sysutil.RunHidden("networksetup", "-getproxybypassdomains", service)
	if err != nil {
		return false, err
	}
	var kept []string
	seen := map[string]bool{}
	removed := false
	for _, line := range strings.Split(out, "\n") {
		p := strings.TrimSpace(line)
		// 列表为空时输出的是一句提示语而非域名。
		if p == "" || strings.Contains(p, " ") {
			continue
		}
		lp := strings.ToLower(p)
		if want[lp] && !add {
			removed = true
			continue
		}
		if seen[lp] {
			continue
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
	args := []string{"-setproxybypassdomains", service}
	if len(kept) == 0 {
		args = append(args, "Empty")
	} else {
		args = append(args, kept...)
	}
	if _, _, err := sysutil.RunHidden("networksetup", args...); err != nil {
		return false, err
	}
	return true, nil
}
