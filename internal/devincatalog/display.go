package devincatalog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"bailan-relay/internal/config"
)

// 和界面里的官方展示名同一套规则：品牌大小写、版本号用点，请求 ID 仍是原来的。
var (
	brandName = map[string]string{
		"gpt": "GPT", "openai": "OpenAI", "claude": "Claude", "deepseek": "DeepSeek",
		"glm": "GLM", "chatglm": "ChatGLM", "kimi": "Kimi", "mimo": "MiMo",
		"gemini": "Gemini", "grok": "Grok", "qwen": "Qwen", "nemotron": "Nemotron", "longcat": "LongCat",
	}
	dateSuffix8 = regexp.MustCompile(`-20\d{6}$`)
	dateSuffix6 = regexp.MustCompile(`-\d{6}$`)
	effortOrder = map[string]int{
		"none": 0, "low": 1, "medium": 2, "high": 3, "xhigh": 4, "max": 5, "ultra": 6, "minimal": 7,
	}
	effortTitle = map[string]string{
		"none": "None", "minimal": "Minimal", "low": "Low", "medium": "Medium",
		"high": "High", "xhigh": "XHigh", "max": "Max", "ultra": "Ultra",
	}
)

type familyEntry struct {
	Key     string
	Order   int
	Name    string
	Control int
}

type costLine struct {
	Label string
	Value float64
	Min   float64
	Max   float64
	Info  string
}

func officialName(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = dateSuffix8.ReplaceAllString(id, "")
	id = dateSuffix6.ReplaceAllString(id, "")
	parts := make([]string, 0)
	for _, part := range strings.Split(id, "-") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) > 0 && parts[len(parts)-1] == "free" {
		parts = parts[:len(parts)-1]
	}
	words := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if i+1 < len(parts) && allDigits(part) && allDigits(parts[i+1]) && len(parts[i+1]) <= 2 {
			words = append(words, part+"."+parts[i+1])
			i++
			continue
		}
		switch {
		case brandName[part] != "":
			words = append(words, brandName[part])
		case len(part) > 1 && (part[0] == 'v' || part[0] == 'V') && part[1] >= '0' && part[1] <= '9':
			words = append(words, "V"+part[1:])
		case len(part) > 1 && (part[0] == 'k' || part[0] == 'K') && part[1] >= '0' && part[1] <= '9':
			words = append(words, "K"+part[1:])
		case part == "vl" || part == "ai":
			words = append(words, strings.ToUpper(part))
		case part != "" && part[0] >= '0' && part[0] <= '9':
			words = append(words, raiseTail(part))
		default:
			words = append(words, titleWord(part))
		}
	}
	if len(words) == 0 {
		return id
	}
	return strings.Join(words, " ")
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func raiseTail(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= 'a' && s[i-1] <= 'z' {
		i--
	}
	if i == len(s) {
		return s
	}
	return s[:i] + strings.ToUpper(s[i:])
}

// rateBadge 是模型名字旁边的蓝色倍率。0 显示 Free，其余显示成 x1.3。
func rateBadge(multiplier float64) string {
	if multiplier <= 0 {
		return "Free"
	}
	text := strconv.FormatFloat(multiplier, 'f', 4, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	return "x" + text
}

func formatTokens(n int) string {
	if n >= 1_000_000 && n%100_000 == 0 {
		return strconv.Itoa(n/1_000_000) + "M"
	}
	if n >= 1000 && n%1000 == 0 {
		return strconv.Itoa(n/1000) + "K"
	}
	return strconv.Itoa(n)
}

func effortMeta(effort string, index int) (int, string) {
	if order, ok := effortOrder[effort]; ok {
		name := effortTitle[effort]
		if name == "" {
			name = titleWord(effort)
		}
		return order, name
	}
	return 100 + index, titleWord(effort)
}

// expandModels 把一个目录模型拆成 Devin 官方选择器能切换的档位。
// 思考强度是 Effort 步进，上下文是 Context 选项；同一家族只占列表里的一行。
func expandModels(models []config.Model) []preparedModel {
	seenID := map[string]bool{}
	var out []preparedModel
	for _, model := range models {
		id := strings.ToLower(strings.TrimSpace(model.ID))
		if id == "" || seenID[id] {
			continue
		}
		seenID[id] = true
		out = append(out, variantsOf(model, id)...)
	}
	disambiguate(out)
	return out
}

func variantsOf(model config.Model, id string) []preparedModel {
	label := officialName(id)
	efforts := cleanEfforts(model.ReasoningEfforts)
	contexts := cleanContexts(model.ContextOptions, model.ContextWindow)
	fallback := model.ContextWindow
	if fallback <= 0 {
		fallback = defaultContext
	}
	if len(contexts) == 0 {
		contexts = []int{fallback}
	}
	maxContext := contexts[0]
	for _, n := range contexts[1:] {
		if n > maxContext {
			maxContext = n
		}
	}
	defaultEffort := ""
	if len(efforts) > 0 {
		defaultEffort = efforts[0]
	}
	costs := priceLines(model, len(efforts) > 1)
	effortSlots := efforts
	if len(effortSlots) == 0 {
		effortSlots = []string{""}
	}
	var out []preparedModel
	for i, effort := range effortSlots {
		order, name := 0, ""
		if effort != "" && len(efforts) > 1 {
			order, name = effortMeta(effort, i)
		}
		for _, context := range contexts {
			entries := make([]familyEntry, 0, 2)
			if len(efforts) > 1 {
				entries = append(entries, familyEntry{Key: "Reasoning Effort", Order: order, Name: name, Control: 1})
			}
			if len(contexts) > 1 {
				entries = append(entries, familyEntry{Key: "Context", Order: context, Name: formatTokens(context), Control: 2})
			}
			maxOutput := model.MaxOutput
			if maxOutput <= 0 {
				maxOutput = defaultMaxOutput
			}
			if maxOutput > context {
				maxOutput = context
			}
			rowLabel := label
			if name != "" {
				rowLabel = label + " " + name
			}
			if len(contexts) > 1 {
				rowLabel = rowLabel + " " + formatTokens(context)
			}
			// 只有一档时也带上 @。否则本地 Agent 会收到它不认识的编号，选择直接失败。
			token := effort
			if token == "" {
				token = "-"
			}
			uid := fmt.Sprintf("%s@%s@%d", id, token, context)
			out = append(out, preparedModel{
				ID: uid, BaseID: id, Label: rowLabel, Context: context, MaxOutput: maxOutput,
				Thinking: model.SupportsThinking || (effort != "" && effort != "none"),
				Family:   label, Entries: entries, Costs: costs,
				Default:  effort == defaultEffort && context == maxContext,
				Rate:     rateBadge(model.CategoryMultiplier),
				Category: strings.TrimSpace(model.CategoryName),
			})
		}
	}
	return out
}

func cleanEfforts(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, effort := range in {
		effort = strings.ToLower(strings.TrimSpace(effort))
		if effort == "" || seen[effort] {
			continue
		}
		seen[effort] = true
		out = append(out, effort)
	}
	return out
}

func cleanContexts(in []int, window int) []int {
	var out []int
	seen := map[int]bool{}
	for _, n := range in {
		if n <= 0 || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if window > 0 && len(out) > 0 && !seen[window] {
		out = append(out, window)
	}
	return out
}

func priceLines(model config.Model, effortNote bool) []costLine {
	// Price* 已是后台下发的现价，这里不再乘 Ratio。
	if model.Free {
		return nil
	}
	in := model.PriceIn
	out := model.PriceOut
	cache := model.PriceCache
	if in <= 0 && out <= 0 && cache <= 0 {
		return nil
	}
	top := in
	if out > top {
		top = out
	}
	if cache > top {
		top = cache
	}
	if top <= 0 {
		top = 1
	}
	info := ""
	if effortNote {
		info = "Higher effort consumes more tokens"
	}
	return []costLine{
		{Label: "Input", Value: in, Min: 0, Max: top, Info: info},
		{Label: "Cached input", Value: cache, Min: 0, Max: top},
		{Label: "Output", Value: out, Min: 0, Max: top},
	}
}

// providerOf 按模型 ID 推断厂商，Devin 用它画模型名前面的图标。
// 编号对应 exa.codeium_common_pb.ModelProvider。
// 返回 0 表示 Devin 没有这个厂商的图标，列表里就不画，不再借 OpenAI 的标。
func providerOf(id string) int {
	id = strings.ToLower(id)
	switch {
	case strings.HasPrefix(id, "swe-"):
		return 1
	case strings.HasPrefix(id, "gpt"), strings.HasPrefix(id, "openai"), strings.HasPrefix(id, "o1"), strings.HasPrefix(id, "o3"), strings.HasPrefix(id, "o4"):
		return 2
	case strings.HasPrefix(id, "claude"), strings.HasPrefix(id, "anthropic"):
		return 3
	case strings.HasPrefix(id, "gemini"), strings.HasPrefix(id, "google"):
		return 4
	case strings.HasPrefix(id, "grok"), strings.HasPrefix(id, "xai"):
		return 5
	case strings.HasPrefix(id, "deepseek"):
		return 6
	case strings.HasPrefix(id, "kimi"), strings.HasPrefix(id, "moonshot"):
		return 7
	case strings.HasPrefix(id, "qwen"):
		return 8
	case strings.HasPrefix(id, "glm"), strings.HasPrefix(id, "chatglm"), strings.HasPrefix(id, "zhipu"), strings.HasPrefix(id, "zai"):
		return 9
	case strings.HasPrefix(id, "minimax"):
		return 10
	case strings.HasPrefix(id, "nvidia"):
		return 11
	case strings.HasPrefix(id, "llama"), strings.HasPrefix(id, "meta"):
		return 13
	default:
		return 0
	}
}

func disambiguate(models []preparedModel) {
	bases := map[string]map[string]bool{}
	for _, model := range models {
		if bases[model.Label] == nil {
			bases[model.Label] = map[string]bool{}
		}
		bases[model.Label][model.BaseID] = true
	}
	for i := range models {
		if len(bases[models[i].Label]) > 1 {
			models[i].Label = models[i].Label + " · " + models[i].BaseID
			models[i].Family = models[i].Label
		}
	}
}
