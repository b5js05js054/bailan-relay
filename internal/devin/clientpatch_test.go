package devin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyChatModelListAndWake(t *testing.T) {
	root := t.TempDir()
	chatDir := filepath.Join(root, "app", "out", "vs", "workbench", "windsurf-chat-client")
	extDir := filepath.Join(root, "app", "extensions", "windsurf", "dist")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chat := "head();" + pickerBefore + ";mid();" + chatSelectOld + ":tail"
	if err := os.WriteFile(filepath.Join(chatDir, "index.js"), []byte(chat), 0o644); err != nil {
		t.Fatal(err)
	}
	ext := "prefix" + wakeOld + "(e,t){"
	if err := os.WriteFile(filepath.Join(extDir, "extension.js"), []byte(ext), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, msg := applyChatModelList(root)
	if !wrote || !strings.Contains(msg, "普通对话") {
		t.Fatalf("chat: wrote=%v msg=%s", wrote, msg)
	}
	body, err := os.ReadFile(filepath.Join(chatDir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, pickerBefore) || !strings.Contains(text, "devinhub-family-") || !strings.Contains(text, "api/set_model") {
		t.Fatal("chat patch incomplete")
	}
	wrote, msg = applyChatModelList(root)
	if wrote || !strings.Contains(msg, "已经写好") {
		t.Fatalf("chat second: wrote=%v msg=%s", wrote, msg)
	}

	wrote, msg = applyCatalogWake(root)
	if !wrote || !strings.Contains(msg, "叫醒") {
		t.Fatalf("wake: wrote=%v msg=%s", wrote, msg)
	}
	extBody, err := os.ReadFile(filepath.Join(extDir, "extension.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(extBody), "api/catalog_wait") {
		t.Fatal("wake patch missing")
	}
	wrote, msg = applyCatalogWake(root)
	if wrote || !strings.Contains(msg, "已经接上") {
		t.Fatalf("wake second: wrote=%v msg=%s", wrote, msg)
	}
}

func TestRestoreChatAndWakeBackToOriginal(t *testing.T) {
	root := t.TempDir()
	chatDir := filepath.Join(root, "app", "out", "vs", "workbench", "windsurf-chat-client")
	extDir := filepath.Join(root, "app", "extensions", "windsurf", "dist")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chat := "head();" + pickerBefore + ";mid();" + chatSelectOld + ":tail"
	ext := "prefix" + wakeOld + "(e,t){"
	if err := os.WriteFile(filepath.Join(chatDir, "index.js"), []byte(chat), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "extension.js"), []byte(ext), 0o644); err != nil {
		t.Fatal(err)
	}
	if wrote, msg := applyChatModelList(root); !wrote {
		t.Fatal(msg)
	}
	if wrote, msg := applyCatalogWake(root); !wrote {
		t.Fatal(msg)
	}
	// 本机已经写过的叫醒代码可能多一个分号，还原也要能去掉。
	extBody, err := os.ReadFile(filepath.Join(extDir, "extension.js"))
	if err != nil {
		t.Fatal(err)
	}
	doubled := strings.Replace(string(extBody), "bailanArm(-1);async", "bailanArm(-1);;async", 1)
	if err := os.WriteFile(filepath.Join(extDir, "extension.js"), []byte(doubled), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, msg := restoreChatModelList(root); !strings.Contains(msg, "已还原普通对话") {
		t.Fatal(msg)
	}
	if _, msg := restoreCatalogWake(root); !strings.Contains(msg, "已去掉") {
		t.Fatal(msg)
	}
	gotChat, err := os.ReadFile(filepath.Join(chatDir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotChat) != chat {
		t.Fatal("chat file was not restored")
	}
	gotExt, err := os.ReadFile(filepath.Join(extDir, "extension.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotExt) != ext {
		t.Fatalf("wake file was not restored: %s", gotExt)
	}
}

func TestRateBadgePatchRoundTrip(t *testing.T) {
	root := t.TempDir()
	chatDir := filepath.Join(root, "app", "out", "vs", "workbench", "windsurf-chat-client")
	assetDir := filepath.Join(root, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chatPath := filepath.Join(chatDir, "index.js")
	agentPath := filepath.Join(assetDir, "app.js")
	if err := os.WriteFile(chatPath, []byte("head"+chatRateOld+"tail"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, []byte("head"+agentRateOld()+"tail"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, msg := applyRateBadges(root)
	if !wrote || !strings.Contains(msg, "倍率") {
		t.Fatalf("apply: wrote=%v msg=%s", wrote, msg)
	}
	chat, err := os.ReadFile(chatPath)
	if err != nil || !strings.Contains(string(chat), `key:"rate"`) || strings.Contains(string(chat), chatRateOld) {
		t.Fatal("chat rate patch missing")
	}
	agent, err := os.ReadFile(agentPath)
	if err != nil || !strings.Contains(string(agent), "key:`rate`") {
		t.Fatal("agent rate patch missing")
	}
	restored, msg := restoreRateBadges(root)
	if !restored || !strings.Contains(msg, "去掉") {
		t.Fatalf("restore: restored=%v msg=%s", restored, msg)
	}
	chat, _ = os.ReadFile(chatPath)
	agent, _ = os.ReadFile(agentPath)
	if string(chat) != "head"+chatRateOld+"tail" || string(agent) != "head"+agentRateOld()+"tail" {
		t.Fatal("rate patch was not restored")
	}
}

func TestInstalledDevinHasRateAnchor(t *testing.T) {
	chat := filepath.Join(`E:\Devin`, "resources", "app", "out", "vs", "workbench", "windsurf-chat-client", "index.js")
	body, err := os.ReadFile(chat)
	if err != nil {
		t.Skip(err)
	}
	text := string(body)
	chatRate := strings.Contains(text, chatRateOld) || strings.Contains(text, chatRateNew) || strings.Contains(text, chatRate126Old) || strings.Contains(text, chatRate126New) || strings.Contains(text, chatRate238Old) || strings.Contains(text, chatRate238New)
	assets := filepath.Join(`E:\Devin`, "resources", "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(assets, entry.Name()))
		if err != nil {
			continue
		}
		file := string(raw)
		if strings.Contains(file, agentRateOld()) || strings.Contains(file, agentRateNew()) || strings.Contains(file, agentRate238Old()) || strings.Contains(file, agentRate238New()) || strings.Contains(file, agentRate312Old()) || strings.Contains(file, agentRate312New()) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("agent picker rate anchor does not match this Devin")
	}
	if !chatRate && !found {
		t.Fatal("chat picker rate anchor does not match this Devin")
	}
}

func TestIDEShownModelIsReported(t *testing.T) {
	original := "function bailanChatModel(){}" + agentSyncJS() + chatShownSyncOld + "tail"
	next, ok, note := patchChatSession(original)
	if !ok || note != "" {
		t.Fatalf("patch: ok=%v note=%s", ok, note)
	}
	if !strings.Contains(next, "function bailanSyncShown(") || !strings.Contains(next, "bailanSyncShown(c,J)") {
		t.Fatal("shown model was not reported")
	}
	back, ok, note := restoreChatSession(next)
	if !ok || note != "" {
		t.Fatalf("restore: ok=%v note=%s", ok, note)
	}
	if back != original {
		t.Fatal("shown sync was not restored")
	}
}

func TestDevin126ChatWakeAndRateRoundTrip(t *testing.T) {
	root := t.TempDir()
	chatDir := filepath.Join(root, "app", "out", "vs", "workbench", "windsurf-chat-client")
	extDir := filepath.Join(root, "app", "extensions", "windsurf", "dist")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chat := strings.Join([]string{
		pickerBefore126,
		chatSelectOld,
		chatShown126,
		chatSnapOld,
		chatIconFn126,
		chatIconRow126Old,
		chatCardIcon126,
		chatSortMenuOld,
		chatRate126Old,
	}, ";")
	ext := "prefix" + wakeAnchor126 + "(e,t){"
	chatPath := filepath.Join(chatDir, "index.js")
	extPath := filepath.Join(extDir, "extension.js")
	if err := os.WriteFile(chatPath, []byte(chat), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extPath, []byte(ext), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, msg := applyChatModelList(root)
	if !wrote {
		t.Fatal(msg)
	}
	body, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"devinhub-family-", "api/set_model", "api/set_session_model", "function bailanChatModel(", "function bailanSrc(e){", "rwo(e.modelUid)", chatSortMenuNew} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(text, pickerBefore126) || strings.Contains(text, chatShown126) {
		t.Fatal("old chat anchors still present")
	}

	wrote, msg = applyRateBadges(root)
	if !wrote || !strings.Contains(msg, "倍率") {
		t.Fatalf("rate: wrote=%v msg=%s", wrote, msg)
	}
	wrote, msg = applyCatalogWake(root)
	if !wrote || !strings.Contains(msg, "叫醒") {
		t.Fatalf("wake: wrote=%v msg=%s", wrote, msg)
	}
	extBody, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(extBody), "api/catalog_wait") || !strings.Contains(string(extBody), "B(e,0)") {
		t.Fatalf("wake body: %s", extBody)
	}

	if _, msg := restoreRateBadges(root); !strings.Contains(msg, "去掉") {
		t.Fatal(msg)
	}
	if _, msg := restoreChatModelList(root); !strings.Contains(msg, "已还原普通对话") {
		t.Fatal(msg)
	}
	if _, msg := restoreCatalogWake(root); !strings.Contains(msg, "已去掉") {
		t.Fatal(msg)
	}
	gotChat, err := os.ReadFile(chatPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotChat) != chat {
		t.Fatalf("chat was not restored")
	}
	gotExt, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotExt) != ext {
		t.Fatalf("wake was not restored: %s", gotExt)
	}
}
