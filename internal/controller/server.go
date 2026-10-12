package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

// Handler 返回控制面 HTTP 处理器：静态 UI + /api/* 接口。
func (c *Controller) Handler(uiFS fs.FS) http.Handler {
	routes := c.routes()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		c.serveAPI(w, r, routes)
	})
	mux.HandleFunc("/userfile/", c.serveUserFile)
	mux.HandleFunc("/userbg/", c.serveUserBackground)
	mux.Handle("/", http.FileServer(http.FS(uiFS)))
	return mux
}

func (c *Controller) routes() map[string]func([]any) any {
	return map[string]func([]any) any{
		"get_bootstrap":           c.apiGetBootstrap,
		"client_config":           c.apiClientConfig,
		"get_status":              c.apiGetStatus,
		"refresh_status":          c.apiRefreshStatus,
		"get_logs":                c.apiGetLogs,
		"clear_log_buffer":        c.apiClearLogBuffer,
		"load_log_history":        c.apiLoadLogHistory,
		"set_model":               c.apiSetModel,
		"set_option":              c.apiSetOption,
		"save_config":             c.apiSaveConfig,
		"scan_processes":          c.apiScanProcesses,
		"browse_devin_executable": c.apiBrowseDevinExecutable,
		"test_relay":              c.apiTestRelay,
		"test_dns":                c.apiTestDNS,
		"start_proxy":             c.apiStartProxy,
		"stop_proxy":              c.apiStopProxy,
		"open_mitmweb":            c.apiOpenMitmweb,
		"open_url":                c.apiOpenURL,
		"apply_update":            c.apiApplyUpdate,
		"update_progress":         c.apiUpdateProgress,
		"install_certificate":     c.apiInstallCertificate,
		"uninstall_certificate":   c.apiUninstallCertificate,
		"relaunch_as_admin":       c.apiRelaunchAsAdmin,
		"quit":                    c.apiQuit,
		"heartbeat":               c.apiHeartbeat,
		"ping_relay":              c.apiPingRelay,
		"closing":                 c.apiClosing,
		"activate_license":        c.apiActivateLicense,
		"license_status":          c.apiLicenseStatus,
		"deactivate_license":      c.apiDeactivateLicense,
		"redeem_topup":            c.apiRedeemTopup,
		"get_usage":               c.apiGetUsage,
		"user_session":            c.apiUserSession,
		"user_login":              c.apiUserLogin,
		"user_register":           c.apiUserRegister,
		"user_set_background":     c.apiUserSetBackground,
		"user_clear_background":   c.apiUserClearBackground,
		"user_logout":             c.apiUserLogout,
		"user_upload_progress":    c.apiUserUploadProgress,
		"pick_image":              c.apiPickImage,
	}
}

func (c *Controller) serveAPI(w http.ResponseWriter, r *http.Request, routes map[string]func([]any) any) {
	method := strings.TrimPrefix(r.URL.Path, "/api/")
	handler, ok := routes[method]
	w.Header().Set("content-type", "application/json; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "API 不存在：" + method})
		return
	}

	var args []any
	if body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)); err == nil && len(body) > 0 {
		_ = json.Unmarshal(body, &args)
	}

	defer func() {
		if rec := recover(); rec != nil {
			c.log.Log(fmt.Sprintf("[Manager] API %s 异常：%v", method, rec))
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": fmt.Sprintf("%v", rec)})
		}
	}()

	result := handler(args)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		c.log.Log("[Manager] API 编码失败：" + err.Error())
	}
}
