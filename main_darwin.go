//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <stdlib.h>

// bailanInstallMenu 安装应用菜单与「编辑」菜单。
// WKWebView 的 Cmd+C/V/X/A/Z 依赖主菜单里的对应菜单项分发，缺了就全部失效。
static void bailanInstallMenu(const char *appName) {
	NSString *name = [NSString stringWithUTF8String:appName];
	NSMenu *bar = [[NSMenu alloc] init];

	NSMenuItem *appItem = [[NSMenuItem alloc] init];
	NSMenu *appMenu = [[NSMenu alloc] init];
	[appMenu addItemWithTitle:[@"隐藏 " stringByAppendingString:name] action:@selector(hide:) keyEquivalent:@"h"];
	[appMenu addItem:[NSMenuItem separatorItem]];
	[appMenu addItemWithTitle:[@"退出 " stringByAppendingString:name] action:@selector(terminate:) keyEquivalent:@"q"];
	[appItem setSubmenu:appMenu];
	[bar addItem:appItem];

	NSMenuItem *editItem = [[NSMenuItem alloc] init];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"编辑"];
	[edit addItemWithTitle:@"撤销" action:@selector(undo:) keyEquivalent:@"z"];
	NSMenuItem *redo = [edit addItemWithTitle:@"重做" action:@selector(redo:) keyEquivalent:@"z"];
	[redo setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagShift];
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItemWithTitle:@"剪切" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"拷贝" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"粘贴" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"全选" action:@selector(selectAll:) keyEquivalent:@"a"];
	[editItem setSubmenu:edit];
	[bar addItem:editItem];

	[NSApp setMainMenu:bar];
	[NSApp activateIgnoringOtherApps:YES];
}
*/
import "C"

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	webview "github.com/webview/webview_go"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

// Cocoa 要求窗口与事件循环跑在进程主线程：main 包初始化时就把主 goroutine 钉在主线程上。
func init() { runtime.LockOSThread() }

const (
	appWinWidth  = 1180
	appWinHeight = 760
)

// openAppWindow 用 WKWebView 原生窗口承载控制台（系统标题栏，不注入 bailanWin）。
// 窗口关闭后返回 true（由调用方做收尾退出）；创建失败返回 false 退回浏览器。
func openAppWindow(url string) (ok bool) {
	// webview_go 初始化失败时会 panic（如系统版本不支持），转成失败返回。
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	w := webview.New(false)
	if w == nil {
		return false
	}
	defer w.Destroy()
	name := C.CString(config.AppTitle)
	C.bailanInstallMenu(name)
	C.free(unsafe.Pointer(name))
	w.SetTitle(config.AppTitle)
	w.SetSize(appWinWidth, appWinHeight, webview.HintNone)
	w.Navigate(url)
	w.Run()
	return true
}

// fatal 打印错误并用系统对话框提示（.app 双击启动时没有终端可看）。
func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	script := `display alert "` + config.AppTitle + `" message "` + escapeAppleScript(message) + `" as critical`
	_, _, _ = sysutil.RunHidden("osascript", "-e", script)
	os.Exit(2)
}

// openBrowser 用系统默认浏览器打开控制台。
func openBrowser(url string) { _ = sysutil.OpenURL(url) }

func escapeAppleScript(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\\' || r == '"' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}
