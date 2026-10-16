// BaiLan Devin 中继客户端（Go 版）
//
// 与旧版最大的区别：不再使用 mitmproxy 中间人。改为
//  1. 把目标域名写入 Windows hosts，指向 127.0.0.1:443；
//  2. 本进程原生启动 HTTPS 服务终止 TLS（自带 CA，按 SNI 动态签发叶子证书）；
//  3. 直接完成屏蔽遥测 / 本地空应答 / 转发中继 / 透传源站 + 模型替换。
//
// 界面沿用原 HTML/CSS/JS，通过本地控制服务(HTTP)提供与 pywebview 一致的 /api/* 接口。
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bailan-relay/internal/account"
	"bailan-relay/internal/applog"
	"bailan-relay/internal/certstore"
	"bailan-relay/internal/config"
	"bailan-relay/internal/controller"
	"bailan-relay/internal/hostsfile"
	"bailan-relay/internal/license"
	"bailan-relay/internal/netdns"
	"bailan-relay/internal/proxybypass"
	"bailan-relay/internal/proxysrv"
	"bailan-relay/internal/sysutil"
)

//go:embed all:ui
var uiEmbed embed.FS

// 控制面首选端口，被占用则退回随机端口。
const preferredControlPort = 5099

func main() {
	// 非管理员时先尝试提权重启（写 hosts / 装证书都需要管理员）。
	// 设置 BAILAN_NO_ELEVATE=1 可跳过提权（仅用于调试）。
	if !sysutil.IsAdmin() && os.Getenv("BAILAN_NO_ELEVATE") != "1" {
		if sysutil.Elevate(os.Args[1:]) {
			return
		}
		fmt.Fprintln(os.Stderr, "提示：未获得管理员权限，hosts 劫持与证书安装可能失败。")
	}

	logger := applog.New()
	logger.Log("[Manager] 本地拦截组件已就绪（Go 原生，无中间人）")

	// 自愈：清理上一轮异常退出/被强杀时残留在 hosts 里的劫持段（无残留则无操作）。
	_, _ = hostsfile.Restore()

	// 首次运行释放默认配置。
	cfg := config.Load()

	// 客户版：重写 config.json，抹掉磁盘上可能残留的核心敏感字段。
	if config.IsCustomer() {
		config.Save(nil)
	}

	ca, err := certstore.LoadOrCreate()
	if err != nil {
		fatal("初始化本地根证书失败：" + err.Error())
	}
	// 打开软件即自动信任根证书；退出时由 restoreSystem 卸载。
	if sysutil.IsAdmin() {
		go func() {
			_, msg := certstore.EnsureInstalled()
			logger.Log("[Manager] " + msg)
		}()
	}

	resolver := netdns.New(func() []string { return config.Load().EffectiveDNSServers() })
	proxy := proxysrv.New(ca, resolver, logger)
	proxy.UpdateConfig(cfg)

	ctrl := controller.New(logger, ca, resolver, proxy)

	// 卡密授权：激活/心跳续期，并把令牌接入转发流程。
	licenseMgr := license.New(cfg.LicenseAPIURL, config.LicenseStatePath(), resolver, logger)
	ctrl.SetLicenseManager(licenseMgr)
	// 账号会话：未登录不可使用；令牌同时用于卡密绑定与模型目录过滤。
	ctrl.SetAccountManager(account.New(func() string { return config.Load().LicenseAPIURL }, config.UserStatePath(), resolver.DialContext(10*time.Second)))
	licenseMgr.Start()

	// 控制面 HTTP 服务：静态 UI + /api/*。
	uiFS, err := fs.Sub(uiEmbed, "ui")
	if err != nil {
		fatal("加载界面资源失败：" + err.Error())
	}
	listener, addr := listenControl()
	server := &http.Server{Handler: ctrl.Handler(uiFS)}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Log("[Manager] 控制服务异常：" + err.Error())
		}
	}()

	url := "http://" + addr + "/"
	logger.Log("[Manager] 控制台地址：" + url)

	// restoreSystem 退出收尾：恢复 hosts、代理绕过列表，并卸载根证书。
	restoreSystem := func() {
		_, _ = hostsfile.Restore()
		_, _ = proxybypass.Restore(config.Load().HijackDomains())
		if sysutil.IsAdmin() {
			logger.Log("[Manager] " + certstore.AutoUninstall())
		}
	}
	cleanup := func() {
		licenseMgr.Stop()
		ctrl.Shutdown() // 内部触发 onExit -> restoreSystem（只执行一次）
	}
	ctrl.SetOnExit(restoreSystem)
	sysutil.SetConsoleCleanup(cleanup)

	if os.Getenv("BAILAN_NO_BROWSER") != "1" {
		if openAppWindow(url) {
			// 内嵌窗口已关闭：收尾后退出。
			fmt.Println("控制台窗口已关闭，正在退出并恢复 hosts…")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = server.Shutdown(shutdownCtx)
			cancel()
			cleanup()
			return
		}
		// 内嵌窗口不可用：退回浏览器打开。
		openBrowser(url)
	}
	fmt.Println("白岚 Devin 中继已启动：" + url)
	fmt.Println("关闭此窗口即可退出（会自动恢复 hosts）。")

	// 等待退出信号，收尾恢复 hosts。
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("正在退出并恢复 hosts…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	cleanup()
}

func listenControl() (net.Listener, string) {
	addr := fmt.Sprintf("127.0.0.1:%d", preferredControlPort)
	if ln, err := net.Listen("tcp", addr); err == nil {
		return ln, addr
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatal("无法启动本地控制服务：" + err.Error())
	}
	return ln, ln.Addr().String()
}
