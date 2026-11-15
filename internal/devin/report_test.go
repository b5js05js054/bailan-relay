package devin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPatch(t *testing.T) {
	cases := []struct {
		changed bool
		msg     string
		want    string
	}{
		{true, "已把你的模型留在列表里", PatchOK},
		{true, "", PatchOK},
		{false, "普通对话的模型列表对不上，没有改动", PatchMismatch},
		{false, "写入普通对话的模型列表失败", PatchMismatch},
		{false, "上下文用量对不上，没有改动", PatchMismatch},
		{false, "普通对话的模型列表已经写好", PatchOK},
		{false, "模型变化后叫醒 Devin 的那一段已经接上", PatchOK},
		{false, "", PatchNA},
		{false, "这版普通对话和 Agent 共用模型列表", PatchNA},
		{false, "未找到普通对话的模型列表", PatchNA},
	}
	for _, c := range cases {
		if got := classifyPatch(c.changed, c.msg); got != c.want {
			t.Errorf("classifyPatch(%v, %q) = %q, want %q", c.changed, c.msg, got, c.want)
		}
	}
}

func TestDevinInfo(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "resources", "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "Devin.exe")

	// windsurfVersion 优先。
	if err := os.WriteFile(filepath.Join(appDir, "product.json"),
		[]byte(`{"windsurfVersion":"3.12.28","version":"1.141.2","commit":"91c3aec4"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, c := DevinInfo(exe); v != "3.12.28" || c != "91c3aec4" {
		t.Errorf("DevinInfo = (%q, %q), want (3.12.28, 91c3aec4)", v, c)
	}

	// 没有 windsurfVersion 时回退到 version。
	if err := os.WriteFile(filepath.Join(appDir, "product.json"),
		[]byte(`{"version":"1.141.2"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, _ := DevinInfo(exe); v != "1.141.2" {
		t.Errorf("DevinInfo fallback = %q, want 1.141.2", v)
	}

	// 读不到文件时返回空。
	if v, c := DevinInfo(filepath.Join(t.TempDir(), "Nope.exe")); v != "" || c != "" {
		t.Errorf("DevinInfo missing = (%q, %q), want empty", v, c)
	}
}

// 只读确认：本机装了 Devin 时，能从真实 product.json 读出版本号。没装则跳过。
func TestDevinInfoRealInstall(t *testing.T) {
	exe := `E:\Devin\Devin.exe`
	if _, err := os.Stat(exe); err != nil {
		t.Skip("本机没有 E:\\Devin，跳过")
	}
	v, c := DevinInfo(exe)
	t.Logf("真实 Devin 版本=%q 提交=%q", v, c)
	if v == "" {
		t.Error("应能从真实 product.json 读出版本号")
	}
}

// TestDumpLiveReport 把真实 Devin 的资源拷到临时目录，用新代码跑一遍补丁，
// 产出真实的适配报告写到 %TEMP%\bailan_report.json，供界面卡片预览用。
// 只在设置了 BAILAN_DUMP_REPORT=1 时运行，日常 go test 会跳过，不碰真实安装。
func TestDumpLiveReport(t *testing.T) {
	if os.Getenv("BAILAN_DUMP_REPORT") == "" {
		t.Skip("未设置 BAILAN_DUMP_REPORT，跳过")
	}
	srcRoot := `E:\Devin\resources`
	exe := `E:\Devin\Devin.exe`
	if _, err := os.Stat(exe); err != nil {
		t.Skip("本机没有 E:\\Devin")
	}
	root := t.TempDir()
	// 把补丁会动到的三处文件/目录按相对路径拷到临时根。
	copyFile := func(rel string) {
		src := filepath.Join(srcRoot, rel)
		dst := filepath.Join(root, rel)
		body, err := os.ReadFile(src)
		if err != nil {
			return
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, body, 0o644)
	}
	assetsRel := filepath.Join("app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	if entries, err := os.ReadDir(filepath.Join(srcRoot, assetsRel)); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
				copyFile(filepath.Join(assetsRel, e.Name()))
			}
		}
	}
	copyFile(filepath.Join("app", "out", "vs", "workbench", "windsurf-chat-client", "index.js"))
	copyFile(filepath.Join("app", "extensions", "windsurf", "dist", "extension.js"))

	version, commit := DevinInfo(exe)
	report := PatchReport{DevinFound: true, DevinVersion: version, DevinCommit: commit, At: "实测"}
	for _, step := range applySteps() {
		changed, msg := step.fn(root)
		status := classifyPatch(changed, msg)
		report.Features = append(report.Features, PatchFeature{Key: step.key, Name: step.name, Status: status, Detail: msg})
		report.Total++
		switch status {
		case PatchOK:
			report.OKCount++
		case PatchMismatch:
			report.MismatchCount++
		}
	}
	// 确认新代码确实把上下文圈和倍率改成了「认结构」的形态。
	if assetEntries, err := os.ReadDir(filepath.Join(root, assetsRel)); err == nil {
		for _, e := range assetEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(root, assetsRel, e.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(string(body), "contextLimit:bailanCtx") {
				t.Log("上下文圈：已套用正则版 contextLimit:bailanCtx")
			}
			if strings.Contains(string(body), "key:`rate`") {
				t.Log("倍率角标：已套用 key:`rate`")
			}
		}
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	out := filepath.Join(os.TempDir(), "bailan_report.json")
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("报告已写到 %s\n%s", out, string(raw))
}
