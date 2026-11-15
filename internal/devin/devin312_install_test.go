package devin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledDevin312ContextWindow(t *testing.T) {
	src := filepath.Join(`E:\Devin\resources`, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Skip(err.Error())
	}
	var name string
	var body []byte
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		next, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(next), agentContext312Old) || strings.Contains(string(next), agentContext312New) {
			name = entry.Name()
			body = next
			break
		}
	}
	if name == "" {
		t.Skip("installed Devin has no 3.12 context ring")
	}
	root := t.TempDir()
	assets := filepath.Join(root, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
	wrote, msg := applyAgentContextWindow(root)
	if strings.Contains(msg, "没有改动") || strings.Contains(msg, "没能还原") {
		t.Fatalf("apply wrote=%v msg=%s", wrote, msg)
	}
	got, err := os.ReadFile(filepath.Join(assets, name))
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	// 新的正则路径产出 contextLimit:bailanCtx；旧的精确锚点产出 contextLimit:n，两者都算生效。
	ringPatched := strings.Contains(gotStr, "contextLimit:bailanCtx") || strings.Contains(gotStr, "contextLimit:n")
	if !ringPatched || !strings.Contains(gotStr, "globalThis.bailanLast") {
		t.Fatal("context ring was not patched")
	}
	restored, note := restoreAgentContextWindow(root)
	if !restored || strings.Contains(note, "没能还原") {
		t.Fatalf("restore restored=%v note=%s", restored, note)
	}
	back, err := os.ReadFile(filepath.Join(assets, name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(back), "contextLimit:bailanCtx") || strings.Contains(string(back), "contextLimit:n") {
		t.Fatal("context ring survived restore")
	}
}

func TestInstalledDevin312RoundTrip(t *testing.T) {
	src := `E:\Devin\resources`
	files := []string{
		filepath.Join("app", "out", "vs", "workbench", "devin-webapp-host", "assets", "app-initial-DlcIkwcB.js"),
		filepath.Join("app", "out", "vs", "workbench", "devin-webapp-host", "assets", "app-initial-B6DX4ee6.js"),
		filepath.Join("app", "extensions", "windsurf", "dist", "extension.js"),
	}
	if _, err := os.Stat(filepath.Join(src, files[0])); err != nil {
		t.Skip(err.Error())
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

	for _, apply := range []func(string) (bool, string){
		applyAgentModelList,
		applyAgentSortMenu,
		applyAgentIcons,
		applyRateBadges,
		applyCatalogWake,
	} {
		wrote, msg := apply(root)
		if strings.Contains(msg, "没有改动") || strings.Contains(msg, "没能还原") || (!wrote && msg != "" && !strings.Contains(msg, "已经")) {
			t.Fatalf("apply wrote=%v msg=%s", wrote, msg)
		}
	}

	agentBody, err := os.ReadFile(filepath.Join(root, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	agent := string(agentBody)
	for _, want := range []string{
		"devinhub-family-",
		`t.category==="model"`,
		"set_session_model",
		"swe-1-6-slow",
		"Ur(t||{",
	} {
		if !strings.Contains(agent, want) {
			t.Fatalf("agent missing %s", want)
		}
	}
	if strings.Contains(agent, agentShown312Old()) || strings.Contains(agent, agentListFlat312Old) {
		t.Fatal("old agent anchors still present")
	}

	iconBody, err := os.ReadFile(filepath.Join(root, files[1]))
	if err != nil {
		t.Fatal(err)
	}
	icon := string(iconBody)
	// 排序：正则版把判断改成恒真（||!0），旧的精确锚点是 n.slice()，两者都算生效。
	sortPatched := strings.Contains(icon, "||!0)") || strings.Contains(icon, "n.slice()")
	if !strings.Contains(icon, "bailanSrc({modelUid:S.modelUid,label:S.label})") || !strings.Contains(icon, "key:`rate`") || !sortPatched {
		t.Fatal("icon, rate, or sort was not patched")
	}

	extBody, err := os.ReadFile(filepath.Join(root, files[2]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(extBody), "api/catalog_wait") || !strings.Contains(string(extBody), "q(e,0)") {
		t.Fatal("wake was not attached")
	}

	for _, restore := range []func(string) (bool, string){
		restoreAgentModelList,
		restoreAgentSortMenu,
		restoreAgentIcons,
		restoreRateBadges,
		restoreCatalogWake,
	} {
		_, note := restore(root)
		if strings.Contains(note, "没有改动") || strings.Contains(note, "没能还原") {
			t.Fatalf("restore msg=%s", note)
		}
	}
	restored, err := os.ReadFile(filepath.Join(root, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(restored), `t.category==="model"`) || strings.Contains(string(restored), "set_session_model") {
		t.Fatal("agent patch survived restore")
	}
	restoredExt, err := os.ReadFile(filepath.Join(root, files[2]))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(restoredExt), "bailanArm") {
		t.Fatal("wake survived restore")
	}
}
