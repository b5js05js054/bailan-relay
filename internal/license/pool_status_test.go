package license

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPoolStatusRequestsLicenseAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pool-status" || r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, `{"detail":"bad request"}`, http.StatusUnauthorized)
			return
		}
		if r.URL.RawQuery != "" {
			t.Errorf("号池请求不应带查询参数: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"ok":true,"stale":false,"total":1,"groups":[],"items":[{"id":"abc","name":"ma***72","status":"active","usage_windows":[]}]}`))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	raw, err := m.PoolStatus()
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	var got struct {
		OK    bool `json:"ok"`
		Total int  `json:"total"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || !got.OK || got.Total != 1 {
		t.Fatalf("raw=%s err=%v", raw, err)
	}
	if strings.Contains(string(raw), upstreamNeedle()) {
		t.Fatalf("响应里出现了中转站域名: %s", raw)
	}
}

func TestPoolStatusOldBackendReturnsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	if _, err := m.PoolStatus(); !errors.Is(err, ErrPoolUnsupported) {
		t.Fatalf("err=%v", err)
	}
}

func TestPoolStatusRequiresActivation(t *testing.T) {
	m := newTestManager(t, "http://127.0.0.1:1")
	m.state.Activated = false
	if _, err := m.PoolStatus(); !errors.Is(err, ErrNotActivated) {
		t.Fatalf("err=%v", err)
	}
}

func TestClientSourcesDoNotNameUpstream(t *testing.T) {
	root := filepath.Join("..", "..")
	needle := upstreamNeedle()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".go", ".js", ".html", ".css", ".json":
		default:
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), needle) {
			t.Errorf("%s 包含中转站域名", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func upstreamNeedle() string {
	return "api" + "proxy"
}
