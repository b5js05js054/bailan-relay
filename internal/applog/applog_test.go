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
}
