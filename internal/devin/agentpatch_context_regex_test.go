package devin

import (
	"strings"
	"testing"
)

// 把一段原文套上再还原，应当原样回到出发点，且中间确实改动过、带上我们的标记。
func roundTripContextRing(t *testing.T, label, original string) {
	t.Helper()
	applied, ok, bad := tryContextRingApply(original)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !ok {
		t.Fatalf("%s：没识别出上下文圈", label)
	}
	if !strings.Contains(applied, contextRingMark) {
		t.Fatalf("%s：套完没有我们的标记 %q", label, contextRingMark)
	}
	if applied == original {
		t.Fatalf("%s：套补丁没改动", label)
	}
	// 幂等：已经套过的再套一次不应再改。
	if again, ok2, _ := tryContextRingApply(applied); ok2 || again != applied {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, rok := tryContextRingRestore(applied)
	if !rok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != original {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, original, restored)
	}
}

func TestContextRingRoundTrip(t *testing.T) {
	// 3.11.238 形态。
	roundTripContextRing(t, "Dm", agentContextOld)
	// 3.12.28 形态。
	roundTripContextRing(t, "ed", agentContext312Old)
	// 模拟将来 Devin 重新压缩、换了一批名字：补丁应当自动认出并原样还原。
	future := `function Qz(){let t=Xy9(t=>t.status.contextUsage);return t?(0,Ab2.jsx)(Cd7,{tokensUsed:t.used,contextLimit:t.size}):null}`
	roundTripContextRing(t, "future-renamed", future)
}

// 结构里同一变量的引用对不上时，宁可不动，交给人工看。
func TestContextRingRejectsInconsistent(t *testing.T) {
	// tokensUsed 用的是 e，contextLimit 却用了 x：不是同一个 usage 变量，拒绝改。
	badText := `function ed(){let e=_r(e=>e.status.contextUsage);return e?(0,td.jsx)(Hr,{tokensUsed:e.used,contextLimit:x.size}):null}`
	if _, ok, _ := tryContextRingApply(badText); ok {
		t.Error("引用不一致时不应改动")
	}
}

// 已经被旧版（精确锚点、用 n 作变量）套过的文件，正则路径不应误判或重复改。
func TestContextRingLeavesExactFormAlone(t *testing.T) {
	if _, ok, bad := tryContextRingApply(agentContext312New); ok || bad {
		t.Errorf("精确锚点形态不应被正则路径改动 ok=%v bad=%v", ok, bad)
	}
}
