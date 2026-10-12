// Package devin 负责发现 Devin 主程序、重启 Devin（关掉后通过 explorer 降权重启，
// 并强制 Chromium 走本机解析、关掉 DoH/QUIC）。移植自旧版 app.py。
package devin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

// DiscoverExecutable 查找 Devin.exe，找不到返回空串。
// 分级探测、逐级惰性执行：已配置路径与常见目录 stat（零进程调用）→ 运行中进程 →
// 注册表安装记录 → 固定磁盘常见目录扫描。任一级命中即返回，常见场景毫秒级完成。
func DiscoverExecutable(configured string) string {
	// 第 1 级：已配置路径 + 历史/标准安装目录（纯 stat，零开销）。
	fast := []string{strings.TrimSpace(configured),
		`E:\Devin\Devin.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Devin", "Devin.exe"),
		filepath.Join(defaultProgramFiles(), "Devin", "Devin.exe"),
	}
	if p := firstExisting(fast); p != "" {
		return p
	}
	// 第 2 级：正在运行的进程取真实路径（装在非标准位置时最准）。
	if p := runningProcessPath(); fileExists(p) {
		return p
	}
	// 第 3 级：注册表 App Paths / Uninstall 记录。
	if p := firstExisting(registryCandidates()); p != "" {
		return p
	}
	// 第 4 级：逐固定磁盘扫常见目录（跳过网盘/可移动介质，防止 ReadDir 阻塞）。
	return firstExisting(driveScanCandidates())
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func firstExisting(candidates []string) string {
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

// runningProcessPath 从正在运行的 Devin 进程取真实路径（装在非标准位置时最准）。
func runningProcessPath() string {
	out, _, err := sysutil.RunHidden("powershell", "-NoProfile", "-Command",
		"(Get-Process -Name Devin -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Path)")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

var iconIndexRe = regexp.MustCompile(`,\s*-?\d+$`)

// cleanIconPath 规整注册表 DisplayIcon 值：去引号、去 ,图标序号 后缀。
func cleanIconPath(s string) string {
	p := strings.TrimSpace(s)
	p = iconIndexRe.ReplaceAllString(p, "")
	p = strings.Trim(p, "\"'")
	return strings.TrimSpace(p)
}

// registryCandidates 从注册表找安装位置：App Paths 默认值的 exe 全路径，
// 以及 Uninstall 键里 DisplayName 含 Devin 的 InstallLocation / DisplayIcon。
func registryCandidates() []string {
	var out []string
	if res, _, err := sysutil.RunHidden("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\Devin.exe`, "/ve"); err == nil {
		for _, line := range strings.Split(res, "\n") {
			i := strings.Index(line, "REG_SZ")
			if i < 0 {
				continue
			}
			if p := cleanIconPath(line[i+len("REG_SZ"):]); p != "" {
				out = append(out, p)
			}
		}
	}
	const ps = `Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*','HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*','HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -match 'Devin' } | ForEach-Object { $_.InstallLocation; $_.DisplayIcon }`
	if res, _, err := sysutil.RunHidden("powershell", "-NoProfile", "-Command", ps); err == nil {
		for _, line := range strings.Split(res, "\n") {
			p := cleanIconPath(line)
			if p == "" {
				continue
			}
			if strings.HasSuffix(strings.ToLower(p), ".exe") {
				out = append(out, p)
			} else {
				out = append(out, filepath.Join(p, "Devin.exe"))
			}
		}
	}
	return out
}

// driveScanCandidates 在固定磁盘上枚举常见安装位置（含每用户 AppData）。
func driveScanCandidates() []string {
	var out []string
	subdirs := []string{`Devin`, `Program Files\Devin`, `Program Files (x86)\Devin`}
	for _, root := range fixedDriveRoots() {
		for _, d := range subdirs {
			out = append(out, filepath.Join(root, d, "Devin.exe"))
		}
		if entries, err := os.ReadDir(filepath.Join(root, "Users")); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					out = append(out, filepath.Join(root, "Users", e.Name(),
						"AppData", "Local", "Programs", "Devin", "Devin.exe"))
				}
			}
		}
	}
	return out
}

const driveTypeFixed = 3 // DRIVE_FIXED

// fixedDriveRoots 枚举本机固定磁盘（跳过映射网盘/可移动介质，枚举其 Users 可能阻塞数十秒）。
func fixedDriveRoots() []string {
	var roots []string
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []string{`C:\`}
	}
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		ptr, err := windows.UTF16PtrFromString(root)
		if err == nil && windows.GetDriveType(ptr) == driveTypeFixed {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, `C:\`)
	}
	return roots
}

func defaultProgramFiles() string {
	if pf := os.Getenv("PROGRAMFILES"); pf != "" {
		return pf
	}
	return `C:\Program Files`
}

// DiscoverInterceptProcesses 扫描安装目录，找出真正发 API 的进程名。
func DiscoverInterceptProcesses(executable string) []string {
	names := append([]string(nil), config.DefaultInterceptProcesses...)
	exe := DiscoverExecutable(executable)
	if exe != "" {
		names = append(names, filepath.Base(exe))
		dir := filepath.Dir(exe)
		binDir := filepath.Join(dir, "resources", "app", "extensions", "windsurf", "bin")
		if entries, err := os.ReadDir(binDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasPrefix(e.Name(), "language_server") && strings.HasSuffix(e.Name(), ".exe") {
					names = append(names, e.Name())
				}
			}
		}
		nested := filepath.Join(dir, "resources", "app", "extensions", "windsurf", "devin", "bin", "devin.exe")
		if info, err := os.Stat(nested); err == nil && !info.IsDir() {
			names = append(names, filepath.Base(nested))
		}
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
		names = append(names, filepath.Base(executable))
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

func hostResolverRules() string {
	var rules []string
	for _, d := range config.Load().HijackDomains() {
		rules = append(rules, "MAP "+d+" 127.0.0.1")
		rules = append(rules, "MAP *."+d+" 127.0.0.1")
	}
	return strings.Join(rules, ", ")
}

func writeLauncher(devinExe string) (string, error) {
	launcher := filepath.Join(config.AppDir(), "launch_devin.cmd")
	rules := strings.ReplaceAll(hostResolverRules(), `"`, "")
	lines := []string{
		"@echo off",
		"set GODEBUG=netdns=cgo",
		"set API_SERVER_CLIENT_USE_HTTP_2=1",
		`cd /d "` + filepath.Dir(devinExe) + `"`,
		`start "" "` + devinExe + `" --disable-quic --disable-features=DnsOverHttps,AsyncDns --host-resolver-rules="` + rules + `"`,
	}
	content := strings.Join(lines, "\r\n") + "\r\n"
	if err := os.MkdirAll(config.AppDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(launcher, []byte(content), 0o644); err != nil {
		return "", err
	}
	return launcher, nil
}

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
// 输入：Devin.exe 路径；输出：关闭结果与状态说明。
func Stop(executable string) (bool, string) {
	if !isRunning(executable) {
		return true, "Devin 未运行"
	}
	names := imageNames(executable)
	ok := terminateProcessTree(
		names,
		func() bool { return isRunning(executable) },
		func(name string) { _, _, _ = sysutil.Taskkill("/T", "/IM", name) },
		time.Sleep,
		10*time.Second,
	)
	if !ok {
		return false, "Devin 进程未能完全退出"
	}
	return true, "Devin 已关闭"
}

// Restart 关闭 Devin 及其 language server，再通过 explorer 降权重启。
func Restart(executable string) (bool, string) {
	devinExe := DiscoverExecutable(executable)
	if devinExe == "" {
		target := executable
		if target == "" {
			target = `E:\Devin\Devin.exe`
		}
		return false, "Devin 程序不存在: " + target
	}
	if ok, message := Stop(devinExe); !ok {
		return false, message
	}

	launcher, err := writeLauncher(devinExe)
	if err != nil {
		return false, "Devin 自动重启失败: " + err.Error()
	}
	if err := sysutil.StartDetached(filepath.Dir(devinExe), "explorer.exe", launcher); err != nil {
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
