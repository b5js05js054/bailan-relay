package proxysrv

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatchBillingJSONClearsFreeQuota(t *testing.T) {
	in := []byte(`{"billing_error":"out_of_quota","plan_slug":"free","has_subscription_or_credits":false,"is_subscription_valid":false,"available_credits":0,"org":"keep"}`)
	out, report := patchBillingJSON(in)
	if !report.ok {
		t.Fatal("expected rewrite")
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["billing_error"] != nil {
		t.Fatalf("billing_error = %#v", obj["billing_error"])
	}
	if obj["plan_slug"] != "pro" || obj["has_subscription_or_credits"] != true || obj["is_subscription_valid"] != true {
		t.Fatalf("flags = %#v", obj)
	}
	if obj["available_credits"] != float64(999999) || obj["org"] != "keep" {
		t.Fatalf("credits/org = %#v %#v", obj["available_credits"], obj["org"])
	}
}

func TestPatchBillingJSONLeavesUnrelatedBody(t *testing.T) {
	in := []byte(`{"ok":true}`)
	out, report := patchBillingJSON(in)
	if report.ok || string(out) != string(in) {
		t.Fatalf("changed unrelated body: %s", out)
	}
}

func TestPatchBillingStatusRewritesGzip(t *testing.T) {
	raw := []byte(`{"billing_error":"out_of_quota","plan_slug":"trial","available_credits":-1}`)
	packed, err := gzipPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{originClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app.devin.ai" || !strings.Contains(req.URL.Path, "/billing/status") {
			t.Fatalf("upstream %s%s", req.URL.Host, req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}},
			Body:       io.NopCloser(strings.NewReader(string(packed))),
		}, nil
	})}}
	req := httptest.NewRequest(http.MethodGet, "https://app.devin.ai/api/org/billing/status", nil)
	w := httptest.NewRecorder()
	server.patchBillingStatus(w, req, "app.devin.ai", nil)
	res := w.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("encoding %q body %s", res.Header.Get("Content-Encoding"), body)
	}
	zr, err := gzip.NewReader(strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(plain, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["plan_slug"] != "pro" || obj["billing_error"] != nil || obj["available_credits"] != float64(999999) {
		t.Fatalf("rewritten = %#v", obj)
	}
}
