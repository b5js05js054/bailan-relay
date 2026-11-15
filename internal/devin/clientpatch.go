package devin

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed patchdata/picker_before.txt
var pickerBefore string

//go:embed patchdata/picker_after.txt
var pickerAfter string

//go:embed patchdata/picker_before_126.txt
var pickerBefore126 string

//go:embed patchdata/picker_after_126.txt
var pickerAfter126 string

//go:embed patchdata/picker_before_238.txt
var pickerBefore238 string

//go:embed patchdata/picker_after_238.txt
var pickerAfter238 string

const pickerGroupOld = `F=(0,tK.useMemo)(()=>{return B.length>0?[{name:"All",groups:[{groupName:"Devin_hub",options:B}],isDefault:!0}]:[]`

const pickerGroupFlat = `F=(0,tK.useMemo)(()=>{let groups=[];(M||[]).forEach(n=>{(n.groups||[]).forEach(g=>{let options=(g.options||[]).filter(n=>String(n.modelInfo?.modelFamilyUid||"").startsWith("devinhub-family-")).map(e=>s(n4o(e),e));if(options.length)groups.push({groupName:g.groupName,options})})});if(!groups.length&&B.length)groups.push({groupName:"未分类",options:B});return groups.length?[{name:"All",groups,isDefault:!0}]:[]`

func pickerGroupNew() string {
	const mark = "F=(0,tK.useMemo)"
	i := strings.Index(pickerAfter, mark)
	if i < 0 {
		return ""
	}
	return strings.TrimRight(pickerAfter[i:], "\r\n")
}

const chatSelectNotify = "(function(e){let u=e&&e.modelUid||\"\";if(u.indexOf(\"@\")<0)return;let p=u.split(\"@\");if(p.length!==3)return;globalThis.bailanLast=u;try{let n=new XMLHttpRequest;n.open(\"POST\",\"http://127.0.0.1:5099/api/set_model\",!1);n.setRequestHeader(\"content-type\",\"text/plain\");n.send(JSON.stringify([p[0]]));let t=new XMLHttpRequest;t.open(\"POST\",\"http://127.0.0.1:5099/api/set_model_option\",!1);t.setRequestHeader(\"content-type\",\"text/plain\");t.send(JSON.stringify([p[0],p[1]===\"-\"?\"\":p[1],Number(p[2])||0]))}catch(e){}})(e);"

// 上下文圆环原来用 Agent 回报的窗口。Agent 只认识 swe-1-6-slow，所以一直停在 200K。
// 选中档的编号第三段才是这次要展示的上下文。
const chatContextOld = `a=nQi().find(e=>"model"===e.category&&"select"===e.type),o=n4h(nQa(a?.id??"model")??(a?.type==="select"?a.currentValue:void 0),tVx(e=>e.state.visibleModelConfigs)),{userStatus:s}=tcw(),l=tgK(s?.planStatus?.planInfo);if(!i||0===i.size)return null;let c=null!=n&&eld(n),u=n4g(i);return(0,tY.jsx)(nQm,{tokensUsed:i.used,contextLimit:i.size,`

// chatContextBad 在 return 之后又调用了一次 tVx。没有用量时走不到，用量一出现 Hook 就变多，React 报 #300，界面重挂后次数又对齐，所以会自己好。
const chatContextBad = `a=nQi().find(e=>"model"===e.category&&"select"===e.type),v=nQa(a?.id??"model")??(a?.type==="select"?a.currentValue:void 0),o=n4h(v,tVx(e=>e.state.visibleModelConfigs)),{userStatus:s}=tcw(),l=tgK(s?.planStatus?.planInfo);if(!i||0===i.size)return null;let c=null!=n&&eld(n),u=n4g(i),w=(()=>{let s=v;if(!(typeof s=="string"&&s.indexOf("@")>=0)&&typeof globalThis.bailanLast=="string")s=globalThis.bailanLast;let p=typeof s=="string"?s.split("@"):[];let n=p.length===3?Number(p[2]):0;if(n>0)return n;let m=tVx(e=>e.state.visibleModelConfigs).find(e=>e.modelUid===s);return m&&m.maxTokens>0?m.maxTokens:i.size})();return(0,tY.jsx)(nQm,{tokensUsed:i.used,contextLimit:w,`

const chatContextNew = `a=nQi().find(e=>"model"===e.category&&"select"===e.type),v=nQa(a?.id??"model")??(a?.type==="select"?a.currentValue:void 0),k=tVx(e=>e.state.visibleModelConfigs),o=n4h(v,k),{userStatus:s}=tcw(),l=tgK(s?.planStatus?.planInfo);if(!i||0===i.size)return null;let c=null!=n&&eld(n),u=n4g(i),w=(()=>{let s=v;if(!(typeof s=="string"&&s.indexOf("@")>=0)&&typeof globalThis.bailanLast=="string")s=globalThis.bailanLast;let p=typeof s=="string"?s.split("@"):[];let n=p.length===3?Number(p[2]):0;if(n>0)return n;let m=(k||[]).find(e=>e.modelUid===s);return m&&m.maxTokens>0?m.maxTokens:i.size})();return(0,tY.jsx)(nQm,{tokensUsed:i.used,contextLimit:w,`

const chatSelectOld = "let Z=(0,tK.useCallback)(e=>{if(c&&!tlc.getState().acp.pendingSwitchTarget?tlc.dispatch(elZ({sessionId:c,configId:U,value:e.modelUid}))"

const chatSelectNew = "let Z=(0,tK.useCallback)(e=>{" + chatSelectNotify + "if(c&&!tlc.getState().acp.pendingSwitchTarget?tlc.dispatch(elZ({sessionId:c,configId:U,value:e.modelUid}))"

// Devin 3.11.238 把选中回调从 Z/tlc/elZ 换成了 et/tld/el0，配置编号从 U 换成 J。
const chatSelect238Old = "let et=(0,tK.useCallback)(e=>{if(c&&!tld.getState().acp.pendingSwitchTarget?tld.dispatch(el0({sessionId:c,configId:J,value:e.modelUid}))"

const chatSelect238New = "let et=(0,tK.useCallback)(e=>{" + chatSelectNotify + "if(c&&!tld.getState().acp.pendingSwitchTarget?tld.dispatch(el0({sessionId:c,configId:J,value:e.modelUid}))"

// 官方菜单会藏起默认排序的名字。分组是打开列表时的默认，也要能点到。
const chatSortMenuOld = `f=(0,tK.useMemo)(()=>r.filter(e=>e!==n),[r,n])`
const chatSortMenuNew = `f=(0,tK.useMemo)(()=>r.slice(),[r])`

const wakeOld = "},1e4)}async function H"

const wakeNew = "},1e4)};function bailanArm(seen){if(bailanBusy)return;let e;try{e=h.MetadataProvider.getInstance().getMetadata()}catch(err){setTimeout(()=>bailanArm(seen),1000);return}if(!e||!e.apiKey){setTimeout(()=>bailanArm(seen),1000);return}let body=seen<0?\"[]\":JSON.stringify([seen]);fetch(\"http://127.0.0.1:5099/api/catalog_wait\",{method:\"POST\",body:body}).then(n=>n.json()).then(n=>{let r=n&&n.epoch;if(typeof r!=\"number\"){setTimeout(()=>bailanArm(seen),2000);return}if(seen<0||r===seen){bailanArm(r);return}bailanBusy=!0;G(e,0).catch(()=>{}).finally(()=>{bailanBusy=!1;bailanArm(r)})}).catch(()=>setTimeout(()=>bailanArm(seen),2000))}let bailanBusy=!1;bailanArm(-1);async function H"

// Devin 1.126 把 10 秒重试写成了 function H，后面的函数改叫 j。
// 重新拉目录的是 B(metadata, 0)，不再是旧包里的 G。
const wakeAnchor126 = "},1e4)}async function j"

const wakeNew126 = "},1e4)};function bailanArm(seen){if(bailanBusy)return;let e;try{e=h.MetadataProvider.getInstance().getMetadata()}catch(err){setTimeout(()=>bailanArm(seen),1000);return}if(!e||!e.apiKey){setTimeout(()=>bailanArm(seen),1000);return}let body=seen<0?\"[]\":JSON.stringify([seen]);fetch(\"http://127.0.0.1:5099/api/catalog_wait\",{method:\"POST\",body:body}).then(n=>n.json()).then(n=>{let r=n&&n.epoch;if(typeof r!=\"number\"){setTimeout(()=>bailanArm(seen),2000);return}if(seen<0||r===seen){bailanArm(r);return}bailanBusy=!0;B(e,0).catch(()=>{}).finally(()=>{bailanBusy=!1;bailanArm(r)})}).catch(()=>setTimeout(()=>bailanArm(seen),2000))}let bailanBusy=!1;bailanArm(-1);async function j"

// Devin 3.12.28 把 10 秒重试后面的函数改叫 V，目录提供方是 g，重新拉目录的是 q(metadata, 0)。
const wakeAnchor312 = "},1e4)}async function V"

const wakeNew312 = "},1e4)};function bailanArm(seen){if(bailanBusy)return;let e;try{e=g.MetadataProvider.getInstance().getMetadata()}catch(err){setTimeout(()=>bailanArm(seen),1000);return}if(!e||!e.apiKey){setTimeout(()=>bailanArm(seen),1000);return}let body=seen<0?\"[]\":JSON.stringify([seen]);fetch(\"http://127.0.0.1:5099/api/catalog_wait\",{method:\"POST\",body:body}).then(n=>n.json()).then(n=>{let r=n&&n.epoch;if(typeof r!=\"number\"){setTimeout(()=>bailanArm(seen),2000);return}if(seen<0||r===seen){bailanArm(r);return}bailanBusy=!0;q(e,0).catch(()=>{}).finally(()=>{bailanBusy=!1;bailanArm(r)})}).catch(()=>setTimeout(()=>bailanArm(seen),2000))}let bailanBusy=!1;bailanArm(-1);async function V"

// ApplyClientPatches 在 Devin 关掉之后，把产品需要的三处改动写进它的安装目录。
// 换一台电脑、点一次启动，就会补上，不依赖事先改好的那份 Devin。
// 具体执行与结构化报告在 applyWithReport（report.go）里，这里只保留兼容旧调用的返回值。
func ApplyClientPatches(executable string) (bool, string) {
	wrote, joined, _ := applyWithReport(executable)
	return wrote, joined
}

func applyChatModelList(resources string) (bool, string) {
	path := filepath.Join(resources, "app", "out", "vs", "workbench", "windsurf-chat-client", "index.js")
	body, err := os.ReadFile(path)
	if err != nil {
		return false, "未找到普通对话的模型列表"
	}
	text := string(body)
	changed := false
	grouped := false
	for _, pair := range [][2]string{{pickerBefore238, pickerAfter238}, {pickerBefore126, pickerAfter126}, {pickerBefore, pickerAfter}} {
		if !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return false, "普通对话的模型列表对不上，没有改动"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if !changed && strings.Contains(text, pickerGroupFlat) {
		if strings.Count(text, pickerGroupFlat) != 1 {
			return false, "普通对话的分组对不上，没有改动"
		}
		text = strings.Replace(text, pickerGroupFlat, pickerGroupNew(), 1)
		changed = true
		grouped = true
	} else if !changed && strings.Contains(text, pickerGroupOld) {
		if strings.Count(text, pickerGroupOld) != 1 {
			return false, "普通对话的分组对不上，没有改动"
		}
		text = strings.Replace(text, pickerGroupOld, pickerGroupNew(), 1)
		changed = true
		grouped = true
	}
	if !strings.Contains(text, "127.0.0.1:5099/api/set_model") {
		for _, pair := range [][2]string{{chatSelect238Old, chatSelect238New}, {chatSelectOld, chatSelectNew}} {
			if !strings.Contains(text, pair[0]) {
				continue
			}
			if strings.Count(text, pair[0]) != 1 {
				return false, "普通对话的选中通知对不上，没有改动"
			}
			text = strings.Replace(text, pair[0], pair[1], 1)
			changed = true
			break
		}
	}
	menuChanged := false
	if !strings.Contains(text, chatSortMenuNew) && strings.Contains(text, chatSortMenuOld) {
		if strings.Count(text, chatSortMenuOld) != 1 {
			return false, "普通对话的排序菜单对不上，没有改动"
		}
		text = strings.Replace(text, chatSortMenuOld, chatSortMenuNew, 1)
		menuChanged = true
	}
	iconChanged := false
	if next, ok, note := patchChatIcons(text); note != "" && !ok {
		return false, "普通对话的" + note
	} else if ok {
		text = next
		iconChanged = true
	}
	if next, ok, note := patchChatCards(text); note != "" {
		return false, note
	} else if ok {
		text = next
		iconChanged = true
	}
	if strings.Contains(text, chatSnapOld) || strings.Contains(text, chatShownOld) || strings.Contains(text, chatShown126) || strings.Contains(text, chatShown238) || strings.Contains(text, "function bailanChatModel(") {
		next, ok, note := patchChatSession(text)
		if note != "" && !ok {
			return false, note
		}
		if ok {
			text = next
			iconChanged = true
		}
	}
	contextChanged := false
	switch {
	case strings.Count(text, chatContextOld) == 1:
		text = strings.Replace(text, chatContextOld, chatContextNew, 1)
		contextChanged = true
	case strings.Count(text, chatContextBad) == 1:
		text = strings.Replace(text, chatContextBad, chatContextNew, 1)
		contextChanged = true
	case strings.Count(text, chatContextOld) > 1, strings.Count(text, chatContextBad) > 1:
		return false, "上下文用量对不上，没有改动"
	}
	if !changed && !menuChanged && !iconChanged && !contextChanged {
		if strings.Contains(text, "devinhub-family-") && strings.Contains(text, "127.0.0.1:5099/api/set_model") {
			if !strings.Contains(text, chatSortMenuNew) && strings.Contains(text, "modelSelect.orderBy") {
				return false, "普通对话的排序菜单对不上，没有改动"
			}
			return false, "普通对话的模型列表已经写好"
		}
		if strings.Contains(text, "useModelCatalog:()=>") && !strings.Contains(text, "Available models") {
			return false, "这版普通对话和 Agent 共用模型列表"
		}
		return false, "这版 Devin 的普通对话列表对不上，没有改动"
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return false, "写入普通对话的模型列表失败"
	}
	var notes []string
	if changed {
		if grouped {
			notes = append(notes, "已按分类整理普通对话的模型列表")
		} else {
			notes = append(notes, "已把你的模型留在普通对话的列表里")
		}
	}
	if menuChanged {
		notes = append(notes, "排序里可以按分组看")
	}
	if iconChanged {
		notes = append(notes, "对不上的模型用上了 Hub 的图标")
	}
	if contextChanged {
		notes = append(notes, "上下文用量按选中的档位显示")
	}
	return true, strings.Join(notes, "；")
}

func applyCatalogWake(resources string) (bool, string) {
	path := filepath.Join(resources, "app", "extensions", "windsurf", "dist", "extension.js")
	body, err := os.ReadFile(path)
	if err != nil {
		return false, "未找到叫醒 Devin 的程序"
	}
	text := string(body)
	if strings.Contains(text, "api/catalog_wait") {
		return false, "模型变化后叫醒 Devin 的那一段已经接上"
	}
	// 先用「认结构」的正则注入；没套上再退回按版本精确锚点。
	if out, ok, _ := tryCatalogWakeApply(text); ok {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			return false, "写入叫醒 Devin 的程序失败"
		}
		return true, "已接上模型变化后叫醒 Devin"
	}
	var anchor, replacement string
	switch {
	case strings.Count(text, wakeAnchor312) == 1:
		anchor, replacement = wakeAnchor312, wakeNew312
	case strings.Count(text, wakeOld) == 1:
		anchor, replacement = wakeOld, wakeNew
	case strings.Count(text, wakeAnchor126) == 1:
		anchor, replacement = wakeAnchor126, wakeNew126
	default:
		return false, "这版 Devin 的叫醒位置对不上，没有改动"
	}
	text = strings.Replace(text, anchor, replacement, 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return false, "写入叫醒 Devin 的程序失败"
	}
	return true, "已接上模型变化后叫醒 Devin"
}

// RestoreClientPatches 去掉 Hub 写进 Devin 的改动，使安装目录回到改动前。
func RestoreClientPatches(executable string) (bool, string) {
	exe := DiscoverExecutable(executable)
	if exe == "" {
		return false, ""
	}
	root := resourcesDir(exe)
	var restored bool
	var messages []string
	for _, restore := range []func(string) (bool, string){
		restoreAgentModelList,
		restoreAgentSortMenu,
		restoreAgentContextWindow,
		restoreAgentIcons,
		restoreRequireModel,
		restoreChatModelList,
		restoreRateBadges,
		restoreCatalogWake,
	} {
		changed, message := restore(root)
		if message != "" {
			messages = append(messages, message)
		}
		if changed {
			restored = true
		}
	}
	if restored {
		clearDevinScriptCache()
	}
	if len(messages) == 0 {
		return false, "Devin 里没有 Hub 的改动"
	}
	return restored, strings.Join(messages, "；")
}

func restoreChatModelList(resources string) (bool, string) {
	path := filepath.Join(resources, "app", "out", "vs", "workbench", "windsurf-chat-client", "index.js")
	body, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	text := string(body)
	changed := false
	for _, pair := range [][2]string{{pickerAfter238, pickerBefore238}, {pickerAfter126, pickerBefore126}, {pickerAfter, pickerBefore}} {
		if !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return false, "普通对话的模型列表对不上，没能还原"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if next, ok, note := restoreChatSession(text); note != "" {
		return false, note
	} else if ok {
		text = next
		changed = true
	}
	for _, pair := range [][2]string{{chatSelect238New, chatSelect238Old}, {chatSelectNew, chatSelectOld}} {
		if !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return false, "普通对话的选中通知对不上，没能还原"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if strings.Contains(text, chatSortMenuNew) {
		if strings.Count(text, chatSortMenuNew) != 1 {
			return false, "普通对话的排序菜单对不上，没能还原"
		}
		text = strings.Replace(text, chatSortMenuNew, chatSortMenuOld, 1)
		changed = true
	}
	if next, ok := restoreChatIcons(text); ok {
		text = next
		changed = true
	}
	if next, ok, note := restoreChatCards(text); note != "" {
		return false, note
	} else if ok {
		text = next
		changed = true
	}
	if !changed {
		return false, ""
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return false, "还原普通对话的模型列表失败"
	}
	return true, "已还原普通对话的模型列表"
}

func restoreCatalogWake(resources string) (bool, string) {
	path := filepath.Join(resources, "app", "extensions", "windsurf", "dist", "extension.js")
	body, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	text := string(body)
	next, changed := stripCatalogWake(text)
	if !changed {
		return false, ""
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, "还原叫醒 Devin 的程序失败"
	}
	return true, "已去掉模型变化后叫醒 Devin 的那一段"
}

// stripCatalogWake 删掉叫醒代码。多出来的分号一并去掉，还原成原来的 `}async function`。
func stripCatalogWake(text string) (string, bool) {
	const startMark = ";function bailanArm(seen)"
	const endMark = "bailanArm(-1);"
	start := strings.Index(text, startMark)
	end := strings.Index(text, endMark)
	if start < 0 || end < 0 || end < start {
		return text, false
	}
	end += len(endMark)
	for end < len(text) && text[end] == ';' {
		end++
	}
	return text[:start] + text[end:], true
}

// 模型名字旁边的蓝色字。官方用来写 New，这里改成显示后台倍率。
const chatRateOld = `a=[];return e.isBeta?a.push({...r.beta,icon:e.betaWarningMessage?(0,tY.jsx)(eT$,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&a.push(r.new),e.fastStatus?.isActive&&a.push({...r.fast,tooltip:e.fastStatus.tooltip}),0===a.length?i?(0,tY.jsx)(rwH,{tooltip:rwz(i,n),children:(0,tY.jsx)(rwJ,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,tY.jsx)("div",{className:"flex items-center gap-1",children:a.map((e,r)=>(0,tY.jsx)(rwV,{config:e,promo:0===r?i:void 0,t:n},e.key))})`

const chatRateNew = `a=[];i&&i.label&&a.push({key:"rate",label:i.label,className:"text-text-accent-primary"});return e.isBeta?a.push({...r.beta,icon:e.betaWarningMessage?(0,tY.jsx)(eT$,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&a.push(r.new),e.fastStatus?.isActive&&a.push({...r.fast,tooltip:e.fastStatus.tooltip}),0===a.length?i?(0,tY.jsx)(rwH,{tooltip:rwz(i,n),children:(0,tY.jsx)(rwJ,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,tY.jsx)("div",{className:"flex items-center gap-1",children:a.map((e,r)=>(0,tY.jsx)(rwV,{config:e,promo:e.key==="rate"?void 0:0===r?i:void 0,t:n},e.key))})`

// Devin 1.126 把倍率旁边的组件名换成了 rw$ / rwV / rwH / rwY。
const chatRate126Old = `a=[];return e.isBeta?a.push({...r.beta,icon:e.betaWarningMessage?(0,tY.jsx)(eT$,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&a.push(r.new),e.fastStatus?.isActive&&a.push({...r.fast,tooltip:e.fastStatus.tooltip}),0===a.length?i?(0,tY.jsx)(rw$,{tooltip:rwV(i,n),children:(0,tY.jsx)(rwH,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,tY.jsx)("div",{className:"flex items-center gap-1",children:a.map((e,r)=>(0,tY.jsx)(rwY,{config:e,promo:0===r?i:void 0,t:n},e.key))})`

const chatRate126New = `a=[];i&&i.label&&a.push({key:"rate",label:i.label,className:"text-text-accent-primary"});return e.isBeta?a.push({...r.beta,icon:e.betaWarningMessage?(0,tY.jsx)(eT$,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&a.push(r.new),e.fastStatus?.isActive&&a.push({...r.fast,tooltip:e.fastStatus.tooltip}),0===a.length?i?(0,tY.jsx)(rw$,{tooltip:rwV(i,n),children:(0,tY.jsx)(rwH,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,tY.jsx)("div",{className:"flex items-center gap-1",children:a.map((e,r)=>(0,tY.jsx)(rwY,{config:e,promo:e.key==="rate"?void 0:0===r?i:void 0,t:n},e.key))})`

// Devin 3.11.238 把倍率旁边的组件换成了 eTK / rkm / rku / rkc / rkp。
const chatRate238Old = `a=[];return e.isBeta?a.push({...r.beta,icon:e.betaWarningMessage?(0,tY.jsx)(eTK,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&a.push(r.new),e.fastStatus?.isActive&&a.push({...r.fast,tooltip:e.fastStatus.tooltip}),0===a.length?i?(0,tY.jsx)(rkm,{tooltip:rku(i,n),children:(0,tY.jsx)(rkc,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,tY.jsx)("div",{className:"flex items-center gap-1",children:a.map((e,r)=>(0,tY.jsx)(rkp,{config:e,promo:0===r?i:void 0,t:n},e.key))})`

const chatRate238New = `a=[];i&&i.label&&a.push({key:"rate",label:i.label,className:"text-text-accent-primary"});return e.isBeta?a.push({...r.beta,icon:e.betaWarningMessage?(0,tY.jsx)(eTK,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&a.push(r.new),e.fastStatus?.isActive&&a.push({...r.fast,tooltip:e.fastStatus.tooltip}),0===a.length?i?(0,tY.jsx)(rkm,{tooltip:rku(i,n),children:(0,tY.jsx)(rkc,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,tY.jsx)("div",{className:"flex items-center gap-1",children:a.map((e,r)=>(0,tY.jsx)(rkp,{config:e,promo:e.key==="rate"?void 0:0===r?i:void 0,t:n},e.key))})`

func agentRateOld() string {
	bt := "\x60"
	return "i=[];return e.isBeta?i.push({...n.beta,icon:e.betaWarningMessage?(0,G.jsx)(Ye,{size:12,className:" + bt + "flex-none" + bt + "}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&i.push(n.new),e.fastStatus?.isActive&&i.push({...n.fast,tooltip:e.fastStatus.tooltip}),i.length===0?r?(0,G.jsx)(xn,{tooltip:yn(r,t),children:(0,G.jsx)(m,{size:12,className:" + bt + "text-text-accent-primary flex-none" + bt + "})}):null:(0,G.jsx)(" + bt + "div" + bt + ",{className:" + bt + "flex items-center gap-1" + bt + ",children:i.map((e,n)=>(0,G.jsx)(Sn,{config:e,promo:n===0?r:void 0,t},e.key))})"
}

func agentRate312Old() string {
	bt := "\x60"
	return "i=[];return e.isBeta?i.push({...n.beta,icon:e.betaWarningMessage?(0,K.jsx)(Le,{size:12,className:" + bt + "flex-none" + bt + "}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&i.push(n.new),e.fastStatus?.isActive&&i.push({...n.fast,tooltip:e.fastStatus.tooltip}),i.length===0?r?(0,K.jsx)(Dn,{tooltip:Tn(r,t),children:(0,K.jsx)(f,{size:12,className:" + bt + "text-text-accent-primary flex-none" + bt + "})}):null:(0,K.jsx)(" + bt + "div" + bt + ",{className:" + bt + "flex items-center gap-1" + bt + ",children:i.map((e,n)=>(0,K.jsx)(On,{config:e,promo:n===0?r:void 0,t},e.key))})"
}

func agentRate312New() string {
	bt := "\x60"
	return "i=[];r&&r.label&&i.push({key:" + bt + "rate" + bt + ",label:r.label,className:" + bt + "text-text-accent-primary" + bt + "});return e.isBeta?i.push({...n.beta,icon:e.betaWarningMessage?(0,K.jsx)(Le,{size:12,className:" + bt + "flex-none" + bt + "}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&i.push(n.new),e.fastStatus?.isActive&&i.push({...n.fast,tooltip:e.fastStatus.tooltip}),i.length===0?r?(0,K.jsx)(Dn,{tooltip:Tn(r,t),children:(0,K.jsx)(f,{size:12,className:" + bt + "text-text-accent-primary flex-none" + bt + "})}):null:(0,K.jsx)(" + bt + "div" + bt + ",{className:" + bt + "flex items-center gap-1" + bt + ",children:i.map((e,n)=>(0,K.jsx)(On,{config:e,promo:e.key===" + bt + "rate" + bt + "?void 0:n===0?r:void 0,t},e.key))})"
}

func agentRate238Old() string {
	bt := "\x60"
	return "i=[];return e.isBeta?i.push({...n.beta,icon:e.betaWarningMessage?(0,G.jsx)(Qe,{size:12,className:" + bt + "flex-none" + bt + "}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&i.push(n.new),e.fastStatus?.isActive&&i.push({...n.fast,tooltip:e.fastStatus.tooltip}),i.length===0?r?(0,G.jsx)(En,{tooltip:wn(r,t),children:(0,G.jsx)(m,{size:12,className:" + bt + "text-text-accent-primary flex-none" + bt + "})}):null:(0,G.jsx)(" + bt + "div" + bt + ",{className:" + bt + "flex items-center gap-1" + bt + ",children:i.map((e,n)=>(0,G.jsx)(Dn,{config:e,promo:n===0?r:void 0,t},e.key))})"
}

func agentRate238New() string {
	bt := "\x60"
	return "i=[];r&&r.label&&i.push({key:" + bt + "rate" + bt + ",label:r.label,className:" + bt + "text-text-accent-primary" + bt + "});return e.isBeta?i.push({...n.beta,icon:e.betaWarningMessage?(0,G.jsx)(Qe,{size:12,className:" + bt + "flex-none" + bt + "}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&i.push(n.new),e.fastStatus?.isActive&&i.push({...n.fast,tooltip:e.fastStatus.tooltip}),i.length===0?r?(0,G.jsx)(En,{tooltip:wn(r,t),children:(0,G.jsx)(m,{size:12,className:" + bt + "text-text-accent-primary flex-none" + bt + "})}):null:(0,G.jsx)(" + bt + "div" + bt + ",{className:" + bt + "flex items-center gap-1" + bt + ",children:i.map((e,n)=>(0,G.jsx)(Dn,{config:e,promo:e.key===" + bt + "rate" + bt + "?void 0:n===0?r:void 0,t},e.key))})"
}

func agentRateNew() string {
	bt := "\x60"
	return "i=[];r&&r.label&&i.push({key:" + bt + "rate" + bt + ",label:r.label,className:" + bt + "text-text-accent-primary" + bt + "});return e.isBeta?i.push({...n.beta,icon:e.betaWarningMessage?(0,G.jsx)(Ye,{size:12,className:" + bt + "flex-none" + bt + "}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&i.push(n.new),e.fastStatus?.isActive&&i.push({...n.fast,tooltip:e.fastStatus.tooltip}),i.length===0?r?(0,G.jsx)(xn,{tooltip:yn(r,t),children:(0,G.jsx)(m,{size:12,className:" + bt + "text-text-accent-primary flex-none" + bt + "})}):null:(0,G.jsx)(" + bt + "div" + bt + ",{className:" + bt + "flex items-center gap-1" + bt + ",children:i.map((e,n)=>(0,G.jsx)(Sn,{config:e,promo:e.key===" + bt + "rate" + bt + "?void 0:n===0?r:void 0,t},e.key))})"
}

func applyRateBadges(resources string) (bool, string) {
	chatPath := filepath.Join(resources, "app", "out", "vs", "workbench", "windsurf-chat-client", "index.js")
	wrote := false
	if patchChatRateFile(chatPath) {
		wrote = true
	}
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
				continue
			}
			path := filepath.Join(assets, entry.Name())
			if patchAgentRateFile(path) {
				wrote = true
			}
		}
	}
	if !wrote {
		return false, ""
	}
	return true, "已在模型名字旁边标上倍率"
}

func restoreRateBadges(resources string) (bool, string) {
	chatPath := filepath.Join(resources, "app", "out", "vs", "workbench", "windsurf-chat-client", "index.js")
	restored := false
	if patchChatRateRestoreFile(chatPath) {
		restored = true
	}
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
				continue
			}
			path := filepath.Join(assets, entry.Name())
			if patchAgentRateRestoreFile(path) {
				restored = true
			}
		}
	}
	if !restored {
		return false, ""
	}
	return true, "已去掉模型名字旁边的倍率"
}
