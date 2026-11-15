package devin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 适配结果状态：生效 / 需适配（对不上） / 不适用（这版没有这一处）。
const (
	PatchOK       = "ok"
	PatchMismatch = "mismatch"
	PatchNA       = "na"
)

// PatchFeature 是单项补丁在当前 Devin 上的适配结果。
type PatchFeature struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Status string `json:"status"` // ok | mismatch | na
	Detail string `json:"detail"`
}

// PatchReport 是一次整体适配的结果，供界面显示「适配状态」。
type PatchReport struct {
	DevinFound    bool           `json:"devin_found"`
	DevinVersion  string         `json:"devin_version"`
	DevinCommit   string         `json:"devin_commit"`
	At            string         `json:"at"`
	Features      []PatchFeature `json:"features"`
	OKCount       int            `json:"ok_count"`
	MismatchCount int            `json:"mismatch_count"`
	Total         int            `json:"total"`
}

// patchStep 把一项补丁和它面向用户的名字绑在一起。
type patchStep struct {
	key  string
	name string
	fn   func(string) (bool, string)
}

// applySteps 是启动时按顺序执行的全部补丁，顺序与名字集中在这里，界面报告按同一顺序显示。
func applySteps() []patchStep {
	return []patchStep{
		{"agent_list", "Agent 模型列表", applyAgentModelList},
		{"agent_sort", "Agent 排序菜单", applyAgentSortMenu},
		{"context", "上下文用量", applyAgentContextWindow},
		{"agent_icons", "Agent 图标", applyAgentIcons},
		{"require_model", "没选模型不能发送", applyRequireModel},
		{"chat_list", "普通对话列表", applyChatModelList},
		{"rate", "倍率角标", applyRateBadges},
		{"wake", "模型变化叫醒 Devin", applyCatalogWake},
	}
}

// classifyPatch 把单项补丁的 (是否改动, 说明) 归成三种状态。
// 关键是把「对不上/失败」判成需适配——这是提醒你 Devin 换版本、需要重新适配的信号。
func classifyPatch(changed bool, msg string) string {
	if changed {
		return PatchOK
	}
	switch {
	case strings.Contains(msg, "对不上"), strings.Contains(msg, "失败"):
		return PatchMismatch
	case strings.Contains(msg, "已经"):
		return PatchOK
	default:
		return PatchNA
	}
}

var (
	reportMu   sync.RWMutex
	lastReport PatchReport
)

// LastPatchReport 返回最近一次适配结果（界面读取用）。
func LastPatchReport() PatchReport {
	reportMu.RLock()
	defer reportMu.RUnlock()
	return lastReport
}

func setLastReport(r PatchReport) {
	reportMu.Lock()
	lastReport = r
	reportMu.Unlock()
}

// applyWithReport 执行全部补丁并产出结构化报告。
// 返回：wrote 是否有文件被改动；joined 合并后的中文说明（兼容旧调用）；report 结构化结果。
func applyWithReport(executable string) (wrote bool, joined string, report PatchReport) {
	report.At = time.Now().Format("2006-01-02 15:04:05")
	exe := DiscoverExecutable(executable)
	if exe == "" {
		setLastReport(report)
		return false, "未找到 Devin，客户端未改", report
	}
	report.DevinFound = true
	report.DevinVersion, report.DevinCommit = DevinInfo(exe)
	root := resourcesDir(exe)
	var messages []string
	for _, step := range applySteps() {
		changed, message := step.fn(root)
		status := classifyPatch(changed, message)
		report.Features = append(report.Features, PatchFeature{
			Key:    step.key,
			Name:   step.name,
			Status: status,
			Detail: message,
		})
		report.Total++
		switch status {
		case PatchOK:
			report.OKCount++
		case PatchMismatch:
			report.MismatchCount++
		}
		if message != "" {
			messages = append(messages, message)
		}
		if changed {
			wrote = true
		}
	}
	setLastReport(report)
	return wrote, strings.Join(messages, "；"), report
}

// DevinInfo 读取安装目录的 product.json，返回 Devin 版本号与提交号（读不到时为空）。
func DevinInfo(executable string) (version, commit string) {
	path := filepath.Join(resourcesDir(executable), "app", "product.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var info struct {
		WindsurfVersion string `json:"windsurfVersion"`
		Version         string `json:"version"`
		Commit          string `json:"commit"`
	}
	if json.Unmarshal(body, &info) != nil {
		return "", ""
	}
	v := info.WindsurfVersion
	if v == "" {
		v = info.Version
	}
	return v, info.Commit
}
