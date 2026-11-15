package devin

import (
	"strings"
	"testing"
)

func roundTripWake(t *testing.T, label, original, refetch string) {
	t.Helper()
	applied, changed, bad := tryCatalogWakeApply(original)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !changed {
		t.Fatalf("%s：没识别出叫醒位置", label)
	}
	if !strings.Contains(applied, "api/catalog_wait") || !strings.Contains(applied, refetch+"(e,0)") || !strings.Contains(applied, "bailanArm(-1);") {
		t.Fatalf("%s：套完缺少叫醒块或刷新调用 %s(e,0)", label, refetch)
	}
	if _, again, _ := tryCatalogWakeApply(applied); again {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, ok := stripCatalogWake(applied)
	if !ok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != original {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, original, restored)
	}
}

func TestCatalogWakeRoundTrip(t *testing.T) {
	// 贴近真实 3.12.28 的结构：持有变量 g、刷新函数 q、后面的函数 V。
	region := `AA;x=setTimeout(()=>{g.MetadataProvider.getInstance().getMetadata().apiKey===e.apiKey&&q(e,t).catch(()=>{H(e,t)})},1e4)}async function V(e,t){const n=g.MetadataProvider.getInstance(),i=n.getMetadata();return i}ZZ`
	roundTripWake(t, "3.12.28", region, "q")

	// 模拟将来重新压缩、换名：持有变量 pr、刷新 rf、后面的函数 Nx。
	future := `AA;x=setTimeout(()=>{pr.MetadataProvider.getInstance().getMetadata().apiKey===e.apiKey&&rf(e,t).catch(()=>{Rt(e,t)})},1e4)}async function Nx(e,t){const m=pr.MetadataProvider.getInstance();return m}ZZ`
	roundTripWake(t, "future", future, "rf")
}

// 没有这段 10 秒重试结构的文本，不应被改动。
func TestCatalogWakeLeavesOtherAlone(t *testing.T) {
	if _, changed, _ := tryCatalogWakeApply(`function foo(){setTimeout(()=>{bar()},1e4)}async function baz(){}`); changed {
		t.Error("不该改动无关文本")
	}
}
