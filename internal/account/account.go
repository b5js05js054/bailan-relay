// Package account 管理客户端账号会话：注册（带头像）、登录、资料、自定义背景。
//
// 会话令牌持久化到 user.json；上传走硬化拨号并实时记录已发送字节，供界面轮询进度条。
// 账号令牌同时提供给 license 包（激活/心跳绑定卡密）与模型目录拉取（按用户过滤）。
// 自定义背景只存本地：user.json 同级 backgrounds/user-<id>.<ext>，按账号区分，退出登录不删除。
package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Profile 是后台 public_profile 的客户端视图；头像地址已改写为本地代理路径 /userfile/<name>，
// 背景地址指向本地文件 /userbg/?v=<修改时间>（无背景为空）。
type Profile struct {
	ID            int    `json:"id"`
	Email         string `json:"email"`
	AvatarURL     string `json:"avatar_url"`
	BackgroundURL string `json:"background_url"`
}

type state struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      Profile   `json:"user"`
}

// Progress 是当前上传的实时状态。Phase: uploading（发送中）/ processing（已发完等待服务器）/ done / error。
type Progress struct {
	Active  bool    `json:"active"`
	Phase   string  `json:"phase"`
	Sent    int64   `json:"sent"`
	Total   int64   `json:"total"`
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
}

// ErrUnauthorized 后台判定账号令牌无效/过期/账号被禁用。
var ErrUnauthorized = errors.New("登录已失效，请重新登录")

// FileNameRE 与后台随机文件名规则一致，防止代理路径被用来穿越。
var FileNameRE = regexp.MustCompile(`^[0-9a-f]{32}\.(png|jpg|gif|webp)$`)

const uploadTimeout = 5 * time.Minute

// BackgroundMaxBytes 本地背景图大小上限。
const BackgroundMaxBytes = 5 << 20

// backgroundExts 按内容嗅探出的 MIME → 本地扩展名。
var backgroundExts = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}

// Manager 持有账号会话与上传进度。
type Manager struct {
	apiURL func() string
	path   string
	client *http.Client

	mu    sync.RWMutex
	state state

	progMu sync.RWMutex
	prog   Progress
}

// New 输入：apiURL 返回当前后台地址；path 会话文件；dial 硬化拨号（nil 用默认）。输出：已加载本地会话的管理器。
func New(apiURL func() string, path string, dial func(context.Context, string, string) (net.Conn, error)) *Manager {
	m := &Manager{apiURL: apiURL, path: path, client: &http.Client{Transport: &http.Transport{DialContext: dial}}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &m.state)
	}
	return m
}

// Token 返回未过期的账号令牌；未登录/已过期返回空串。
func (m *Manager) Token() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state.Token == "" || !time.Now().Before(m.state.ExpiresAt) {
		return ""
	}
	return m.state.Token
}

// Session 返回 (当前资料, 是否已登录)；BackgroundURL 按本地背景文件实时填充。
func (m *Manager) Session() (Profile, bool) {
	ok := m.Token() != ""
	m.mu.RLock()
	p := m.state.User
	m.mu.RUnlock()
	p.BackgroundURL = ""
	if path, found := m.backgroundFile(p.ID); ok && found {
		if st, err := os.Stat(path); err == nil {
			p.BackgroundURL = fmt.Sprintf("/userbg/?v=%d", st.ModTime().UnixNano())
		}
	}
	return p, ok
}

// Validate 向后台确认会话仍有效并刷新资料。令牌失效则清空会话；网络错误保留本地会话（离线可继续展示）。
func (m *Manager) Validate() (Profile, bool) {
	token := m.Token()
	if token == "" {
		return Profile{}, false
	}
	out, err := m.do(http.MethodGet, "/api/user/me", token, "", nil, false)
	if errors.Is(err, ErrUnauthorized) {
		m.Logout()
		return Profile{}, false
	}
	if err == nil {
		m.setUser(out.User)
	}
	return m.Session()
}

// Login 输入邮箱/密码，成功后保存会话并返回资料。
func (m *Manager) Login(email, password string) (Profile, error) {
	payload, _ := json.Marshal(map[string]string{"email": strings.TrimSpace(email), "password": password})
	return m.saveSession(m.do(http.MethodPost, "/api/user/login", "", "application/json", payload, false))
}

// Register 输入邮箱/密码/头像字节，multipart 上传（带进度），成功即登录。
func (m *Manager) Register(email, password string, avatar []byte, filename string) (Profile, error) {
	body, ctype := multipartBody(map[string]string{"email": strings.TrimSpace(email), "password": password}, "avatar", filename, avatar)
	return m.saveSession(m.do(http.MethodPost, "/api/user/register", "", ctype, body, true))
}

// SetBackground 输入图片字节，校验（≤5MB，按内容识别 png/jpg/gif/webp）后保存为当前账号的本地背景，返回更新后的资料。
func (m *Manager) SetBackground(image []byte) (Profile, error) {
	p, ok := m.Session()
	if !ok {
		return Profile{}, ErrUnauthorized
	}
	if len(image) > BackgroundMaxBytes {
		return Profile{}, fmt.Errorf("背景图不能超过 5MB")
	}
	ext, valid := backgroundExts[http.DetectContentType(image)]
	if !valid {
		return Profile{}, fmt.Errorf("背景图仅支持 jpg / png / webp / gif")
	}
	if err := m.removeBackground(p.ID); err != nil {
		return Profile{}, err
	}
	dir := m.backgroundDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Profile{}, fmt.Errorf("保存背景失败：%w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("user-%d%s", p.ID, ext)), image, 0o644); err != nil {
		return Profile{}, fmt.Errorf("保存背景失败：%w", err)
	}
	p, _ = m.Session()
	return p, nil
}

// ClearBackground 删除当前账号的本地背景，恢复默认。
func (m *Manager) ClearBackground() (Profile, error) {
	p, ok := m.Session()
	if !ok {
		return Profile{}, ErrUnauthorized
	}
	if err := m.removeBackground(p.ID); err != nil {
		return Profile{}, err
	}
	p, _ = m.Session()
	return p, nil
}

// BackgroundFile 输出当前已登录账号的本地背景文件路径；未登录或无背景时 ok=false。
func (m *Manager) BackgroundFile() (string, bool) {
	p, ok := m.Session()
	if !ok {
		return "", false
	}
	return m.backgroundFile(p.ID)
}

func (m *Manager) backgroundDir() string { return filepath.Join(filepath.Dir(m.path), "backgrounds") }

// backgroundFile 输入用户 id，输出已存在的背景文件路径。
func (m *Manager) backgroundFile(id int) (string, bool) {
	if id <= 0 {
		return "", false
	}
	for _, ext := range backgroundExts {
		path := filepath.Join(m.backgroundDir(), fmt.Sprintf("user-%d%s", id, ext))
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func (m *Manager) removeBackground(id int) error {
	for {
		path, ok := m.backgroundFile(id)
		if !ok {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("删除旧背景失败：%w", err)
		}
	}
}

// Logout 清除本地会话。
func (m *Manager) Logout() {
	m.mu.Lock()
	m.state = state{}
	m.persistLocked()
	m.mu.Unlock()
}

// Progress 返回当前上传进度快照。
func (m *Manager) Progress() Progress {
	m.progMu.RLock()
	defer m.progMu.RUnlock()
	return m.prog
}

// FetchFile 输入合法的随机文件名，输出后台图片响应（调用方负责关闭 Body）。
func (m *Manager) FetchFile(ctx context.Context, name string) (*http.Response, error) {
	if !FileNameRE.MatchString(name) {
		return nil, fmt.Errorf("文件名不合法")
	}
	base := m.base()
	if base == "" {
		return nil, fmt.Errorf("未配置授权服务器地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/user/files/"+name, nil)
	if err != nil {
		return nil, err
	}
	return m.client.Do(req)
}

// sessionResponse 是 register/login/me 的公共响应体。
type sessionResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      Profile   `json:"user"`
}

func (m *Manager) saveSession(out *sessionResponse, err error) (Profile, error) {
	if err != nil {
		return Profile{}, err
	}
	if out.Token == "" {
		return Profile{}, fmt.Errorf("服务器未返回登录令牌")
	}
	out.User = localize(out.User)
	m.mu.Lock()
	m.state = state{Token: out.Token, ExpiresAt: out.ExpiresAt, User: out.User}
	m.persistLocked()
	m.mu.Unlock()
	p, _ := m.Session()
	return p, nil
}

func (m *Manager) setUser(p Profile) {
	m.mu.Lock()
	m.state.User = localize(p)
	m.persistLocked()
	m.mu.Unlock()
}

func (m *Manager) persistLocked() {
	_ = os.MkdirAll(filepath.Dir(m.path), 0o755)
	data, _ := json.MarshalIndent(m.state, "", "  ")
	_ = os.WriteFile(m.path, append(data, '\n'), 0o600)
}

func (m *Manager) base() string { return strings.TrimRight(strings.TrimSpace(m.apiURL()), "/") }

// do 输入：方法/路径/令牌/内容类型/请求体/是否记录上传进度。
// 输出：解析后的 sessionResponse；非 2xx 转为后台 detail 文案，401 统一为 ErrUnauthorized。
func (m *Manager) do(method, path, token, ctype string, body []byte, track bool) (*sessionResponse, error) {
	base := m.base()
	if base == "" {
		return nil, fmt.Errorf("未配置授权服务器地址")
	}
	timeout := 15 * time.Second
	var reader io.Reader = bytes.NewReader(body)
	if track {
		timeout = uploadTimeout
		m.setProgress(Progress{Active: true, Phase: "uploading", Total: int64(len(body))})
		reader = &progressReader{r: reader, m: m}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
	if err != nil {
		return nil, m.fail(track, err)
	}
	req.ContentLength = int64(len(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, m.fail(track, fmt.Errorf("无法连接服务器：%w", err))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized && token != "" {
		return nil, m.fail(track, ErrUnauthorized)
	}
	if resp.StatusCode/100 != 2 {
		return nil, m.fail(track, detailError(resp.StatusCode, raw))
	}
	var out sessionResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, m.fail(track, fmt.Errorf("服务器返回异常（HTTP %d）", resp.StatusCode))
	}
	if track {
		m.setProgress(Progress{Phase: "done", Sent: int64(len(body)), Total: int64(len(body)), Percent: 100})
	}
	return &out, nil
}

func (m *Manager) fail(track bool, err error) error {
	if track {
		p := m.Progress()
		m.setProgress(Progress{Phase: "error", Sent: p.Sent, Total: p.Total, Percent: p.Percent, Message: err.Error()})
	}
	return err
}

func (m *Manager) setProgress(p Progress) {
	m.progMu.Lock()
	m.prog = p
	m.progMu.Unlock()
}

// addSent 累加已发送字节；发完后进入 processing（等待服务器校验落盘）。
func (m *Manager) addSent(n int64) {
	m.progMu.Lock()
	defer m.progMu.Unlock()
	m.prog.Sent += n
	if m.prog.Total > 0 {
		m.prog.Percent = float64(m.prog.Sent) * 100 / float64(m.prog.Total)
	}
	if m.prog.Sent >= m.prog.Total {
		m.prog.Phase = "processing"
	}
}

type progressReader struct {
	r io.Reader
	m *Manager
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.m.addSent(int64(n))
	}
	return n, err
}

// multipartBody 输入表单字段与单个文件，输出完整请求体（先整体构造以得到准确总长）与 Content-Type。
func multipartBody(fields map[string]string, fileField, filename string, data []byte) ([]byte, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if filename == "" {
		filename = "image"
	}
	part, _ := w.CreateFormFile(fileField, filepath.Base(filename))
	_, _ = part.Write(data)
	_ = w.Close()
	return buf.Bytes(), w.FormDataContentType()
}

// detailError 把 FastAPI 的 {"detail": "..."} 或校验错误列表转为可读错误。
func detailError(status int, raw []byte) error {
	var e struct {
		Detail any `json:"detail"`
	}
	if json.Unmarshal(raw, &e) == nil {
		switch d := e.Detail.(type) {
		case string:
			if d != "" {
				return errors.New(d)
			}
		case []any:
			return errors.New("提交的信息不完整或格式有误")
		}
	}
	return fmt.Errorf("服务器返回异常（HTTP %d）", status)
}

// localize 把后台头像路径 /api/user/files/<name> 改写为本地代理 /userfile/<name>，不暴露后台地址。
func localize(p Profile) Profile {
	p.AvatarURL = localPath(p.AvatarURL)
	p.BackgroundURL = ""
	return p
}

func localPath(u string) string {
	name := u[strings.LastIndex(u, "/")+1:]
	if !FileNameRE.MatchString(name) {
		return ""
	}
	return "/userfile/" + name
}
