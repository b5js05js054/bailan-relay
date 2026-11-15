package devin

import (
	"os"
	"regexp"
	"strings"
)

// 倍率角标的「认结构、放过名字」匹配（Agent 侧）。
//
// 各版本原文结构一致，只有压缩名字和组件名在变。我们只动两处，放过中间所有易变名字：
//  1. 开头的 `i=[];return e.isBeta?i.push`：在 i=[] 之后插入一条「倍率」角标 push。
//  2. map 里的 `config:e,promo:n===0?r:void 0`：改成遇到倍率角标时不显示官方 promo。
//
// 中间那段（beta/new/fast 的 push、各组件名）原样不动，因此 Devin 重新压缩换名字也不受影响。
var (
	// i=[];return e.isBeta?i.push
	rateAgentAccRe = regexp.MustCompile(
		`(\w+)` + regexp.QuoteMeta("=[];return e.isBeta?") + `(\w+)` + regexp.QuoteMeta(".push"),
	)
	// config:e,promo:n===0?r:void 0
	rateAgentPromoRe = regexp.MustCompile(
		regexp.QuoteMeta("config:") + `(\w+)` + regexp.QuoteMeta(",promo:") + `(\w+)` +
			regexp.QuoteMeta("===0?") + `(\w+)` + regexp.QuoteMeta(":void 0"),
	)
	// 套过补丁后的 promo：config:e,promo:e.key===`rate`?void 0:n===0?r:void 0
	rateAgentAppliedPromoRe = regexp.MustCompile(
		regexp.QuoteMeta("config:") + `(\w+)` + regexp.QuoteMeta(",promo:") + `(\w+)` +
			regexp.QuoteMeta(".key===`rate`?void 0:") + `(\w+)` + regexp.QuoteMeta("===0?") +
			`(\w+)` + regexp.QuoteMeta(":void 0"),
	)
	// 套过补丁后插入的那条 push：r&&r.label&&i.push({key:`rate`,label:r.label,className:`text-text-accent-primary`});
	rateAgentPushRe = regexp.MustCompile(
		`(\w+)` + regexp.QuoteMeta("&&") + `(\w+)` + regexp.QuoteMeta(".label&&") + `(\w+)` +
			regexp.QuoteMeta(".push({key:`rate`,label:") + `(\w+)` +
			regexp.QuoteMeta(".label,className:`text-text-accent-primary`});"),
	)
)

// rateAgentMark 是套过补丁的标记（反引号包着 rate，和 Agent 侧一致；普通对话用双引号，互不相干）。
const rateAgentMark = "key:`rate`"

// tryAgentRateApply 用稳定结构套上 Agent 倍率角标。
// 返回：新文本、是否改动、是否对不上（结构残缺/捕获不一致）。
func tryAgentRateApply(text string) (string, bool, bool) {
	if strings.Contains(text, rateAgentMark) {
		return text, false, false // 已经套过
	}
	pm := rateAgentPromoRe.FindAllStringSubmatch(text, 2)
	am := rateAgentAccRe.FindAllStringSubmatch(text, 2)
	if len(pm) == 0 && len(am) == 0 {
		return text, false, false // 这份文件没有倍率角标那段
	}
	if len(pm) != 1 || len(am) != 1 {
		return text, false, true // 残缺或出现多次，宁可不动
	}
	cfg, idx, src := pm[0][1], pm[0][2], pm[0][3]
	acc1, acc2 := am[0][1], am[0][2]
	if acc1 != acc2 {
		return text, false, true
	}
	acc := acc1

	accOld := am[0][0]
	accNew := acc + "=[];" + src + "&&" + src + ".label&&" + acc +
		".push({key:`rate`,label:" + src + ".label,className:`text-text-accent-primary`});return e.isBeta?" + acc + ".push"

	promoOld := pm[0][0]
	promoNew := "config:" + cfg + ",promo:" + cfg + ".key===`rate`?void 0:" + idx + "===0?" + src + ":void 0"

	out := strings.Replace(text, accOld, accNew, 1)
	out = strings.Replace(out, promoOld, promoNew, 1)
	return out, true, false
}

// tryAgentRateRestore 把正则版倍率角标还原回原文。
func tryAgentRateRestore(text string) (string, bool) {
	if !strings.Contains(text, rateAgentMark) {
		return text, false
	}
	pm := rateAgentAppliedPromoRe.FindAllStringSubmatch(text, 2)
	push := rateAgentPushRe.FindAllStringSubmatch(text, 2)
	if len(pm) != 1 || len(push) != 1 {
		return text, false
	}
	cfg, idx, src := pm[0][1], pm[0][3], pm[0][4]
	promoBack := "config:" + cfg + ",promo:" + idx + "===0?" + src + ":void 0"

	out := strings.Replace(text, push[0][0], "", 1)
	out = strings.Replace(out, pm[0][0], promoBack, 1)
	return out, true
}

// patchAgentRateFile 对单个 Agent 资源文件套倍率角标：先走正则，再退回按版本精确锚点。
func patchAgentRateFile(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(body)
	if out, changed, _ := tryAgentRateApply(text); changed {
		return os.WriteFile(path, []byte(out), 0o644) == nil
	}
	for _, pair := range [][2]string{
		{agentRate312Old(), agentRate312New()},
		{agentRate238Old(), agentRate238New()},
		{agentRateOld(), agentRateNew()},
	} {
		if strings.Count(text, pair[0]) == 1 {
			text = strings.Replace(text, pair[0], pair[1], 1)
			return os.WriteFile(path, []byte(text), 0o644) == nil
		}
	}
	return false
}

// ---------- 普通对话（windsurf-chat-client）侧的倍率角标 ----------
//
// 结构和 Agent 一样，只有两点不同：promo 写成 0===r（不是 r===0），key/className 用双引号。
// 累加器那一处 `a=[];return e.isBeta?a.push` 和 Agent 完全相同，直接复用 rateAgentAccRe。
var (
	// config:e,promo:0===r?i:void 0
	rateChatPromoRe = regexp.MustCompile(
		regexp.QuoteMeta("config:") + `(\w+)` + regexp.QuoteMeta(",promo:0===") + `(\w+)` +
			regexp.QuoteMeta("?") + `(\w+)` + regexp.QuoteMeta(":void 0"),
	)
	// 套过补丁后的 promo：config:e,promo:e.key==="rate"?void 0:0===r?i:void 0
	rateChatAppliedPromoRe = regexp.MustCompile(
		regexp.QuoteMeta("config:") + `(\w+)` + regexp.QuoteMeta(",promo:") + `(\w+)` +
			regexp.QuoteMeta(`.key==="rate"?void 0:0===`) + `(\w+)` + regexp.QuoteMeta("?") +
			`(\w+)` + regexp.QuoteMeta(":void 0"),
	)
	// 套过补丁后插入的那条 push：i&&i.label&&a.push({key:"rate",label:i.label,className:"text-text-accent-primary"});
	rateChatPushRe = regexp.MustCompile(
		`(\w+)` + regexp.QuoteMeta("&&") + `(\w+)` + regexp.QuoteMeta(".label&&") + `(\w+)` +
			regexp.QuoteMeta(`.push({key:"rate",label:`) + `(\w+)` +
			regexp.QuoteMeta(`.label,className:"text-text-accent-primary"});`),
	)
)

// rateChatMark 是普通对话侧套过补丁的标记（双引号包着 rate）。
const rateChatMark = `key:"rate"`

func tryChatRateApply(text string) (string, bool, bool) {
	if strings.Contains(text, rateChatMark) {
		return text, false, false
	}
	pm := rateChatPromoRe.FindAllStringSubmatch(text, 2)
	am := rateAgentAccRe.FindAllStringSubmatch(text, 2)
	if len(pm) == 0 && len(am) == 0 {
		return text, false, false
	}
	if len(pm) != 1 || len(am) != 1 {
		return text, false, true
	}
	cfg, idx, src := pm[0][1], pm[0][2], pm[0][3]
	acc1, acc2 := am[0][1], am[0][2]
	if acc1 != acc2 {
		return text, false, true
	}
	acc := acc1

	accOld := am[0][0]
	accNew := acc + "=[];" + src + "&&" + src + ".label&&" + acc +
		`.push({key:"rate",label:` + src + `.label,className:"text-text-accent-primary"});return e.isBeta?` + acc + ".push"

	promoOld := pm[0][0]
	promoNew := "config:" + cfg + ",promo:" + cfg + `.key==="rate"?void 0:0===` + idx + "?" + src + ":void 0"

	out := strings.Replace(text, accOld, accNew, 1)
	out = strings.Replace(out, promoOld, promoNew, 1)
	return out, true, false
}

func tryChatRateRestore(text string) (string, bool) {
	if !strings.Contains(text, rateChatMark) {
		return text, false
	}
	pm := rateChatAppliedPromoRe.FindAllStringSubmatch(text, 2)
	push := rateChatPushRe.FindAllStringSubmatch(text, 2)
	if len(pm) != 1 || len(push) != 1 {
		return text, false
	}
	cfg, idx, src := pm[0][1], pm[0][3], pm[0][4]
	promoBack := "config:" + cfg + ",promo:0===" + idx + "?" + src + ":void 0"

	out := strings.Replace(text, push[0][0], "", 1)
	out = strings.Replace(out, pm[0][0], promoBack, 1)
	return out, true
}

// patchChatRateFile 对普通对话的 index.js 套倍率角标：先走正则，再退回按版本精确锚点。
func patchChatRateFile(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(body)
	if out, changed, _ := tryChatRateApply(text); changed {
		return os.WriteFile(path, []byte(out), 0o644) == nil
	}
	for _, pair := range [][2]string{
		{chatRate238Old, chatRate238New},
		{chatRate126Old, chatRate126New},
		{chatRateOld, chatRateNew},
	} {
		if strings.Count(text, pair[0]) == 1 {
			text = strings.Replace(text, pair[0], pair[1], 1)
			return os.WriteFile(path, []byte(text), 0o644) == nil
		}
	}
	return false
}

func patchChatRateRestoreFile(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(body)
	if out, changed := tryChatRateRestore(text); changed {
		return os.WriteFile(path, []byte(out), 0o644) == nil
	}
	for _, pair := range [][2]string{
		{chatRate238New, chatRate238Old},
		{chatRate126New, chatRate126Old},
		{chatRateNew, chatRateOld},
	} {
		if strings.Count(text, pair[0]) == 1 {
			text = strings.Replace(text, pair[0], pair[1], 1)
			return os.WriteFile(path, []byte(text), 0o644) == nil
		}
	}
	return false
}

// patchAgentRateRestoreFile 对单个 Agent 资源文件还原倍率角标：先走正则，再退回精确锚点。
func patchAgentRateRestoreFile(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(body)
	if out, changed := tryAgentRateRestore(text); changed {
		return os.WriteFile(path, []byte(out), 0o644) == nil
	}
	for _, pair := range [][2]string{
		{agentRate312New(), agentRate312Old()},
		{agentRate238New(), agentRate238Old()},
		{agentRateNew(), agentRateOld()},
	} {
		if strings.Count(text, pair[0]) == 1 {
			text = strings.Replace(text, pair[0], pair[1], 1)
			return os.WriteFile(path, []byte(text), 0o644) == nil
		}
	}
	return false
}
