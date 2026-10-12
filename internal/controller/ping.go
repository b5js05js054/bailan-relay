package controller

import (
	"io"
	"net/http"
	"strings"
	"time"

	"bailan-relay/internal/config"
)

// apiPingRelay 是「心跳监控」使用的轻量探测：只对中继 /health 发一次 GET，
// 返回往返延迟（毫秒）。它不修改状态快照、不写日志，前端每几秒调用一次。
//
// 返回字段：
//
//	ok      是否得到 HTTP 200
//	ms      往返延迟（毫秒）；探测失败时为 0
//	status  HTTP 状态码；连接失败时为 0
//	error   失败原因（客户版脱敏为固定文案）
//	at      本次探测完成的本地时间 HH:mm:ss
func (c *Controller) apiPingRelay([]any) any {
	out := map[string]any{"ok": false, "ms": 0, "status": 0, "error": "", "at": time.Now().Format("15:04:05")}
	customer := config.IsCustomer()

	relayURL := strings.TrimSpace(config.Load().RelayServerURL)
	if relayURL == "" {
		out["error"] = "未配置中继地址"
		if customer {
			out["error"] = "未连接"
		}
		return out
	}

	start := time.Now()
	resp, err := c.relayClient.Get(strings.TrimRight(relayURL, "/") + "/health")
	if err != nil {
		out["error"] = err.Error()
		if customer {
			out["error"] = "未连接"
		}
		return out
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	ms := time.Since(start).Milliseconds()
	if ms < 1 {
		ms = 1
	}
	out["ms"] = ms
	out["status"] = resp.StatusCode
	out["ok"] = resp.StatusCode == http.StatusOK
	if resp.StatusCode != http.StatusOK {
		out["error"] = "HTTP " + http.StatusText(resp.StatusCode)
		if customer {
			out["error"] = "服务异常"
		}
	}
	return out
}
