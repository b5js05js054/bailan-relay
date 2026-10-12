//go:build darwin

// Package sysutil 封装本地端用到的 macOS 系统命令：管理员判断、提权重启、
// 执行命令、刷新 DNS、进程查询与结束、系统文件选择框等。
// 与 sysutil_windows.go 保持同一套 API，供上层平台无关代码调用。
package sysutil

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// IsAdmin 返回当前进程是否具有管理员（root）权限。
func IsAdmin() bool { return os.Geteuid() == 0 }

// Elevate 以管理员身份重新启动当前程序，成功发起返回 true。
// macOS 无 UAC：用 osascript 的 administrator privileges 弹系统授权框，
// 并把发起提权的真实用户信息透传给新进程，避免数据写进 /var/root。
func Elevate(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	parts := []string{
		"BAILAN_USER_HOME=" + shellQuote(realUserHome()),
		"SUDO_USER=" + shellQuote(realUserName()),
		"nohup",
		shellQuote(exe),
	}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	command := strings.Join(parts, " ") + " >/dev/null 2>&1 &"
	script := "do shell script \"" + escapeAppleScript(command) + "\" with administrator privileges"
	_, code, err := RunHidden("osascript", "-e", script)
	return err == nil && code == 0
}

// realUserHome 返回发起提权的真实用户主目录（提权后为 /var/root 时回溯）。
func realUserHome() string {
	if home := os.Getenv("BAILAN_USER_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return home
}

// realUserName 返回发起提权的真实用户名。
func realUserName() string {
	if name := os.Getenv("SUDO_USER"); name != "" {
		return name
	}
	return os.Getenv("USER")
}

// shellQuote 用单引号包裹参数，防止空格与特殊字符被 shell 解析。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// escapeAppleScript 转义 AppleScript 字符串字面量中的反斜杠与双引号。
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// RunHidden 执行命令并返回合并后的输出与退出码（macOS 无控制台窗口概念）。
func RunHidden(name string, args ...string) (string, int, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return string(out), code, err
}

// FlushDNS 刷新系统 DNS 解析缓存并让 mDNSResponder 重载 hosts。
func FlushDNS() {
	_, _, _ = RunHidden("dscacheutil", "-flushcache")
	_, _, _ = RunHidden("killall", "-HUP", "mDNSResponder")
}

// SetConsoleCleanup 注册中断/终止信号时的清理回调。
func SetConsoleCleanup(fn func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sig
		fn()
		os.Exit(0)
	}()
}

// IsProcessRunning 判断指定进程名是否在运行。
// macOS 进程名有长度截断，精确匹配不到时退回命令行匹配。
func IsProcessRunning(imageName string) bool {
	name := imageBase(imageName)
	if out, _, err := RunHidden("pgrep", "-x", name); err == nil && strings.TrimSpace(out) != "" {
		return true
	}
	out, _, err := RunHidden("pgrep", "-f", name)
	return err == nil && strings.TrimSpace(out) != ""
}

// KillImage 按进程名强制结束进程（对应 Windows 的 taskkill）。
func KillImage(imageName string) (string, int, error) {
	return RunHidden("pkill", "-f", imageBase(imageName))
}

// imageBase 去掉可能在配置里残留的 .exe 后缀，得到 macOS 侧的进程名。
func imageBase(imageName string) string {
	name := strings.TrimSpace(imageName)
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name
}

// StartDetached 在指定目录启动一个不阻塞、脱离当前会话的进程。
func StartDetached(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// OpenURL 用系统默认浏览器打开链接。
func OpenURL(raw string) error {
	return StartDetached("", "open", raw)
}

// OpenExeDialog 弹出系统文件选择框（.app），返回所选路径。取消返回错误。
func OpenExeDialog(current string) (string, error) {
	script := `POSIX path of (choose file with prompt "选择 Devin 程序" of type {"app"}`
	if dir := filepath.Dir(strings.TrimSpace(current)); dir != "" && dir != "." {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			script += ` default location POSIX file "` + escapeAppleScript(dir) + `"`
		}
	}
	script += ")"
	out, code, err := RunHidden("osascript", "-e", script)
	if err != nil || code != 0 {
		return "", errors.New("未选择文件")
	}
	path := strings.TrimSpace(out)
	if path == "" {
		return "", errors.New("未选择文件")
	}
	return strings.TrimSuffix(path, "/"), nil
}
