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
		userToken: func() string { return "user-1" },
		state: State{
			Activated: true, CardKey: "ABCD-EFGH", DeviceID: "dev", Token: "tok-1",
			TokenExp: time.Now().Add(time.Hour),
		},
	}
}

func TestRedeemUpdatesBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/license/user-redeem" || r.Header.Get("Authorization") != "Bearer user-1" {
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

func TestRedeemRejectsWhenNotLoggedInOrEmpty(t *testing.T) {
	m := newTestManager(t, "http://127.0.0.1:1")
	if ok, _ := m.Redeem(""); ok {
		t.Fatal("空充值码应失败")
	}
	m.userToken = func() string { return "" }
	if ok, msg := m.Redeem("TP-1"); ok || msg == "" {
		t.Fatalf("未登录应失败: %v %q", ok, msg)
	}
}

func TestRedeemAutoActivatesWhenNotActivated(t *testing.T) {
	var activated bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/license/user-redeem":
			_, _ = w.Write([]byte(`{"ok":true,"message":"充值成功","card_key":"AAAA-BBBB","balance":10,"billing_enabled":true,"currency":"¥"}`))
		case "/api/license/activate":
			activated = true
			_, _ = w.Write([]byte(`{"ok":true,"token":"t","token_expires_at":"2099-01-01T00:00:00Z","card_expires_at":null,"duration_days":0,"message":"激活成功","balance":10,"billing_enabled":true,"currency":"¥"}`))
		}
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	m.state = State{DeviceID: "dev", CardKey: "STALE-CODE"}
	if ok, msg := m.Redeem("TP-1"); !ok || !activated {
		t.Fatalf("充值后应自动激活: ok=%v activated=%v msg=%q", ok, activated, msg)
	}
	if !m.Valid() {
		t.Fatal("自动激活后授权应有效")
	}
}

func TestRedeemWithoutCardSendsNoCardKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, has := body["card_key"]; has {
			t.Errorf("未激活时不应携带 card_key: %v", body)
		}
		if body["device_id"] != "dev" {
			t.Errorf("应带上本机 device_id: %v", body)
		}
		_, _ = w.Write([]byte(`{"ok":true,"message":"充值成功","balance":10,"billing_enabled":true,"currency":"¥"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	m.state = State{DeviceID: "dev"}
	if ok, msg := m.Redeem("TP-1"); !ok {
		t.Fatalf("未激活也应能充值: %q", msg)
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
