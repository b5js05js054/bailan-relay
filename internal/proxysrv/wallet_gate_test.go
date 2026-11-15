package proxysrv

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bailan-relay/internal/applog"
)

const chatURL = "https://server.codeium.com/exa.api_server_pb.ApiServerService/GetChatMessage"

// newGateTestServer 返回带假中继的拦截服务；forwarded 记录中继收到的每个请求带的授权令牌。
func newGateTestServer(t *testing.T, forwarded *[]string) *Server {
	t.Helper()
	// 日志文件一直打开，Windows 下 t.TempDir 清理会失败，所以用不自动删除的临时目录。
	logRoot, err := os.MkdirTemp("", "bailan-wallet-gate-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", logRoot)
	s := &Server{log: applog.New()}
	s.relayClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*forwarded = append(*forwarded, req.Header.Get("X-License-Token"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/connect+proto"}},
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})}
	return s
}

func relayOnce(s *Server, target, contentType string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.Header.Set("Content-Type", contentType)
	s.forwardRelay(rec, req, "server.codeium.com", []byte("hello"), "http://relay.test", "")
	return rec
}

// 不开通也能启动拦截，但没有钱包（没开通效益包、也没充值）时发不了消息：
// 对话请求在本地拦下，不交给中继，并用 IDE 能显示的格式告诉用户去商城。
func TestForwardRelayRejectsChatWithoutWallet(t *testing.T) {
	var forwarded []string
	s := newGateTestServer(t, &forwarded)
	s.SetTokenProvider(func() (string, string, string) { return "", "", "dev" })

	rec := relayOnce(s, chatURL, "application/connect+proto")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/connect+proto" {
		t.Fatalf("Connect 请求应回 200 + 错误帧: status=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	raw := rec.Body.Bytes()
	if len(raw) < 5 || raw[0] != 0x02 {
		t.Fatalf("应是 end-of-stream 错误帧: %q", raw)
	}
	var trailer struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw[5:], &trailer); err != nil {
		t.Fatal(err)
	}
	if trailer.Error.Code != "resource_exhausted" {
		t.Fatalf("code=%q", trailer.Error.Code)
	}
	for _, want := range []string{"商城", "效益包", "余额"} {
		if !strings.Contains(trailer.Error.Message, want) {
			t.Fatalf("提示里应提到「%s」: %q", want, trailer.Error.Message)
		}
	}

	rec = relayOnce(s, chatURL, "application/proto")
	if rec.Code != http.StatusPaymentRequired || !strings.Contains(rec.Body.String(), "商城") {
		t.Fatalf("非 Connect 请求应回 402 + 文本: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(forwarded) != 0 {
		t.Fatalf("没有钱包时不应转发给中继: %v", forwarded)
	}
}

func TestForwardRelayForwardsChatWithWallet(t *testing.T) {
	var forwarded []string
	s := newGateTestServer(t, &forwarded)
	s.SetTokenProvider(func() (string, string, string) { return "tok-1", "ABCD-EFGH", "dev" })

	rec := relayOnce(s, chatURL, "application/connect+proto")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if len(forwarded) != 1 || forwarded[0] != "tok-1" {
		t.Fatalf("应带着授权令牌转发: %v", forwarded)
	}
}

// 本地只拦对话；其它走中继的接口照常转发，放不放行由中继决定。
func TestForwardRelayWithoutWalletStillForwardsNonChat(t *testing.T) {
	var forwarded []string
	s := newGateTestServer(t, &forwarded)
	s.SetTokenProvider(func() (string, string, string) { return "", "", "dev" })

	rec := relayOnce(s, "https://server.codeium.com/exa.api_server_pb.ApiServerService/Other", "application/connect+proto")
	if rec.Code != http.StatusOK || len(forwarded) != 1 {
		t.Fatalf("status=%d forwarded=%v", rec.Code, forwarded)
	}
}
