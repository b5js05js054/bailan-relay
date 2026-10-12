//go:build darwin

package sysutil

import "testing"

// TestElevatedCommandInUserSession 校验提权命令把 root 进程放进登录用户的 GUI 会话
// （launchctl asuser），带上真实用户信息，并把输出重定向到日志。
func TestElevatedCommandInUserSession(t *testing.T) {
	got := elevatedCommand(
		"/Applications/Devin_hub.app/Contents/MacOS/bailan-relay",
		[]string{"-x"},
		"501",
		"/Users/ys",
		"ys",
		"/Users/ys/Library/Application Support/Devin_hub/logs",
	)
	want := "/bin/mkdir -p '/Users/ys/Library/Application Support/Devin_hub/logs' && " +
		"/bin/launchctl asuser 501 /usr/bin/env BAILAN_USER_HOME='/Users/ys' SUDO_USER='ys' " +
		"/usr/bin/nohup '/Applications/Devin_hub.app/Contents/MacOS/bailan-relay' '-x' " +
		">>'/Users/ys/Library/Application Support/Devin_hub/logs/elevate.log' 2>&1 &"
	if got != want {
		t.Fatalf("提权命令不正确\n得到: %s\n期望: %s", got, want)
	}
}

// TestElevatedCommandWithoutGUIUser 校验拿不到桌面用户时不带 launchctl asuser，
// 只做普通提权（此时窗口可能无法出现，但至少留下日志）。
func TestElevatedCommandWithoutGUIUser(t *testing.T) {
	got := elevatedCommand("/tmp/app", nil, "", "/Users/ys", "ys", "/tmp/logs")
	want := "/bin/mkdir -p '/tmp/logs' && " +
		"/usr/bin/env BAILAN_USER_HOME='/Users/ys' SUDO_USER='ys' " +
		"/usr/bin/nohup '/tmp/app' " +
		">>'/tmp/logs/elevate.log' 2>&1 &"
	if got != want {
		t.Fatalf("提权命令不正确\n得到: %s\n期望: %s", got, want)
	}
}
