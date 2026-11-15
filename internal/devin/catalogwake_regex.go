package devin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 叫醒的「认结构、放过名字」匹配。
//
// 真实 extension.js 里，10 秒重试长这样（名字每版会变）：
//
//	REFETCH(e,t).catch(()=>{RETRY(e,t)})},1e4)}async function NEXT(e,t){const n=PROVIDER.MetadataProvider.getInstance()...
//
// MetadataProvider 是固定类名；靠这段结构一次捕获「重新拉目录的函数 REFETCH」「后面的函数名 NEXT」
// 「持有 MetadataProvider 的变量 PROVIDER」，就能把叫醒块注入进去，不依赖压缩名字。
// 还原沿用 stripCatalogWake（认我们自己的 bailanArm 标记），与版本无关。
var wakeRe = regexp.MustCompile(
	`(\w+)\(e,t\)\.catch\(\(\)=>\{\w+\(e,t\)\}\)\},1e4\)\}async function (\w+)\(e,t\)\{const \w+=(\w+)\.MetadataProvider`,
)

func wakeArmBlock(provider, refetch string) string {
	return "function bailanArm(seen){if(bailanBusy)return;let e;try{e=" + provider +
		".MetadataProvider.getInstance().getMetadata()}catch(err){setTimeout(()=>bailanArm(seen),1000);return}" +
		"if(!e||!e.apiKey){setTimeout(()=>bailanArm(seen),1000);return}" +
		"let body=seen<0?\"[]\":JSON.stringify([seen]);" +
		"fetch(\"http://127.0.0.1:5099/api/catalog_wait\",{method:\"POST\",body:body}).then(n=>n.json()).then(n=>{" +
		"let r=n&&n.epoch;if(typeof r!=\"number\"){setTimeout(()=>bailanArm(seen),2000);return}" +
		"if(seen<0||r===seen){bailanArm(r);return}bailanBusy=!0;" + refetch +
		"(e,0).catch(()=>{}).finally(()=>{bailanBusy=!1;bailanArm(r)})}).catch(()=>setTimeout(()=>bailanArm(seen),2000))}" +
		"let bailanBusy=!1;bailanArm(-1);"
}

// tryCatalogWakeApply 用稳定结构注入叫醒块。返回：新文本、是否改动、是否对不上。
func tryCatalogWakeApply(text string) (string, bool, bool) {
	if strings.Contains(text, "api/catalog_wait") {
		return text, false, false // 已经接上
	}
	matches := wakeRe.FindAllStringSubmatch(text, 2)
	if len(matches) == 0 {
		return text, false, false
	}
	if len(matches) > 1 {
		return text, false, true
	}
	m := matches[0]
	refetch, next, provider := m[1], m[2], m[3]
	anchor := "},1e4)}async function " + next + "("
	if strings.Count(text, anchor) != 1 {
		return text, false, true
	}
	replacement := "},1e4)};" + wakeArmBlock(provider, refetch) + "async function " + next + "("
	return strings.Replace(text, anchor, replacement, 1), true, false
}

// applyCatalogWakeRegex 对 extension.js 套叫醒正则补丁（apply 用）。
func applyCatalogWakeRegex(resources string) (changed bool, mismatch bool) {
	path := filepath.Join(resources, "app", "extensions", "windsurf", "dist", "extension.js")
	body, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	out, ok, bad := tryCatalogWakeApply(string(body))
	if bad {
		return false, true
	}
	if !ok {
		return false, false
	}
	return os.WriteFile(path, []byte(out), 0o644) == nil, false
}
