package controller

import (
	"testing"

	"bailan-relay/internal/config"
)

// 适配状态只给完整版看；客户版脱敏时必须删掉，避免给客户看到开发信息。
func TestExportStatusHidesAdaptationForCustomer(t *testing.T) {
	orig := config.Edition
	defer func() { config.Edition = orig }()

	build := func() map[string]any {
		return map[string]any{
			"adaptation": map[string]any{"devin_found": true, "mismatch_count": 0},
			"relay_ok":   true,
		}
	}

	config.Edition = "full"
	if _, ok := exportStatusForEdition(build())["adaptation"]; !ok {
		t.Error("完整版应保留 adaptation")
	}

	config.Edition = "customer"
	if _, ok := exportStatusForEdition(build())["adaptation"]; ok {
		t.Error("客户版应删掉 adaptation")
	}
}
