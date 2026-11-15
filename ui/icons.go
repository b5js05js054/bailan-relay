package ui

import "embed"

// Icons 是 Hub 页面上的厂商图标。Devin 没有对应厂商时，列表行用这里的图。
//
//go:embed icons/*.svg
var Icons embed.FS
