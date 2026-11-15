package devin

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const updateModeKey = `"update.mode"`

// UserSettingsPath 是 Devin 读取的用户设置。Windows 在 %APPDATA%\Devin\User。
func UserSettingsPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Devin", "User", "settings.json")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "Devin", "User", "settings.json")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "Devin", "User", "settings.json")
	}
}

// SetUpdateBlocked 把 Devin 的 update.mode 写成 none，或在关闭限制时删掉我们写上的 none。
// 只动这一项，其余设置保持原样。Devin 下次启动时才读到。
func SetUpdateBlocked(block bool) error {
	path := UserSettingsPath()
	if runtime.GOOS == "windows" && strings.TrimSpace(os.Getenv("APPDATA")) == "" {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	next := applyUpdateMode(string(raw), block)
	if next == string(raw) {
		return nil
	}
	return os.WriteFile(path, []byte(next), 0o644)
}

var updateModeValue = regexp.MustCompile(`"update\.mode"\s*:\s*"([^"\\]*)"`)

// applyUpdateMode 在设置文本里写入或去掉 update.mode=none。
func applyUpdateMode(text string, block bool) string {
	if loc := updateModeValue.FindStringSubmatchIndex(text); loc != nil {
		current := text[loc[2]:loc[3]]
		if block {
			if current == "none" {
				return text
			}
			return text[:loc[2]] + "none" + text[loc[3]:]
		}
		if current != "none" {
			return text
		}
		return removeUpdateMode(text, loc)
	}
	if !block {
		return text
	}
	line := updateModeKey + `: "none"`
	if strings.TrimSpace(text) == "" {
		return "{\n    " + line + "\n}\n"
	}
	return insertBeforeLastBrace(text, line)
}

func removeUpdateMode(text string, loc []int) string {
	start := loc[0]
	end := loc[1]
	for start > 0 && (text[start-1] == ' ' || text[start-1] == '\t') {
		start--
	}
	if start > 0 && text[start-1] == '\n' {
		start--
		if start > 0 && text[start-1] == '\r' {
			start--
		}
	}
	if end < len(text) && text[end] == ',' {
		end++
	}
	for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
		end++
	}
	if end < len(text) && (text[end] == '\n' || text[end] == '\r') {
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			end += 2
		} else {
			end++
		}
	}
	return text[:start] + text[end:]
}

func insertBeforeLastBrace(text, line string) string {
	i := strings.LastIndex(text, "}")
	if i < 0 {
		return "{\n    " + line + "\n}\n"
	}
	head := strings.TrimRight(text[:i], " \t\r\n")
	switch {
	case head == "" || strings.HasSuffix(head, "{"):
		head += "\n    " + line + "\n"
	case strings.HasSuffix(head, ","):
		head += "\n    " + line + "\n"
	default:
		head += ",\n    " + line + "\n"
	}
	tail := text[i:]
	if !strings.HasSuffix(tail, "\n") {
		tail += "\n"
	}
	return head + tail
}
