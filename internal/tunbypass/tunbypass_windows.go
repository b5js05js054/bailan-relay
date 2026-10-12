//go:build windows

package tunbypass

import (
	"encoding/binary"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IP_UNICAST_IF（IPPROTO_IP）：强制 socket 从指定网卡发出，绕过路由表/TUN。
const ipUnicastIf = 31

var procSetsockopt = syscall.NewLazyDLL("ws2_32.dll").NewProc("setsockopt")

// bindSocket 用 IP_UNICAST_IF 把 socket 绑到 ifIndex（要求网络字节序）。
// v6 socket 上设置 v4 选项会失败，静默忽略。
func bindSocket(fd uintptr, idx uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], idx)
	_, _, _ = procSetsockopt.Call(fd, 0, ipUnicastIf, uintptr(unsafe.Pointer(&buf[0])), 4)
}

// tunnelCarriesTraffic Windows 下隧道网卡只在 VPN/TUN 运行时才处于 Up 状态，存在即视为激活。
func tunnelCarriesTraffic(map[int]bool) bool { return true }

// gatewayIfaces 经 GetAdaptersInfo 返回「配置了默认网关且描述不像隧道」的 ifIndex 集合。
func gatewayIfaces() map[int]bool {
	out := map[int]bool{}
	var size uint32
	_ = windows.GetAdaptersInfo(nil, &size) // 首次调用必然 BUFFER_OVERFLOW，只为拿 size
	if size == 0 {
		return out
	}
	buf := make([]byte, size)
	if err := windows.GetAdaptersInfo((*windows.IpAdapterInfo)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return out
	}
	for p := (*windows.IpAdapterInfo)(unsafe.Pointer(&buf[0])); p != nil; p = p.Next {
		gw := windows.ByteSliceToString(p.GatewayList.IpAddress.String[:])
		ip := windows.ByteSliceToString(p.IpAddressList.IpAddress.String[:])
		if gw == "" || gw == "0.0.0.0" || ip == "" || ip == "0.0.0.0" {
			continue
		}
		if nameTunnelLike(windows.ByteSliceToString(p.Description[:])) {
			continue
		}
		out[int(p.Index)] = true
	}
	return out
}
