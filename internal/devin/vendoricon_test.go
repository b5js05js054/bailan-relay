package devin

import (
	"strings"
	"testing"
)

func TestBailanSrcCarriesMiMoIcon(t *testing.T) {
	js, err := bailanSrcFunction()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(js, "function bailanSrc(e){if(!e||e.provider)return\"\";") {
		t.Fatalf("prefix: %.80s", js)
	}
	if !strings.Contains(js, `,"mimo","xiaomi"]`) || !strings.Contains(js, "data:image/svg+xml;base64,") {
		t.Fatal("mimo icon was not embedded")
	}
	sample := "function rwX(e,n=16){if(void 0===e)return null;let x=1}" + chatIconRowOld
	next, ok, note := patchVendorIcons(sample, chatIconFnOld, chatIconRowOld, chatIconRowNew)
	if !ok || note != "" || !strings.Contains(next, "function bailanSrc(e){") || !strings.Contains(next, chatIconRowNew) {
		t.Fatalf("patch ok=%v note=%s", ok, note)
	}
	back, restored := restoreVendorIcons(next, chatIconFnOld, chatIconRowOld, chatIconRowNew)
	if !restored || back != sample {
		t.Fatal("restore did not return the original row")
	}
	sample238 := agentIconFn238() + "let x=1;" + agentIconRow238Old()
	next238, ok238, note238 := patchVendorIcons(sample238, agentIconFn238(), agentIconRow238Old(), agentIconRow238New())
	if !strings.Contains(agentListIcon238New(), "bailanSrc({modelUid:C.modelUid,label:C.label})") || !strings.Contains(agentListIcon238Old(), "Kn(C.provider)") {
		t.Fatal("agent list icon fallback was not attached")
	}
	if !ok238 || note238 != "" || !strings.Contains(next238, "bailanSrc({modelUid:e.modelUid,label:e.label})") {
		t.Fatalf("agent 238 icon ok=%v note=%s", ok238, note238)
	}
	if !strings.HasPrefix(agentCardIconNew(), agentCardIconOld()) || !strings.Contains(agentCardIconNew(), "bailanSrc(S)") {
		t.Fatal("agent card icon fallback was not attached")
	}
	if !strings.HasPrefix(chatCardIconNew(), chatCardIconOld) || !strings.Contains(chatCardIconNew(), "bailanSrc(R)") {
		t.Fatal("ide card icon fallback was not attached")
	}
}
