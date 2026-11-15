package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"bailan-relay/internal/applog"
)

// 页面那头先断开造成的回包失败不算软件故障，不该记进日志；结果本身编不出来才是真问题。
func TestClientGone(t *testing.T) {
	live := httptest.NewRequest(http.MethodPost, "/api/get_status", nil)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	gone := live.WithContext(cancelled)
	aborted := &net.OpError{Op: "write", Net: "tcp", Err: errors.New("wsasend: An established connection was aborted by the software in your host machine.")}

	cases := []struct {
		name string
		req  *http.Request
		err  error
		want bool
	}{
		{"对方断开导致写失败", live, aborted, true},
		{"请求已被取消", gone, errors.New("short write"), true},
		{"结果本身编不出来", live, &json.UnsupportedTypeError{Type: reflect.TypeOf(make(chan int))}, false},
		{"读失败不算对方断开", live, &net.OpError{Op: "read", Net: "tcp", Err: errors.New("reset")}, false},
	}
	for _, tc := range cases {
		if got := clientGone(tc.req, tc.err); got != tc.want {
			t.Errorf("%s：clientGone = %v，期望 %v", tc.name, got, tc.want)
		}
	}
}

// writeSpy 记下回包时第一次写失败的错误，用来确认测试里真的出现了写失败。
type writeSpy struct {
	http.ResponseWriter
	err error
}

func (s *writeSpy) Write(p []byte) (int, error) {
	n, err := s.ResponseWriter.Write(p)
	if err != nil && s.err == nil {
		s.err = err
	}
	return n, err
}

func apiFailureLogged(c *Controller) bool {
	entries, _ := c.log.GetLogs(0, 1000)["entries"].([]applog.Record)
	for _, entry := range entries {
		if strings.Contains(entry.Text, "API 编码失败") {
			return true
		}
	}
	return false
}

// 走一遍真实的 HTTP 连接：页面在回包前断开，日志里不该多出「API 编码失败」；
// 结果本身编不出来时仍然要记，证明这条日志没有被整个关掉。
func TestServeAPISkipsLogWhenClientDisconnects(t *testing.T) {
	logRoot, err := os.MkdirTemp("", "bailan-controller-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", logRoot)
	c := &Controller{log: applog.New()}

	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	// 远大于响应缓冲，保证回包时数据真的写到连接上，而不是留在缓冲里。
	big := strings.Repeat("x", 4<<20)
	routes := map[string]func([]any) any{
		"slow": func([]any) any {
			close(entered)
			<-release
			return map[string]any{"data": big}
		},
		"bad": func([]any) any { return map[string]any{"ch": make(chan int)} },
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spy := &writeSpy{ResponseWriter: w}
		c.serveAPI(spy, r, routes)
		if strings.HasSuffix(r.URL.Path, "/slow") {
			finished <- spy.err
		}
	}))
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(conn, "POST /api/slow HTTP/1.1\r\nHost: test\r\nContent-Length: 2\r\n\r\n[]"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("请求没有进到处理函数")
	}
	// 像关掉窗口那样直接断开，不等回包。
	_ = conn.(*net.TCPConn).SetLinger(0)
	_ = conn.Close()
	time.Sleep(200 * time.Millisecond)
	close(release)

	select {
	case writeErr := <-finished:
		if writeErr == nil {
			t.Fatal("回包没有出现写失败，这个测试没有测到断开的情况")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("处理函数没有结束")
	}
	if apiFailureLogged(c) {
		t.Error("页面断开造成的回包失败被记进了日志")
	}

	resp, err := http.Post(srv.URL+"/api/bad", "application/json", strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !apiFailureLogged(c) {
		t.Error("结果编不出来时应该记日志")
	}
}
