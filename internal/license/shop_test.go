package license

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const packTokenJSON = `{"ok":true,"token":"tok-2","token_expires_at":"2099-01-01T00:00:00Z","card_expires_at":"2099-02-01T00:00:00Z","duration_days":30,"message":"兑换成功：效益包 +30 天","card_key":"WALL-ET01","balance":12,"billing_enabled":true,"currency":"¥"}`

// 本机已有钱包时，卡密的天数并入钱包：之后带的卡密必须是钱包，不是用户输入的那张。
func TestActivateRedeemsKeyIntoWallet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/license/pack/redeem" || r.Header.Get("Authorization") != "Bearer user-1" {
			http.Error(w, `{"detail":"bad request"}`, http.StatusBadRequest)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["card_key"] != "NEWK-EY01" || body["device_id"] != "dev" {
			t.Errorf("请求体不对: %v", body)
		}
		_, _ = w.Write([]byte(packTokenJSON))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	m.state.CardKey = "WALL-ET01"
	ok, msg := m.Activate(" newk-ey01 ")
	if !ok || msg != "兑换成功：效益包 +30 天" {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
	token, cardKey, _ := m.HeaderValues()
	if cardKey != "WALL-ET01" || token != "tok-2" {
		t.Fatalf("应继续使用钱包: card=%q token=%q", cardKey, token)
	}
	if st := m.Status(); st["pack_active"] != true || st["balance"] != 12.0 {
		t.Fatalf("status=%v", st)
	}
}

// 后台还没升级（没有 /pack/redeem）时退回原来的激活接口，输入的卡密就是钱包。
func TestActivateFallsBackToLegacyEndpoint(t *testing.T) {
	var legacy bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/license/pack/redeem":
			http.Error(w, `{"detail":"Not Found"}`, http.StatusNotFound)
		case "/api/license/activate":
			legacy = true
			_, _ = w.Write([]byte(`{"ok":true,"token":"tok-9","token_expires_at":"2099-01-01T00:00:00Z","card_expires_at":"2099-02-01T00:00:00Z","duration_days":30,"message":"激活成功"}`))
		}
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	m.state = State{DeviceID: "dev"}
	if ok, msg := m.Activate("OLDK-EY01"); !ok || !legacy {
		t.Fatalf("ok=%v legacy=%v msg=%q", ok, legacy, msg)
	}
	if _, cardKey, _ := m.HeaderValues(); cardKey != "OLDK-EY01" {
		t.Fatalf("card=%q", cardKey)
	}
}

// 卡密被拒时不能动现有钱包。
func TestActivateRejectedKeepsWallet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"token":"","token_expires_at":"2020-01-01T00:00:00Z","card_expires_at":null,"duration_days":0,"message":"卡密不存在"}`))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	if ok, msg := m.Activate("NOPE-0000"); ok || msg != "卡密不存在" {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
	token, cardKey, _ := m.HeaderValues()
	if cardKey != "ABCD-EFGH" || token != "tok-1" || !m.Valid() {
		t.Fatalf("钱包被改动: card=%q token=%q valid=%v", cardKey, token, m.Valid())
	}
}

func TestShopReturnsStorefront(t *testing.T) {
	const payload = `{"ok":true,"currency":"¥","products":[{"id":3,"name":"月卡","days":30,"price":9.9,"description":""}],"wallet":{"has_wallet":true,"pack_active":false,"pack_expires_at":null,"balance":20}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/license/shop" || r.URL.Query().Get("device_id") != "dev" ||
			r.Header.Get("Authorization") != "Bearer user-1" {
			http.Error(w, `{"detail":"bad request"}`, http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	// 没有钱包的新用户也要能看商城：只认账号登录态。
	m.state = State{DeviceID: "dev"}
	raw, err := m.Shop()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Products []struct {
			ID    int     `json:"id"`
			Price float64 `json:"price"`
		} `json:"products"`
	}
	if json.Unmarshal(raw, &got) != nil || len(got.Products) != 1 || got.Products[0].ID != 3 || got.Products[0].Price != 9.9 {
		t.Fatalf("raw=%s", raw)
	}
}

func TestShopErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	if _, err := m.Shop(); !errors.Is(err, ErrShopUnsupported) {
		t.Fatalf("旧后台应报不支持: %v", err)
	}
	m.userToken = func() string { return "" }
	if _, err := m.Shop(); err == nil || !strings.Contains(err.Error(), "登录") {
		t.Fatalf("未登录应提示登录: %v", err)
	}
}

func TestBuyPackCommitsWallet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/license/shop/buy" || r.Header.Get("Authorization") != "Bearer user-1" {
			http.Error(w, `{"detail":"bad request"}`, http.StatusBadRequest)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["product_id"] != float64(3) || body["device_id"] != "dev" {
			t.Errorf("请求体不对: %v", body)
		}
		_, _ = w.Write([]byte(`{"ok":true,"token":"tok-3","token_expires_at":"2099-01-01T00:00:00Z","card_expires_at":"2099-03-01T00:00:00Z","duration_days":30,"message":"已开通「月卡」，效益包延长 30 天","card_key":"ABCD-EFGH","balance":10.1,"billing_enabled":true,"currency":"¥"}`))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	m.state.Balance = 20
	ok, msg := m.BuyPack(3)
	if !ok || !strings.Contains(msg, "月卡") {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
	st := m.Status()
	if st["pack_active"] != true || st["balance"] != 10.1 || st["card_exp"] == "" {
		t.Fatalf("status=%v", st)
	}
	if token, _, _ := m.HeaderValues(); token != "tok-3" {
		t.Fatalf("token=%q", token)
	}
}

func TestBuyPackRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"token":"","token_expires_at":"2020-01-01T00:00:00Z","card_expires_at":null,"duration_days":0,"message":"余额不足：「月卡」售价 ¥9.9，当前余额 ¥2，请先充值"}`))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	m.state.Balance = 2
	ok, msg := m.BuyPack(3)
	if ok || !strings.Contains(msg, "余额不足") {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
	if token, _, _ := m.HeaderValues(); token != "tok-1" || m.Status()["balance"] != 2.0 {
		t.Fatalf("失败不应改动钱包: %v", m.Status())
	}

	if ok, _ := m.BuyPack(0); ok {
		t.Fatal("商品编号无效应失败")
	}
	m.userToken = func() string { return "" }
	if ok, msg := m.BuyPack(3); ok || !strings.Contains(msg, "登录") {
		t.Fatalf("未登录应失败: %v %q", ok, msg)
	}
}

// 效益包和余额是钱包里互不影响的两样东西，界面要能分别显示。
func TestStatusReportsPackAndBalanceSeparately(t *testing.T) {
	future, past := time.Now().Add(72*time.Hour), time.Now().Add(-time.Hour)
	for _, c := range []struct {
		name                           string
		state                          State
		valid, packActive, packExpired bool
	}{
		{"效益包有效、没有余额", State{Activated: true, CardKey: "K", CardExp: &future}, true, true, false},
		{"效益包到期、还有余额", State{Activated: true, CardKey: "K", CardExp: &past, Balance: 5}, true, false, true},
		{"效益包到期、余额为 0", State{Activated: true, CardKey: "K", CardExp: &past}, false, false, true},
		{"只有余额、没开通过效益包", State{Activated: true, CardKey: "K", Balance: 5}, true, false, false},
		{"没有钱包", State{}, false, false, false},
	} {
		m := newTestManager(t, "http://127.0.0.1:1")
		m.state = c.state
		st := m.Status()
		if st["valid"] != c.valid || st["pack_active"] != c.packActive || st["pack_expired"] != c.packExpired {
			t.Errorf("%s: valid=%v pack_active=%v pack_expired=%v", c.name, st["valid"], st["pack_active"], st["pack_expired"])
		}
	}
}
