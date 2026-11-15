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
	"bailan-relay/internal/sysutil"
)

// 需要吞掉的系统菜单触发消息：左键点左上角图标 / 右键点标题栏 / Alt+Space。
const (
	wmGetMinMaxInfo   = 0x0024
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
	procGetWindowRect       = modAppUser32.NewProc("GetWindowRect")
	procGetClassLongPtr     = modAppUser32.NewProc("GetClassLongPtrW")
	procLoadIcon            = modAppUser32.NewProc("LoadIconW")
	procShowWindow          = modAppUser32.NewProc("ShowWindow")
	procSetForegroundWindow = modAppUser32.NewProc("SetForegroundWindow")
	procCreatePopupMenu     = modAppUser32.NewProc("CreatePopupMenu")
	procAppendMenu          = modAppUser32.NewProc("AppendMenuW")
	procTrackPopupMenu      = modAppUser32.NewProc("TrackPopupMenu")
	procDestroyMenu         = modAppUser32.NewProc("DestroyMenu")

	modShell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIcon = modShell32.NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy       = 0x0002
	wmNull          = 0x0000
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmTray          = 0x8000 + 1
	wmSyncTray      = 0x8000 + 2

	swHide    = 0
	swRestore = 9

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 1
	nifIcon    = 2
	nifTip     = 4

	tpmReturnCmd = 0x0100
	tpmNonotify  = 0x0080

	trayShow = 1
	trayQuit = 2
)

// notifyIconData 是 Shell_NotifyIconW 用的旧版结构，够放图标、提示和点击回调。
type notifyIconData struct {
	Size     uint32
	Hwnd     uintptr
	ID       uint32
	Flags    uint32
	Callback uint32
	Icon     uintptr
	Tip      [128]uint16
}

var (
	trayShown    bool
	quitFromTray bool
)

// windowShell 记录 100% 界面缩放时的外框像素尺寸，供整体缩放时同步改 Win32 窗口。
type windowShell struct {
	baseW, baseH uint
}

// minMaxInfo 对应 Win32 MINMAXINFO。系统在发出 WM_GETMINMAXINFO 前已填好其余字段。
type minMaxInfo struct {
	Reserved, MaxSize, MaxPosition, MinTrack, MaxTrack struct{ X, Y int32 }
}

const (
	spiGetWorkArea      = 0x0030
	preferredWinWidth   = 1280 // 96 DPI 逻辑宽度，按系统 DPI 换算为物理像素
	preferredWinHeight  = 820
	maxWorkAreaFraction = 0.9
	// minTrackNumer/minTrackDenom 是拖动下限，等于界面缩放的 80% 档。再小顶栏和状态卡会挤成一条。
	minTrackNumer = 4
	minTrackDenom = 5
)

// minTrackSize 返回窗口拖动时允许的最小外框像素。输入是 100% 缩放下的外框尺寸。
func minTrackSize(baseW, baseH uint) (int32, int32) {
	w := int32(baseW) * minTrackNumer / minTrackDenom
	h := int32(baseH) * minTrackNumer / minTrackDenom
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

func init() {
	sysutil.EnablePerMonitorDpiV2()
}

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

func appWindowSizeForUIScale(uiPercent int) (uint, uint) {
	baseW, baseH := appWindowSize()
	f := float64(config.NormalizeUIScalePercent(uiPercent)) / 100
	return uint(float64(baseW) * f), uint(float64(baseH) * f)
}

func (s *windowShell) resizeForUIScale(percent int, hwnd uintptr) {
	if s == nil || s.baseW == 0 || s.baseH == 0 {
		return
	}
	if zoomed, _, _ := procIsZoomed.Call(hwnd); zoomed != 0 {
		_, _, _ = procPostMessageW.Call(hwnd, wmSysCommand, scRestore, 0)
	}
	p := config.NormalizeUIScalePercent(percent)
	f := float64(p) / 100
	newW := int32(float64(s.baseW) * f)
	newH := int32(float64(s.baseH) * f)
	minW, minH := minTrackSize(s.baseW, s.baseH)
	if newW < minW {
		newW = minW
	}
	if newH < minH {
		newH = minH
	}
	var rc struct {
		Left, Top, Right, Bottom int32
	}
	_, _, _ = procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	cx := (rc.Left + rc.Right) / 2
	cy := (rc.Top + rc.Bottom) / 2
	_, _, _ = procSetWindowPos.Call(
		hwnd, 0,
		uintptr(cx-newW/2), uintptr(cy-newH/2),
		uintptr(newW), uintptr(newH),
		0,
	)
	_ = config.SaveUIScalePercent(p)
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

	uiPercent := config.LoadUIScalePercent()
	baseW, baseH := appWindowSize()
	shell := &windowShell{baseW: baseW, baseH: baseH}
	width, height := appWindowSizeForUIScale(uiPercent)
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
	bindUiScale(w, hwnd, shell)
	hookWindowProc(hwnd)
	if config.Load().CloseToTray {
		ensureTray(hwnd)
	}
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
		case "tray-sync":
			post(wmSyncTray, 0, 0)
		}
		return zoomed != 0
	})
}

func bindUiScale(w webview2.WebView, hwnd uintptr, shell *windowShell) {
	_ = w.Bind("bailanUiScale", func(percent float64) bool {
		shell.resizeForUIScale(int(percent), hwnd)
		return true
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
//  2. 拖动时限制最小外框（WM_GETMINMAXINFO）；
//  3. 拦截所有会弹出系统菜单的消息；其余原样转发。
func hookWindowProc(hwnd uintptr) {
	var origProc uintptr
	cb := windows.NewCallback(func(h, msg, wp, lp uintptr) uintptr {
		switch msg {
		case wmGetMinMaxInfo:
			if lp != 0 {
				info := (*minMaxInfo)(unsafe.Pointer(lp))
				bw, bh := appWindowSize()
				info.MinTrack.X, info.MinTrack.Y = minTrackSize(bw, bh)
			}
			return 0
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
		case wmClose:
			if !quitFromTray && config.Load().CloseToTray && ensureTray(h) {
				hideMainWindow(h)
				return 0
			}
			removeTray(h)
			quitFromTray = false
		case wmSyncTray:
			if config.Load().CloseToTray {
				ensureTray(h)
			} else {
				removeTray(h)
				showMainWindow(h)
			}
			return 0
		case wmTray:
			switch lp {
			case wmLButtonUp, wmLButtonDblClk:
				showMainWindow(h)
			case wmRButtonUp:
				if trayMenu(h) == trayQuit {
					quitFromTray = true
					_, _, _ = procPostMessageW.Call(h, wmClose, 0, 0)
				}
			}
			return 0
		case wmDestroy:
			removeTray(h)
		}
		r, _, _ := procCallWindowProcW.Call(origProc, h, msg, wp, lp)
		return r
	})
	origProc, _, _ = procSetWindowLongPtrW.Call(hwnd, uintptr(gwlpWndProc), cb)
	// 让系统按新的 WM_NCCALCSIZE 重新计算一次边框。
	_, _, _ = procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder)
}

func trayData(hwnd uintptr) notifyIconData {
	data := notifyIconData{
		Size:     uint32(unsafe.Sizeof(notifyIconData{})),
		Hwnd:     hwnd,
		ID:       1,
		Flags:    nifMessage | nifIcon | nifTip,
		Callback: wmTray,
	}
	icon, _, _ := procGetClassLongPtr.Call(hwnd, ^uintptr(13)) // GCLP_HICON
	if icon == 0 {
		icon, _, _ = procLoadIcon.Call(0, 32512) // IDI_APPLICATION
	}
	data.Icon = icon
	tip, err := windows.UTF16FromString(config.AppTitle)
	if err == nil {
		copy(data.Tip[:], tip)
	}
	return data
}

func ensureTray(hwnd uintptr) bool {
	data := trayData(hwnd)
	cmd := uintptr(nimAdd)
	if trayShown {
		cmd = nimModify
	}
	ok, _, _ := procShellNotifyIcon.Call(cmd, uintptr(unsafe.Pointer(&data)))
	if ok == 0 && !trayShown {
		ok, _, _ = procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
	}
	if ok == 0 {
		return false
	}
	trayShown = true
	return true
}

func removeTray(hwnd uintptr) {
	if !trayShown {
		return
	}
	data := notifyIconData{
		Size: uint32(unsafe.Sizeof(notifyIconData{})),
		Hwnd: hwnd,
		ID:   1,
	}
	_, _, _ = procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&data)))
	trayShown = false
}

func showMainWindow(hwnd uintptr) {
	_, _, _ = procShowWindow.Call(hwnd, swRestore)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
}

func hideMainWindow(hwnd uintptr) {
	_, _, _ = procShowWindow.Call(hwnd, swHide)
}

func trayMenu(hwnd uintptr) uintptr {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0
	}
	defer procDestroyMenu.Call(menu)
	show, err1 := windows.UTF16PtrFromString("显示窗口")
	quit, err2 := windows.UTF16PtrFromString("退出")
	if err1 != nil || err2 != nil {
		return 0
	}
	_, _, _ = procAppendMenu.Call(menu, 0, trayShow, uintptr(unsafe.Pointer(show)))
	_, _, _ = procAppendMenu.Call(menu, 0, trayQuit, uintptr(unsafe.Pointer(quit)))
	var pt struct{ X, Y int32 }
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmNonotify, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	_, _, _ = procPostMessageW.Call(hwnd, wmNull, 0, 0)
	return cmd
}
