//go:build windows

package main

import (
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"bailan-relay/internal/config"
)

// 需要吞掉的系统菜单触发消息：左键点左上角图标 / 右键点标题栏 / Alt+Space。
const (
	wmClose           = 0x0010
	wmNCCalcSize      = 0x0083
	wmNCLButtonDown   = 0x00A1
	wmNCLButtonDblClk = 0x00A3
	wmNCRButtonUp     = 0x00A5
	wmSysKeyDown      = 0x0104
	wmSysChar         = 0x0106
	wmSysCommand      = 0x0112
	wmSysMenu         = 0x0313

	htCaption = 2
	htSysMenu = 4
	htTop     = 12
	vkSpace   = 0x20

	scMinimize = 0xF020
	scMaximize = 0xF030
	scRestore  = 0xF120

	smCyFrame        = 33
	smCxPaddedBorder = 92

	swpFrameChanged = 0x0020
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
)

var gwlpWndProc = -4 // GWLP_WNDPROC，须为变量才能转成 uintptr 传给 syscall

var (
	modAppUser32            = windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtrW   = modAppUser32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW     = modAppUser32.NewProc("CallWindowProcW")
	procSystemParamsInfoW   = modAppUser32.NewProc("SystemParametersInfoW")
	procGetDpiForSystem     = modAppUser32.NewProc("GetDpiForSystem")
	procGetDpiForWindow     = modAppUser32.NewProc("GetDpiForWindow")
	procGetSysMetricsForDpi = modAppUser32.NewProc("GetSystemMetricsForDpi")
	procIsZoomed            = modAppUser32.NewProc("IsZoomed")
	procReleaseCapture      = modAppUser32.NewProc("ReleaseCapture")
	procGetCursorPos        = modAppUser32.NewProc("GetCursorPos")
	procPostMessageW        = modAppUser32.NewProc("PostMessageW")
	procSetWindowPos        = modAppUser32.NewProc("SetWindowPos")
)

const (
	spiGetWorkArea      = 0x0030
	preferredWinWidth   = 1180 // 96 DPI 下的首选尺寸，按系统缩放换算
	preferredWinHeight  = 760
	maxWorkAreaFraction = 0.9
)

// appWindowSize 按系统缩放与屏幕工作区（不含任务栏）计算窗口尺寸，小屏/高缩放下不超出屏幕。
func appWindowSize() (uint, uint) {
	scale := 1.0
	if procGetDpiForSystem.Find() == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi > 0 {
			scale = float64(dpi) / 96
		}
	}
	w, h := float64(preferredWinWidth)*scale, float64(preferredWinHeight)*scale
	var rc struct{ Left, Top, Right, Bottom int32 }
	if ok, _, _ := procSystemParamsInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&rc)), 0); ok != 0 {
		w = min(w, float64(rc.Right-rc.Left)*maxWorkAreaFraction)
		h = min(h, float64(rc.Bottom-rc.Top)*maxWorkAreaFraction)
	}
	return uint(w), uint(h)
}

// webview2RuntimeAvailable 按官方方式检测 WebView2 Runtime 是否安装。
func webview2RuntimeAvailable() bool {
	const clientID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, prefix := range []string{`SOFTWARE`, `SOFTWARE\WOW6432Node`} {
			k, err := registry.OpenKey(root, prefix+`\Microsoft\EdgeUpdate\Clients\`+clientID, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			pv, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && pv != "" && pv != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

// openAppWindow 用内嵌 WebView2 窗口承载控制台：没有浏览器系统菜单/右键菜单，
// 窗口关闭后返回 true（由调用方做收尾退出）；运行时缺失或创建失败返回 false。
func openAppWindow(url string) bool {
	if !webview2RuntimeAvailable() {
		return false
	}
	// Win32 消息循环必须运行在创建窗口的线程上。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	width, height := appWindowSize()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false, // Debug=false 时库会同时关闭 WebView2 默认右键菜单与 DevTools
		AutoFocus: true,
		DataPath:  filepath.Join(config.AppDir(), "webview2"),
		WindowOptions: webview2.WindowOptions{
			Title:  config.AppTitle,
			Width:  width,
			Height: height,
			IconId: 1, // RT_GROUP_ICON 资源 ID（由 rsrc_windows_amd64.syso 提供）
			Center: true,
		},
	})
	if w == nil {
		return false
	}
	hwnd := uintptr(w.Window())
	bindWindowControls(w, hwnd)
	hookWindowProc(hwnd)
	w.Navigate(url)
	w.Run()
	return true
}

// bindWindowControls 向页面暴露 window.bailanWin(action)，供自绘标题栏使用。
// Input : action = drag | resize-top | min | toggle | close | state
// Output: 调用时窗口是否最大化
// 全部用 PostMessage 投递，避免在 WebView2 消息回调里重入窗口过程。
func bindWindowControls(w webview2.WebView, hwnd uintptr) {
	post := func(msg, wp, lp uintptr) { _, _, _ = procPostMessageW.Call(hwnd, msg, wp, lp) }
	_ = w.Bind("bailanWin", func(action string) bool {
		zoomed, _, _ := procIsZoomed.Call(hwnd)
		switch action {
		case "drag", "resize-top":
			hit := uintptr(htCaption)
			if action == "resize-top" {
				hit = htTop
			}
			// 拖动循环以 lParam 中的屏幕坐标为起点，必须给真实光标位置。
			var pt struct{ X, Y int32 }
			_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			_, _, _ = procReleaseCapture.Call()
			post(wmNCLButtonDown, hit, uintptr(uint16(pt.X))|uintptr(uint16(pt.Y))<<16)
		case "min":
			post(wmSysCommand, scMinimize, 0)
		case "toggle":
			if zoomed != 0 {
				post(wmSysCommand, scRestore, 0)
			} else {
				post(wmSysCommand, scMaximize, 0)
			}
		case "close":
			post(wmClose, 0, 0)
		}
		return zoomed != 0
	})
}

// maximizedInset 最大化时系统把窗口外扩一圈边框，客户区顶部需让出同样的高度。
func maximizedInset(hwnd uintptr) int32 {
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	frame, _, _ := procGetSysMetricsForDpi.Call(smCyFrame, dpi)
	pad, _, _ := procGetSysMetricsForDpi.Call(smCxPaddedBorder, dpi)
	return int32(frame + pad)
}

// hookWindowProc 子类化窗口过程：
//  1. 去掉系统标题栏（WM_NCCALCSIZE 保留左右下的缩放边框，只把顶部并入客户区）；
//  2. 拦截所有会弹出系统菜单的消息；其余原样转发。
func hookWindowProc(hwnd uintptr) {
	var origProc uintptr
	cb := windows.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
		switch msg {
		case wmNCCalcSize:
			if wp != 0 {
				rc := (*struct{ Left, Top, Right, Bottom int32 })(*(*unsafe.Pointer)(unsafe.Pointer(&lp)))
				top := rc.Top
				r, _, _ := procCallWindowProcW.Call(origProc, h, msg, wp, lp)
				rc.Top = top
				if z, _, _ := procIsZoomed.Call(h); z != 0 {
					rc.Top += maximizedInset(h)
				}
				return r
			}
		case wmNCLButtonDown, wmNCLButtonDblClk:
			if wp == htSysMenu {
				return 0
			}
		case wmNCRButtonUp:
			if wp == htCaption || wp == htSysMenu {
				return 0
			}
		case wmSysKeyDown, wmSysChar:
			if wp == vkSpace {
				return 0
			}
		case wmSysMenu:
			return 0
		}
		r, _, _ := procCallWindowProcW.Call(origProc, h, msg, wp, lp)
		return r
	})
	origProc, _, _ = procSetWindowLongPtrW.Call(hwnd, uintptr(gwlpWndProc), cb)
	// 让系统按新的 WM_NCCALCSIZE 重新计算一次边框。
	_, _, _ = procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder)
}
