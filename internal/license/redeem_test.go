package license

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T, apiURL string) *Manager {
	t.Helper()
	return &Manager{
		apiURL:    apiURL,
		statePath: filepath.Join(t.TempDir(), "license.json"),
		client:    &http.Client{Timeout: 3 * time.Second},
		stopCh:    make(chan struct{}),
		state: State{
			Activated: true, CardKey: "ABCD-EFGH", DeviceID: "dev", Token: "tok-1",
			TokenExp: time.Now().Add(time.Hour),
		},
	}
}

func TestRedeemUpdatesBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/license/redeem" || r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, `{"detail":"bad request"}`, http.StatusUnauthorized)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["topup_code"] != "TP-XYZ" {
			t.Errorf("充值码应被规范为大写: %v", body)
		}
		_, _ = w.Write([]byte(`{"ok":true,"message":"充值成功 +10","balance":12.5,"billing_enabled":true,"currency":"¥"}`))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	ok, msg := m.Redeem("  tp-xyz ")
	if !ok || msg != "充值成功 +10" {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
	st := m.Status()
	if st["balance"] != 12.5 || st["billing_enabled"] != true || st["balance_text"] != "¥12.5000" {
		t.Fatalf("status=%v", st)
	}
}

func TestRedeemSurfacesBackendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"授权已失效"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	ok, msg := m.Redeem("TP-1")
	if ok || msg == "" {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
}

func TestRedeemRejectsWhenNotActivatedOrEmpty(t *testing.T) {
	m := newTestManager(t, "http://127.0.0.1:1")
	if ok, _ := m.Redeem(""); ok {
		t.Fatal("空充值码应失败")
	}
	m.state.Activated = false
	if ok, msg := m.Redeem("TP-1"); ok || msg == "" {
		t.Fatalf("未激活应失败: %v %q", ok, msg)
	}
}

func TestRefreshBalanceUpdatesState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"balance":3.25,"billing_enabled":true,"currency":"$"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	m.RefreshBalance()
	if got := m.Status()["balance_text"]; got != "$3.2500" {
		t.Fatalf("balance_text=%v", got)
	}
}
