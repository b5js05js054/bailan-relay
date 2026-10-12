//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"

	"bailan-relay/internal/sysutil"
)

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	_, _, _ = sysutil.RunHidden("cmd", "/c", "echo "+message)
	os.Exit(2)
}

// openBrowser 优先以 Edge 应用窗口打开，退回默认浏览器。
func openBrowser(url string) {
	for _, edge := range edgeCandidates() {
		if _, err := os.Stat(edge); err == nil {
			if sysutil.StartDetached("", edge, "--app="+url) == nil {
				return
			}
		}
	}
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func edgeCandidates() []string {
	var out []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		if base := os.Getenv(env); base != "" {
			out = append(out, base+`\Microsoft\Edge\Application\msedge.exe`)
		}
	}
	return out
}
