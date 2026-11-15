package config

import (
	"context"
	"testing"
	"time"
)

func TestValidModelOptionAcceptsDevinVariant(t *testing.T) {
	SetSelectableModels([]Model{{
		ID: "claude-opus-4-7", ReasoningEfforts: []string{"Low", "High"},
		ContextOptions: []int{200000}, ContextWindow: 1000000,
	}})
	t.Cleanup(func() { SetSelectableModels(nil) })

	got := ValidModelOption("claude-opus-4-7", ModelOption{Effort: "high", Context: 1000000})
	if got.Effort != "High" || got.Context != 1000000 {
		t.Fatalf("option: %+v", got)
	}
	if got := ValidModelOption("claude-opus-4-7", ModelOption{Effort: "max", Context: 999}); got != (ModelOption{}) {
		t.Fatalf("unknown option kept: %+v", got)
	}
}

func TestCatalogEpochBumpsOnlyWhenModelsChange(t *testing.T) {
	SetSelectableModels(nil)
	t.Cleanup(func() { SetSelectableModels(nil) })
	start := CatalogEpoch()

	SetSelectableModels([]Model{{ID: "claude-opus-4-7", Label: "Claude Opus 4.7"}})
	if CatalogEpoch() != start+1 {
		t.Fatalf("epoch after new model: %d, start %d", CatalogEpoch(), start)
	}
	SetSelectableModels([]Model{{ID: "claude-opus-4-7", Label: "Claude Opus 4.7"}})
	if CatalogEpoch() != start+1 {
		t.Fatalf("same catalog should keep epoch, got %d", CatalogEpoch())
	}
	SetSelectableModels([]Model{{ID: "mimo-v2.6-pro", Label: "MiMo V2.6 Pro"}})
	if CatalogEpoch() != start+2 {
		t.Fatalf("epoch after another model: %d, start %d", CatalogEpoch(), start)
	}
}

func TestWaitCatalogChangeWakesWhenModelsChange(t *testing.T) {
	SetSelectableModels(nil)
	t.Cleanup(func() { SetSelectableModels(nil) })
	seen := CatalogEpoch()
	got := make(chan uint64, 1)
	go func() {
		got <- WaitCatalogChange(context.Background(), seen, true)
	}()
	time.Sleep(50 * time.Millisecond)
	SetSelectableModels([]Model{{ID: "mimo-v2.6-pro", Label: "MiMo V2.6 Pro"}})
	select {
	case epoch := <-got:
		if epoch == seen || epoch != CatalogEpoch() {
			t.Fatalf("woke with %d, seen %d, current %d", epoch, seen, CatalogEpoch())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("模型变化后没有叫醒等待方")
	}
}

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
