//go:build darwin

package tunbypass

import (
	"net"
	"strings"
	"syscall"

	"bailan-relay/internal/sysutil"
)

// macOS 的网卡绑定选项：IP_BOUND_IF（IPPROTO_IP）/ IPV6_BOUND_IF（IPPROTO_IPV6），值为主机字节序 ifIndex。
const (
	ipBoundIf   = 25
	ipv6BoundIf = 125
)

// probeIP 用于判断「公网流量实际从哪张网卡出去」的探测地址（只查路由表，不发包）。
const probeIP = "223.5.5.5"

// bindSocket 把 socket 绑到 ifIndex；按 socket 族各试一次，失败静默忽略。
func bindSocket(fd uintptr, idx uint32) {
	if syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBoundIf, int(idx)) != nil {
		_ = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, ipv6BoundIf, int(idx))
	}
}

// tunnelCarriesTraffic macOS 上 utun 网卡常驻（iCloud 等系统服务），名称匹配不代表 VPN 在跑；
// 只有公网路由确实指向疑似隧道网卡时才视为激活。
func tunnelCarriesTraffic(tunnel map[int]bool) bool {
	name := routeInterface(probeIP)
	if name == "" {
		return false
	}
	ifc, err := net.InterfaceByName(name)
	return err == nil && tunnel[ifc.Index]
}

// gatewayIfaces 从路由表收集所有 default 路由所在、且不像隧道的网卡 ifIndex。
func gatewayIfaces() map[int]bool {
	out := map[int]bool{}
	res, _, err := sysutil.RunHidden("netstat", "-rn", "-f", "inet")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(res, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "default" {
			continue
		}
		name := fields[len(fields)-1]
		if nameTunnelLike(name) {
			continue
		}
		if ifc, err := net.InterfaceByName(name); err == nil {
			out[ifc.Index] = true
		}
	}
	return out
}

// routeInterface 返回系统路由表中到达 ip 所用的网卡名。
func routeInterface(ip string) string {
	res, _, err := sysutil.RunHidden("route", "-n", "get", ip)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(res, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}
	return ""
}
