package proxysrv

import (
	"bytes"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bailan-relay/internal/applog"
	"bailan-relay/internal/bodydelta"
)

// fakeRelay 按中继协议还原请求体；legacy=true 模拟不认识增量头的旧版中继。
type fakeRelay struct {
	t        *testing.T
	legacy   bool
	cache    map[string][]byte
	received [][]byte
	wire     []int
	modes    []string
}

func (f *fakeRelay) RoundTrip(req *http.Request) (*http.Response, error) {
	raw, _ := io.ReadAll(req.Body)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))}
	mode := req.Header.Get(bodydelta.HeaderMode)
	f.modes = append(f.modes, mode)
	f.wire = append(f.wire, len(raw))
	if f.legacy {
		if mode != bodydelta.ModeRaw {
			resp.StatusCode = http.StatusBadRequest
			return resp, nil
		}
		f.received = append(f.received, raw)
		return resp, nil
	}
	resp.Header.Set(bodydelta.HeaderSupport, "1")
	payload := raw
	if req.Header.Get(bodydelta.HeaderEncoding) == "gzip" {
		payload, _ = bodydelta.Gunzip(raw, bodydelta.MaxSize)
	}
	body := payload
	if mode == bodydelta.ModeDelta {
		base := f.cache[req.Header.Get(bodydelta.HeaderBase)]
		if base == nil {
			resp.StatusCode = http.StatusConflict
			resp.Header.Set(bodydelta.HeaderSupport, "miss")
			return resp, nil
		}
		var err error
		if body, err = bodydelta.Apply(base, payload, bodydelta.MaxSize); err != nil {
			f.t.Fatal(err)
		}
	}
	sum := req.Header.Get(bodydelta.HeaderSum)
	if bodydelta.Sum(body) != sum {
		f.t.Fatal("还原后的请求体与原始请求不一致")
	}
	if len(body) >= bodydelta.MinSize {
		f.cache[sum] = body
		resp.Header.Set(bodydelta.HeaderStored, sum)
	}
	f.received = append(f.received, body)
	return resp, nil
}

func newUploadTestServer(t *testing.T, relay *fakeRelay) *Server {
	logRoot, err := os.MkdirTemp("", "bailan-upload-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", logRoot)
	return &Server{log: applog.New(), relayClient: &http.Client{Transport: relay}}
}

func sendChat(t *testing.T, s *Server, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://server.codeium.com/x/GetChatMessage", nil)
	s.forwardRelay(rec, req, "server.codeium.com", body, "http://relay.test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get(bodydelta.HeaderSupport) != "" || rec.Header().Get(bodydelta.HeaderStored) != "" {
		t.Fatal("内部协议头不应透传给 IDE")
	}
	return rec
}

func TestForwardRelayUploadsDeltaAndFallsBackToFull(t *testing.T) {
	relay := &fakeRelay{t: t, cache: map[string][]byte{}}
	s := newUploadTestServer(t, relay)
	rnd := rand.New(rand.NewSource(7))
	conv := make([]byte, 1<<20)
	rnd.Read(conv)

	sendChat(t, s, conv) // 第一次：还不知道中继是否支持 → 原样上传
	turn2 := append(append([]byte(nil), conv...), []byte("second turn")...)
	sendChat(t, s, turn2)
	if relay.modes[1] != bodydelta.ModeDelta || relay.wire[1] > 4<<10 {
		t.Fatalf("第二轮应增量上传: mode=%s wire=%d", relay.modes[1], relay.wire[1])
	}

	relay.cache = map[string][]byte{} // 中继重启，缓存丢失
	turn3 := append(append([]byte(nil), turn2...), []byte("third turn")...)
	sendChat(t, s, turn3)
	if got := relay.modes[2:]; len(got) != 2 || got[0] != bodydelta.ModeDelta || got[1] != bodydelta.ModeFull {
		t.Fatalf("基准缺失应整包重发: %v", got)
	}
	for i, want := range [][]byte{conv, turn2, turn3} {
		if !bytes.Equal(relay.received[i], want) {
			t.Fatalf("第%d次中继收到的内容与原始请求不一致", i+1)
		}
	}
}

func TestForwardRelayFallsBackToRawForLegacyRelay(t *testing.T) {
	relay := &fakeRelay{t: t, cache: map[string][]byte{}}
	s := newUploadTestServer(t, relay)
	body := bytes.Repeat([]byte("abc"), 100<<10)
	sendChat(t, s, body)
	sendChat(t, s, append(append([]byte(nil), body...), 'x'))

	relay.legacy = true // 中继被换回旧版本
	relay.modes = nil
	sendChat(t, s, append(append([]byte(nil), body...), 'y'))
	if len(relay.modes) != 2 || relay.modes[1] != bodydelta.ModeRaw {
		t.Fatalf("旧版中继应退回原始上传: %v", relay.modes)
	}
	sendChat(t, s, append(append([]byte(nil), body...), 'z'))
	if relay.modes[2] != bodydelta.ModeRaw {
		t.Fatal("确认不支持后应直接原始上传")
	}
}
