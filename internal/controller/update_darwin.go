//go:build darwin

package controller

import "errors"

// selfUpdateSupported macOS 的 .app 替换涉及签名与 Gatekeeper，暂不做自动更新。
const selfUpdateSupported = false

func installUpdate(string, string, string) error {
	return errors.New("Mac 版暂不支持自动更新")
}
