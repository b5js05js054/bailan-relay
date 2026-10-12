package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bailan-relay/internal/config"
)

// clientAnnouncement 是后台下发到客户端的公告。
// Enabled 用指针区分「未携带」与「显式 false」：旧版后台只在启用时才返回对象且不附 enabled。
type clientAnnouncement struct {
	Enabled *bool  `json:"enabled"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Level   string `json:"level"`
}

// clientUpdate 是后台下发到客户端的软件更新配置。
type clientUpdate struct {
	Enabled     *bool  `json:"enabled"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	URLMacOS    string `json:"url_macos"`
	SHA256MacOS string `json:"sha256_macos"`
	Mandatory   bool   `json:"mandatory"`
	Note        string `json:"note"`
}

type clientBundle struct {
	models       []config.Model
	announcement *clientAnnouncement
	update       *clientUpdate
}

// updateView 是返回给前端展示的更新信息。
type updateView struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256,omitempty"`
	Mandatory bool   `json:"mandatory"`
	HasUpdate bool   `json:"has_update"`
	Current   string `json:"current"`
	Note      string `json:"note"`
}

// updateProgressView 是前端轮询的实时下载状态。
type updateProgressView struct {
	Active     bool    `json:"active"`
	Phase      string  `json:"phase"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Percent    float64 `json:"percent"`
	Message    string  `json:"message"`
}

func (c *Controller) setUpdateProgress(progress updateProgressView) {
	c.updateProgressMu.Lock()
	c.updateProgress = progress
	c.updateProgressMu.Unlock()
}

func (c *Controller) apiUpdateProgress([]any) any {
	c.updateProgressMu.RLock()
	progress := c.updateProgress
	c.updateProgressMu.RUnlock()
	return map[string]any{
		"ok":         true,
		"active":     progress.Active,
		"phase":      progress.Phase,
		"downloaded": progress.Downloaded,
		"total":      progress.Total,
		"percent":    progress.Percent,
		"message":    progress.Message,
	}
}

const clientConfigCacheTTL = 10 * time.Second

// clientBundle 返回客户端发布配置（模型目录 + 公告 + 更新），优先使用缓存。
func (c *Controller) clientBundle(force bool) clientBundle {
	c.modelsMu.RLock()
	cached := clientBundle{
		models:       append([]config.Model(nil), c.models...),
		announcement: c.announcement,
		update:       c.update,
	}
	fresh := !force && len(cached.models) > 0 && time.Since(c.modelsAt) < clientConfigCacheTTL
	c.modelsMu.RUnlock()
	if fresh {
		return cached
	}

	bundle, err := c.fetchClientBundle()
	if err == nil {
		if len(bundle.models) > 0 {
			config.SetSelectableModels(bundle.models)
			bundle.models = config.Models()
		} else {
			bundle.models = config.Models()
		}
		c.modelsMu.Lock()
		c.models = append([]config.Model(nil), bundle.models...)
		c.announcement = bundle.announcement
		c.update = bundle.update
		c.modelsAt = time.Now()
		c.modelsMu.Unlock()
		return bundle
	}
	if len(cached.models) > 0 {
		return cached
	}
	return clientBundle{models: config.Models()}
}

func (c *Controller) fetchClientBundle() (clientBundle, error) {
	base := strings.TrimRight(strings.TrimSpace(config.Load().LicenseAPIURL), "/")
	if base == "" {
		return clientBundle{}, fmt.Errorf("未配置授权服务地址")
	}
	var payload struct {
		Models       []config.Model      `json:"models"`
		Announcement *clientAnnouncement `json:"announcement"`
		Update       *clientUpdate       `json:"update"`
	}
	if err := c.getJSON(base+"/api/license/client-config", &payload); err != nil {
		// 兼容旧版后台：只有 /api/license/models。
		var legacy struct {
			Models []config.Model `json:"models"`
		}
		if legacyErr := c.getJSON(base+"/api/license/models", &legacy); legacyErr != nil {
			return clientBundle{}, err
		}
		payload.Models = legacy.Models
	}
	return clientBundle{
		models:       payload.Models,
		announcement: normalizeAnnouncement(payload.Announcement),
		update:       normalizeUpdate(payload.Update),
	}, nil
}

func (c *Controller) getJSON(url string, out any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// 带账号令牌：后台按用户过滤指定用户可见的模型。
	if token := c.userToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.modelClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}

func normalizeAnnouncement(in *clientAnnouncement) *clientAnnouncement {
	if in == nil || (in.Enabled != nil && !*in.Enabled) {
		return nil
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return nil
	}
	level := strings.ToLower(strings.TrimSpace(in.Level))
	switch level {
	case "success", "warning", "error":
	default:
		level = "info"
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "公告"
	}
	return &clientAnnouncement{Title: title, Content: content, Level: level}
}

func normalizeUpdate(in *clientUpdate) *clientUpdate {
	if in == nil || (in.Enabled != nil && !*in.Enabled) {
		return nil
	}
	version := strings.TrimSpace(in.Version)
	if version == "" {
		return nil
	}
	return &clientUpdate{
		Version:     version,
		URL:         strings.TrimSpace(in.URL),
		SHA256:      strings.ToLower(strings.TrimSpace(in.SHA256)),
		URLMacOS:    strings.TrimSpace(in.URLMacOS),
		SHA256MacOS: strings.ToLower(strings.TrimSpace(in.SHA256MacOS)),
		Mandatory:   in.Mandatory,
		Note:        strings.TrimSpace(in.Note),
	}
}

// buildUpdateView 生成前端展示用的更新信息，下载地址按当前平台选取。
func buildUpdateView(upd *clientUpdate) *updateView {
	if upd == nil {
		return nil
	}
	url, sha256, supported := platformUpdateSource(upd)
	return &updateView{
		Version:   upd.Version,
		URL:       url,
		SHA256:    sha256,
		Mandatory: upd.Mandatory,
		HasUpdate: supported && compareVersions(config.AppVersion, upd.Version) < 0,
		Current:   config.AppVersion,
		Note:      upd.Note,
	}
}

func (c *Controller) updateView(force bool) *updateView {
	return buildUpdateView(c.clientBundle(force).update)
}

func versionParts(v string) []int {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' })
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

// compareVersions 返回 -1/0/1，按数字段比较（1.0.1 > 1.0.0）。
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(pa) {
			av = pa[i]
		}
		if i < len(pb) {
			bv = pb[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

// apiApplyUpdate 下载更新包、启动替换脚本并退出当前进程（脚本完成覆盖并重启）。
func (c *Controller) apiApplyUpdate([]any) any {
	if !c.updateMu.TryLock() {
		return map[string]any{"ok": false, "message": "更新已在进行中"}
	}
	c.setUpdateProgress(updateProgressView{Active: true, Phase: "preparing", Message: "正在准备更新…"})
	fail := func(message string) any {
		c.setUpdateProgress(updateProgressView{Phase: "error", Message: message})
		c.updateMu.Unlock()
		return map[string]any{"ok": false, "message": message}
	}
	upd := c.updateView(true)
	if upd == nil || !upd.HasUpdate {
		return fail("当前已是最新版本")
	}
	if upd.URL == "" {
		return fail("后台未配置更新下载地址")
	}
	if !strings.HasPrefix(upd.URL, "https://") && !strings.HasPrefix(upd.URL, "http://") {
		return fail("更新下载地址仅支持 http/https")
	}

	exePath, err := os.Executable()
	if err != nil {
		return fail("无法获取程序路径：" + err.Error())
	}
	if abs, err := filepath.Abs(exePath); err == nil {
		exePath = abs
	}
	safeVersion := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, upd.Version)
	// 更新包形式与落盘路径按平台决定（Windows 单 exe；macOS 内含 .app 的 zip）。
	newPkg := updatePackagePath(safeVersion)
	var downloadedBytes, totalBytes int64
	if err := c.downloadUpdate(upd.URL, newPkg, upd.SHA256, func(downloaded, total int64) {
		downloadedBytes, totalBytes = downloaded, total
		percent := 0.0
		if total > 0 {
			percent = float64(downloaded) * 100 / float64(total)
			if percent >= 100 {
				percent = 99
			}
		}
		c.setUpdateProgress(updateProgressView{
			Active:     true,
			Phase:      "downloading",
			Downloaded: downloaded,
			Total:      total,
			Percent:    percent,
			Message:    "正在下载更新包…",
		})
	}); err != nil {
		return fail("更新包下载失败：" + err.Error())
	}
	c.setUpdateProgress(updateProgressView{Active: true, Phase: "installing", Downloaded: downloadedBytes, Total: totalBytes, Percent: 99, Message: "下载完成，正在准备安装…"})

	if err := installUpdate(newPkg, exePath, safeVersion); err != nil {
		return fail(err.Error())
	}
	c.setUpdateProgress(updateProgressView{Phase: "ready", Downloaded: downloadedBytes, Total: totalBytes, Percent: 100, Message: "更新包已下载，正在重启…"})
	c.log.Log(fmt.Sprintf("[更新] 已下载更新包 %s，准备退出并更新到 %s", newPkg, upd.Version))
	go func() {
		time.Sleep(700 * time.Millisecond)
		c.Shutdown()
		os.Exit(0)
	}()
	return map[string]any{"ok": true, "message": "更新包已下载，程序即将自动重启完成更新"}
}

func (c *Controller) downloadUpdate(url, dest, expectSHA256 string, report func(downloaded, total int64)) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha256.New()
	progress := &downloadProgressWriter{total: resp.ContentLength, report: report}
	_, copyErr := io.Copy(io.MultiWriter(f, h), io.TeeReader(resp.Body, progress))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if expectSHA256 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, expectSHA256) {
			_ = os.Remove(tmp)
			return fmt.Errorf("SHA256 校验失败（期望 %s，实际 %s）", expectSHA256, got)
		}
	}
	// Windows 下 os.Rename 不能覆盖已存在的目标，先删除旧的更新包。
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}

type downloadProgressWriter struct {
	downloaded int64
	total      int64
	report     func(downloaded, total int64)
}

func (w *downloadProgressWriter) Write(p []byte) (int, error) {
	w.downloaded += int64(len(p))
	if w.report != nil {
		w.report(w.downloaded, w.total)
	}
	return len(p), nil
}
