package proxysrv

import (
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// relayReconnectWindow 中继不可达（多为后台重启）时，单个对话请求最多等待重连的时长。
// 请求尚未被中继接收，重发是安全的；超过窗口才把错误交给 IDE。
var relayReconnectWindow = 60 * time.Second

// relayReconnectBackoff 第 n 次重连前的等待：0.5s 起逐次递增，封顶 3s。
func relayReconnectBackoff(attempt int) time.Duration {
	return min(time.Duration(attempt)*500*time.Millisecond, 3*time.Second)
}

// isRetryableRelayError 中继还没开始响应就失败：拨号失败（重启中端口无人监听），
// 或旧进程平滑退出时关掉的空闲连接（EOF / reset）。这些情况下中继都没有处理该请求。
func isRetryableRelayError(err error) bool {
	if err == nil {
		return false
	}
	if isDialError(err) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, m := range []string{"connection reset", "forcibly closed", "server closed idle connection", "connection refused", "broken pipe"} {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// reconnectState 正在等待中继恢复的对话数，供界面显示「服务器正在重启，正在重新连接」。
type reconnectState struct {
	mu          sync.Mutex
	waiting     int
	since       time.Time
	attempts    int
	recoveredAt time.Time
	failedAt    time.Time
}

func (r *reconnectState) begin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.waiting == 0 {
		r.since = time.Now()
		r.attempts = 0
	}
	r.waiting++
}

func (r *reconnectState) attempt() {
	r.mu.Lock()
	r.attempts++
	r.mu.Unlock()
}

func (r *reconnectState) end(ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.waiting = max(r.waiting-1, 0)
	if ok {
		r.recoveredAt = time.Now()
	} else {
		r.failedAt = time.Now()
	}
}

// ReconnectStatus 界面轮询用：waiting=正在重连；recovered=10 秒内刚恢复；failed=10 秒内重连超时。
func (s *Server) ReconnectStatus() map[string]any {
	r := &s.reconnect
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]any{"waiting": r.waiting > 0}
	if r.waiting > 0 {
		out["count"] = r.waiting
		out["seconds"] = int(time.Since(r.since).Seconds())
		out["attempts"] = r.attempts
		out["window"] = int(relayReconnectWindow.Seconds())
		return out
	}
	const recent = 10 * time.Second
	out["recovered"] = !r.recoveredAt.IsZero() && time.Since(r.recoveredAt) < recent && r.recoveredAt.After(r.failedAt)
	out["failed"] = !r.failedAt.IsZero() && time.Since(r.failedAt) < recent && r.failedAt.After(r.recoveredAt)
	return out
}
