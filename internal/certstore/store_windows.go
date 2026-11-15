//go:build windows

package certstore

import (
	"os"
	"syscall"
	"unsafe"

	"bailan-relay/internal/config"
)

const (
	certStoreProvSystemW                = 10
	certSystemStoreLocalMachine         = 0x20000
	certStoreOpenExistingFlag           = 0x4000
	certStoreReadOnlyFlag               = 0x8000
	x509AsnEncoding                     = 0x00000001
	pkcs7AsnEncoding                    = 0x00010000
	certEncoding                        = x509AsnEncoding | pkcs7AsnEncoding
	certFindSubjectStr                  = 0x00080007
	certStoreAddReplaceExisting         = 3
	cryptENotFound              uintptr = 0x80092004
)

var (
	crypt32             = syscall.NewLazyDLL("crypt32.dll")
	procCertOpenStore   = crypt32.NewProc("CertOpenStore")
	procCertFind        = crypt32.NewProc("CertFindCertificateInStore")
	procCertAdd         = crypt32.NewProc("CertAddEncodedCertificateToStore")
	procCertDelete      = crypt32.NewProc("CertDeleteCertificateFromStore")
	procCertClose       = crypt32.NewProc("CertCloseStore")
	procCertFreeContext = crypt32.NewProc("CertFreeCertificateContext")
)

func openRootStore(readonly bool) (syscall.Handle, error) {
	name, err := syscall.UTF16PtrFromString("ROOT")
	if err != nil {
		return 0, err
	}
	flags := uintptr(certSystemStoreLocalMachine | certStoreOpenExistingFlag)
	if readonly {
		flags |= certStoreReadOnlyFlag
	}
	handle, _, callErr := procCertOpenStore.Call(
		certStoreProvSystemW,
		0,
		0,
		flags,
		uintptr(unsafe.Pointer(name)),
	)
	if handle == 0 {
		if callErr != nil {
			return 0, callErr
		}
		return 0, syscall.EINVAL
	}
	return syscall.Handle(handle), nil
}

func closeRootStore(store syscall.Handle) {
	if store != 0 {
		_, _, _ = procCertClose.Call(uintptr(store), 0)
	}
}

func findRootCert(store syscall.Handle, cn string) uintptr {
	subject, err := syscall.UTF16PtrFromString(cn)
	if err != nil {
		return 0
	}
	ctx, _, _ := procCertFind.Call(
		uintptr(store),
		certEncoding,
		0,
		certFindSubjectStr,
		uintptr(unsafe.Pointer(subject)),
		0,
	)
	return ctx
}

func freeCertContext(ctx uintptr) {
	if ctx != 0 {
		_, _, _ = procCertFreeContext.Call(ctx)
	}
}

// IsInstalled 检查 CA 是否已装入本机受信任的根证书存储。
func IsInstalled() (bool, string) {
	store, err := openRootStore(true)
	if err != nil {
		return false, err.Error()
	}
	defer closeRootStore(store)
	ctx := findRootCert(store, config.CertCN())
	if ctx == 0 {
		return false, ""
	}
	freeCertContext(ctx)
	return true, ""
}

// Install 生成缺失的证书后，把 CA 写入本机受信任的根证书存储。
func Install() (bool, string) {
	if !FileExists() {
		if _, err := LoadOrCreate(); err != nil {
			return false, "证书自动生成失败：" + err.Error()
		}
	}
	der, err := os.ReadFile(config.CertFile())
	if err != nil {
		return false, err.Error()
	}
	store, err := openRootStore(false)
	if err != nil {
		return false, err.Error()
	}
	defer closeRootStore(store)
	ok, _, callErr := procCertAdd.Call(
		uintptr(store),
		certEncoding,
		uintptr(unsafe.Pointer(&der[0])),
		uintptr(len(der)),
		certStoreAddReplaceExisting,
		0,
	)
	if ok == 0 {
		if callErr != nil {
			return false, callErr.Error()
		}
		return false, "写入根证书存储失败"
	}
	return true, ""
}

// Uninstall 从本机受信任的根证书存储移除 CA。
func Uninstall() (bool, string) {
	store, err := openRootStore(false)
	if err != nil {
		return false, err.Error()
	}
	defer closeRootStore(store)
	ctx := findRootCert(store, config.CertCN())
	if ctx == 0 {
		return true, ""
	}
	// CertDeleteCertificateFromStore 会释放上下文，不能再 Free。
	ok, _, callErr := procCertDelete.Call(ctx)
	if ok == 0 {
		freeCertContext(ctx)
		if errno, isErrno := callErr.(syscall.Errno); isErrno && uintptr(errno) == cryptENotFound {
			return true, ""
		}
		if callErr != nil {
			return false, callErr.Error()
		}
		return false, "移除根证书失败"
	}
	return true, ""
}
