//go:build darwin && !cgo

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// openAppWindow 无 cgo 时没有 WKWebView：返回 false 让 main 退回默认浏览器。
func openAppWindow(string) bool { return false }

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(2)
}

// openBrowser 用系统默认浏览器打开控制台。
func openBrowser(url string) { _ = exec.Command("open", url).Start() }
