package devin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledDevin238RoundTrip(t *testing.T) {
	src := `E:\Devin\resources`
	files := []string{
		filepath.Join("app", "out", "vs", "workbench", "windsurf-chat-client", "index.js"),
		filepath.Join("app", "out", "vs", "workbench", "devin-webapp-host", "assets", "app-initial-DnnoLajw.js"),
		filepath.Join("app", "out", "vs", "workbench", "devin-webapp-host", "assets", "app-initial-CECksEiP.js"),
		filepath.Join("app", "extensions", "windsurf", "dist", "extension.js"),
	}
	if _, err := os.Stat(filepath.Join(src, files[1])); err != nil {
		t.Skip("3.11.238 agent bundle is not installed")
	}
	root := t.TempDir()
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(src, rel))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(rel, "extension.js") {
			if next, ok := stripCatalogWake(string(body)); ok {
				body = []byte(next)
			}
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var notes []string
	for _, apply := range []func(string) (bool, string){
		applyAgentModelList,
		applyAgentSortMenu,
		applyChatModelList,
		applyRateBadges,
		applyCatalogWake,
	} {
		wrote, msg := apply(root)
		notes = append(notes, msg)
		if strings.Contains(msg, "没有改动") || strings.Contains(msg, "没能还原") || (!wrote && msg != "" && !strings.Contains(msg, "已经")) {
			t.Fatalf("apply wrote=%v msg=%s", wrote, msg)
		}
	}
	agentBody, err := os.ReadFile(filepath.Join(root, files[1]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agentBody), `t.category==="model"`) || strings.Contains(string(agentBody), agentShownKeepBroad()) {
		t.Fatal("mode button still uses the session model")
	}

	patchedChat, err := os.ReadFile(filepath.Join(root, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	text := string(patchedChat)
	for _, want := range []string{"devinhub-family-", "function bailanChatModel(", "function bailanSrc(e){", "bailanSyncShown(c,H)", "rwO(e.modelUid)", "rkp,"} {
		if !strings.Contains(text, want) {
			t.Fatalf("chat missing %s", want)
		}
	}

	notes = nil
	var restored bool
	for _, restore := range []func(string) (bool, string){
		restoreAgentModelList,
		restoreAgentSortMenu,
		restoreChatModelList,
		restoreRateBadges,
		restoreCatalogWake,
	} {
		changed, note := restore(root)
		if note != "" {
			notes = append(notes, note)
		}
		if strings.Contains(note, "没有改动") || strings.Contains(note, "没能还原") {
			t.Fatalf("restore msg=%s", note)
		}
		if changed {
			restored = true
		}
	}
	if !restored {
		t.Fatalf("nothing restored: %s", strings.Join(notes, "；"))
	}
	restoredAgent, err := os.ReadFile(filepath.Join(root, files[1]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(restoredAgent), `t.category==="model"`) {
		t.Fatal("mode guard survived restore")
	}
}
