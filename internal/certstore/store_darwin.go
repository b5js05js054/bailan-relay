//go:build darwin

package certstore

import (
	"strings"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

const systemKeychain = "/Library/Keychains/System.keychain"

// IsInstalled 检查 CA 是否已存在于系统钥匙串，且被系统信任为 SSL 根证书。
func IsInstalled() (bool, string) {
	out, code, _ := sysutil.RunHidden("security", "find-certificate", "-c", config.CertCN(), systemKeychain)
	if code != 0 {
		return false, out
	}
	vout, vcode, _ := sysutil.RunHidden("security", "verify-cert", "-c", config.CertFile(), "-p", "ssl")
	return vcode == 0, strings.TrimSpace(out + "\n" + vout)
}

// Install 生成缺失的证书后，把 CA 以「始终信任」写入系统钥匙串（需要 root）。
func Install() (bool, string) {
	if !FileExists() {
		if _, err := LoadOrCreate(); err != nil {
			return false, "证书自动生成失败：" + err.Error()
		}
	}
	out, code, _ := sysutil.RunHidden("security", "add-trusted-cert", "-d", "-r", "trustRoot",
		"-k", systemKeychain, config.CertFile())
	return code == 0, out
}

// Uninstall 从系统钥匙串移除 CA（同名证书可能多份，逐个删到没有为止）。
func Uninstall() (bool, string) {
	var log []string
	for i := 0; i < 10; i++ {
		out, code, _ := sysutil.RunHidden("security", "delete-certificate", "-c", config.CertCN(), "-t", systemKeychain)
		if code != 0 {
			if i == 0 {
				return false, out
			}
			break
		}
		log = append(log, strings.TrimSpace(out))
	}
	return true, strings.Join(log, "\n")
}
