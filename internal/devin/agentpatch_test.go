package devin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAgentModelListKeepsHubModels(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	original := "before();" + agentListFlatOld + ",middle();" + agentListReturn + ";after();"
	path := filepath.Join(assets, "app-initial-test.js")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, msg := applyAgentModelList(root)
	if !wrote || !strings.Contains(msg, "写进 Agent") {
		t.Fatalf("first apply: wrote=%v msg=%s", wrote, msg)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, agentListFlatOld) || !strings.Contains(text, agentListMarker()) {
		t.Fatalf("patch missing from %s", text)
	}
	if !strings.Contains(text, "groupName:\x60未分类\x60") {
		t.Fatal("default group was not filled")
	}

	wrote, msg = applyAgentModelList(root)
	if wrote || !strings.Contains(msg, "已经写好") {
		t.Fatalf("second apply: wrote=%v msg=%s", wrote, msg)
	}
	restored, msg := restoreAgentModelList(root)
	if !restored || !strings.Contains(msg, "已还原") {
		t.Fatalf("restore: restored=%v msg=%s", restored, msg)
	}
	body, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != original {
		t.Fatal("agent list was not restored")
	}
}

func TestNewAgentSessionUsesDisplayedModel(t *testing.T) {
	original := "head;" + agentShownNew() + "mid;" + agentNewSessionOld() + "tail"
	next, ok := patchAgentChoice(original)
	if !ok {
		t.Fatal("patch did not apply")
	}
	if !strings.Contains(next, "let picked=n?.value") || !strings.Contains(next, "set_session_model") {
		t.Fatal("new session notify missing")
	}
	if !strings.Contains(next, `typeof a=="string"&&a.indexOf("@")>=0?a:d`) {
		t.Fatal("displayed model was not preferred")
	}
	back := restoreAgentChoice(next)
	if strings.Contains(back, "set_session_model") || strings.Contains(back, "let picked=") {
		t.Fatal("new session patch did not restore")
	}
	if !strings.Contains(back, agentNewSessionOld()) || !strings.Contains(back, agentShownOld()) {
		t.Fatal("restore did not return to the original anchors")
	}
}

func TestModeSelectorDoesNotUseSessionModel(t *testing.T) {
	original := "head;" + agentShownKeepBroad() + "tail"
	next, ok := patchAgentChoice(original)
	if !ok {
		t.Fatal("patch did not apply")
	}
	if !strings.Contains(next, `t.category==="model"`) || strings.Contains(next, agentShownKeepBroad()) {
		t.Fatal("mode and model still share the model value")
	}
	back := restoreAgentChoice(next)
	if !strings.Contains(back, agentShownOld()) {
		t.Fatal("restore did not return to the original value expression")
	}
}

func TestApplyAgentModelListLeavesUnknownBuild(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "other.js"), []byte("no model list"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, msg := applyAgentModelList(root)
	if wrote || !strings.Contains(msg, "对不上") {
		t.Fatalf("unknown build: wrote=%v msg=%s", wrote, msg)
	}
}
