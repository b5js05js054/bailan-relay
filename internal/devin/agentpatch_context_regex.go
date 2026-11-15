package devin

import (
	"regexp"
	"strings"
)

// 上下文圈的「认结构、放过名字」匹配。
//
// 各版本原文结构一致，只有压缩名字在变：
//
//	function Dm(){let e=Ku(e=>e.status.contextUsage);return e?(0,Om.jsx)(Ui,{tokensUsed:e.used,contextLimit:e.size}):null}
//	function ed(){let e=_r(e=>e.status.contextUsage);return e?(0,td.jsx)(Hr,{tokensUsed:e.used,contextLimit:e.size}):null}
//
// 用稳定的 .status.contextUsage / tokensUsed / contextLimit 定位，把 Dm/Ku/Om/Ui 这些
// 易变名字用捕获组放过。Devin 只是重新压缩（结构没变）的新版本，不用再手动找锚点。
var contextRingRe = regexp.MustCompile(
	`function (\w+)\(\)\{let (\w+)=(\w+)\((\w+)=>(\w+)\.status\.contextUsage\);return (\w+)\?\(0,(\w+)\.jsx\)\((\w+),\{tokensUsed:(\w+)\.used,contextLimit:(\w+)\.size\}\):null\}`,
)

// contextRingAppliedRe 匹配套过补丁后的形态，用于还原。bailanCtx 是我们注入的本地变量名，稳定可认。
var contextRingAppliedRe = regexp.MustCompile(
	`function (\w+)\(\)\{let (\w+)=(\w+)\((\w+)=>\w+\.status\.contextUsage\);if\(!\w+\)return null;let bailanCtx=\(\(\)=>\{let s=typeof globalThis\.bailanLast=="string"\?globalThis\.bailanLast:"";let p=s\.split\("@"\);let c=p\.length===3\?Number\(p\[2\]\):0;return c>0\?c:\w+\.size\}\)\(\);return\(0,(\w+)\.jsx\)\((\w+),\{tokensUsed:\w+\.used,contextLimit:bailanCtx\}\)\}`,
)

// contextRingMark 是套过补丁的标记，命中即视为已生效。
const contextRingMark = "contextLimit:bailanCtx"

// tryContextRingApply 用稳定结构套上上下文圈补丁。
// 返回：新文本、是否改动、是否对不上（结构在但捕获不一致，或出现多次）。
// 返回 (text,false,false) 表示这份文件里没有这段原文，交给调用方退回按版本的精确锚点。
func tryContextRingApply(text string) (string, bool, bool) {
	if strings.Contains(text, contextRingMark) {
		return text, false, false // 已经套过（正则版）
	}
	matches := contextRingRe.FindAllStringSubmatch(text, 2)
	if len(matches) == 0 {
		return text, false, false
	}
	if len(matches) > 1 {
		return text, false, true // 出现多次，宁可不动
	}
	m := matches[0]
	name, u, hook, p, pUse, uCheck, jsx, comp, uUsed, uSize :=
		m[1], m[2], m[3], m[4], m[5], m[6], m[7], m[8], m[9], m[10]
	// 压缩名字可变，但同一函数里对同一变量的引用必须一致，否则这段不是我们要改的那处。
	if !(u == uCheck && u == uUsed && u == uSize && p == pUse) {
		return text, false, true
	}
	orig := m[0]
	replacement := "function " + name + "(){let " + u + "=" + hook + "(" + p + "=>" + p +
		".status.contextUsage);if(!" + u + ")return null;let bailanCtx=(()=>{let s=typeof globalThis.bailanLast==\"string\"?globalThis.bailanLast:\"\";let p=s.split(\"@\");let c=p.length===3?Number(p[2]):0;return c>0?c:" + u +
		".size})();return(0," + jsx + ".jsx)(" + comp + ",{tokensUsed:" + u + ".used,contextLimit:bailanCtx})}"
	return strings.Replace(text, orig, replacement, 1), true, false
}

// tryContextRingRestore 把正则版补丁还原回原文。返回新文本与是否改动。
func tryContextRingRestore(text string) (string, bool) {
	if !strings.Contains(text, contextRingMark) {
		return text, false
	}
	matches := contextRingAppliedRe.FindAllStringSubmatch(text, 2)
	if len(matches) != 1 {
		return text, false
	}
	m := matches[0]
	name, u, hook, p, jsx, comp := m[1], m[2], m[3], m[4], m[5], m[6]
	orig := m[0]
	restored := "function " + name + "(){let " + u + "=" + hook + "(" + p + "=>" + p +
		".status.contextUsage);return " + u + "?(0," + jsx + ".jsx)(" + comp +
		",{tokensUsed:" + u + ".used,contextLimit:" + u + ".size}):null}"
	return strings.Replace(text, orig, restored, 1), true
}
