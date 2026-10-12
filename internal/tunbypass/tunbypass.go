// Package tunbypass 应对 TUN 类全局代理（Clash TUN / v2ray / WireGuard 等）：
// 隧道会按路由表吞掉进程的全部出站 TCP/UDP，导致中继连接走代理节点、抖动丢包。
// 这里在检测到活跃隧道网卡时，给我们的出站 socket 绑定物理网卡
// （Windows: IP_UNICAST_IF；macOS: IP_BOUND_IF），直接绕过路由表离开物理网卡。
//
// 检测不到隧道、或找不到可用物理网卡时一律退化为普通拨号，不劣化原行为。
package tunbypass

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 隧道/虚拟网卡特征：匹配 net.Interface.Name 与适配器 Description（小写子串）。
var tunnelPatterns = []string{
	"tun", "tap", "wintun", "meta", "mihomo", "clash", "sing-box", "singbox",
	"v2ray", "xray", "wireguard", "vpn", "utun", "nekoray", "nekobox", "tunnel",
	"vethernet", "hyper-v", "vmware", "virtualbox", "virtual", "loopback",
	"bluetooth", "wan miniport", "filtering platform", "ndis",
}

// 状态缓存：网卡枚举和路由查询有开销，且网络环境变化不频繁。
var (
	cacheMu     sync.Mutex
	cacheAt     time.Time
	cacheActive bool
	cacheIndex  uint32
	cacheName   string
)

// NewDialer 返回挂好隧道绕过的拨号器（无隧道时与普通拨号器行为一致）。
func NewDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second, Control: Control}
}

// Control 是 net.Dialer.Control 实现：隧道激活时把 socket 绑到物理网卡。
// 任何失败都不返回错误——绑定只是优化，失败了照常走系统路由。
func Control(network, address string, c syscall.RawConn) error {
	if !strings.HasPrefix(network, "tcp") && !strings.HasPrefix(network, "udp") {
		return nil
	}
	if host, _, err := net.SplitHostPort(address); err == nil {
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() {
			return nil // 回环目标不绑，防止把本机连接挤出物理网卡
		}
	}
	active, idx, _ := state()
	if !active || idx == 0 {
		return nil
	}
	_ = c.Control(func(fd uintptr) { bindSocket(fd, idx) })
	return nil
}

// Status 返回用于日志的状态描述；未检测到隧道返回空串。
func Status() string {
	active, idx, name := state()
	if !active {
		return ""
	}
	if idx == 0 {
		return "检测到隧道网卡，但未找到可绑定的物理网卡（出站仍走系统路由）"
	}
	return fmt.Sprintf("检测到隧道网卡，出站将绑定物理网卡 %q (ifIndex=%d) 绕过 TUN", name, idx)
}

func state() (bool, uint32, string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if time.Since(cacheAt) < 30*time.Second {
		return cacheActive, cacheIndex, cacheName
	}
	cacheActive, cacheIndex, cacheName = detect()
	cacheAt = time.Now()
	return cacheActive, cacheIndex, cacheName
}

// detect 枚举网卡找出疑似隧道，经平台确认确实在接管流量后，再选定物理网卡。
func detect() (bool, uint32, string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false, 0, ""
	}
	tunnel := map[int]bool{}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		like := nameTunnelLike(ifc.Name)
		if !like {
			// Clash/mihomo 的 fake-ip 网段：198.18.0.0/15
			if addrs, err := ifc.Addrs(); err == nil {
				for _, a := range addrs {
					ipn, ok := a.(*net.IPNet)
					if ok && isFakeIP(ipn.IP) {
						like = true
						break
					}
				}
			}
		}
		if like {
			tunnel[ifc.Index] = true
		}
	}
	if len(tunnel) == 0 || !tunnelCarriesTraffic(tunnel) {
		return false, 0, ""
	}
	idx, name := physicalIndex(ifaces, tunnel)
	return true, idx, name
}

func nameTunnelLike(name string) bool {
	l := strings.ToLower(name)
	for _, p := range tunnelPatterns {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func isFakeIP(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)
}

func hasRealIPv4(ifc net.Interface) bool {
	addrs, err := ifc.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		v4 := ipn.IP.To4()
		if v4 == nil || ipn.IP.IsLinkLocalUnicast() || v4[0] == 0 {
			continue
		}
		return true
	}
	return false
}

// physicalIndex 选定物理网卡：优先有默认网关的非隧道网卡，其次任何可用物理网卡。
func physicalIndex(ifaces []net.Interface, tunnel map[int]bool) (uint32, string) {
	gw := gatewayIfaces()
	pick := func(requireGW bool) (uint32, string) {
		var idx uint32
		var name string
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			if tunnel[ifc.Index] || nameTunnelLike(ifc.Name) {
				continue
			}
			// 无网关的兜底挑选才要求 MAC（排除纯虚拟接口；PPPoE 无 MAC 但有网关）
			if !requireGW && len(ifc.HardwareAddr) == 0 {
				continue
			}
			if requireGW && !gw[ifc.Index] {
				continue
			}
			if !hasRealIPv4(ifc) {
				continue
			}
			if idx == 0 || uint32(ifc.Index) < idx {
				idx, name = uint32(ifc.Index), ifc.Name
			}
		}
		return idx, name
	}
	if idx, name := pick(true); idx != 0 {
		return idx, name
	}
	return pick(false)
}
