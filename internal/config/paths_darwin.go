//go:build darwin

package config

import (
	"os"
	"os/user"
	"path/filepath"
	"sync"
)

// DefaultInterceptProcesses 默认监控/重启的进程名（macOS 进程名无扩展名）。
// language server 名称按 Codeium 命名规律推断，需在真机上核实。
var DefaultInterceptProcesses = []string{
	"Devin",
	"language_server_macos_arm",
	"language_server_macos_x64",
}

func defaultDevinExecutable() string { return "/Applications/Devin.app" }

var (
	appDir     string
	appDirOnce sync.Once
)

// AppDir 返回 ~/Library/Application Support/<应用名>。
// 以 sudo 提权运行时 HOME 指向 /var/root，此时回到发起提权的真实用户目录。
func AppDir() string {
	appDirOnce.Do(func() {
		appDir = filepath.Join(RealUserHome(), "Library", "Application Support", appDirName())
	})
	return appDir
}

// RealUserHome 返回发起程序的真实用户主目录（兼容提权场景）。
func RealUserHome() string {
	if home := os.Getenv("BAILAN_USER_HOME"); home != "" {
		return home
	}
	if name := os.Getenv("SUDO_USER"); name != "" && name != "root" {
		if u, err := user.Lookup(name); err == nil {
			return u.HomeDir
		}
	}
	home, _ := os.UserHomeDir()
	return home
}

// HostsPath 返回 macOS hosts 文件路径。
func HostsPath() string { return "/etc/hosts" }
