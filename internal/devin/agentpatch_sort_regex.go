package devin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 排序菜单的「认结构、放过名字」匹配。
//
// 各版本都是把当前项从排序选项里 filter 掉：()=>ARR.filter(P=>P!==X),[ARR,X]
// （agent 有 (0,ut.useMemo)/(0,ht.useMemo)/D(...) 几种写法，普通对话是 (0,tK.useMemo)）。
// 我们只把判断改成恒真（P!==X||!0），等于「全部显示」，原样可逆、不丢信息，换名字也认得出。
var (
	sortApplyRe = regexp.MustCompile(
		`\(\)=>(\w+)\.filter\((\w+)=>(\w+)!==(\w+)\),\[(\w+),(\w+)\]`,
	)
	sortAppliedRe = regexp.MustCompile(
		`\(\)=>(\w+)\.filter\((\w+)=>(\w+)!==(\w+)\|\|!0\),\[(\w+),(\w+)\]`,
	)
)

// trySortApply 把排序 filter 改成恒真。返回：新文本、是否改动、是否对不上。
func trySortApply(text string) (string, bool, bool) {
	if sortAppliedRe.MatchString(text) {
		return text, false, false // 已经套过
	}
	matches := sortApplyRe.FindAllStringSubmatch(text, 2)
	if len(matches) == 0 {
		return text, false, false
	}
	if len(matches) > 1 {
		return text, false, true // 多处同形，宁可不动
	}
	m := matches[0]
	arr, p, p2, x, arr2, x2 := m[1], m[2], m[3], m[4], m[5], m[6]
	if !(p == p2 && arr == arr2 && x == x2) {
		return text, false, true
	}
	replacement := "()=>" + arr + ".filter(" + p + "=>" + p + "!==" + x + "||!0),[" + arr + "," + x + "]"
	return strings.Replace(text, m[0], replacement, 1), true, false
}

// trySortRestore 把恒真 filter 还原回原来的「排除当前项」。
func trySortRestore(text string) (string, bool) {
	matches := sortAppliedRe.FindAllStringSubmatch(text, 2)
	if len(matches) != 1 {
		return text, false
	}
	m := matches[0]
	arr, p, x := m[1], m[2], m[4]
	restored := "()=>" + arr + ".filter(" + p + "=>" + p + "!==" + x + "),[" + arr + "," + x + "]"
	return strings.Replace(text, m[0], restored, 1), true
}

// applySortRegexInAssets 对 Agent 资源目录里带排序菜单的文件套/还原排序正则补丁。
// 只在含 modelSelect.orderBy 的文件上动手，避免误伤别处同形的 filter。
// 返回：是否有文件改动、是否已经是套过的形态。
func applySortRegexInAssets(resources string, apply bool) (changed, already bool) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		path := filepath.Join(assets, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(body)
		if apply {
			if !strings.Contains(text, "modelSelect.orderBy") {
				continue
			}
			if sortAppliedRe.MatchString(text) {
				already = true
				continue
			}
			if out, ok, _ := trySortApply(text); ok {
				if os.WriteFile(path, []byte(out), 0o644) == nil {
					changed = true
				}
			}
		} else {
			if out, ok := trySortRestore(text); ok {
				if os.WriteFile(path, []byte(out), 0o644) == nil {
					changed = true
				}
			}
		}
	}
	return changed, already
}
