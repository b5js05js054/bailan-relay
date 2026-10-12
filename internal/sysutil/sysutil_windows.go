//go:build windows

// Package sysutil 封装本地端用到的 Windows 系统调用与命令：管理员判断、
// 提权重启、隐藏窗口执行命令、刷新 DNS、进程查询与结束等。
package sysutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const createNoWindow = 0x08000000

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procIsUserAdmin  = shell32.NewProc("IsUserAnAdmin")
	procShellExecute = shell32.NewProc("ShellExecuteW")

	comdlg32            = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileName = comdlg32.NewProc("GetOpenFileNameW")

	ole32        = syscall.NewLazyDLL("ole32.dll")
	procCoInit   = ole32.NewProc("CoInitializeEx")
	procCoUninit = ole32.NewProc("CoUninitialize")
)

// IsAdmin 返回当前进程是否具有管理员权限。
func IsAdmin() bool {
	ret, _, _ := procIsUserAdmin.Call()
	return ret != 0
}

// Elevate 以管理员身份重新启动当前程序，成功发起返回 true。
// args 为要传给新进程的参数（不含程序本身）。
func Elevate(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(buildCmdline(args))
	cwd, _ := syscall.UTF16PtrFromString(workingDir())
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		uintptr(unsafe.Pointer(cwd)),
		1, // SW_SHOWNORMAL
	)
	return ret > 32
}

func workingDir() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return ""
}

// buildCmdline 用与 Windows 一致的规则拼接命令行参数。
func buildCmdline(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}

// RunHidden 以隐藏窗口执行命令，返回合并后的输出与退出码。
func RunHidden(name string, args ...string) (string, int, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return string(out), code, err
}

// FlushDNS 刷新系统 DNS 解析缓存。
func FlushDNS() {
	_, _, _ = RunHidden("ipconfig", "/flushdns")
}

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

// SetConsoleCleanup 注册控制台关闭/注销/关机/Ctrl+C 时的清理回调。
func SetConsoleCleanup(fn func()) {
	cb := syscall.NewCallback(func(ctrlType uint32) uintptr {
		fn()
		return 0 // FALSE：交给默认处理器继续终止流程
	})
	_, _, _ = procSetConsoleHandler.Call(cb, 1)
}

// IsProcessRunning 判断指定映像名的进程是否在运行。
func IsProcessRunning(imageName string) bool {
	out, _, _ := RunHidden("tasklist", "/FI", "IMAGENAME eq "+imageName)
	return strings.Contains(strings.ToLower(out), strings.ToLower(imageName))
}

// Taskkill 强制结束进程树。
func Taskkill(args ...string) (string, int, error) {
	return RunHidden("taskkill", append([]string{"/F"}, args...)...)
}

// StartDetached 以隐藏窗口方式在指定目录启动一个不阻塞的进程。
func StartDetached(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd.Start()
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

const (
	ofnHideReadonly  = 0x00000004
	ofnNoChangeDir   = 0x00000008
	ofnPathMustExist = 0x00000800
	ofnFileMustExist = 0x00001000
	ofnExplorer      = 0x00080000
)

func utf16Multi(parts ...string) []uint16 {
	var buf []uint16
	for _, p := range parts {
		buf = append(buf, utf16.Encode([]rune(p))...)
		buf = append(buf, 0)
	}
	return append(buf, 0)
}

// OpenExeDialog 弹出系统文件选择框（.exe），返回所选路径。取消返回错误。
func OpenExeDialog(current string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_, _, _ = procCoInit.Call(0, 0x2) // COINIT_APARTMENTTHREADED，best-effort
	defer procCoUninit.Call()

	buf := make([]uint16, 2048)
	if strings.TrimSpace(current) != "" {
		if enc := utf16.Encode([]rune(current)); len(enc) < len(buf) {
			copy(buf, enc)
		}
	}
	filter := utf16Multi("可执行文件 (*.exe)", "*.exe", "所有文件 (*.*)", "*.*")
	title, _ := syscall.UTF16PtrFromString("选择 Devin 程序")

	var initialDir *uint16
	if strings.TrimSpace(current) != "" {
		dir := filepath.Dir(current)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			initialDir, _ = syscall.UTF16PtrFromString(dir)
		}
	}

	ofn := openFileName{
		lStructSize:     uint32(unsafe.Sizeof(openFileName{})),
		lpstrFilter:     &filter[0],
		lpstrFile:       &buf[0],
		nMaxFile:        uint32(len(buf)),
		lpstrInitialDir: initialDir,
		lpstrTitle:      title,
		flags:           ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnHideReadonly | ofnNoChangeDir,
	}
	ret, _, _ := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		return "", errors.New("未选择文件")
	}
	return syscall.UTF16ToString(buf), nil
}
