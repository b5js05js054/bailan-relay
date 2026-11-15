package devin

import (
	"regexp"
	"strings"
)

// 图标行的「认结构、放过名字」匹配（Agent 侧列表项图标）。
//
// 各版本结构一致，只有 jsx 名和「厂商图标函数」名在变；那串 className
// `flex size-[18px] shrink-0 items-center justify-center` 是固定锚点。
// 我们用同一套构造函数生成新/旧两种形态，套用/还原时都校验「重建结果 == 实际匹配」，
// 捕获错了就不动，避免改坏文件。
func iconRowOldStr(item, jsx, fn string) string {
	return "(" + item + ".icon||" + item + ".provider)&&(0," + jsx +
		".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:" +
		item + ".icon??" + fn + "(" + item + ".provider)})"
}

func iconRowNewStr(item, jsx, fn string) string {
	lookup := "bailanSrc({modelUid:" + item + ".modelUid,label:" + item + ".label})"
	return "(" + item + ".icon||" + item + ".provider||" + lookup + ")&&(0," + jsx +
		".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:" +
		item + ".icon??" + fn + "(" + item + ".provider)??(" + lookup + "?(0," + jsx +
		".jsx)(`img`,{src:" + lookup + ",alt:``,draggable:!1,width:16,height:16,style:{display:`block`,objectFit:`contain`}}):null)})"
}

var (
	iconRowOldRe = regexp.MustCompile(
		regexp.QuoteMeta("(") + `(\w+)` + regexp.QuoteMeta(".icon||") + `\w+` +
			regexp.QuoteMeta(".provider)&&(0,") + `(\w+)` +
			regexp.QuoteMeta(".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:") +
			`\w+` + regexp.QuoteMeta(".icon??") + `(\w+)` + regexp.QuoteMeta("(") + `\w+` +
			regexp.QuoteMeta(".provider)})"),
	)
	iconRowNewRe = regexp.MustCompile(
		regexp.QuoteMeta("(") + `(\w+)` + regexp.QuoteMeta(".icon||") + `\w+` +
			regexp.QuoteMeta(".provider||bailanSrc({modelUid:") + `\w+` + regexp.QuoteMeta(".modelUid,label:") +
			`\w+` + regexp.QuoteMeta(".label}))&&(0,") + `(\w+)` +
			regexp.QuoteMeta(".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:") +
			`\w+` + regexp.QuoteMeta(".icon??") + `(\w+)` + regexp.QuoteMeta("(") + `\w+` +
			regexp.QuoteMeta(".provider)??(bailanSrc({modelUid:") + `\w+` + regexp.QuoteMeta(".modelUid,label:") +
			`\w+` + regexp.QuoteMeta(".label})?(0,") + `\w+` +
			regexp.QuoteMeta(".jsx)(`img`,{src:bailanSrc({modelUid:") + `\w+` + regexp.QuoteMeta(".modelUid,label:") +
			`\w+` + regexp.QuoteMeta(".label}),alt:``,draggable:!1,width:16,height:16,style:{display:`block`,objectFit:`contain`}}):null)})"),
	)
)

const iconRowMark = "||bailanSrc({modelUid:"

// tryAgentIconRowApply 把图标行改成带 bailanSrc 兜底，并在厂商图标函数前插入 bailanSrc 定义。
// 返回：新文本、是否改动、是否对不上。
func tryAgentIconRowApply(text string) (string, bool, bool) {
	src, err := bailanSrcFunction()
	if err != nil {
		return text, false, true
	}
	if strings.Contains(text, iconRowMark) {
		return text, false, false // 已经套过
	}
	matches := iconRowOldRe.FindAllStringSubmatch(text, 2)
	if len(matches) == 0 {
		return text, false, false
	}
	if len(matches) > 1 {
		return text, false, true
	}
	m := matches[0]
	item, jsx, fn := m[1], m[2], m[3]
	// 校验：用捕获的名字重建的旧行必须和实际匹配到的一模一样，否则捕获有误，不动。
	if iconRowOldStr(item, jsx, fn) != m[0] {
		return text, false, true
	}
	out := strings.Replace(text, m[0], iconRowNewStr(item, jsx, fn), 1)
	// 在厂商图标函数定义前插入 bailanSrc（卡片/列表那几处也靠它）。
	anchor := "function " + fn + "(e,t=16){if(e===void 0)return null;"
	if !strings.Contains(out, "function bailanSrc(e){") {
		if strings.Count(out, anchor) != 1 {
			return text, false, true
		}
		out = strings.Replace(out, anchor, src+anchor, 1)
	}
	return out, true, false
}

// ---------- 列表/当前模型那处图标（3.12.28 的 S 版渲染）----------
//
// 结构：n=S.icon??(t?FN2(e.modelUid)??FN(S.provider):FN(S.provider));return n?(0,JSX.jsx)(span,{...children:n}):null
// 自带 jsx 工厂，和图标行一样用「构造函数 + 校验重建==匹配」的办法，放过压缩名字。
func iconListOldStr(s, e, fn2, fn, jsx string) string {
	return "n=" + s + ".icon??(t?" + fn2 + "(" + e + ".modelUid)??" + fn + "(" + s + ".provider):" + fn +
		"(" + s + ".provider));return n?(0," + jsx +
		".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:n}):null"
}

func iconListNewStr(s, e, fn2, fn, jsx string) string {
	lookup := "bailanSrc({modelUid:" + s + ".modelUid,label:" + s + ".label})"
	img := "(" + lookup + "?(0," + jsx + ".jsx)(`img`,{src:" + lookup +
		",alt:``,draggable:!1,width:16,height:16,style:{display:`block`,objectFit:`contain`}}):null)"
	return "n=" + s + ".icon??(t?" + fn2 + "(" + e + ".modelUid)??" + fn + "(" + s + ".provider):" + fn +
		"(" + s + ".provider))??" + img + ";return n?(0," + jsx +
		".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:n}):null"
}

var (
	iconListOldRe = regexp.MustCompile(
		regexp.QuoteMeta("n=") + `(\w+)` + regexp.QuoteMeta(".icon??(t?") + `(\w+)` + regexp.QuoteMeta("(") +
			`(\w+)` + regexp.QuoteMeta(".modelUid)??") + `(\w+)` + regexp.QuoteMeta("(") + `\w+` +
			regexp.QuoteMeta(".provider):") + `\w+` + regexp.QuoteMeta("(") + `\w+` +
			regexp.QuoteMeta(".provider));return n?(0,") + `(\w+)` +
			regexp.QuoteMeta(".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:n}):null"),
	)
	iconListNewRe = regexp.MustCompile(
		regexp.QuoteMeta("n=") + `(\w+)` + regexp.QuoteMeta(".icon??(t?") + `(\w+)` + regexp.QuoteMeta("(") +
			`(\w+)` + regexp.QuoteMeta(".modelUid)??") + `(\w+)` + regexp.QuoteMeta("(") + `\w+` +
			regexp.QuoteMeta(".provider):") + `\w+` + regexp.QuoteMeta("(") + `\w+` +
			regexp.QuoteMeta(".provider))??(bailanSrc({modelUid:") + `\w+` + regexp.QuoteMeta(".modelUid,label:") +
			`\w+` + regexp.QuoteMeta(".label})?(0,") + `(\w+)` +
			regexp.QuoteMeta(".jsx)(`img`,{src:bailanSrc({modelUid:") + `\w+` + regexp.QuoteMeta(".modelUid,label:") +
			`\w+` + regexp.QuoteMeta(".label}),alt:``,draggable:!1,width:16,height:16,style:{display:`block`,objectFit:`contain`}}):null);return n?(0,") +
			`\w+` + regexp.QuoteMeta(".jsx)(`span`,{className:`flex size-[18px] shrink-0 items-center justify-center`,children:n}):null"),
	)
)

const iconListMark = ".provider))??(bailanSrc({modelUid:"

func tryAgentIconListApply(text string) (string, bool, bool) {
	if strings.Contains(text, iconListMark) {
		return text, false, false // 已经套过
	}
	matches := iconListOldRe.FindAllStringSubmatch(text, 2)
	if len(matches) == 0 {
		return text, false, false
	}
	if len(matches) > 1 {
		return text, false, true
	}
	m := matches[0]
	s, fn2, e, fn, jsx := m[1], m[2], m[3], m[4], m[5]
	if iconListOldStr(s, e, fn2, fn, jsx) != m[0] {
		return text, false, true
	}
	return strings.Replace(text, m[0], iconListNewStr(s, e, fn2, fn, jsx), 1), true, false
}

func tryAgentIconListRestore(text string) (string, bool) {
	if !strings.Contains(text, iconListMark) {
		return text, false
	}
	matches := iconListNewRe.FindAllStringSubmatch(text, 2)
	if len(matches) != 1 {
		return text, false
	}
	m := matches[0]
	s, fn2, e, fn, jsx := m[1], m[2], m[3], m[4], m[5]
	if iconListNewStr(s, e, fn2, fn, jsx) != m[0] {
		return text, false
	}
	return strings.Replace(text, m[0], iconListOldStr(s, e, fn2, fn, jsx), 1), true
}

// tryAgentIconRowRestore 还原图标行并移除 bailanSrc 定义。
func tryAgentIconRowRestore(text string) (string, bool) {
	if !strings.Contains(text, iconRowMark) {
		return text, false
	}
	matches := iconRowNewRe.FindAllStringSubmatch(text, 2)
	if len(matches) != 1 {
		return text, false
	}
	m := matches[0]
	item, jsx, fn := m[1], m[2], m[3]
	if iconRowNewStr(item, jsx, fn) != m[0] {
		return text, false
	}
	out := strings.Replace(text, m[0], iconRowOldStr(item, jsx, fn), 1)
	if src, err := bailanSrcFunction(); err == nil {
		out = strings.Replace(out, src, "", 1)
	}
	return out, true
}
