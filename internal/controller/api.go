package controller

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"bailan-relay/internal/certstore"
	"bailan-relay/internal/config"
	"bailan-relay/internal/devin"
	"bailan-relay/internal/hostsfile"
	"bailan-relay/internal/license"
	"bailan-relay/internal/proxybypass"
	"bailan-relay/internal/sysutil"
	"bailan-relay/internal/tunbypass"
)

func (c *Controller) apiGetBootstrap([]any) any {
	bundle := c.clientBundle(false)
	return map[string]any{
		"app_title":      config.AppTitle,
		"ui_version":     config.UIVersion,
		"app_version":    config.AppVersion,
		"edition":        config.Edition,
		"platform":       runtime.GOOS,
		"admin":          sysutil.IsAdmin(),
		"config_path":    config.ConfigPath(),
		"cert_path":      certstore.CertPath(),
		"models":         bundle.models,
		"catalog_synced": config.CatalogSynced(),
		"announcement":   bundle.announcement,
		"update":         buildUpdateView(bundle.update),
		"config":         c.publicConfig(config.Load()),
		"status":         c.statusSnapshot(),
		"license":        c.licenseStatus(),
		"account":        accountView(c.account.Session()),
	}
}

// apiActivateLicense 用卡密激活当前设备。
func (c *Controller) apiActivateLicense(args []any) any {
	if c.license == nil {
		return map[string]any{"ok": false, "message": "授权模块未初始化"}
	}
	code := strings.TrimSpace(argString(args, 0))
	ok, msg := c.license.Activate(code)
	tone := "success"
	if !ok {
		tone = "error"
	}
	c.setNotice(msg, tone, nil, false)
	go c.refreshStatusData(false)
	return map[string]any{"ok": ok, "message": msg, "license": c.licenseStatus()}
}

// apiLicenseStatus 返回当前授权状态。
func (c *Controller) apiLicenseStatus([]any) any {
	return map[string]any{"ok": true, "license": c.licenseStatus()}
}

// apiRedeemTopup 用充值码给当前卡密充值余额（高级模型按倍率从余额扣费）。
func (c *Controller) apiRedeemTopup(args []any) any {
	if c.license == nil {
		return map[string]any{"ok": false, "message": "授权模块未初始化"}
	}
	ok, msg := c.license.Redeem(strings.TrimSpace(argString(args, 0)))
	tone := "success"
	if !ok {
		tone = "error"
	}
	c.setNotice(msg, tone, nil, false)
	return map[string]any{"ok": ok, "message": msg, "license": c.licenseStatus()}
}

// apiGetUsage 查询当前卡密的用量与扣费（参数：days, page, page_size）；后台响应放在 data 中原样透传。
// 失败时 reason 区分 not_activated / unsupported / error，便于界面给出对应的空状态。
func (c *Controller) apiGetUsage(args []any) any {
	if c.license == nil {
		return map[string]any{"ok": false, "reason": "error", "message": "授权模块未初始化"}
	}
	data, err := c.license.Usage(argInt(args, 0, 30), argInt(args, 1, 1), argInt(args, 2, 20))
	if err != nil {
		reason := "error"
		switch {
		case errors.Is(err, license.ErrNotActivated):
			reason = "not_activated"
		case errors.Is(err, license.ErrUsageUnsupported):
			reason = "unsupported"
		}
		return map[string]any{"ok": false, "reason": reason, "message": err.Error()}
	}
	return map[string]any{"ok": true, "data": data}
}

// apiDeactivateLicense 清除本地授权。
func (c *Controller) apiDeactivateLicense([]any) any {
	if c.license == nil {
		return map[string]any{"ok": false, "message": "授权模块未初始化"}
	}
	c.license.Deactivate()
	c.setNotice("已清除本地卡密授权", "success", nil, false)
	go c.refreshStatusData(false)
	return map[string]any{"ok": true, "message": "已清除本地卡密授权", "license": c.licenseStatus()}
}

func (c *Controller) apiGetStatus([]any) any { return c.statusSnapshot() }

// apiClientConfig 返回后台发布的客户端配置（模型目录 + 公告 + 更新），供界面周期刷新。
// 界面每 15s 轮询一次，这里强制拉新以保证近实时；失败自动回退缓存（clientBundle 内处理）。
func (c *Controller) apiClientConfig([]any) any {
	bundle := c.clientBundle(true)
	return map[string]any{
		"ok":             true,
		"models":         bundle.models,
		"catalog_synced": config.CatalogSynced(),
		"announcement":   bundle.announcement,
		"update":         buildUpdateView(bundle.update),
	}
}

func (c *Controller) apiRefreshStatus([]any) any { return c.refreshStatusData(true) }

func (c *Controller) apiGetLogs(args []any) any {
	after := argInt(args, 0, 0)
	limit := argInt(args, 1, 600)
	return c.log.GetLogs(after, limit)
}

func (c *Controller) apiClearLogBuffer([]any) any { return c.log.Clear() }

func (c *Controller) apiLoadLogHistory([]any) any { return c.log.LoadHistory() }

func (c *Controller) apiSetModel(args []any) any {
	normalized := config.NormalizeModelID(argString(args, 0))
	cfg := config.Save(func(x *config.Config) { x.OverrideModel = normalized })
	c.proxy.SetModel(normalized)
	display := config.ModelLabel(normalized)
	c.log.Log(fmt.Sprintf("[Manager] 指定模型: %s (%s)", display, normalized))
	go c.refreshStatusData(false)
	return map[string]any{"ok": true, "message": "模型路由已切换为 " + display, "config": c.publicConfig(cfg)}
}

func (c *Controller) apiSetOption(args []any) any {
	name := argString(args, 0)
	if name == "show_mitmweb" {
		cfg := config.Save(func(x *config.Config) { x.ShowMitmweb = false })
		return map[string]any{"ok": true, "config": c.publicConfig(cfg)}
	}
	if name != "restart_devin_on_start" {
		return map[string]any{"ok": false, "message": "不支持的启动选项"}
	}
	enabled := argBool(args, 1, false)
	cfg := config.Save(func(x *config.Config) { x.RestartDevinOnStart = enabled })
	state := "关闭"
	if enabled {
		state = "开启"
	}
	c.log.Log("[Manager] 启动后重启 Devin：" + state)
	return map[string]any{"ok": true, "config": c.publicConfig(cfg)}
}

func (c *Controller) apiSaveConfig(args []any) any {
	payload := argMap(args, 0)
	if payload == nil {
		return map[string]any{"ok": false, "message": "配置数据格式无效"}
	}
	executable := strings.TrimSpace(mapString(payload, "devin_executable"))
	found := devin.DiscoverExecutable(executable)
	finalExe := executable
	if found != "" {
		finalExe = found
	}
	domains := config.NormalizeTargetDomains(mapStringSlice(payload, "target_domains"))
	var paths []string
	for _, p := range mapStringSlice(payload, "target_paths") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		paths = append([]string(nil), config.DefaultTargetPaths...)
	}
	var stub []string
	for _, p := range mapStringSlice(payload, "stub_paths") {
		if p = strings.TrimSpace(p); p != "" {
			stub = append(stub, p)
		}
	}
	cfg := config.Save(func(x *config.Config) {
		if config.IsCustomer() {
			// 客户版：只接受 Devin 程序 / 模型 / 重启选项，核心字段保持内置默认。
			x.InterceptProcess = devin.DiscoverInterceptProcesses(finalExe)
			x.DevinExecutable = finalExe
			x.OverrideModel = config.NormalizeModelID(mapString(payload, "override_model"))
			x.RestartDevinOnStart = mapBool(payload, "restart_devin_on_start", true)
			return
		}
		x.RelayServerURL = strings.TrimSpace(mapString(payload, "relay_server_url"))
		x.InterceptProcess = devin.DiscoverInterceptProcesses(finalExe)
		x.DevinExecutable = finalExe
		x.RelayAuthToken = strings.TrimSpace(mapString(payload, "relay_auth_token"))
		x.DNSServers = config.NormalizeDNSServers(mapStringSlice(payload, "dns_servers"))
		x.StubPaths = stub
		x.TargetDomains = domains
		x.TargetPaths = paths
		x.OverrideModel = config.NormalizeModelID(mapString(payload, "override_model"))
		x.RestartDevinOnStart = mapBool(payload, "restart_devin_on_start", true)
		if v := strings.TrimSpace(mapString(payload, "license_api_url")); v != "" {
			x.LicenseAPIURL = v
		}
	})
	c.proxy.UpdateConfig(cfg)
	if c.license != nil {
		c.license.SetAPIURL(cfg.LicenseAPIURL)
	}
	c.log.Log("[Manager] 配置已保存")
	c.setNotice("配置已保存 · "+time.Now().Format("15:04:05"), "success", nil, false)
	go c.refreshStatusData(false)
	return map[string]any{"ok": true, "message": "配置已保存", "config": c.publicConfig(cfg)}
}

func (c *Controller) apiScanProcesses(args []any) any {
	payload := argMap(args, 0)
	executable := strings.TrimSpace(mapString(payload, "devin_executable"))
	found := devin.DiscoverExecutable(executable)
	if found == "" {
		return map[string]any{"ok": false, "message": "没有找到 Devin 程序，请手动选择程序路径"}
	}
	merged := devin.DiscoverInterceptProcesses(found)
	cfg := config.Save(func(x *config.Config) {
		x.InterceptProcess = merged
		x.DevinExecutable = found
	})
	c.log.Log("[Manager] 已自动找到 Devin：" + found)
	return map[string]any{
		"ok":         true,
		"message":    "已找到 " + found,
		"executable": found,
		"config":     c.publicConfig(cfg),
	}
}

func (c *Controller) apiBrowseDevinExecutable(args []any) any {
	current := argString(args, 0)
	path, err := sysutil.OpenExeDialog(current)
	if err != nil {
		return map[string]any{"ok": false, "message": err.Error()}
	}
	return map[string]any{"ok": true, "path": path}
}

func (c *Controller) apiTestRelay([]any) any {
	if config.IsCustomer() {
		c.statusMu.Lock()
		c.status["relay_checking"] = true
		c.statusMu.Unlock()
		ok, _ := c.checkRelayServer()
		c.statusMu.Lock()
		c.status["relay_ok"] = ok
		c.status["relay_detail"] = detailForCustomer(ok)
		c.status["relay_checking"] = false
		c.status["updated_at"] = time.Now().Format("15:04:05")
		c.statusMu.Unlock()
		msg := "服务连接正常"
		tone := "success"
		if !ok {
			msg = "服务暂时不可用，请稍后重试"
			tone = "error"
		}
		c.setNotice(msg, tone, nil, false)
		return map[string]any{"ok": ok, "detail": detailForCustomer(ok), "url": ""}
	}
	c.log.Log("[Manager] 正在测试中继连接…")
	c.statusMu.Lock()
	c.status["relay_checking"] = true
	c.statusMu.Unlock()
	ok, detail := c.checkRelayServer()
	relayURL := config.Load().RelayServerURL
	if relayURL == "" {
		relayURL = "未配置"
	}
	c.statusMu.Lock()
	c.status["relay_ok"] = ok
	c.status["relay_detail"] = detail
	c.status["relay_checking"] = false
	c.status["updated_at"] = time.Now().Format("15:04:05")
	c.statusMu.Unlock()
	result := "失败"
	tone := "error"
	if ok {
		result = "成功"
		tone = "success"
	}
	message := fmt.Sprintf("中继测试%s · %s · %s", result, relayURL, detail)
	c.setNotice(message, tone, nil, false)
	c.log.Log(fmt.Sprintf("[Manager] 中继测试%s: %s", result, detail))
	return map[string]any{"ok": ok, "detail": detail, "url": relayURL}
}

func (c *Controller) apiTestDNS(args []any) any {
	if config.IsCustomer() {
		c.setNotice("网络检测完成", "success", nil, false)
		return map[string]any{"ok": true, "message": "网络检测完成", "results": []any{}, "servers": []any{}}
	}
	payload := argMap(args, 0)
	var domains []string
	if payload != nil {
		if raw := mapStringSlice(payload, "target_domains"); len(raw) > 0 {
			domains = config.NormalizeTargetDomains(raw)
		}
	}
	if len(domains) == 0 {
		domains = config.Load().HijackDomains()
	}
	c.log.Log("[Manager] 正在检测目标域名解析…")
	var results []map[string]any
	var failed []string
	for _, d := range domains {
		name := config.SanitizeDomain(d)
		if name == "" {
			continue
		}
		ip, via := c.resolver.Resolve(name)
		results = append(results, map[string]any{"domain": name, "ok": ip != "", "ip": ip, "via": via})
		if ip == "" {
			failed = append(failed, name)
			c.log.Log("[DNS] " + name + " 解析失败")
		} else {
			c.log.Log(fmt.Sprintf("[DNS] %s → %s (%s)", name, ip, via))
		}
	}
	ok := len(failed) == 0
	var message string
	if ok {
		message = fmt.Sprintf("全部 %d 个域名均可解析真实 IP", len(results))
		c.setNotice(message, "success", nil, false)
	} else {
		message = "无法解析：" + strings.Join(failed, "、") + "（劫持后会回环，模型可能加载不出来）"
		c.setNotice(message, "error", nil, false)
	}
	return map[string]any{"ok": ok, "message": message, "results": results, "servers": config.Load().EffectiveDNSServers()}
}

func (c *Controller) apiStartProxy([]any) any {
	c.statusMu.RLock()
	state := c.proxyState
	c.statusMu.RUnlock()
	if state == "starting" || state == "stopping" {
		return map[string]any{"ok": false, "message": "代理操作正在进行"}
	}
	if c.proxy.Running() {
		t := true
		c.setProxyState("running", "应用级拦截正在运行", &t)
		return map[string]any{"ok": false, "message": "代理已经在运行"}
	}
	if !sysutil.IsAdmin() {
		return map[string]any{"ok": false, "message": "修改 hosts 需要管理员权限，请先以管理员身份启动"}
	}
	// 强制更新闸门：后台发布 mandatory 更新且当前版本过低时拒绝启动。
	if upd := c.updateView(false); upd != nil && upd.Mandatory && upd.HasUpdate {
		msg := fmt.Sprintf("发现新版本 %s（当前 %s），请先完成更新后再启动", upd.Version, upd.Current)
		c.setNotice(msg, "error", nil, false)
		return map[string]any{"ok": false, "message": msg, "need_update": true, "update": upd}
	}
	// 账号闸门：未登录不可使用。
	if c.userToken() == "" {
		msg := "请先登录账号"
		c.setNotice(msg, "error", nil, false)
		return map[string]any{"ok": false, "message": msg, "need_login": true}
	}
	// 卡密授权闸门：未激活或已失效则拒绝启动。
	if c.license != nil {
		if ok, msg := c.license.EnsureFresh(); !ok {
			notice := "请先激活卡密：" + msg
			c.setNotice(notice, "error", nil, false)
			return map[string]any{"ok": false, "message": notice, "need_license": true, "license": c.licenseStatus()}
		}
	}
	c.setProxyState("starting", "正在启动本地代理", boolPtr(false))
	c.log.Log("[Manager] 正在启动本地代理…")
	go c.startProxyTask()
	return map[string]any{"ok": true, "accepted": true, "message": "正在启动本地代理"}
}

func (c *Controller) startProxyTask() {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	defer func() { go c.refreshStatusData(true) }()

	cfg := config.Load()
	c.proxy.UpdateConfig(cfg)
	// 停止拦截时会卸载根证书，重新启动前补装，否则 TLS 握手全部失败。
	certOK, certMsg := certstore.EnsureInstalled()
	if !certOK {
		c.failStart(certMsg)
		return
	}
	c.log.Log("[Manager] " + certMsg)
	c.log.Log("[Manager] 拦截方式: hosts → 127.0.0.1:443（原生 Go，无中间人）")
	c.log.Log("[Manager] 目标域名: " + strings.Join(cfg.HijackDomains(), "、"))

	if err := c.proxy.Start(); err != nil {
		msg := "启动失败：" + err.Error()
		c.setProxyState("error", msg, boolPtr(false))
		action := "log"
		c.setNotice(msg, "error", &action, false)
		c.log.Log("[Manager] " + msg)
		return
	}

	ok, hostsMsg := hostsfile.Apply(cfg.HijackDomains())
	c.log.Log("[Manager] " + hostsMsg)
	if !ok {
		c.proxy.Stop()
		c.failStart(hostsMsg)
		return
	}

	// 系统代理兼容：把目标域名写进 WinINET 绕过列表，让遵守系统代理的
	// 进程对这些域名恢复直连（否则 hosts/host-resolver-rules 都摸不到）。
	if _, err := proxybypass.Apply(cfg.HijackDomains()); err != nil {
		c.log.Log("[Manager] 写入系统代理绕过列表失败（若开启系统代理，拦截将不生效）：" + err.Error())
	} else {
		switch {
		case proxybypass.PACMode():
			c.log.Log("[Manager] 检测到 PAC 自动脚本代理：绕过列表不生效，请在代理工具中把目标域名设为直连")
		case proxybypass.Enabled():
			c.log.Log("[Manager] 系统代理已开启：目标域名已加入代理绕过列表，拦截继续生效")
		}
	}
	if st := tunbypass.Status(); st != "" {
		c.log.Log("[Manager] " + st)
	}

	restarted := false
	if cfg.RestartDevinOnStart {
		rok, rmsg := devin.Restart(cfg.DevinExecutable)
		c.log.Log("[Manager] " + rmsg)
		if !rok {
			c.proxy.Stop()
			_, _ = hostsfile.Restore()
			c.failStart(rmsg)
			return
		}
		restarted = true
		c.log.Log("[拦截成功] hosts 劫持已开启，Devin 已自动重启")
	} else {
		c.log.Log("[拦截成功] hosts 劫持已开启（未重启 Devin）")
	}

	restartNote := "未重启 Devin"
	if restarted {
		restartNote = "Devin 已自动重启"
	}
	c.setProxyState("running", "拦截已开启，"+restartNote, boolPtr(true))
	action := "log"
	c.setNotice("拦截已开启 · "+restartNote+" · 正在显示实时日志", "success", &action, true)
}

func (c *Controller) failStart(reason string) {
	msg := "启动失败：" + reason
	c.setProxyState("error", msg, boolPtr(false))
	action := "log"
	c.setNotice(msg, "error", &action, false)
	c.log.Log("[Manager] " + msg)
}

func (c *Controller) apiStopProxy([]any) any {
	c.statusMu.RLock()
	state := c.proxyState
	c.statusMu.RUnlock()
	if state == "stopping" {
		return map[string]any{"ok": false, "message": "代理正在停止"}
	}
	c.setProxyState("stopping", "正在停止拦截并关闭 Devin", nil)
	c.log.Log("[Manager] 正在停止拦截并关闭 Devin…")
	go c.stopProxyTask()
	return map[string]any{"ok": true, "accepted": true, "message": "正在停止拦截并关闭 Devin"}
}

func (c *Controller) stopProxyTask() {
	c.opMu.Lock()
	defer c.opMu.Unlock()

	devinClosed, devinMessage := devin.Stop(config.Load().DevinExecutable)
	c.log.Log("[Manager] " + devinMessage)
	c.proxy.Stop()
	ok, hostsMsg := hostsfile.Restore()
	if _, err := proxybypass.Restore(config.Load().HijackDomains()); err != nil {
		c.log.Log("[Manager] 移除代理绕过条目失败：" + err.Error())
	}
	certMsg := certstore.AutoUninstall()
	c.log.Log("[Manager] " + certMsg)
	message := "拦截已停止，hosts 已恢复，" + certMsg + "，" + devinMessage
	if !ok {
		message = "拦截已停止，但" + hostsMsg + "；" + certMsg + "；" + devinMessage
	}
	tone := "success"
	if !devinClosed {
		message += "；请检查 Devin 进程"
		tone = "error"
	}
	c.setProxyState("idle", message, boolPtr(false))
	c.setNotice(message, tone, nil, false)
	c.log.Log("[Manager] " + message)
	go c.refreshStatusData(true)
}

func (c *Controller) apiOpenMitmweb([]any) any {
	return map[string]any{"ok": false, "message": "当前版本已无代理页面"}
}

func (c *Controller) apiOpenURL(args []any) any {
	raw := strings.TrimSpace(argString(args, 0))
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") {
		return map[string]any{"ok": false, "message": "仅支持打开网页链接"}
	}
	if err := sysutil.OpenURL(raw); err != nil {
		return map[string]any{"ok": false, "message": "无法打开链接：" + err.Error()}
	}
	return map[string]any{"ok": true, "message": "已在浏览器打开"}
}

func (c *Controller) apiInstallCertificate([]any) any {
	if !sysutil.IsAdmin() {
		return map[string]any{"ok": false, "message": "安装证书需要管理员权限"}
	}
	ok, msg := certstore.Install()
	result := "成功"
	tone := "success"
	notice := "证书已安装到系统根证书存储"
	if !ok {
		result = "失败"
		tone = "error"
		notice = "证书安装失败：" + msg
	}
	c.log.Log(fmt.Sprintf("[Manager] 证书安装%s: %s", result, strings.TrimSpace(msg)))
	c.setNotice(notice, tone, nil, false)
	c.refreshStatusData(true)
	message := notice
	if ok && strings.TrimSpace(msg) != "" {
		message = notice
	} else if !ok {
		message = msg
	}
	return map[string]any{"ok": ok, "message": message}
}

func (c *Controller) apiUninstallCertificate([]any) any {
	if !sysutil.IsAdmin() {
		return map[string]any{"ok": false, "message": "卸载证书需要管理员权限"}
	}
	ok, msg := certstore.Uninstall()
	result := "成功"
	tone := "success"
	notice := "证书已从系统根证书存储卸载"
	if !ok {
		result = "失败"
		tone = "error"
		notice = "证书卸载失败：" + msg
	}
	c.log.Log(fmt.Sprintf("[Manager] 证书卸载%s: %s", result, strings.TrimSpace(msg)))
	c.setNotice(notice, tone, nil, false)
	c.refreshStatusData(true)
	message := notice
	if !ok {
		message = msg
	}
	return map[string]any{"ok": ok, "message": message}
}

func (c *Controller) apiRelaunchAsAdmin([]any) any {
	if sysutil.IsAdmin() {
		return map[string]any{"ok": true, "message": "当前已拥有管理员权限"}
	}
	if !sysutil.Elevate(os.Args[1:]) {
		return map[string]any{"ok": false, "message": "管理员授权已取消或启动失败"}
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		c.Shutdown()
		os.Exit(0)
	}()
	return map[string]any{"ok": true, "message": "正在以管理员身份重新启动"}
}

func (c *Controller) apiQuit([]any) any {
	c.log.Log("[Manager] 收到退出请求，正在恢复 hosts 并退出…")
	go func() {
		time.Sleep(300 * time.Millisecond)
		c.Shutdown() // 停止拦截 + 触发 onExit 恢复 hosts
		os.Exit(0)
	}()
	return map[string]any{"ok": true, "message": "正在退出并恢复 hosts…"}
}

// apiHeartbeat 由活动页面周期性调用，用于取消「关闭窗口即退出」的待执行计时器。
func (c *Controller) apiHeartbeat([]any) any {
	c.cancelPendingClose()
	return map[string]any{"ok": true}
}

// apiClosing 由前端在窗口关闭(pagehide)时通过 sendBeacon 调用。
// 仅客户版生效：排一个短延时退出，若窗口只是刷新则新页面的心跳会取消它。
func (c *Controller) apiClosing([]any) any {
	if !config.IsCustomer() {
		return map[string]any{"ok": true}
	}
	c.closeMu.Lock()
	if c.closeTimer == nil {
		c.closeTimer = time.AfterFunc(3*time.Second, func() {
			c.log.Log("[Manager] 窗口已关闭，正在恢复 hosts 并退出…")
			c.Shutdown()
			os.Exit(0)
		})
	}
	c.closeMu.Unlock()
	return map[string]any{"ok": true}
}

func boolPtr(b bool) *bool { return &b }
