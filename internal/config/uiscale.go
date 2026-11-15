package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultUIScalePercent = 100

// LoadUIScalePercent 读取界面缩放（80–120），用于启动时窗口尺寸。
func LoadUIScalePercent() int {
	path := filepath.Join(AppDir(), "ui-scale.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		return defaultUIScalePercent
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return defaultUIScalePercent
	}
	return NormalizeUIScalePercent(n)
}

// SaveUIScalePercent 持久化界面缩放比例。
func SaveUIScalePercent(percent int) error {
	percent = NormalizeUIScalePercent(percent)
	if err := os.MkdirAll(AppDir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(AppDir(), "ui-scale.txt")
	return os.WriteFile(path, []byte(strconv.Itoa(percent)), 0o644)
}

// NormalizeUIScalePercent 限制在可选档位内。
func NormalizeUIScalePercent(percent int) int {
	switch percent {
	case 80, 90, 100, 110, 120:
		return percent
	}
	if percent < 85 {
		return 80
	}
	if percent < 95 {
		return 90
	}
	if percent <= 105 {
		return 100
	}
	if percent <= 115 {
		return 110
	}
	return 120
}
