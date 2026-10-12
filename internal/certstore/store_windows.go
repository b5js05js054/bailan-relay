//go:build windows

package certstore

import (
	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

// IsInstalled 通过 certutil 检查 CA 是否已装入 Root 存储。
func IsInstalled() (bool, string) {
	out, code, _ := sysutil.RunHidden("certutil", "-verifystore", "Root", config.CertCN())
	return code == 0, out
}

// Install 生成缺失的证书后，把 CA 写入 Windows Root 存储。
func Install() (bool, string) {
	if !FileExists() {
		if _, err := LoadOrCreate(); err != nil {
			return false, "证书自动生成失败：" + err.Error()
		}
	}
	out, code, _ := sysutil.RunHidden("certutil", "-addstore", "Root", config.CertFile())
	return code == 0, out
}

// Uninstall 从 Windows Root 存储移除 CA。
func Uninstall() (bool, string) {
	out, code, _ := sysutil.RunHidden("certutil", "-delstore", "Root", config.CertCN())
	return code == 0, out
}
