package proxysrv

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bailan-relay/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type recordingWriter struct {
	header  http.Header
	status  int
	body    strings.Builder
	flushed bool
}

func (w *recordingWriter) Header() http.Header { return w.header }
func (w *recordingWriter) WriteHeader(status int) {
	w.status = status
}
func (w *recordingWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *recordingWriter) Flush()                      { w.flushed = true }

func TestStreamResponseFlushesAndReportsStats(t *testing.T) {
	w := &recordingWriter{header: make(http.Header)}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/connect+proto"}},
		Body:       io.NopCloser(strings.NewReader("stream-body")),
		Trailer:    make(http.Header),
	}
	stats := streamResponse(w, resp)
	if w.status != http.StatusOK || !w.flushed {
		t.Fatalf("status=%d flushed=%v", w.status, w.flushed)
	}
	if got := w.body.String(); got != "stream-body" {
		t.Fatalf("body=%q", got)
	}
	if stats.Bytes != len("stream-body") || stats.Chunks < 1 || stats.FirstByteAt.IsZero() || stats.EndAt.IsZero() || stats.Err != nil {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

type errAfterReader struct {
	data []byte
	err  error
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func connectEnvelope(flags byte, payload string) []byte {
	return append([]byte{flags, 0, 0, 0, byte(len(payload))}, payload...)
}

// IDE 收到 end-of-stream 帧后立即断开，读响应体报 context canceled：数据已完整，不应记为中断；
// 尚未收到 end-of-stream 帧时断开则仍是中断。
func TestStreamResponseCancelAfterEndOfStreamIsComplete(t *testing.T) {
	full := append(connectEnvelope(0, "hello"), connectEnvelope(0x02, "{}")...)
	for _, c := range []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{"完整", full, false},
		{"缺 end-of-stream", connectEnvelope(0, "hello"), true},
		{"end-of-stream 不完整", full[:len(full)-1], true},
	} {
		w := &recordingWriter{header: make(http.Header)}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/connect+proto"}},
			Body:       io.NopCloser(&errAfterReader{data: c.body, err: context.Canceled}),
		}
		if stats := streamResponse(w, resp); (stats.Err != nil) != c.wantErr {
			t.Fatalf("%s: err=%v wantErr=%v", c.name, stats.Err, c.wantErr)
		}
	}
}

func TestConnectEOSByteByByte(t *testing.T) {
	data := append(append(connectEnvelope(0, ""), connectEnvelope(0, "abc")...), connectEnvelope(0x02, "")...)
	c := &connectEOS{}
	for i, b := range data {
		if c.done {
			t.Fatalf("提前判定结束于第 %d 字节", i)
		}
		c.feed([]byte{b})
	}
	if !c.done {
		t.Fatal("应识别出 end-of-stream 帧")
	}
}

func TestAuthAndTeamSettingsUsePolicyPassthrough(t *testing.T) {
	for _, path := range []string{
		"server.codeium.com/exa.auth_pb.AuthService/GetUserJwt",
		"server.codeium.com/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings",
	} {
		if !isPolicyPassthrough(path) {
			t.Fatalf("%s must bypass local empty responses", path)
		}
		if containsAny(path, blockKeywords) {
			t.Fatalf("%s must not be blocked", path)
		}
	}
	if isPolicyPassthrough("server.codeium.com/exa.api_server_pb.ApiServerService/GetChatMessage") {
		t.Fatal("GetChatMessage must not match policy passthrough")
	}
}

func TestCustomerTeamSettingsReturnsCompleteOfficialResponse(t *testing.T) {
	server := &Server{originClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Host != "server.codeium.com" {
			t.Fatalf("request host = %q", req.Host)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/proto"}, "X-Test": {"official"}},
			Body:       io.NopCloser(strings.NewReader("official-team-settings")),
		}, nil
	})}}
	req := httptest.NewRequest(http.MethodPost, "https://server.codeium.com/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings", strings.NewReader("request"))
	w := httptest.NewRecorder()

	server.customerTeamSettings(w, req, "server.codeium.com", []byte("request"), time.Second)

	resp := w.Result()
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(payload) != "official-team-settings" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, payload)
	}
	if got := resp.Header.Get("X-Test"); got != "official" {
		t.Fatalf("official response header lost: %q", got)
	}
}

func TestCustomerTeamSettingsFallsBackBeforeDevinDeadline(t *testing.T) {
	server := &Server{originClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}}
	req := httptest.NewRequest(http.MethodPost, "https://server.codeium.com/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings", nil)
	w := httptest.NewRecorder()

	server.customerTeamSettings(w, req, "server.codeium.com", nil, 10*time.Millisecond)

	resp := w.Result()
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(payload) != 0 {
		t.Fatalf("status=%d body=%q", resp.StatusCode, payload)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/proto" {
		t.Fatalf("content-type=%q", got)
	}
}

func TestCustomerTeamSettingsFastPathIsEditionScoped(t *testing.T) {
	original := config.Edition
	defer func() { config.Edition = original }()

	path := "server.codeium.com/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings"
	config.Edition = "full"
	if isCustomerTeamSettings(path) {
		t.Fatal("full edition must keep the original passthrough behavior")
	}
	config.Edition = "customer"
	if !isCustomerTeamSettings(path) {
		t.Fatal("customer edition must use the bounded team-settings path")
	}
}

// 策略/认证请求与其它透传共用 originClient；该 transport 必须保留 h2 协商能力，
// 否则源站回的 HTTP/2 帧会被 h1-only 栈当成 malformed response。
func TestOriginClientSupportsHTTP2(t *testing.T) {
	s := New(nil, nil, nil)
	transport, ok := s.originClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("originClient transport is not *http.Transport")
	}
	if transport.Protocols != nil && !transport.Protocols.HTTP2() {
		t.Fatalf("originClient must allow HTTP/2, protocols=%#v", transport.Protocols)
	}
	if !transport.ForceAttemptHTTP2 && transport.Protocols == nil {
		t.Fatal("originClient must negotiate HTTP/2 (ForceAttemptHTTP2 or Protocols)")
	}
}

func TestNormalizeStubPathsRemovesAuthAndTeamSettings(t *testing.T) {
	got := config.NormalizeStubPaths([]string{" GetUserJwt ", " GetCliTeamSettings ", "SafeStub", "safestub"})
	if len(got) != 1 || got[0] != "SafeStub" {
		t.Fatalf("normalized stub paths = %#v, want [SafeStub]", got)
	}
}
