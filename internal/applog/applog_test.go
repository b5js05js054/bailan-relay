package applog

import "testing"

func TestSanitizeForCustomerDropsAllPassthroughVariants(t *testing.T) {
	lines := []string{
		"[PASSTHROUGH] 目标服务/GetUserStatus",
		"[PASSTHROUGH-401] 目标服务/GetProfileData · ct:application/json · 121B",
		"[PASSTHROUGH-ERR] 目标服务/GetCliTeamSettings · context canceled",
		"[passthrough-502] target/path",
	}
	for _, line := range lines {
		if got := sanitizeForCustomer(line); got != "" {
			t.Fatalf("passthrough detail leaked: %q", got)
		}
	}

	const safe = "[认证加速] 团队配置读取完成"
	if got := sanitizeForCustomer(safe); got != safe {
		t.Fatalf("unrelated customer log changed: %q", got)
	}
	if got := sanitizeForCustomer("[Manager] 请求里的会话编号对不上已记住的模型: abc"); got != "" {
		t.Fatalf("session debug leaked: %q", got)
	}
	if got := screenLog("[会话等待] 模型：MiMo"); got != "正在发送：MiMo" {
		t.Fatalf("chat log was not simplified: %q", got)
	}
	if got := screenLog("[会话等待] 模型：GPT-6.1 Terra · 来自 Devin · 原文：swe-1-6-slow · 按会话 · 思考：low · 上下文：1000000 · 请求：25059B · 原始上传 · 正在等待中继响应头"); got != "正在发送：GPT-6.1 Terra" {
		t.Fatalf("session wait leaked internals: %q", got)
	}
	if got := screenLog("[会话完成] 模型：GPT-6.1 Terra · 首包：2.150s · 总耗时：9.150s · 1259B/7块"); got != "对话已完成：GPT-6.1 Terra" {
		t.Fatalf("session done leaked timings: %q", got)
	}
	if got := screenLog("[会话响应头] 模型：GPT-6.1 Terra · 状态码：200 · 等待：0.650s"); got != "" {
		t.Fatalf("response header leaked: %q", got)
	}
	if got := screenLog("[账单改写] 成功 billing_error 空→空，plan free→pro，has_credits true→true"); got != "" {
		t.Fatalf("billing rewrite leaked: %q", got)
	}
	if got := screenLog("[账单流量] GET 目标服务/api/org/billing/status"); got != "" {
		t.Fatalf("billing traffic leaked: %q", got)
	}
	if got := screenLog("[Manager] 控制台地址：http://127.0.0.1:5099/"); got != "" {
		t.Fatalf("console address leaked: %q", got)
	}
	if got := screenLog("[Manager] 拦截已停止，hosts 已恢复，根证书已从系统信任库卸载，Devin 已关闭"); got != "拦截已停止，Devin 已关闭" {
		t.Fatalf("stop line was not simplified: %q", got)
	}
	if got := screenLog("[拦截成功] hosts 劫持已开启，Devin 已自动重启"); got != "拦截已开启，Devin 已重启" {
		t.Fatalf("start line was not simplified: %q", got)
	}
	if got := screenLog("18:35:12 | [会话等待] 模型：GPT-6.1 Terra · 来自 Devin · 原文：swe-1-6-slow"); got != "18:35:12 | 正在发送：GPT-6.1 Terra" {
		t.Fatalf("timestamped session line leaked: %q", got)
	}
	if got := screenLog("[Manager] 模型选项 mimo-v2.6-pro: 思考强度=\"low\" 上下文=1000000"); got != "" {
		t.Fatalf("model option leaked: %q", got)
	}
	if got := screenLog("[Manager] 会话 devin-acp/devin-cli/clarity-pawpaw 使用模型：GPT-6.1 Terra (gpt-6.1-terra)"); got != "" {
		t.Fatalf("session route leaked: %q", got)
	}
	if got := screenLog("[模型目录] GetUserStatus 已写入 Devin 列表 · 10 个"); got != "" {
		t.Fatalf("catalog debug leaked: %q", got)
	}
	if got := screenLog("[Manager] 已写入 hosts：目标服务"); got != "" {
		t.Fatalf("hosts debug leaked: %q", got)
	}
}
