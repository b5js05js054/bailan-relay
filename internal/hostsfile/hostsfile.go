// Package hostsfile 负责把目标域名写入/移除 Windows hosts，使 HTTPS 打到
// 127.0.0.1:443 的本地拦截服务。对应旧版 app.py 的 hosts 劫持逻辑。
package hostsfile

import (
	"os"
	"strings"
	"sync"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

var mu sync.Mutex

// Apply 把 domains 写进 hosts（127.0.0.1 与 ::1），需要管理员权限。
func Apply(domains []string) (bool, string) {
	if len(domains) == 0 {
		return false, "没有可劫持的目标域名"
	}
	if !sysutil.IsAdmin() {
		return false, "写入 hosts 需要管理员权限"
	}
	block := []string{config.HostsBegin}
	for _, d := range domains {
		block = append(block, "127.0.0.1 "+d)
		block = append(block, "::1 "+d)
	}
	block = append(block, config.HostsEnd)

	mu.Lock()
	defer mu.Unlock()
	text, err := readHosts()
	if err != nil {
		return false, "写入 hosts 失败：" + err.Error()
	}
	merged := stripBlock(text) + "\n" + strings.Join(block, "\n") + "\n"
	if err := os.WriteFile(config.HostsPath(), []byte(merged), 0o644); err != nil {
		return false, "写入 hosts 失败：" + err.Error()
	}
	sysutil.FlushDNS()
	return true, "已写入 hosts：" + strings.Join(domains, "、")
}

// Restore 移除本工具写入的 hosts 段。
func Restore() (bool, string) {
	mu.Lock()
	defer mu.Unlock()
	if _, err := os.Stat(config.HostsPath()); err != nil {
		return true, "hosts 不存在，无需恢复"
	}
	text, err := readHosts()
	if err != nil {
		return false, "恢复 hosts 失败：" + err.Error()
	}
	if !strings.Contains(text, config.HostsBegin) && !strings.Contains(text, config.HostsEnd) {
		return true, "hosts 未改动"
	}
	if err := os.WriteFile(config.HostsPath(), []byte(stripBlock(text)), 0o644); err != nil {
		return false, "恢复 hosts 失败：" + err.Error()
	}
	sysutil.FlushDNS()
	return true, "已恢复 hosts"
}

func readHosts() (string, error) {
	raw, err := os.ReadFile(config.HostsPath())
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func stripBlock(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var kept []string
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == config.HostsBegin {
			skipping = true
			continue
		}
		if trimmed == config.HostsEnd {
			skipping = false
			continue
		}
		if !skipping {
			kept = append(kept, strings.TrimRight(line, " \t"))
		}
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n"
}
