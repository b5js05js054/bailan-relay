//go:build darwin

package controller

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"

	"bailan-relay/internal/sysutil"
)

const pickImageMax = 5 * 1024 * 1024

// apiPickImage 用系统文件框选一张图片并读成 data URL（macOS 下 WKWebView 的 <input type=file> 弹不出面板）。
// 输入：[提示语]；输出：{ok, name, type, size, data_url} 或 {ok:false, message}，取消时 cancelled=true。
func (c *Controller) apiPickImage(args []any) any {
	prompt := argString(args, 0)
	if prompt == "" {
		prompt = "选择图片"
	}
	home, _ := os.UserHomeDir()
	if u, err := sysutil.GUIUser(); err == nil {
		home = u.HomeDir
	}
	path, err := sysutil.ChooseFile(prompt, []string{"public.png", "public.jpeg", "org.webmproject.webp", "com.compuserve.gif"}, filepath.Join(home, "Pictures"))
	if err != nil {
		return map[string]any{"ok": false, "cancelled": true, "message": err.Error()}
	}
	info, err := os.Stat(path)
	if err != nil {
		return map[string]any{"ok": false, "message": "读取图片失败：" + err.Error()}
	}
	if info.Size() > pickImageMax {
		return map[string]any{"ok": false, "message": "图片不能超过 5MB"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"ok": false, "message": "读取图片失败：" + err.Error()}
	}
	mime := http.DetectContentType(data)
	return map[string]any{
		"ok": true, "name": filepath.Base(path), "type": mime, "size": len(data),
		"data_url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
	}
}
