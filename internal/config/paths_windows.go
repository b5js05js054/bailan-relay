//go:build windows

package config

import (
	"os"
	"path/filepath"
	"sync"
)

// DefaultInterceptProcesses 默认监控/重启的进程名。
var DefaultInterceptProcesses = []string{
	"Devin.exe",
	"devin.exe",
	"language_server_windows_x64.exe",
}

func defaultDevinExecutable() string { return `E:\Devin\Devin.exe` }

var (
	appDir     string
	appDirOnce sync.Once
)

// AppDir 返回配置/日志/证书的根目录：%LOCALAPPDATA%\BaiLan Proxy Manager。
func AppDir() string {
	appDirOnce.Do(func() {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Local")
		}
		appDir = filepath.Join(base, appDirName())
	})
	return appDir
}

// HostsPath 返回 Windows hosts 文件路径。
func HostsPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}
