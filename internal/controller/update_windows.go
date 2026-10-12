//go:build windows

package controller

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"bailan-relay/internal/sysutil"
)

const selfUpdateSupported = true

// installUpdate 写出替换脚本并以独立进程启动：等本进程退出后覆盖 exe 并重启。
// 输入：已下载的新 exe、当前 exe 路径、清洗后的版本号；输出：启动失败的原因。
func installUpdate(newExe, exePath, safeVersion string) error {
	scriptPath := filepath.Join(os.TempDir(), "bailan-relay-update-"+safeVersion+".ps1")
	if err := os.WriteFile(scriptPath, []byte(updateScript), 0o600); err != nil {
		return errors.New("无法写入更新脚本：" + err.Error())
	}
	args := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath,
		"-WaitPid", strconv.Itoa(os.Getpid()),
		"-Source", newExe,
		"-Target", exePath,
	}
	if err := sysutil.StartDetached("", "powershell.exe", args...); err != nil {
		return errors.New("无法启动更新程序：" + err.Error())
	}
	return nil
}

// updateScript 等待当前进程退出后覆盖 exe 并重新启动。
const updateScript = `param(
    [int]$WaitPid,
    [string]$Source,
    [string]$Target
)
$ErrorActionPreference = 'Stop'
while (Get-Process -Id $WaitPid -ErrorAction SilentlyContinue) {
    Start-Sleep -Milliseconds 500
}
$deadline = (Get-Date).AddMinutes(2)
while ($true) {
    try {
        Copy-Item -LiteralPath $Source -Destination $Target -Force
        break
    } catch {
        if ((Get-Date) -gt $deadline) { throw }
        Start-Sleep -Milliseconds 500
    }
}
Start-Process -FilePath $Target -WorkingDirectory (Split-Path -Path $Target -Parent)
Remove-Item -LiteralPath $Source -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue
`
