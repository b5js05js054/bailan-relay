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
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"bailan-relay/internal/config"
)

// IsAdmin 返回当前进程是否具有管理员（root）权限。
func IsAdmin() bool { return os.Geteuid() == 0 }

// Elevate 以管理员身份重新启动当前程序，成功发起返回 true。
// macOS 无 UAC：用 osascript 的 administrator privileges 弹系统授权框，
// 并把发起提权的真实用户信息透传给新进程，避免数据写进 /var/root。
//
// 关键点：本程序必须拿到 root 才能绑定 127.0.0.1:443，但 root 进程默认落在
// 系统会话里，连不上 WindowServer —— 表现为授权后进程静默退出、窗口永远不出现。
// 因此经 launchctl asuser 把 root 进程放回登录用户的 GUI 会话（root 身份不变）。
func Elevate(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// 取不到桌面用户（如纯 SSH 会话）时 uid 为空，退回普通提权。
	uid := ""
	if u, err := GUIUser(); err == nil && u.Uid != "0" {
		uid = u.Uid
	}
	command := elevatedCommand(exe, args, uid, realUserHome(), realUserName(), config.LogDir())
	script := "do shell script \"" + escapeAppleScript(command) + "\" with administrator privileges"
	_, code, err := RunHidden("osascript", "-e", script)
	return err == nil && code == 0
}

// elevatedCommand 拼出提权重启用的 shell 命令。
// 输入：可执行文件路径、原样透传的命令行参数、桌面用户 uid（空=退回普通提权）、
// 真实用户主目录与用户名、日志目录；
// 输出：mkdir -p <日志目录> && launchctl asuser <uid> env <变量> nohup <exe> <参数> >> <日志> 2>&1 &
// uid 非空时把 root 进程放进该用户的 GUI 会话（root 身份不变），否则窗口无法出现。
// 提权进程的 stdout/stderr 落到日志文件，授权后窗口没出现时可据此排查。
func elevatedCommand(exe string, args []string, uid, home, name, logDir string) string {
	inner := []string{
		"/usr/bin/env",
		"BAILAN_USER_HOME=" + shellQuote(home),
		"SUDO_USER=" + shellQuote(name),
		"/usr/bin/nohup",
		shellQuote(exe),
	}
	for _, a := range args {
		inner = append(inner, shellQuote(a))
	}
	if uid != "" {
		inner = append([]string{"/bin/launchctl", "asuser", uid}, inner...)
	}
	logPath := filepath.Join(logDir, "elevate.log")
	return "/bin/mkdir -p " + shellQuote(logDir) + " && " +
		strings.Join(inner, " ") + " >>" + shellQuote(logPath) + " 2>&1 &"
}

// realUserHome 返回发起提权的真实用户主目录（提权后为 /var/root 时回溯）。
func realUserHome() string {
	if home := os.Getenv("BAILAN_USER_HOME"); home != "" {
		return home
	}
	if name := os.Getenv("SUDO_USER"); name != "" && name != "root" {
		if u, err := user.Lookup(name); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/var/root" {
		return home
	}
	if u, err := GUIUser(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	home, _ := os.UserHomeDir()
	return home
}

// realUserName 返回发起提权的真实用户名。
func realUserName() string {
	if name := os.Getenv("SUDO_USER"); name != "" && name != "root" {
		return name
	}
	if u, err := GUIUser(); err == nil && u.Username != "" {
		return u.Username
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
// 以 root 运行时浏览器必须由登录用户拉起：root 身份或系统会话下打开的窗口
// 不会出现在用户桌面上（表现为「点了没反应」），故经 launchctl asuser 降回用户身份。
func OpenURL(raw string) error {
	if IsAdmin() {
		if u, err := GUIUser(); err == nil && u.Uid != "0" {
			return StartDetached("", "launchctl", "asuser", u.Uid, "sudo", "-u", u.Username, "-H", "open", raw)
		}
	}
	return StartDetached("", "open", raw)
}

// OpenExeDialog 弹出系统文件选择框（.app），返回所选路径。取消返回错误。
func OpenExeDialog(current string) (string, error) {
	dir := filepath.Dir(strings.TrimSpace(current))
	path, err := ChooseFile("选择 Devin 程序", []string{"app"}, dir)
	return strings.TrimSuffix(path, "/"), err
}

// ChooseFile 弹出系统文件选择框并返回所选文件的 POSIX 路径；取消返回错误。
// 输入：提示语、允许的类型（扩展名或 UTI，空=不限）、默认目录（不存在则忽略）。
// 本程序以 root 运行时 WKWebView/osascript 的面板不会出现在用户桌面上，
// 因此经 launchctl asuser + sudo -u 回到登录用户的 GUI 会话里弹框。
func ChooseFile(prompt string, types []string, dir string) (string, error) {
	script := `activate` + "\n" + `POSIX path of (choose file with prompt "` + escapeAppleScript(prompt) + `"`
	if len(types) > 0 {
		quoted := make([]string, len(types))
		for i, t := range types {
			quoted[i] = `"` + escapeAppleScript(t) + `"`
		}
		script += ` of type {` + strings.Join(quoted, ",") + `}`
	}
	if dir != "" && dir != "." {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			script += ` default location POSIX file "` + escapeAppleScript(dir) + `"`
		}
	}
	script += ")"
	name, args := "osascript", []string{"-e", script}
	if IsAdmin() {
		if u, err := GUIUser(); err == nil && u.Uid != "0" {
			name, args = "launchctl", append([]string{"asuser", u.Uid, "sudo", "-u", u.Username, "-H", "osascript"}, args...)
		}
	}
	out, code, err := RunHidden(name, args...)
	path := strings.TrimSpace(out)
	if err != nil || code != 0 || !strings.HasPrefix(path, "/") {
		return "", errors.New("未选择文件")
	}
	return path, nil
}

// GUIUser 返回当前桌面的真实用户：提权前的用户，否则当前控制台登录用户。
func GUIUser() (*user.User, error) {
	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" {
		out, _, err := RunHidden("stat", "-f", "%Su", "/dev/console")
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(out)
	}
	return user.Lookup(name)
}
