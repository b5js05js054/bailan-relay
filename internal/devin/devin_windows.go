//go:build windows

package devin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

const defaultExecutable = `E:\Devin\Devin.exe`

// DiscoverExecutable 查找 Devin.exe，找不到返回空串。
// 分级探测、逐级惰性执行：已配置路径与常见目录 stat（零进程调用）→ 运行中进程 →
// 注册表安装记录 → 固定磁盘常见目录扫描。任一级命中即返回，常见场景毫秒级完成。
func DiscoverExecutable(configured string) string {
	// 第 1 级：已配置路径 + 历史/标准安装目录（纯 stat，零开销）。
	fast := []string{strings.TrimSpace(configured),
		defaultExecutable,
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Devin", "Devin.exe"),
		filepath.Join(defaultProgramFiles(), "Devin", "Devin.exe"),
	}
	if p := firstExisting(fast, fileExists); p != "" {
		return p
	}
	// 第 2 级：正在运行的进程取真实路径（装在非标准位置时最准）。
	if p := runningProcessPath(); fileExists(p) {
		return p
	}
	// 第 3 级：注册表 App Paths / Uninstall 记录。
	if p := firstExisting(registryCandidates(), fileExists); p != "" {
		return p
	}
	// 第 4 级：逐固定磁盘扫常见目录（跳过网盘/可移动介质，防止 ReadDir 阻塞）。
	return firstExisting(driveScanCandidates(), fileExists)
}

// processName 返回 Devin 主程序对应的进程映像名（即 exe 文件名）。
func processName(exe string) string { return filepath.Base(exe) }

// bundledProcesses 返回安装目录里随 Devin 分发的 language server / CLI 映像名。
func bundledProcesses(exe string) []string {
	var names []string
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
	if fileExists(nested) {
		names = append(names, filepath.Base(nested))
	}
	return names
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

// launch 写出启动脚本，经 explorer 以普通用户权限（降权）启动 Devin。
func launch(devinExe string) error {
	launcher, err := writeLauncher(devinExe)
	if err != nil {
		return err
	}
	return sysutil.StartDetached(filepath.Dir(devinExe), "explorer.exe", launcher)
}

func writeLauncher(devinExe string) (string, error) {
	launcher := filepath.Join(config.AppDir(), "launch_devin.cmd")
	lines := []string{"@echo off"}
	for _, kv := range launchEnv {
		lines = append(lines, "set "+kv)
	}
	lines = append(lines,
		`cd /d "`+filepath.Dir(devinExe)+`"`,
		`start "" "`+devinExe+`" `+windowsArgs(),
	)
	content := strings.Join(lines, "\r\n") + "\r\n"
	if err := os.MkdirAll(config.AppDir(), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(launcher, []byte(content), 0o644); err != nil {
		return "", err
	}
	return launcher, nil
}

// windowsArgs 把 Chromium 参数拼成 cmd 命令行；输出：启动脚本用的参数串（值含空格的开关加引号）。
func windowsArgs() string {
	args := chromiumArgs()
	for i, arg := range args {
		if k, v, ok := strings.Cut(arg, "="); ok && strings.Contains(v, " ") {
			args[i] = k + `="` + v + `"`
		}
	}
	return strings.Join(args, " ")
}
