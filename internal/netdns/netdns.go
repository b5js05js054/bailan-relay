// Package netdns 通过公共 DNS(UDP) 与 DoH 解析域名真实 IP，避免命中本机
// hosts 造成 127.0.0.1 回环。对应旧版 proxy_addon.py / app.py 的解析逻辑。
package netdns

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/config"
	"bailan-relay/internal/tunbypass"
)

// DoH 兜底端点（用 IP 直连，绕开被污染的 UDP 53）。
var dohEndpoints = []string{
	"https://223.5.5.5/resolve",
	"https://1.1.1.1/dns-query",
}

// Resolver 带缓存的解析器。
type Resolver struct {
	mu      sync.RWMutex
	cache   map[string]string
	servers func() []string
	doh     *http.Client
}

// New 创建解析器。servers 每次调用返回当前 DNS 列表。
func New(servers func() []string) *Resolver {
	return &Resolver{
		cache:   map[string]string{},
		servers: servers,
		doh: &http.Client{
			Timeout: 4 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				DialContext:     tunbypass.NewDialer(4 * time.Second).DialContext,
			},
		},
	}
}

func (r *Resolver) dnsServers() []string {
	if r.servers != nil {
		if s := r.servers(); len(s) > 0 {
			return s
		}
	}
	return config.DefaultDNSServers
}

// ResolveRealIP 解析真实 IP，失败返回空串（绝不回落被劫持域名）。
func (r *Resolver) ResolveRealIP(domain string) string {
	host := strings.ToLower(strings.TrimRight(strings.TrimSpace(domain), "."))
	if host == "" || host == "127.0.0.1" || host == "::1" || host == "localhost" {
		return host
	}
	r.mu.RLock()
	cached := r.cache[host]
	r.mu.RUnlock()
	if cached != "" {
		return cached
	}
	ip, _ := r.Resolve(host)
	if ip != "" {
		r.mu.Lock()
		r.cache[host] = ip
		r.mu.Unlock()
	}
	return ip
}

// Refresh 强制重新解析并更新缓存，返回真实 IP（失败返回空串且不写缓存）。
// 用于拨号失败后丢弃可能已失效的缓存 IP（CDN 边缘轮换/隧道环境变化）。
func (r *Resolver) Refresh(domain string) string {
	host := strings.ToLower(strings.TrimRight(strings.TrimSpace(domain), "."))
	if host == "" {
		return ""
	}
	ip, _ := r.Resolve(host)
	if ip != "" {
		r.mu.Lock()
		r.cache[host] = ip
		r.mu.Unlock()
	}
	return ip
}

// DialContext 返回可直接挂到 http.Transport.DialContext 的硬化拨号函数。
// 链式策略：真实 IP+物理网卡绑定 → 同 IP 走系统路由 → 原始域名+绑卡 → 原始域名+系统路由。
// 专门对抗 TUN/fake-ip（系统 DNS 返回 198.18.x.x 假地址，直连必死）与绑卡误判；
// 无隧道环境下等价于普通拨号，天然不走系统代理（transport Proxy 未设置）。
func (r *Resolver) DialContext(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	bound := tunbypass.NewDialer(timeout)
	plain := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host, port = addr, "443"
		}
		if net.ParseIP(host) != nil {
			if conn, derr := bound.DialContext(ctx, network, addr); derr == nil {
				return conn, nil
			}
			return plain.DialContext(ctx, network, addr)
		}
		if ip := r.ResolveRealIP(host); config.IsPublicIPv4(ip) {
			if conn, derr := bound.DialContext(ctx, network, net.JoinHostPort(ip, port)); derr == nil {
				return conn, nil
			}
			// 绑卡拨号失败（网卡误绑/无路由）：换系统路由再试同一真实 IP。
			if conn, derr := plain.DialContext(ctx, network, net.JoinHostPort(ip, port)); derr == nil {
				return conn, nil
			}
		}
		if conn, derr := bound.DialContext(ctx, network, addr); derr == nil {
			return conn, nil
		}
		return plain.DialContext(ctx, network, addr)
	}
}

// Resolve 返回 (ip, via)，用于 UI 自检；via 是命中的 DNS/DoH 来源。
func (r *Resolver) Resolve(domain string) (string, string) {
	host := strings.ToLower(strings.TrimRight(strings.TrimSpace(domain), "."))
	if host == "" {
		return "", ""
	}
	query := encodeDNSQuery(host)
	for _, server := range r.dnsServers() {
		if ip := queryUDP(server, query); ip != "" {
			return ip, server
		}
	}
	for _, endpoint := range dohEndpoints {
		if ip := r.queryDoH(endpoint, host); ip != "" {
			return ip, "DoH " + endpoint
		}
	}
	return "", ""
}

func queryUDP(server string, query []byte) string {
	conn, err := tunbypass.NewDialer(2*time.Second).DialContext(context.Background(), "udp", net.JoinHostPort(server, "53"))
	if err != nil {
		return ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(query); err != nil {
		return ""
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return ""
	}
	return parseFirstA(buf[:n])
}

func (r *Resolver) queryDoH(endpoint, host string) string {
	req, err := http.NewRequest(http.MethodGet, endpoint+"?name="+host+"&type=A", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("accept", "application/dns-json")
	resp, err := r.doh.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return ""
	}
	var payload struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, a := range payload.Answer {
		if a.Type == 1 && config.IsPublicIPv4(strings.TrimSpace(a.Data)) {
			return strings.TrimSpace(a.Data)
		}
	}
	return ""
}

func encodeDNSQuery(host string) []byte {
	var name []byte
	for _, label := range strings.Split(strings.Trim(host, "."), ".") {
		name = append(name, byte(len(label)))
		name = append(name, []byte(label)...)
	}
	name = append(name, 0)
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:], uint16(rand.Intn(65536)))
	binary.BigEndian.PutUint16(header[2:], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(header[4:], 1)      // 1 question
	q := append(header, name...)
	tail := make([]byte, 4)
	binary.BigEndian.PutUint16(tail[0:], 1) // type A
	binary.BigEndian.PutUint16(tail[2:], 1) // class IN
	return append(q, tail...)
}

func parseFirstA(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	answers := int(binary.BigEndian.Uint16(data[6:8]))
	offset := 12
	for offset < len(data) && data[offset] != 0 {
		offset += 1 + int(data[offset])
	}
	offset += 5 // 结束 0 字节 + QTYPE(2) + QCLASS(2)
	for i := 0; i < answers; i++ {
		if offset+12 > len(data) {
			break
		}
		if data[offset]&0xC0 == 0xC0 {
			offset += 2
		} else {
			for offset < len(data) && data[offset] != 0 {
				offset += 1 + int(data[offset])
			}
			offset++
		}
		if offset+10 > len(data) {
			break
		}
		rtype := binary.BigEndian.Uint16(data[offset : offset+2])
		rdlen := int(binary.BigEndian.Uint16(data[offset+8 : offset+10]))
		offset += 10
		if rtype == 1 && rdlen == 4 && offset+4 <= len(data) {
			ip := net.IPv4(data[offset], data[offset+1], data[offset+2], data[offset+3]).String()
			if config.IsPublicIPv4(ip) {
				return ip
			}
		}
		offset += rdlen
	}
	return ""
}
