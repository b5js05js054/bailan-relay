package account

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const avatarName = "0123456789abcdef0123456789abcdef.png"

func sessionJSON() string {
	exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	return `{"ok":true,"token":"utok","expires_at":"` + exp + `","user":{"id":7,"email":"12345@qq.com","avatar_url":"/api/user/files/` + avatarName + `","background_url":null}}`
}

func newTestManager(t *testing.T, url string) *Manager {
	t.Helper()
	return New(func() string { return url }, filepath.Join(t.TempDir(), "user.json"), nil)
}

func TestRegisterUploadsWithProgressAndPersists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/register" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		f, _, err := r.FormFile("avatar")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(f)
		if r.FormValue("email") != "12345@qq.com" || len(data) != 4096 {
			t.Errorf("email=%q len=%d", r.FormValue("email"), len(data))
		}
		_, _ = w.Write([]byte(sessionJSON()))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	p, err := m.Register(" 12345@qq.com ", "secret1", make([]byte, 4096), "a.png")
	if err != nil {
		t.Fatal(err)
	}
	if p.AvatarURL != "/userfile/"+avatarName || p.BackgroundURL != "" {
		t.Fatalf("profile=%+v", p)
	}
	if prog := m.Progress(); prog.Phase != "done" || prog.Percent != 100 || prog.Total <= 4096 {
		t.Fatalf("progress=%+v", prog)
	}
	reloaded := New(m.apiURL, m.path, nil)
	if reloaded.Token() != "utok" {
		t.Fatal("会话应持久化到 user.json")
	}
}

func TestLoginErrorUsesDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"邮箱或密码错误"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	if _, err := m.Login("12345@qq.com", "bad"); err == nil || err.Error() != "邮箱或密码错误" {
		t.Fatalf("err=%v", err)
	}
	if m.Token() != "" {
		t.Fatal("登录失败不应保存会话")
	}
}

func TestValidateLogsOutOnUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			_, _ = w.Write([]byte(sessionJSON()))
		default:
			if r.Header.Get("Authorization") != "Bearer utok" {
				t.Errorf("auth=%q", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail":"登录已失效，请重新登录"}`))
		}
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	if _, err := m.Login("12345@qq.com", "secret1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Validate(); ok || m.Token() != "" {
		t.Fatal("401 后应清空会话")
	}
}

func TestBackgroundRequiresLoginAndFetchFileRejectsTraversal(t *testing.T) {
	m := newTestManager(t, "http://127.0.0.1:1")
	if _, err := m.SetBackground(pngBytes); err != ErrUnauthorized {
		t.Fatalf("err=%v", err)
	}
	if _, err := m.FetchFile(t.Context(), "../cards.db"); err == nil || !strings.Contains(err.Error(), "不合法") {
		t.Fatalf("err=%v", err)
	}
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)

func TestBackgroundStoredLocallyPerUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/login" {
			t.Errorf("背景不应请求服务器：%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(sessionJSON()))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	if _, err := m.Login("12345@qq.com", "secret1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetBackground([]byte("not an image")); err == nil {
		t.Fatal("非图片应被拒")
	}
	if _, err := m.SetBackground(make([]byte, BackgroundMaxBytes+1)); err == nil {
		t.Fatal("超过 5MB 应被拒")
	}
	p, err := m.SetBackground(pngBytes)
	if err != nil || !strings.HasPrefix(p.BackgroundURL, "/userbg/?v=") {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	path, ok := m.BackgroundFile()
	if !ok || filepath.Base(path) != "user-7.png" {
		t.Fatalf("path=%s", path)
	}

	m.Logout()
	if _, ok := m.BackgroundFile(); ok {
		t.Fatal("退出后不应再输出背景")
	}
	p, _ = m.Login("12345@qq.com", "secret1")
	if p.BackgroundURL == "" {
		t.Fatal("同一账号重新登录应恢复本地背景")
	}
	if p, err = m.ClearBackground(); err != nil || p.BackgroundURL != "" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("清除后文件应删除")
	}
}
