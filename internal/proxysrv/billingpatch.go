package proxysrv

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxBillingBytes = 4 << 20

type billingRewriteReport struct {
	ok            bool
	errorBefore   string
	planBefore    string
	planAfter     string
	creditsBefore any
}

// patchBillingStatus 把账单状态转回官方，再改掉免费额度用尽的字段。
// 输入框横幅看的是这份 JSON，不是 GetUserStatus。
func (s *Server) patchBillingStatus(w http.ResponseWriter, r *http.Request, host string, body []byte) {
	s.logLine("[账单改写] 命中 %s %s", r.Method, r.URL.RequestURI())
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 20*time.Second)
	defer cancel()
	target := "https://" + host + r.URL.RequestURI()
	req, err := http.NewRequestWithContext(ctx, r.Method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "BaiLan billing error", http.StatusBadGateway)
		return
	}
	copyRequestHeaders(req.Header, r.Header, nil)
	req.Host = host
	req.ContentLength = int64(len(body))
	resp, err := s.originClient.Do(req)
	if err != nil {
		s.logLine("[账单改写] 回源失败 %s：%v", r.URL.RequestURI(), err)
		http.Error(w, "BaiLan billing error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBillingBytes+1))
	if err != nil || len(payload) > maxBillingBytes {
		copyResponseHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(payload)
		s.logLine("[账单改写] 响应过大或读失败，已原样返回 %s", r.URL.RequestURI())
		return
	}
	plain := payload
	gzipped := strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip")
	if gzipped {
		if decoded, derr := gunzipPayload(payload); derr == nil {
			plain = decoded
		}
	}
	out, report := patchBillingJSON(plain)
	written := out
	if gzipped && report.ok {
		if packed, perr := gzipPayload(out); perr == nil {
			written = packed
		} else {
			gzipped = false
			written = out
		}
	}
	copyResponseHeaders(w.Header(), resp.Header)
	if report.ok && !gzipped {
		w.Header().Del("Content-Encoding")
	}
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Length", strconv.Itoa(len(written)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(written)
	if report.ok {
		credits := fmt.Sprint(report.creditsBefore)
		if credits == "" || credits == "<nil>" {
			credits = "空"
		}
		s.logLine("[账单改写] 成功 billing_error %s→空，plan %s→%s，has_credits %s→true",
			emptyAs(report.errorBefore, "空"), emptyAs(report.planBefore, "空"), emptyAs(report.planAfter, "空"), credits)
	}
}

func patchBillingJSON(body []byte) ([]byte, billingRewriteReport) {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, billingRewriteReport{}
	}
	_, hasError := obj["billing_error"]
	_, hasPlan := obj["plan_slug"]
	if !hasError && !hasPlan {
		return body, billingRewriteReport{}
	}
	report := billingRewriteReport{
		ok:            true,
		errorBefore:   jsonString(obj["billing_error"]),
		planBefore:    jsonString(obj["plan_slug"]),
		creditsBefore: obj["has_subscription_or_credits"],
	}
	obj["billing_error"] = nil
	obj["has_subscription_or_credits"] = true
	obj["is_subscription_valid"] = true
	plan := strings.ToLower(report.planBefore)
	if plan == "" || plan == "free" || plan == "trial" {
		obj["plan_slug"] = "pro"
	}
	if credits, ok := jsonNumber(obj["available_credits"]); !ok || credits <= 0 {
		obj["available_credits"] = 999999
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body, billingRewriteReport{}
	}
	report.planAfter = jsonString(obj["plan_slug"])
	return out, report
}

func jsonString(v any) string {
	s, _ := v.(string)
	return s
}

func jsonNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func emptyAs(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func gzipPayload(plain []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(plain); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
