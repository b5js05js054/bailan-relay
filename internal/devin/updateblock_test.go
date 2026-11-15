package devin

import (
	"strings"
	"testing"
)

func TestApplyUpdateModeInsertsAndRemoves(t *testing.T) {
	src := "{\n    \"devin.acp.preferredAgent\": \"devin-cli\"\n}\n"
	blocked := applyUpdateMode(src, true)
	if !strings.Contains(blocked, `"update.mode": "none"`) || !strings.Contains(blocked, `"devin.acp.preferredAgent": "devin-cli"`) {
		t.Fatalf("insert failed: %s", blocked)
	}
	again := applyUpdateMode(blocked, true)
	if again != blocked {
		t.Fatal("second block changed the file")
	}
	opened := applyUpdateMode(blocked, false)
	if strings.Contains(opened, "update.mode") {
		t.Fatalf("remove failed: %s", opened)
	}
	if !strings.Contains(opened, `"devin.acp.preferredAgent": "devin-cli"`) {
		t.Fatalf("other settings lost: %s", opened)
	}
}

func TestApplyUpdateModeKeepsForeignValue(t *testing.T) {
	src := "{\n    \"update.mode\": \"manual\"\n}\n"
	if got := applyUpdateMode(src, false); got != src {
		t.Fatalf("foreign value changed: %s", got)
	}
	got := applyUpdateMode(src, true)
	if !strings.Contains(got, `"update.mode": "none"`) || strings.Contains(got, `"manual"`) {
		t.Fatalf("replace failed: %s", got)
	}
}
