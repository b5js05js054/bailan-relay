package devin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireModelRoundTrip(t *testing.T) {
	pairs := requireModelPairs()
	root := t.TempDir()
	assets := filepath.Join(root, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	label := pairs[0][0]
	var composer strings.Builder
	for _, pair := range pairs[1:] {
		composer.WriteString(pair[0])
		composer.WriteString(";")
	}
	if err := os.WriteFile(filepath.Join(assets, "label.js"), []byte("x"+label+"y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "composer.js"), []byte(composer.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	wrote, msg := applyRequireModel(root)
	if !wrote || msg != "没选模型时不能发送" {
		t.Fatalf("apply wrote=%v msg=%s", wrote, msg)
	}
	again, againMsg := applyRequireModel(root)
	if again || againMsg != "" {
		t.Fatalf("second apply wrote=%v msg=%s", again, againMsg)
	}
	body := readRequireModel(t, assets)
	if !strings.Contains(body, "bailanHasModel=Oe.length>0") || !strings.Contains(body, "bailanHasModel===!1") {
		t.Fatal("send gate missing")
	}
	if strings.Count(body, "if(globalThis.bailanHasModel===!1)return;if(Qa)return;") != 1 {
		t.Fatal("queue gate duplicated")
	}

	restored, note := restoreRequireModel(root)
	if !restored || note != "已还原没选模型时的发送限制" {
		t.Fatalf("restore restored=%v note=%s", restored, note)
	}
	body = readRequireModel(t, assets)
	if strings.Contains(body, "bailanHasModel") {
		t.Fatal("gate survived restore")
	}
	for _, pair := range pairs {
		if strings.Count(body, pair[0]) != 1 {
			t.Fatalf("old anchor missing after restore: %s", pair[0])
		}
	}
}

func TestInstalledDevin312RequireModel(t *testing.T) {
	src := filepath.Join(`E:\Devin\resources`, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Skip(err.Error())
	}
	root := t.TempDir()
	assets := filepath.Join(root, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	copied := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.Contains(text, "modelSelect.noneSelected") && !strings.Contains(text, "Pf=K(e=>{Nf()") && !strings.Contains(text, "bailanHasModel") {
			continue
		}
		if err := os.WriteFile(filepath.Join(assets, entry.Name()), body, 0o644); err != nil {
			t.Fatal(err)
		}
		copied++
	}
	if copied == 0 {
		t.Skip("installed Devin has no model button label")
	}

	wrote, msg := applyRequireModel(root)
	if strings.Contains(msg, "没有改动") || strings.Contains(msg, "没能还原") {
		t.Fatalf("apply wrote=%v msg=%s", wrote, msg)
	}
	for _, entry := range mustReadAssets(t, assets) {
		if _, err := exec.LookPath("node"); err != nil {
			break
		}
		out, err := exec.Command("node", "--check", filepath.Join(assets, entry)).CombinedOutput()
		if err != nil {
			t.Fatalf("syntax %s: %v %s", entry, err, out)
		}
	}
	body := readRequireModel(t, assets)
	for _, want := range []string{
		"bailanHasModel=Oe.length>0",
		"bailanHasModel=!0,null",
		"bailanHasModel===!1",
		"if(globalThis.bailanHasModel===!1)return;Nf()",
		"if(globalThis.bailanHasModel===!1)return;if(Qa)return;",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("installed patch missing %s", want)
		}
	}

	restored, note := restoreRequireModel(root)
	if strings.Contains(note, "没有改动") || strings.Contains(note, "没能还原") || !restored {
		t.Fatalf("restore restored=%v note=%s", restored, note)
	}
	body = readRequireModel(t, assets)
	if strings.Contains(body, "bailanHasModel") {
		t.Fatal("installed gate survived restore")
	}
}

func readRequireModel(t *testing.T, assets string) string {
	t.Helper()
	var b strings.Builder
	for _, name := range mustReadAssets(t, assets) {
		body, err := os.ReadFile(filepath.Join(assets, name))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(body)
	}
	return b.String()
}

func mustReadAssets(t *testing.T, assets string) []string {
	t.Helper()
	entries, err := os.ReadDir(assets)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
			names = append(names, entry.Name())
		}
	}
	return names
}
