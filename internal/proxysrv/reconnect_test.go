package proxysrv

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bailan-relay/internal/applog"
)

func TestForwardRelayWaitsForRestartingRelay(t *testing.T) {
	// 日志文件一直打开，Windows 下 t.TempDir 清理会失败，所以用不自动删除的临时目录。
	logRoot, err := os.MkdirTemp("", "bailan-reconnect-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", logRoot)
	var calls atomic.Int32
	s := &Server{log: applog.New()}
	s.relayClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch calls.Add(1) {
		case 1:
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		case 2:
			if !s.ReconnectStatus()["waiting"].(bool) {
				t.Error("重连期间界面应显示 waiting")
			}
			return nil, io.EOF
		}
		body, _ := io.ReadAll(req.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/connect+proto"}},
			Body:       io.NopCloser(strings.NewReader("ok:" + string(body))),
		}, nil
	})}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://server.codeium.com/x/GetChatMessage", nil)
	start := time.Now()
	s.forwardRelay(rec, req, "server.codeium.com", []byte("hello"), "http://relay.test", "")

	if rec.Code != http.StatusOK || rec.Body.String() != "ok:hello" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if calls.Load() != 3 {
		t.Fatalf("calls=%d", calls.Load())
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("重连等待过长: %v", time.Since(start))
	}
	st := s.ReconnectStatus()
	if st["waiting"].(bool) || !st["recovered"].(bool) {
		t.Fatalf("恢复后状态不对: %+v", st)
	}
}

func TestForwardRelayDoesNotRetryAfterRelayResponded(t *testing.T) {
	if isRetryableRelayError(errors.New("http2: response body closed")) {
		t.Fatal("中继已开始处理的错误不应重试")
	}
	if !isRetryableRelayError(errors.New("read tcp: wsarecv: An existing connection was forcibly closed by the remote host.")) {
		t.Fatal("Windows 下旧连接被关闭应可重试")
	}
}
