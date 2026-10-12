//go:build darwin

package controller

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"bailan-relay/internal/config"
	"bailan-relay/internal/sysutil"
)

// platformUpdateSource macOS 只使用后台单独下发的 macOS 下载地址；未配置时视为
// 后台尚未发布 Mac 包，不提示更新——否则会把 Windows 的 exe 当成 Mac 更新包下载。
func platformUpdateSource(upd *clientUpdate) (string, string, bool) {
	if strings.TrimSpace(upd.URLMacOS) == "" {
		return "", "", false
	}
	return upd.URLMacOS, upd.SHA256MacOS, true
}

// updatePackagePath 返回 macOS 更新包（内含 .app 的 zip）的本机落盘路径。
func updatePackagePath(safeVersion string) string {
	return filepath.Join(os.TempDir(), "bailan-relay-"+safeVersion+".zip")
}

const (
	updateTargetApp    = "app" // 替换整个 .app 应用包
	updateTargetBinary = "bin" // 替换单个可执行文件
)

// installUpdate 写出替换脚本并后台启动：等本进程退出后解包、去隔离、重签名、替换并重启。
// 输入：已下载的 zip、当前可执行文件路径、清洗后的版本号；输出：启动失败的原因。
func installUpdate(zipPath, exePath, safeVersion string) error {
	target, kind := updateTarget(exePath)
	if target == "" {
		return errors.New("无法定位应用包路径：" + exePath)
	}
	if err := os.MkdirAll(config.LogDir(), 0o755); err != nil {
		return errors.New("无法创建日志目录：" + err.Error())
	}
	scriptPath := filepath.Join(os.TempDir(), "bailan-relay-update-"+safeVersion+".sh")
	if err := os.WriteFile(scriptPath, []byte(updateScript), 0o700); err != nil {
		return errors.New("无法写入更新脚本：" + err.Error())
	}
	userName, userID := relaunchIdentity()
	args := []string{
		scriptPath,
		strconv.Itoa(os.Getpid()),
		zipPath,
		target,
		kind,
		userName,
		userID,
		filepath.Join(config.LogDir(), "update-"+safeVersion+".log"),
	}
	if err := sysutil.StartDetached("", "/bin/bash", args...); err != nil {
		return errors.New("无法启动更新程序：" + err.Error())
	}
	return nil
}

// updateTarget 由 .app 内部的可执行文件路径反推应用包路径；不在应用包内时按单文件处理。
// 输入：当前可执行文件路径；输出：待替换的目标路径与其类型。
func updateTarget(exePath string) (string, string) {
	const marker = ".app/Contents/MacOS/"
	if i := strings.Index(exePath, marker); i > 0 {
		return exePath[:i+len(".app")], updateTargetApp
	}
	return exePath, updateTargetBinary
}

// relaunchIdentity 返回重启新版本时应使用的用户名与 uid。
// 本程序通常以 root 提权运行，需回到发起程序的真实用户，新进程再自行提权。
// 非 root 运行（BAILAN_NO_ELEVATE=1）时返回空，由脚本直接 open。
// 输出：用户名、uid 字符串（无法确定时为 "0"）。
func relaunchIdentity() (string, string) {
	if !sysutil.IsAdmin() {
		return "", "0"
	}
	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" {
		if out, _, err := sysutil.RunHidden("stat", "-f", "%Su", "/dev/console"); err == nil {
			name = strings.TrimSpace(out)
		}
	}
	if name == "" || name == "root" {
		return "", "0"
	}
	u, err := user.Lookup(name)
	if err != nil {
		return "", "0"
	}
	return u.Username, u.Uid
}

// updateScript 等本进程退出后替换应用包并重启。
// 参数：pid zip target kind runas runuid log
// 说明：macOS 的 .app 是目录，必须整包替换；ditto 保留符号链接、扩展属性与签名，
// 普通 cp -R 会破坏包结构；替换前重新 ad-hoc 签名并清掉下载隔离属性，避免 Gatekeeper 拦下。
const updateScript = `#!/bin/bash
set -u
PID="$1"; ZIP="$2"; TARGET="$3"; KIND="$4"; RUNAS="$5"; RUNUID="$6"; LOG="$7"
: >"$LOG" 2>/dev/null
exec >>"$LOG" 2>&1
echo "[$(date '+%F %T')] 开始更新：pid=$PID kind=$KIND target=$TARGET"

# 1) 等旧进程退出（最多 2 分钟），否则替换正在运行的应用包可能损坏它
i=0
while [ "$i" -lt 240 ]; do
    kill -0 "$PID" 2>/dev/null || break
    sleep 0.5
    i=$((i + 1))
done

WORK="$(mktemp -d /tmp/bailan-update.XXXXXX)" || exit 1
if ! ditto -x -k "$ZIP" "$WORK"; then
    echo "解包失败：$ZIP"
    rm -rf "$WORK"
    exit 1
fi

if [ "$KIND" = "app" ]; then
    # 2) 整包替换
    NEWAPP="$(find "$WORK" -maxdepth 2 -type d -name '*.app' | head -1)"
    if [ -z "$NEWAPP" ] || [ ! -x "$NEWAPP/Contents/MacOS/bailan-relay" ]; then
        echo "更新包内没有可用的 .app"
        rm -rf "$WORK"
        exit 1
    fi
    xattr -dr com.apple.quarantine "$NEWAPP" 2>/dev/null
    codesign --force --deep --sign - "$NEWAPP" || echo "ad-hoc 重新签名失败，继续替换"
    STAGE="${TARGET}.new"
    OLD="${TARGET}.old"
    rm -rf "$STAGE" "$OLD"
    if ! ditto "$NEWAPP" "$STAGE"; then
        echo "暂存新版本失败"
        rm -rf "$WORK" "$STAGE"
        exit 1
    fi
    mv "$TARGET" "$OLD" 2>/dev/null
    if ! mv "$STAGE" "$TARGET"; then
        echo "替换失败，正在回滚"
        mv "$OLD" "$TARGET" 2>/dev/null
        rm -rf "$WORK"
        exit 1
    fi
    rm -rf "$OLD"
else
    # 3) 单文件替换（非应用包安装的调试场景）
    NEWBIN="$(find "$WORK" -maxdepth 4 -type f -name 'bailan-relay' | head -1)"
    if [ -z "$NEWBIN" ]; then
        echo "更新包内没有可用的可执行文件"
        rm -rf "$WORK"
        exit 1
    fi
    chmod 0755 "$NEWBIN"
    if ! mv -f "$NEWBIN" "$TARGET"; then
        echo "替换可执行文件失败"
        rm -rf "$WORK"
        exit 1
    fi
fi

xattr -dr com.apple.quarantine "$TARGET" 2>/dev/null
rm -rf "$WORK" "$ZIP"

# 4) 重启新版本：优先回到发起程序的用户（新进程会自行提权）
opened=0
if [ -n "$RUNAS" ] && [ "$RUNUID" != "0" ]; then
    launchctl asuser "$RUNUID" sudo -u "$RUNAS" -H open "$TARGET" && opened=1
fi
if [ "$opened" != "1" ] && [ "$KIND" = "app" ]; then
    open "$TARGET" && opened=1
fi
echo "[$(date '+%F %T')] 更新完成（重启=$opened）"
`
