package controller

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"bailan-relay/internal/account"
)

// SetAccountManager 注入账号会话管理器，并把账号令牌接入卡密激活/心跳。
func (c *Controller) SetAccountManager(m *account.Manager) {
	c.account = m
	if c.license != nil {
		c.license.SetUserTokenProvider(m.Token)
	}
}

func (c *Controller) userToken() string {
	if c.account == nil {
		return ""
	}
	return c.account.Token()
}

// accountView 输出前端用的会话视图：{logged_in, user}。
func accountView(p account.Profile, ok bool) map[string]any {
	if !ok {
		return map[string]any{"logged_in": false, "user": nil}
	}
	return map[string]any{"logged_in": true, "user": p}
}

func (c *Controller) accountResult(p account.Profile, err error, okMsg string) any {
	if err != nil {
		return map[string]any{"ok": false, "message": err.Error(), "account": accountView(c.account.Session())}
	}
	return map[string]any{"ok": true, "message": okMsg, "account": accountView(p, true)}
}

// onLogin 登录/注册成功后：强制刷新按用户过滤的模型目录，并让旧卡密立即带账号心跳完成绑定。
func (c *Controller) onLogin() {
	go func() {
		c.clientBundle(true)
		if c.license != nil && c.license.Valid() {
			c.license.Refresh()
		}
		c.refreshStatusData(false)
	}()
}

// apiUserSession 参数无；校验本地会话（令牌失效自动登出）并返回 {ok, account}。
func (c *Controller) apiUserSession([]any) any {
	return map[string]any{"ok": true, "account": accountView(c.account.Validate())}
}

// apiUserLogin 参数 [email, password]。
func (c *Controller) apiUserLogin(args []any) any {
	p, err := c.account.Login(argString(args, 0), argString(args, 1))
	if err == nil {
		c.onLogin()
	}
	return c.accountResult(p, err, "登录成功")
}

// apiUserRegister 参数 [{email, password, avatar: base64, filename}]；上传进度经 user_upload_progress 轮询。
func (c *Controller) apiUserRegister(args []any) any {
	payload := argMap(args, 0)
	data, err := decodeImage(mapString(payload, "avatar"))
	if err != nil {
		return map[string]any{"ok": false, "message": "头像数据无效"}
	}
	p, err := c.account.Register(mapString(payload, "email"), mapString(payload, "password"), data, mapString(payload, "filename"))
	if err == nil {
		c.onLogin()
	}
	return c.accountResult(p, err, "注册成功")
}

// apiUserSetBackground 参数 [{image: base64}]；背景只保存在本地，不上传服务器。
func (c *Controller) apiUserSetBackground(args []any) any {
	data, err := decodeImage(mapString(argMap(args, 0), "image"))
	if err != nil {
		return map[string]any{"ok": false, "message": "图片数据无效"}
	}
	p, err := c.account.SetBackground(data)
	return c.accountResult(p, err, "背景已更换")
}

// apiUserClearBackground 参数无；恢复默认背景。
func (c *Controller) apiUserClearBackground([]any) any {
	p, err := c.account.ClearBackground()
	return c.accountResult(p, err, "已恢复默认背景")
}

// apiUserLogout 参数无；清除会话，若拦截在运行则一并停止（未登录不可使用）。
func (c *Controller) apiUserLogout([]any) any {
	c.account.Logout()
	if c.proxy.Running() {
		c.apiStopProxy(nil)
	}
	c.clientBundle(true)
	return map[string]any{"ok": true, "message": "已退出登录", "account": accountView(account.Profile{}, false)}
}

// apiUserUploadProgress 参数无；输出当前上传进度。
func (c *Controller) apiUserUploadProgress([]any) any {
	return map[string]any{"ok": true, "progress": c.account.Progress()}
}

// decodeImage 输入 base64（允许 data:...;base64, 前缀），输出原始字节。
func decodeImage(raw string) ([]byte, error) {
	if i := strings.Index(raw, ","); strings.HasPrefix(raw, "data:") && i > 0 {
		raw = raw[i+1:]
	}
	return base64.StdEncoding.DecodeString(raw)
}

// serveUserBackground 输出当前账号的本地背景文件（GET /userbg/）；无背景返回 404。
func (c *Controller) serveUserBackground(w http.ResponseWriter, r *http.Request) {
	path, ok := c.account.BackgroundFile()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

// serveUserFile 代理 GET /userfile/<name> 到后台图片接口：页面不接触后台地址，且走硬化拨号。
func (c *Controller) serveUserFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/userfile/")
	resp, err := c.account.FetchFile(r.Context(), name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "not found", resp.StatusCode)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	_, _ = io.Copy(w, resp.Body)
}
