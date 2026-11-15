package devin

import (
	"strings"
	"testing"
)

func roundTripIconRow(t *testing.T, label, item, jsx, fn string) {
	t.Helper()
	old := iconRowOldStr(item, jsx, fn)
	// 组一段带「厂商图标函数」的文本，apply 会在该函数前插入 bailanSrc。
	text := "A=" + old + ";function " + fn + "(e,t=16){if(e===void 0)return null;return B(e)}C"

	applied, changed, bad := tryAgentIconRowApply(text)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !changed {
		t.Fatalf("%s：没识别出图标行", label)
	}
	if !strings.Contains(applied, iconRowMark) || !strings.Contains(applied, "function bailanSrc(e){") {
		t.Fatalf("%s：套完缺少标记或没插入 bailanSrc", label)
	}
	if _, again, _ := tryAgentIconRowApply(applied); again {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, ok := tryAgentIconRowRestore(applied)
	if !ok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != text {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, text, restored)
	}
}

func TestAgentIconRowRoundTrip(t *testing.T) {
	roundTripIconRow(t, "3.12.28", "e", "J", "Kn")
	roundTripIconRow(t, "older", "e", "q", "Vn")
	// 模拟将来重新压缩、换名。
	roundTripIconRow(t, "future", "s", "Qw", "Zx")
}

// 用真实常量验证：iconRowOldStr 生成的和代码里的精确锚点一致（防止两边漂移）。
func TestIconRowBuilderMatchesConstants(t *testing.T) {
	if iconRowOldStr("e", "J", "Kn") != agentIconRow312Old() {
		t.Error("iconRowOldStr 和 agentIconRow312Old 不一致")
	}
	if iconRowOldStr("e", "q", "Vn") != agentIconRowOld() {
		t.Error("iconRowOldStr 和 agentIconRowOld 不一致")
	}
}

func roundTripIconList(t *testing.T, label, s, e, fn2, fn, jsx string) {
	t.Helper()
	old := iconListOldStr(s, e, fn2, fn, jsx)
	applied, changed, bad := tryAgentIconListApply(old)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !changed {
		t.Fatalf("%s：没识别出列表图标", label)
	}
	if !strings.Contains(applied, iconListMark) {
		t.Fatalf("%s：套完缺少标记", label)
	}
	if _, again, _ := tryAgentIconListApply(applied); again {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, ok := tryAgentIconListRestore(applied)
	if !ok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != old {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, old, restored)
	}
}

func TestAgentIconListRoundTrip(t *testing.T) {
	roundTripIconList(t, "3.12.28", "S", "e", "Nt", "Kn", "Y")
	roundTripIconList(t, "future", "Zz", "ee", "Aa", "Bb", "Cc")
}

func TestIconListBuilderMatchesConstants(t *testing.T) {
	if iconListOldStr("S", "e", "Nt", "Kn", "Y") != agentListIcon312Old() {
		t.Error("iconListOldStr 和 agentListIcon312Old 不一致")
	}
	if iconListNewStr("S", "e", "Nt", "Kn", "Y") != agentListIcon312New() {
		t.Error("iconListNewStr 和 agentListIcon312New 不一致")
	}
}
