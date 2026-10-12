package config

import "testing"

func TestModelInCatalog(t *testing.T) {
	t.Cleanup(func() { SetSelectableModels(nil) })

	// 目录未同步：一律放行，避免离线/旧后台误拦。
	SetSelectableModels(nil)
	if !ModelInCatalog("any-old-model") {
		t.Fatal("目录未同步时不应判定模型不存在")
	}

	// 目录同步后：后台已隐藏的模型判定为不存在。
	SetSelectableModels([]Model{{ID: "swe-2-high", Label: "SWE-2 High"}})
	if !CatalogSynced() {
		t.Fatal("目录应标记为已同步")
	}
	if !ModelInCatalog("SWE-2-High") {
		t.Fatal("目录内模型应存在（大小写不敏感）")
	}
	if ModelInCatalog("glm-5-2") {
		t.Fatal("目录外模型应判定为不存在")
	}
}
