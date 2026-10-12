// Package devin 负责发现 Devin 主程序、重启 Devin（关掉后降权重启，
// 并强制 Chromium 走本机解析、关掉 DoH/QUIC）。移植自旧版 app.py。
// 平台相关部分（程序发现、进程名、启动方式）见 devin_windows.go / devin_darwin.go。
package devin

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func firstExisting(candidates []string, exists func(string) bool) string {
	for _, c := range candidates {
		if exists(c) {
			return c
		}
	}
	return ""
}

// DiscoverInterceptProcesses 扫描安装目录，找出真正发 API 的进程名。
// 输入：Devin 程序路径（可空）；输出：去重后的进程名列表。
func DiscoverInterceptProcesses(executable string) []string {
	names := append([]string(nil), config.DefaultInterceptProcesses...)
	if exe := DiscoverExecutable(executable); exe != "" {
		names = append(names, processName(exe))
		names = append(names, bundledProcesses(exe)...)
	}
	return dedup(names)
}

func imageNames(executable string) []string {
	cfg := config.Load()
	base := executable
	if base == "" {
		base = cfg.DevinExecutable
	}
	names := DiscoverInterceptProcesses(base)
	if executable != "" {
		names = append(names, processName(executable))
	}
	return dedupFold(names)
}

func isRunning(executable string) bool {
	for _, name := range imageNames(executable) {
		if sysutil.IsProcessRunning(name) {
			return true
		}
	}
	return false
}

// hostResolverRules 生成 Chromium --host-resolver-rules，把目标域名钉到 127.0.0.1。
func hostResolverRules() string {
	var rules []string
	for _, d := range config.Load().HijackDomains() {
		rules = append(rules, "MAP "+d+" 127.0.0.1")
		rules = append(rules, "MAP *."+d+" 127.0.0.1")
	}
	return strings.Join(rules, ", ")
}

// chromiumArgs 返回启动 Devin 时追加的 Chromium 参数（关 QUIC/DoH + 本机解析规则）。
func chromiumArgs() []string {
	return []string{
		"--disable-quic",
		"--disable-features=DnsOverHttps,AsyncDns",
		"--host-resolver-rules=" + strings.ReplaceAll(hostResolverRules(), `"`, ""),
	}
}

// launchEnv 返回启动 Devin 时注入的环境变量。
var launchEnv = []string{"GODEBUG=netdns=cgo", "API_SERVER_CLIENT_USE_HTTP_2=1"}

// terminateProcessTree 结束每个相关映像并等待进程树退出。
// 输入：映像名、运行状态/结束/等待回调与超时；输出：是否全部退出。
func terminateProcessTree(names []string, running func() bool, kill func(string), wait func(time.Duration), timeout time.Duration) bool {
	if !running() {
		return true
	}
	for _, name := range names {
		kill(name)
	}
	deadline := time.Now().Add(timeout)
	for running() && time.Now().Before(deadline) {
		wait(250 * time.Millisecond)
	}
	return !running()
}

// Stop 关闭 Devin 及其 language server，不重新启动。
// 输入：Devin 程序路径；输出：关闭结果与状态说明。
func Stop(executable string) (bool, string) {
	if !isRunning(executable) {
		return true, "Devin 未运行"
	}
	ok := terminateProcessTree(
		imageNames(executable),
		func() bool { return isRunning(executable) },
		func(name string) { _, _, _ = sysutil.KillImage(name) },
		time.Sleep,
		10*time.Second,
	)
	if !ok {
		return false, "Devin 进程未能完全退出"
	}
	return true, "Devin 已关闭"
}

// Restart 关闭 Devin 及其 language server，再以普通用户权限重启。
// 输入：Devin 程序路径；输出：结果与状态说明。
func Restart(executable string) (bool, string) {
	devinExe := DiscoverExecutable(executable)
	if devinExe == "" {
		target := executable
		if target == "" {
			target = defaultExecutable
		}
		return false, "Devin 程序不存在: " + target
	}
	if ok, message := Stop(devinExe); !ok {
		return false, message
	}
	if err := launch(devinExe); err != nil {
		return false, "Devin 自动重启失败: " + err.Error()
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if isRunning(devinExe) {
			return true, "Devin 已自动重启: " + devinExe
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false, "已发送 Devin 启动命令，但未检测到 " + filepath.Base(devinExe)
}

func dedup(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if it != "" && !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

func dedupFold(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		key := strings.ToLower(it)
		if it != "" && !seen[key] {
			seen[key] = true
			out = append(out, it)
		}
	}
	return out
}
