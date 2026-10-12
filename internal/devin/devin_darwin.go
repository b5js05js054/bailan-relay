//go:build darwin

package devin

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

const defaultExecutable = "/Applications/Devin.app"

// appExists 判断路径是否为有效的 .app 应用包。
func appExists(p string) bool {
	if !strings.HasSuffix(p, ".app") {
		return false
	}
	info, err := os.Stat(filepath.Join(p, "Contents", "MacOS"))
	return err == nil && info.IsDir()
}

// DiscoverExecutable 查找 Devin.app，找不到返回空串。
// 输入：已配置路径（.app 或其内部可执行文件均可）；输出：.app 路径。
// 顺序：已配置 → /Applications 与 ~/Applications → Spotlight 索引。
func DiscoverExecutable(configured string) string {
	fast := []string{
		appBundle(strings.TrimSpace(configured)),
		defaultExecutable,
		filepath.Join(config.RealUserHome(), "Applications", "Devin.app"),
	}
	if p := firstExisting(fast, appExists); p != "" {
		return p
	}
	out, _, err := sysutil.RunHidden("mdfind", "kMDItemFSName == 'Devin.app'")
	if err != nil {
		return ""
	}
	return firstExisting(strings.Split(strings.TrimSpace(out), "\n"), appExists)
}

// appBundle 把 .app 内部路径（如 Devin.app/Contents/MacOS/Devin）还原为 .app 路径。
func appBundle(p string) string {
	if i := strings.Index(p, ".app/"); i >= 0 {
		return p[:i+len(".app")]
	}
	return strings.TrimSuffix(p, "/")
}

// mainBinary 从 Info.plist 读取主可执行文件路径，读不到时按应用名推断。
func mainBinary(app string) string {
	out, _, err := sysutil.RunHidden("defaults", "read", filepath.Join(app, "Contents", "Info"), "CFBundleExecutable")
	name := strings.TrimSpace(out)
	if err != nil || name == "" {
		name = strings.TrimSuffix(filepath.Base(app), ".app")
	}
	return filepath.Join(app, "Contents", "MacOS", name)
}

// processName 返回 Devin 主程序的进程名（主可执行文件名）。
func processName(exe string) string {
	app := appBundle(exe)
	if !appExists(app) {
		return strings.TrimSuffix(filepath.Base(app), ".app")
	}
	return filepath.Base(mainBinary(app))
}

// bundledProcesses 返回应用包里随 Devin 分发的 language server 进程名。
func bundledProcesses(exe string) []string {
	var names []string
	binDir := filepath.Join(appBundle(exe), "Contents", "Resources", "app", "extensions", "windsurf", "bin")
	if entries, err := os.ReadDir(binDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), "language_server") {
				names = append(names, e.Name())
			}
		}
	}
	return names
}

// launch 以发起程序的普通用户身份（降权）直接启动 Devin 主程序。
// 本程序以 root 运行时，经 launchctl asuser 进入该用户的 GUI 会话再 sudo -u 降权。
func launch(app string) error {
	cmd := append([]string{"env"}, launchEnv...)
	cmd = append(cmd, mainBinary(app))
	cmd = append(cmd, chromiumArgs()...)
	if !sysutil.IsAdmin() {
		return sysutil.StartDetached(filepath.Dir(app), cmd[0], cmd[1:]...)
	}
	u, err := guiUser()
	if err != nil {
		return err
	}
	args := append([]string{"asuser", u.Uid, "sudo", "-u", u.Username, "-H"}, cmd...)
	return sysutil.StartDetached(filepath.Dir(app), "launchctl", args...)
}

// guiUser 返回应当运行 Devin 的真实用户：提权前的用户，否则当前控制台登录用户。
func guiUser() (*user.User, error) {
	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" {
		out, _, err := sysutil.RunHidden("stat", "-f", "%Su", "/dev/console")
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(out)
	}
	return user.Lookup(name)
}
