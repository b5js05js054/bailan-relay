package proxysrv

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"bailan-relay/internal/config"
	"bailan-relay/internal/connectframe"
	"bailan-relay/internal/tunbypass"
)

// Start 在 127.0.0.1:443 与 [::1]:443 启动 HTTPS 拦截服务。
func (s *Server) Start() error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.running {
		return nil
	}
	tlsCfg := &tls.Config{
		GetCertificate: s.ca.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
		MinVersion:     tls.VersionTLS12,
	}
	addrs := []string{
		net.JoinHostPort("127.0.0.1", fmt.Sprint(config.ProxyListenPort)),
		net.JoinHostPort("::1", fmt.Sprint(config.ProxyListenPort)),
	}
	var servers []*http.Server
	var primaryErr error
	for i, addr := range addrs {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			if i == 0 {
				primaryErr = fmt.Errorf("监听 %s 失败：%w", addr, err)
			} else {
				s.log.Log(fmt.Sprintf("[Manager] 监听 %s 失败（忽略）：%v", addr, err))
			}
			continue
		}
		srv := &http.Server{
			Handler:           s,
			TLSConfig:         tlsCfg.Clone(),
			ReadHeaderTimeout: 30 * time.Second,
		}
		servers = append(servers, srv)
		go func(srv *http.Server, ln net.Listener) {
			_ = srv.ServeTLS(ln, "", "")
		}(srv, ln)
	}
	if primaryErr != nil {
		for _, srv := range servers {
			_ = srv.Close()
		}
		return primaryErr
	}
	if len(servers) == 0 {
		return fmt.Errorf("无法监听本机 %d 端口", config.ProxyListenPort)
	}
	s.servers = servers
	s.running = true
	return nil
}

// Stop 关闭监听并断开所有连接。
func (s *Server) Stop() {
	s.runMu.Lock()
	servers := s.servers
	s.servers = nil
	s.running = false
	s.runMu.Unlock()
	for _, srv := range servers {
		_ = srv.Close()
	}
}

// Running 返回服务是否在运行。
func (s *Server) Running() bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.running
}

// dialOrigin 拨官方源站：解析出真实 IP 后用系统路由直连（与 Devin 自身直连源站同路径）。
// 不默认绑物理网卡——绑卡是给「中继出站绕 TUN」用的，套在源站透传上可能引入 PMTU
// 黑洞/丢包（表现为响应头能到、body 卡住超时）；系统路由失败时才退回绑卡兜底。
func (s *Server) dialOrigin(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
		port = "443"
	}
	plain := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if ip := s.resolver.ResolveRealIP(host); config.IsPublicIPv4(ip) {
		if conn, derr := plain.DialContext(ctx, network, net.JoinHostPort(ip, port)); derr == nil {
			return conn, nil
		}
	}
	// 系统路由失败或缓存 IP 失效：刷新解析后绑物理网卡兜底（系统路由被隧道截获时仍可出网）。
	if fresh := s.resolver.Refresh(host); config.IsPublicIPv4(fresh) {
		return tunbypass.NewDialer(10*time.Second).DialContext(ctx, network, net.JoinHostPort(fresh, port))
	}
	return nil, fmt.Errorf("无法连接 %s 的真实 IP", host)
}

// dialRelay 拨中继服务器：复用 netdns 硬化拨号链（真实 IP+绑卡→系统路由→原始地址兜底）。
func (s *Server) dialRelay(ctx context.Context, network, addr string) (net.Conn, error) {
	return s.resolver.DialContext(10*time.Second)(ctx, network, addr)
}

// ServeHTTP 是拦截入口。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := s.requestHost(r)
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	domains, paths, stub, relayBase, relayToken := s.snapshot()
	isTarget := domains[host]
	pathTail := lastSegment(r.URL.Path)
	matchText := host + r.URL.RequestURI()

	if isTarget {
		// 客户版的 Devin ACP 认证只有 10 秒总预算。GetCliTeamSettings 在冷启动时
		// 偶尔能收到响应头，却无法在 10 秒内读完整个响应体，最终导致整个 ACP
		// authenticate 失败。这里先限时读取官方结果；官方链路足够快时原样返回，
		// 否则在 Devin 截止前返回合法的空 protobuf，让客户端采用默认团队设置。
		if isCustomerTeamSettings(matchText) {
			s.customerTeamSettings(w, r, host, body, customerTeamSettingsBudget)
			return
		}
		if isPolicyPassthrough(matchText) {
			s.passthrough(w, r, host, body)
			return
		}
		if containsAny(matchText, blockKeywords) {
			writeEmpty200(w)
			if !containsAny(pathTail, silentBlockKeywords) {
				s.log.Log(fmt.Sprintf("[BLOCKED] %s (%dB)", pathTail, len(body)))
			}
			return
		}
		if containsAny(matchText, stub) {
			writeEmpty200(w)
			s.log.Log(fmt.Sprintf("[放行] %s → 本地空应答 (%dB)", pathTail, len(body)))
			return
		}
	}

	if !s.isRelayTarget(host, r.URL.Path, isTarget, paths) {
		s.passthrough(w, r, host, body)
		return
	}

	ct := r.Header.Get("content-type")
	if !strings.Contains(strings.ToLower(ct), "proto") {
		s.passthrough(w, r, host, body)
		return
	}

	s.forwardRelay(w, r, host, body, relayBase, relayToken)
}

const (
	// Devin ACP authenticate 对团队设置请求使用 10 秒超时；保留 2 秒用于本地回包。
	customerTeamSettingsBudget = 8 * time.Second
	// 团队设置是小型 protobuf。限制缓冲大小，避免异常源站响应占用过多内存。
	maxCustomerTeamSettingsBytes = 1 << 20
)

// customerTeamSettings 为客户版提供团队设置快速路径。
// 输入：Devin 的原始请求、目标主机与请求体；输出：完整官方响应，或在预算耗尽时返回空 protobuf 200。
// 正常版不会进入该函数，仍沿用 passthrough 的完整流式透传行为。
func (s *Server) customerTeamSettings(w http.ResponseWriter, r *http.Request, host string, body []byte, budget time.Duration) {
	target := "https://" + host + r.URL.RequestURI()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), budget)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.Method, target, bytes.NewReader(body))
	if err == nil {
		copyRequestHeaders(req.Header, r.Header, nil)
		req.Host = host
		req.ContentLength = int64(len(body))

		start := time.Now()
		resp, requestErr := s.originClient.Do(req)
		if requestErr == nil {
			defer resp.Body.Close()
			payload, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCustomerTeamSettingsBytes+1))
			if readErr == nil && len(payload) <= maxCustomerTeamSettingsBytes {
				copyResponseHeaders(w.Header(), resp.Header)
				w.WriteHeader(resp.StatusCode)
				_, _ = w.Write(payload)
				if s.log != nil {
					s.log.Log(fmt.Sprintf("[认证加速] 团队配置读取完成 · %.2fs", time.Since(start).Seconds()))
				}
				return
			}
		}
	}

	writeEmpty200(w)
	if s.log != nil {
		s.log.Log(fmt.Sprintf("[认证兜底] 团队配置未在 %.0fs 内完整返回，已使用本地默认设置", budget.Seconds()))
	}
}

func (s *Server) isRelayTarget(host, path string, isTarget bool, paths map[string]bool) bool {
	if !isTarget {
		return false
	}
	if len(paths) == 0 {
		return true
	}
	return paths[path]
}

// passthrough 把请求原样转回官方源站，解析不到真实 IP 时回 502 阻止回环。
// 认证请求 GetUserJwt 与正常版 GetCliTeamSettings 也走这里：originClient 经
// ALPN 协商 h2 与源站协议一致；不要为其强制 HTTP/1.1，否则源站回的 h2 帧
// 会被当成 malformed HTTP response。
func (s *Server) passthrough(w http.ResponseWriter, r *http.Request, host string, body []byte) {
	ip := s.resolver.ResolveRealIP(host)
	if !config.IsPublicIPv4(ip) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(fmt.Sprintf("BaiLan: 无法解析 %s 的真实 IP，已阻止回环", host)))
		s.log.Log(fmt.Sprintf("[DNS] 无法解析 %s 真实 IP，已拦截回环（检查 DNS 是否可用）", host))
		return
	}
	target := "https://" + host + r.URL.RequestURI()
	// 出站 ctx 与入站 r.Context() 解耦：IDE 侧超时取消不应连带杀掉正在拨号的源站
	// 连接——否则连接永远进不了池、每请求都重新慢拨，形成"慢→取消→重连→更慢"的
	// 恶性循环。解耦后连接能归还连接池复用，后续请求多路复用同一热连接。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, target, bytes.NewReader(body))
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	copyRequestHeaders(req.Header, r.Header, nil)
	req.Host = host
	req.ContentLength = int64(len(body))
	s.log.Log(fmt.Sprintf("[PASSTHROUGH] %s%s", host, r.URL.RequestURI()))
	start := time.Now()
	resp, err := s.originClient.Do(req)
	headersAt := time.Now()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("BaiLan passthrough error: " + err.Error()))
		// 记录真实拨号/传输错误：区分是我方失败还是源站返回错误码。
		s.log.Log(fmt.Sprintf("[PASSTHROUGH-ERR] %s%s · 头:%.1fs · %v", host, r.URL.RequestURI(), headersAt.Sub(start).Seconds(), err))
		return
	}
	defer resp.Body.Close()
	stats := streamResponse(w, resp)
	ct := resp.Header.Get("Content-Type")
	// 2xx 成功只留上面的请求行（GetUserStatus 高频，避免刷屏）；
	// 非 2xx 或流错误打结果 + content-type/声明长度/实收字节，区分"长连接挂起"
	// 还是"带宽慢"以及慢在拨号(头)还是 body 流。
	if stats.Err != nil {
		firstByte := "-"
		if !stats.FirstByteAt.IsZero() {
			firstByte = fmt.Sprintf("%.1fs", stats.FirstByteAt.Sub(start).Seconds())
		}
		s.log.Log(fmt.Sprintf("[PASSTHROUGH-ERR] %s%s · 状态码:%d · ct:%s cl:%d · 头:%.1fs 首包:%s 总:%.1fs %dB · %v",
			host, r.URL.RequestURI(), resp.StatusCode, ct, resp.ContentLength, headersAt.Sub(start).Seconds(), firstByte, stats.EndAt.Sub(start).Seconds(), stats.Bytes, stats.Err))
	} else if resp.StatusCode >= 300 {
		s.log.Log(fmt.Sprintf("[PASSTHROUGH-%d] %s%s · ct:%s · 头:%.1fs 总:%.2fs · %dB", resp.StatusCode, host, r.URL.RequestURI(), ct, headersAt.Sub(start).Seconds(), stats.EndAt.Sub(start).Seconds(), stats.Bytes))
	}
}

// ModelGoneMessage 生成「所选模型已不存在」的用户提示。
func ModelGoneMessage(model string) string {
	return fmt.Sprintf("所选模型「%s」已下线或不存在，请打开「%s」软件重新选择模型后再试", model, config.AppTitle)
}

// rejectModelGone 在本地直接拒绝使用已不存在模型的聊天请求，并以 IDE 能展示的格式返回提示：
// Connect 流式请求 → HTTP 200 + 错误 trailer 帧；其它 → HTTP 400 + 文本。
func (s *Server) rejectModelGone(w http.ResponseWriter, r *http.Request, model string) {
	msg := ModelGoneMessage(model)
	if s.log != nil {
		s.log.Log(fmt.Sprintf("[模型不存在] %s · 已在后台下线或隐藏，请求已在本地拦截，请用户重新选择模型", model))
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "connect") {
		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(connectframe.ErrorTrailer(msg, "invalid_argument"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(msg))
}

// forwardRelay 把 GetChatMessage 请求转发到中继 /relay。
// 客户端只通过请求头声明目标模型，protobuf 的解析与改写统一由服务端完成一次。
func (s *Server) forwardRelay(w http.ResponseWriter, r *http.Request, host string, body []byte, relayBase, relayToken string) {
	if relayBase == "" {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("BaiLan: 未配置中继服务器"))
		s.log.Log("[Manager] 未配置中继服务器，无法转发")
		return
	}

	model := s.currentModel()
	effectiveModel := model
	overrideHeader := ""

	if strings.Contains(r.URL.Path, "GetChatMessage") && model != "" {
		overrideHeader = model
		// 用户本地保存的模型已被后台隐藏/删除时，不再向中继发请求，直接告知用户。
		if !config.ModelInCatalog(model) {
			s.rejectModelGone(w, r, model)
			return
		}
	}

	requestStart := time.Now()
	target := relayBase + "/relay"
	// Request 不可跨 Do 复用，每次重试都重新构造（bytes.Reader 使 GetBody 自动可用）。
	buildReq := func(plan uploadPlan) (*http.Request, error) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(plan.payload))
		if err != nil {
			return nil, err
		}
		copyRequestHeaders(req.Header, r.Header, headersToRemove)
		req.Header.Set("X-Original-Host", host)
		req.Header.Set("X-Original-Path", r.URL.RequestURI())
		if overrideHeader != "" {
			req.Header.Set("X-Override-Model", overrideHeader)
		}
		req.Header.Del("Expect")
		req.Header.Set("Accept-Encoding", "identity")
		if relayToken != "" {
			req.Header.Set("X-Relay-Token", relayToken)
		}
		if licToken, licCard, licDevice := s.licenseHeaders(); licToken != "" {
			req.Header.Set("X-License-Token", licToken)
			if licCard != "" {
				req.Header.Set("X-Card-Key", licCard)
			}
			if licDevice != "" {
				req.Header.Set("X-Device-Id", licDevice)
			}
		}
		plan.apply(req.Header)
		req.ContentLength = int64(len(plan.payload))
		return req, nil
	}

	label := config.ModelLabel(effectiveModel)
	plan := s.upload.plan(relayBase, body)
	s.log.Log(fmt.Sprintf("[会话等待] 模型：%s · 请求：%dB · %s · 正在等待中继响应头", label, len(body), plan.describe(len(body))))

	// 只在中继尚未接收请求时重试（拨号失败 / 平滑重启关闭的空闲连接），最长等 relayReconnectWindow；
	// ctx 取消（IDE 放弃）与中继已开始处理后的错误不重试，避免重复处理。
	send := func(plan uploadPlan) (*http.Response, error) {
		var resp *http.Response
		var err error
		reconnecting := false
		deadline := time.Now().Add(relayReconnectWindow)
		for attempt := 1; ; attempt++ {
			req, rerr := buildReq(plan)
			if rerr != nil {
				err = rerr
				break
			}
			resp, err = s.relayClient.Do(req)
			if err == nil || r.Context().Err() != nil || !isRetryableRelayError(err) || time.Now().After(deadline) {
				break
			}
			if !reconnecting {
				reconnecting = true
				s.reconnect.begin()
			}
			s.reconnect.attempt()
			s.log.Log(fmt.Sprintf("[会话重连] 模型：%s · 中继暂时不可达（可能正在重启），第%d次重连：%v", label, attempt, err))
			select {
			case <-time.After(relayReconnectBackoff(attempt)):
			case <-r.Context().Done():
			}
		}
		if reconnecting {
			s.reconnect.end(err == nil)
			if err == nil {
				s.log.Log(fmt.Sprintf("[会话重连] 模型：%s · 已重新连接中继，对话继续", label))
			}
		}
		return resp, err
	}

	resp, err := send(plan)
	for err == nil {
		next, reason := s.upload.fallback(resp, plan, body)
		if next == nil {
			break
		}
		resp.Body.Close()
		plan = *next
		s.log.Log(fmt.Sprintf("[会话重发] 模型：%s · %s · %s", label, reason, plan.describe(len(body))))
		resp, err = send(plan)
	}
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("BaiLan relay error: " + err.Error()))
		if r.Context().Err() != nil {
			s.log.Log(fmt.Sprintf("[会话取消] 模型：%s · IDE 侧已断开", label))
		} else {
			s.log.Log(fmt.Sprintf("[会话转发失败] 模型：%s · %v", label, err))
		}
		return
	}
	defer resp.Body.Close()
	s.upload.observe(relayBase, resp, plan, body)
	headersAt := time.Now()
	if reported := strings.TrimSpace(resp.Header.Get("X-Bailan-Effective-Model")); reported != "" {
		effectiveModel = reported
	}
	resp.Header.Del("X-Bailan-Effective-Model")

	label = config.ModelLabel(effectiveModel)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.log.Log(fmt.Sprintf("[会话响应头] 模型：%s · 状态码：%d · 等待：%.3fs", label, resp.StatusCode, headersAt.Sub(requestStart).Seconds()))
	} else {
		s.log.Log(fmt.Sprintf("[会话响应头异常] 模型：%s · 状态码：%d · 等待：%.3fs", label, resp.StatusCode, headersAt.Sub(requestStart).Seconds()))
	}
	stats := streamResponse(w, resp)
	firstByte := "-"
	if !stats.FirstByteAt.IsZero() {
		firstByte = fmt.Sprintf("%.3fs", stats.FirstByteAt.Sub(requestStart).Seconds())
	}
	if stats.Err != nil {
		s.log.Log(fmt.Sprintf("[会话流中断] 模型：%s · 首包：%s · 总耗时：%.3fs · %dB/%d块 · %v", label, firstByte, stats.EndAt.Sub(requestStart).Seconds(), stats.Bytes, stats.Chunks, stats.Err))
		return
	}
	s.log.Log(fmt.Sprintf("[会话完成] 模型：%s · 首包：%s · 总耗时：%.3fs · %dB/%d块", label, firstByte, stats.EndAt.Sub(requestStart).Seconds(), stats.Bytes, stats.Chunks))
}

func (s *Server) requestHost(r *http.Request) string {
	candidates := []string{r.Host}
	if r.TLS != nil {
		candidates = append(candidates, r.TLS.ServerName)
	}
	s.mu.RLock()
	domains := s.domains
	s.mu.RUnlock()
	fallback := ""
	for _, c := range candidates {
		h := normalizeHost(c)
		if h == "" {
			continue
		}
		if fallback == "" {
			fallback = h
		}
		if domains[h] {
			return h
		}
	}
	return fallback
}

func normalizeHost(v string) string {
	h := strings.TrimSpace(v)
	if h == "" {
		return ""
	}
	if strings.Contains(h, "/") {
		h = strings.SplitN(h, "/", 2)[0]
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func writeEmpty200(w http.ResponseWriter) {
	w.Header().Set("content-type", "application/proto")
	w.WriteHeader(http.StatusOK)
}

// isDialError 判断错误是否发生在拨号阶段（未向中继写出任何字节，重试不会产生重复请求）。
func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func isPolicyPassthrough(matchText string) bool {
	return containsAny(matchText, policyPassthroughKeywords)
}

func isCustomerTeamSettings(matchText string) bool {
	return config.IsCustomer() && strings.Contains(matchText, "GetCliTeamSettings")
}

func containsAny(text string, keywords []string) bool {
	for _, k := range keywords {
		if k != "" && strings.Contains(text, k) {
			return true
		}
	}
	return false
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// copyRequestHeaders 复制请求头，剔除逐跳头、Content-Length 与 remove 列表。
func copyRequestHeaders(dst, src http.Header, remove []string) {
	removeSet := map[string]bool{}
	for _, h := range remove {
		removeSet[strings.ToLower(h)] = true
	}
	for name, values := range src {
		lower := strings.ToLower(name)
		if hopByHop[lower] || removeSet[lower] || lower == "content-length" || lower == "host" {
			continue
		}
		for _, v := range values {
			dst.Add(name, v)
		}
	}
}

// copyResponseHeaders 复制端到端响应头，过滤逐跳头。
func copyResponseHeaders(dst, src http.Header) {
	for name, values := range src {
		if hopByHop[strings.ToLower(name)] {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

// responseStats 返回流式转发的首包、结束、字节和分块统计。
type responseStats struct {
	FirstByteAt time.Time
	EndAt       time.Time
	Bytes       int
	Chunks      int
	Err         error
}

// streamResponse 把上游响应边收边写回客户端，并返回可用于定位慢点的分段统计。
// connectEOS 跟踪 Connect 流式信封（1 字节 flags + 4 字节大端长度 + 载荷），
// done 表示 end-of-stream 帧（flags&0x02）已完整转发给 IDE。
type connectEOS struct {
	hdr  [5]byte
	hn   int
	left int
	done bool
}

func (c *connectEOS) feed(p []byte) {
	for len(p) > 0 && !c.done {
		if c.hn < len(c.hdr) {
			k := copy(c.hdr[c.hn:], p)
			c.hn += k
			p = p[k:]
			if c.hn < len(c.hdr) {
				return
			}
			c.left = int(binary.BigEndian.Uint32(c.hdr[1:]))
		}
		k := min(c.left, len(p))
		c.left -= k
		p = p[k:]
		if c.left == 0 {
			c.done = c.hdr[0]&0x02 != 0
			c.hn = 0
		}
	}
}

func streamResponse(w http.ResponseWriter, resp *http.Response) responseStats {
	stats := responseStats{}
	dst := w.Header()
	for name, values := range resp.Header {
		if hopByHop[strings.ToLower(name)] {
			continue
		}
		for _, v := range values {
			dst.Add(name, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	rc := http.NewResponseController(w)
	var eos *connectEOS
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "application/connect") {
		eos = &connectEOS{}
	}
	buf := make([]byte, 4*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if stats.FirstByteAt.IsZero() {
				stats.FirstByteAt = time.Now()
			}
			stats.Bytes += n
			stats.Chunks++
			if _, werr := w.Write(buf[:n]); werr != nil {
				stats.Err = werr
				break
			}
			_ = rc.Flush()
			if eos != nil {
				eos.feed(buf[:n])
			}
		}
		if err != nil {
			if err != io.EOF {
				stats.Err = err
			}
			break
		}
	}
	// IDE 收到 end-of-stream 帧即认为本轮结束并立刻断开，常抢在中继关闭响应体之前，
	// 此时读响应体会报 context canceled；数据已完整送达，不算中断。
	if eos != nil && eos.done {
		stats.Err = nil
	}
	// 复制可能存在的 trailer（Connect 协议一般不用，best-effort）。
	for name, values := range resp.Trailer {
		for _, v := range values {
			dst.Add(http.TrailerPrefix+name, v)
		}
	}
	stats.EndAt = time.Now()
	return stats
}
