//go:build windows

package controller

// apiPickImage Windows 直接用网页的 <input type=file>，不走此接口。
func (c *Controller) apiPickImage([]any) any {
	return map[string]any{"ok": false, "message": "当前平台请直接使用网页文件选择"}
}
