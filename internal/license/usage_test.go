package license

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsageClampsArgsAndPassesThroughJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/license/usage" || r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, `{"detail":"bad request"}`, http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		if q.Get("days") != "90" || q.Get("page") != "1" || q.Get("page_size") != "20" {
			t.Errorf("参数应被规范化: %v", q)
		}
		_, _ = w.Write([]byte(`{"ok":true,"currency":"¥","summary":{"total_cost":1.5}}`))
	}))
	defer srv.Close()

	m := newTestManager(t, srv.URL)
	raw, err := m.Usage(365, 0, 0)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	var got struct {
		OK      bool `json:"ok"`
		Summary struct {
			TotalCost float64 `json:"total_cost"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || !got.OK || got.Summary.TotalCost != 1.5 {
		t.Fatalf("raw=%s err=%v", raw, err)
	}
}

func TestUsageOldBackendReturnsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	if _, err := m.Usage(30, 1, 20); !errors.Is(err, ErrUsageUnsupported) {
		t.Fatalf("err=%v", err)
	}
}

func TestUsageSurfacesBackendDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"卡密已失效"}`))
	}))
	defer srv.Close()
	m := newTestManager(t, srv.URL)
	if _, err := m.Usage(7, 2, 50); err == nil || err.Error() != "卡密已失效" {
		t.Fatalf("err=%v", err)
	}
}

func TestUsageRequiresActivation(t *testing.T) {
	m := newTestManager(t, "http://127.0.0.1:1")
	m.state.Activated = false
	if _, err := m.Usage(30, 1, 20); !errors.Is(err, ErrNotActivated) {
		t.Fatalf("err=%v", err)
	}
}
