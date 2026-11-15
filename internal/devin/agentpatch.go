package devin

import (
	"os"
	"path/filepath"
	"strings"
)

// Agent 模式用的是 Devin 安装目录里的另一份模型列表。它只留自己认识的编号，
// 后台模型的编号带思考档和上下文，对不上就会从这组里消失。
// 点启动时由 Hub 把这组补上，不依赖事先改好的那一份安装文件。

const (
	agentListFlatOld = "a=t.models.flatMap(e=>{let t=n.get(e.modelUid);return t?[tr(t,e)]:e.disabled?[nr(e)]:[]})"
	// Devin 3.11.238 把 tr/nr 换成了 wr/Tr。
	agentListFlat238Old = "a=t.models.flatMap(e=>{let t=n.get(e.modelUid);return t?[wr(t,e)]:e.disabled?[Tr(e)]:[]})"
	// Devin 3.12.28 把 wr/Tr 换成了 Ur/Wr。
	agentListFlat312Old = "a=t.models.flatMap(e=>{let t=n.get(e.modelUid);return t?[Ur(t,e)]:e.disabled?[Wr(e)]:[]})"
	agentListReturn     = "if(s.length>0&&l.length>0)return{models:s,sorts:l,catalogMisses:o.map(e=>e.value)}"
)

func agentListFlatPrev() string {
	bt := "\x60"
	return "a=t.models.flatMap(e=>{let t=n.get(e.modelUid);if(t)return[tr(t,e)];if(e.disabled)return[nr(e)];if(String(e.family?.uid||" + bt + bt + ").startsWith(" + bt + "devinhub-family-" + bt + "))return[tr({name:e.label,value:e.modelUid,description:e.description},e)];return[]})"
}

func agentListFlatNew() string {
	return agentListFlatWith("tr")
}

func agentListFlat238New() string {
	return agentListFlatWith("wr")
}

func agentListFlat312New() string {
	return agentListFlatWith("Ur")
}

func agentListFlatWith(name string) string {
	bt := "\x60"
	return "a=t.models.flatMap(e=>{if(!String(e.family?.uid||" + bt + bt + ").startsWith(" + bt + "devinhub-family-" + bt + "))return[];let t=n.get(e.modelUid);return[" + name + "(t||{name:e.label,value:e.modelUid,description:e.description},e)]})"
}

func agentListHubInsertOld() string {
	bt := "\x60"
	return "let hub=s.filter(e=>String(e.familyUid||" + bt + bt + ").startsWith(" + bt + "devinhub-family-" + bt + "));if(hub.length){let def=l.find(e=>e.isDefault)||l[0];if(def){let have=new Set((def.groups||[]).flatMap(e=>(e.options||[]).map(e=>e.modelUid)));let add=hub.filter(e=>!have.has(e.modelUid));if(add.length){let g=def.groups.find(e=>!e.isFeatured);if(g)g.options=add.concat(g.options);else def.groups=[{groupName:" + bt + "Devin_hub" + bt + ",options:add}].concat(def.groups||[])}}}"
}

func agentListHubInsert() string {
	bt := "\x60"
	return "let hub=s.filter(e=>String(e.familyUid||" + bt + bt + ").startsWith(" + bt + "devinhub-family-" + bt + "));if(hub.length){let def=l.find(e=>e.isDefault)||l[0];if(def){let have=new Set((def.groups||[]).flatMap(e=>(e.options||[]).map(e=>e.modelUid)));let add=hub.filter(e=>!have.has(e.modelUid));if(add.length){let g=(def.groups||[]).find(e=>e.groupName===" + bt + "未分类" + bt + ");if(g)g.options=add.concat(g.options);else def.groups=(def.groups||[]).concat([{groupName:" + bt + "未分类" + bt + ",options:add}])}}}"
}

func agentSelect312Old() string {
	bt := "\x60"
	return "te=L(t=>{r(t.modelUid),C(t.modelUid),i(" + bt + "LocalModelPicker:Select:Model" + bt
}

func agentSelect312New() string {
	bt := "\x60"
	return "te=L(t=>{(function(e){let u=e||\"\";if(u.indexOf(\"@\")<0)return;let p=u.split(\"@\");if(p.length!==3)return;try{let n=new XMLHttpRequest;n.open(\"POST\",\"http://127.0.0.1:5099/api/set_model\",!1);n.setRequestHeader(\"content-type\",\"text/plain\");n.send(JSON.stringify([p[0]]));let t=new XMLHttpRequest;t.open(\"POST\",\"http://127.0.0.1:5099/api/set_model_option\",!1);t.setRequestHeader(\"content-type\",\"text/plain\");t.send(JSON.stringify([p[0],p[1]===\"-\"?\"\":p[1],Number(p[2])||0]))}catch(e){}})(t.modelUid);r(t.modelUid),C(t.modelUid),i(" + bt + "LocalModelPicker:Select:Model" + bt
}

func agentSelectOld() string {
	bt := "\x60"
	return "ee=(0,Z.useCallback)(t=>{i(t.modelUid),S(t.modelUid),n(" + bt + "LocalModelPicker:Select:Model" + bt
}

func agentSelectNew() string {
	return "ee=(0,Z.useCallback)(t=>{(function(e){let u=e||\"\";if(u.indexOf(\"@\")<0)return;let p=u.split(\"@\");if(p.length!==3)return;globalThis.bailanLast=u;try{let n=new XMLHttpRequest;n.open(\"POST\",\"http://127.0.0.1:5099/api/set_model\",!1);n.setRequestHeader(\"content-type\",\"text/plain\");n.send(JSON.stringify([p[0]]));let t=new XMLHttpRequest;t.open(\"POST\",\"http://127.0.0.1:5099/api/set_model_option\",!1);t.setRequestHeader(\"content-type\",\"text/plain\");t.send(JSON.stringify([p[0],p[1]===\"-\"?\"\":p[1],Number(p[2])||0]))}catch(e){}})(t.modelUid);i(t.modelUid),S(t.modelUid),n(" + "\x60" + "LocalModelPicker:Select:Model" + "\x60"
}

func agentChoiceOld() string {
	return `function Ca(e,t,n,r){if(e!==void 0){for(let i of[t?.[e.id],n,r])if(i!==void 0&&Sa(e,i))return i}}`
}

func agentChoiceNew() string {
	return `function Ca(e,t,n,r){if(e!==void 0){for(let i of[t?.[e.id],n,r])if(i!==void 0&&(Sa(e,i)||typeof i=="string"&&i.indexOf("@")>=0))return i}}`
}

func agentSendOld() string {
	return `c[l.id]=Ca(l,r,u,a),u!==void 0&&!Sa(l,u)&&i.forget(n,u)`
}

func agentSendNew() string {
	return agentSendWith(agentModelAlias)
}

func agentSendPlain(alias string) string {
	return `c[l.id]=((v)=>typeof v=="string"&&v.indexOf("@")>=0?"` + alias + `":v)(Ca(l,r,u,a)),u!==void 0&&!Sa(l,u)&&String(u).indexOf("@")<0&&i.forget(n,u)`
}

func agentSendWith(alias string) string {
	// 新建会话页上看到的模型在这里确定。先按这个会话记下，再换成 Devin 允许的名字。
	return `c[l.id]=((v)=>{if(typeof v=="string"&&v.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(t,v);try{let n=new XMLHttpRequest;n.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);n.setRequestHeader("content-type","text/plain");n.send(JSON.stringify([t,v]))}catch(e){}return"` + alias + `"}return v})(Ca(l,r,u,a)),u!==void 0&&!Sa(l,u)&&String(u).indexOf("@")<0&&i.forget(n,u)`
}

func agentConfigOld() string {
	return `configId:t.id,value:n}}).then`
}

func agentConfigNew() string {
	return agentConfigWith(agentModelAlias)
}

func agentConfigWith(alias string) string {
	return `configId:t.id,value:typeof n=="string"&&n.indexOf("@")>=0?"` + alias + `":n}}).then`
}

// 新建会话时会把记住的模型编号再交给 Agent。带 @ 的编号它不认识，只能换成它允许的 swe-1-6-slow。
func agentReplayOld() string {
	return "method:`session/set_config_option`,params:{sessionId:t,configId:r,value:a}"
}

func agentReplayNew() string {
	return "method:`session/set_config_option`,params:{sessionId:t,configId:r,value:typeof a==\"string\"&&a.indexOf(\"@\")>=0?\"" + agentModelAlias + "\":a}"
}

const agentModelAlias = "swe-1-6-slow"
const agentModelAliasPrev = "swe-1-6-fast"

func agentKeepOld() string {
	return `(0,Ra.useEffect)(()=>{n!==null&&l!==n.baseValue&&r(null)},[n,l])`
}

func agentKeepNew() string {
	return `(0,Ra.useEffect)(()=>{n!==null&&l!==n.baseValue&&!(typeof n.value=="string"&&n.value.indexOf("@")>=0)&&r(null)},[n,l])`
}

func agentStickOld() string {
	return `function La(e,t){`
}

func agentStickMid() string {
	return `var bailanLast="";function La(e,t){`
}

func agentSyncJS() string {
	return `var bailanSynced="";function bailanSyncModel(u){if(typeof u!="string"||u.indexOf("@")<0||bailanSynced===u)return;let p=u.split("@");if(p.length!==3)return;bailanSynced=u;try{let n=new XMLHttpRequest;n.open("POST","http://127.0.0.1:5099/api/set_model",!1);n.setRequestHeader("content-type","text/plain");n.send(JSON.stringify([p[0]]));let t=new XMLHttpRequest;t.open("POST","http://127.0.0.1:5099/api/set_model_option",!1);t.setRequestHeader("content-type","text/plain");t.send(JSON.stringify([p[0],p[1]==="-"?"":p[1],Number(p[2])||0]))}catch(e){bailanSynced=""}}`
}

func agentSessionMemoryJS() string {
	return `var bailanChat={};globalThis.bailanRemember=function(s,v){if(!s||typeof v!="string"||v.indexOf("@")<0)return;bailanChat[s]=v;var i=String(s).lastIndexOf("/");if(i>=0&&i<s.length-1)bailanChat[s.slice(i+1)]=v};globalThis.bailanLookup=function(s){if(!s)return"";var v=bailanChat[s];if(typeof v=="string"&&v.indexOf("@")>=0)return v;var i=String(s).lastIndexOf("/");v=i>=0?bailanChat[s.slice(i+1)]:"";return typeof v=="string"&&v.indexOf("@")>=0?v:""};`
}

func agentStickPrev() string {
	return `var bailanLast="";` + agentSyncJS() + `function La(e,t){`
}

func agentContextMirror() string {
	return `Object.defineProperty(globalThis,"bailanLast",{configurable:!0,get:()=>bailanLast,set:v=>{bailanLast=v}});`
}

func agentStickNew() string {
	return `var bailanLast="";` + agentContextMirror() + agentSessionMemoryJS() + agentSyncJS() + `function La(e,t){`
}

func agentTrackOld() string {
	return agentKeepNew() + `,(0,Ra.useEffect)(()=>{if(n===null)return;`
}

func agentTrackNew() string {
	return agentTrackOld()
}

func stripAgentModelSync(text string) (string, bool) {
	const bad = `,(0,Ra.useEffect)(()=>{let v=d??n?.value??l;if(!(typeof v=="string"&&v.indexOf("@")>=0))v=typeof bailanLast=="string"?bailanLast:"";bailanSyncModel(v)},[d,n,l])`
	if !strings.Contains(text, bad) {
		return text, false
	}
	return strings.Replace(text, bad, "", 1), true
}

func agentForgetOld() string {
	return `()=>{i(n=>{let i=n[e];if(!i||i[t]!==r)return n;let a={...n},o={...i};return delete o[t],Object.keys(o).length===0?delete a[e]:a[e]=o,a})}`
}

func agentForgetNew() string {
	return `()=>{if(typeof r=="string"&&r.indexOf("@")>=0)return;i(n=>{let i=n[e];if(!i||i[t]!==r)return n;let a={...n},o={...i};return delete o[t],Object.keys(o).length===0?delete a[e]:a[e]=o,a})}`
}

func agentShownOld() string {
	return `{value:d??n?.value??l,onSelect:`
}

func agentShownPrev() string {
	return `{value:(()=>{let v=d??n?.value??l;if(typeof v=="string"&&v.indexOf("@")>=0)return bailanLast=v,v;return typeof bailanLast=="string"&&bailanLast.indexOf("@")>=0?bailanLast:v})(),onSelect:`
}

func agentShownNew() string {
	return `{value:(()=>{if(!(t&&t.category==="model"))return d??n?.value??l;let v=d??n?.value??l;if(typeof v=="string"&&v.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,v);bailanLast=v;return v}let g=globalThis.bailanLookup?globalThis.bailanLookup(a):"";if(g)return g;return typeof bailanLast=="string"&&bailanLast.indexOf("@")>=0?bailanLast:v})(),onSelect:`
}

// 刚点的模型优先。没有点选时，用这个会话创建时记下的模型，避免被服务端写回的默认模型盖掉。
// 这个取值函数模型和模式共用。只改模型选项，否则模式对不上可选项，按钮会退回显示 Mode。
func agentShownKeep() string {
	return `{value:(()=>{if(!(t&&t.category==="model"))return d??n?.value??l;let picked=n?.value;if(typeof picked=="string"&&picked.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,picked);bailanLast=picked;return picked}let g=globalThis.bailanLookup?globalThis.bailanLookup(a):"";if(g)return g;let v=d??l;if(typeof v=="string"&&v.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,v);bailanLast=v;return v}return typeof bailanLast=="string"&&bailanLast.indexOf("@")>=0?bailanLast:v})(),onSelect:`
}

func agentShownKeepBroad() string {
	return `{value:(()=>{let picked=n?.value;if(typeof picked=="string"&&picked.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,picked);bailanLast=picked;return picked}let g=globalThis.bailanLookup?globalThis.bailanLookup(a):"";if(g)return g;let v=d??l;if(typeof v=="string"&&v.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,v);bailanLast=v;return v}return typeof bailanLast=="string"&&bailanLast.indexOf("@")>=0?bailanLast:v})(),onSelect:`
}

// 新建会话时，页面上的当前模型比连接器里上次记住的模型优先。选中的带 @ 编号要告诉 Hub。
func agentNewSessionOld() string {
	return `u!==void 0&&(l[u.id]=Ca(u,r,d,a),d!==void 0&&!Sa(u,d)&&i.forget(n,d));`
}

func agentNewSessionNew() string {
	return `u!==void 0&&(l[u.id]=Ca(u,r,typeof a=="string"&&a.indexOf("@")>=0?a:d,a),d!==void 0&&!Sa(u,d)&&!(typeof a=="string"&&a.indexOf("@")>=0)&&i.forget(n,d),typeof l[u.id]=="string"&&l[u.id].indexOf("@")>=0&&(globalThis.bailanRemember&&globalThis.bailanRemember(t,l[u.id]),(()=>{try{let q=new XMLHttpRequest;q.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);q.setRequestHeader("content-type","text/plain");q.send(JSON.stringify([t,l[u.id]]))}catch(e){}})()));`
}

func agentSortDefaultOld() string {
	return "return{name:e.name,isDefault:e.isDefault,displayMetric:e.displayMetric,groups:t}"
}

func agentSortDefaultNew() string {
	return "return{name:e.name,isDefault:!1,displayMetric:e.displayMetric,groups:t}"
}

func patchAgentSortDefault(text string) (string, bool) {
	old := agentSortDefaultOld()
	if strings.Count(text, old) != 1 {
		return text, false
	}
	return strings.Replace(text, old, agentSortDefaultNew(), 1), true
}

func agentPickOld() string {
	return `te=(0,Z.useMemo)(()=>r!==void 0&&h.some(e=>e.modelUid===r)?[r]:fa,[h,r])`
}

func agentPickMid() string {
	return `te=(0,Z.useMemo)(()=>{if(typeof r!="string")return fa;let hit=h.find(e=>e.modelUid===r||r.indexOf("@")>=0&&String(e.modelUid).split("@")[0]===r.split("@")[0]);return hit?[hit.modelUid]:fa},[h,r])`
}

func agentPickNew() string {
	return `te=(0,Z.useMemo)(()=>{if(typeof r!="string")return fa;let exact=h.find(e=>e.modelUid===r);if(exact)return[exact.modelUid];if(r.indexOf("@")<0)return fa;let base=r.split("@")[0];let hit=h.find(e=>String(e.modelUid).split("@")[0]===base);return hit?[hit.modelUid]:fa},[h,r])`
}

func agentSessionSendOld() string {
	return `u=(0,go.useCallback)(e=>{c(e).then(t=>{t&&l(e)})},[c,l])`
}

func agentSessionSendNew() string {
	return `u=(0,go.useCallback)(e=>{(function(s,u){if(!s||String(u).indexOf("@")<0)return;try{let n=new XMLHttpRequest;n.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);n.setRequestHeader("content-type","text/plain");n.send(JSON.stringify([s,u]))}catch(e){}})(n?.sessionId||"",e);c(e).then(t=>{t&&l(e)})},[c,l,n])`
}

func agentChoice238Old() string {
	return `function no(e,t,n,r){if(e!==void 0){for(let i of[t?.[e.id],n,r])if(i!==void 0&&to(e,i))return i}}`
}

func agentChoice238New() string {
	return `function no(e,t,n,r){if(e!==void 0){for(let i of[t?.[e.id],n,r])if(i!==void 0&&(to(e,i)||typeof i=="string"&&i.indexOf("@")>=0))return i}}`
}

func agentNewSession238Old() string {
	return `u!==void 0&&(l[u.id]=no(u,r,d,a),d!==void 0&&!to(u,d)&&i.forget(n,d));`
}

func agentNewSession238New() string {
	return `u!==void 0&&(l[u.id]=no(u,r,typeof a=="string"&&a.indexOf("@")>=0?a:d,a),d!==void 0&&!to(u,d)&&!(typeof a=="string"&&a.indexOf("@")>=0)&&i.forget(n,d),typeof l[u.id]=="string"&&l[u.id].indexOf("@")>=0&&(globalThis.bailanRemember&&globalThis.bailanRemember(t,l[u.id]),(()=>{try{let q=new XMLHttpRequest;q.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);q.setRequestHeader("content-type","text/plain");q.send(JSON.stringify([t,l[u.id]]))}catch(e){}})()));`
}

func agentKeep238Old() string {
	return `(0,vo.useEffect)(()=>{n!==null&&l!==n.baseValue&&r(null)},[n,l])`
}

func agentKeep238New() string {
	return `(0,vo.useEffect)(()=>{n!==null&&l!==n.baseValue&&!(typeof n.value=="string"&&n.value.indexOf("@")>=0)&&r(null)},[n,l])`
}

func agentStick238Old() string {
	return `function to(e,t){return yr(e.options).some(e=>e.value===t)}`
}

func agentStick238New() string {
	return `var bailanLast="";` + agentContextMirror() + agentSessionMemoryJS() + agentSyncJS() + agentStick238Old()
}

func agentPick238Old() string {
	return `ne=(0,Z.useMemo)(()=>r!==void 0&&h.some(e=>e.modelUid===r)?[r]:Ga,[h,r])`
}

func agentPick238New() string {
	return `ne=(0,Z.useMemo)(()=>{if(typeof r!="string")return Ga;let exact=h.find(e=>e.modelUid===r);if(exact)return[exact.modelUid];if(r.indexOf("@")<0)return Ga;let base=r.split("@")[0];let hit=h.find(e=>String(e.modelUid).split("@")[0]===base);return hit?[hit.modelUid]:Ga},[h,r])`
}

func agentSessionSend312Old() string {
	return `u=L(e=>{c(e).then(t=>{t&&l(e)})},[c,l])`
}

func agentSessionSend312New() string {
	return `u=L(e=>{(function(s,u){if(!s||String(u).indexOf("@")<0)return;try{let n=new XMLHttpRequest;n.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);n.setRequestHeader("content-type","text/plain");n.send(JSON.stringify([s,u]))}catch(e){}})(n?.sessionId||"",e);c(e).then(t=>{t&&l(e)})},[c,l,n])`
}

func agentSessionSend238Old() string {
	return `u=(0,Yo.useCallback)(e=>{c(e).then(t=>{t&&l(e)})},[c,l])`
}

func agentSessionSend238New() string {
	return `u=(0,Yo.useCallback)(e=>{(function(s,u){if(!s||String(u).indexOf("@")<0)return;try{let n=new XMLHttpRequest;n.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);n.setRequestHeader("content-type","text/plain");n.send(JSON.stringify([s,u]))}catch(e){}})(n?.sessionId||"",e);c(e).then(t=>{t&&l(e)})},[c,l,n])`
}

func agentChoice312Old() string {
	bt := "\x60"
	return "function ma(e,t,n,r){if(e!==void 0){for(let i of[t?.[e.id],n,r])if(typeof i==" + bt + "string" + bt + "&&pa(e,i))return i}}"
}

func agentChoice312New() string {
	bt := "\x60"
	return "function ma(e,t,n,r){if(e!==void 0){for(let i of[t?.[e.id],n,r])if(typeof i==" + bt + "string" + bt + "&&(pa(e,i)||i.indexOf(\"@\")>=0))return i}}"
}

func agentNewSession312Old() string {
	return `l!==void 0&&(c[l.id]=ma(l,r,u,a),u!==void 0&&!pa(l,u)&&i.forget(n,u));`
}

func agentNewSession312New() string {
	return `l!==void 0&&(c[l.id]=ma(l,r,typeof a=="string"&&a.indexOf("@")>=0?a:u,a),u!==void 0&&!pa(l,u)&&!(typeof a=="string"&&a.indexOf("@")>=0)&&i.forget(n,u),typeof c[l.id]=="string"&&c[l.id].indexOf("@")>=0&&(globalThis.bailanRemember&&globalThis.bailanRemember(t,c[l.id]),(()=>{try{let q=new XMLHttpRequest;q.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);q.setRequestHeader("content-type","text/plain");q.send(JSON.stringify([t,c[l.id]]))}catch(e){}})()));`
}

func agentReplay312Old() string {
	bt := "\x60"
	return "method:" + bt + "session/set_config_option" + bt + ",params:{sessionId:t,configId:r,...Pr(a)}"
}

func agentReplay312New() string {
	bt := "\x60"
	return "method:" + bt + "session/set_config_option" + bt + ",params:{sessionId:t,configId:r,...Pr(typeof a==\"string\"&&a.indexOf(\"@\")>=0?\"" + agentModelAlias + "\":a)}"
}

func agentLive312Old() string {
	return `params:{sessionId:e.sessionId,configId:t.id,...Pr(n)}`
}

func agentLive312New() string {
	return `params:{sessionId:e.sessionId,configId:t.id,...Pr(typeof n=="string"&&n.indexOf("@")>=0?"` + agentModelAlias + `":n)}`
}

func agentKeep312Old() string {
	return `I(()=>{n!==null&&c!==n.baseValue&&r(null)},[n,c])`
}

func agentKeep312New() string {
	return `I(()=>{n!==null&&c!==n.baseValue&&!(typeof n.value=="string"&&n.value.indexOf("@")>=0)&&r(null)},[n,c])`
}

func agentStick312Old() string {
	return `function pa(e,t){return zr(e,t)}`
}

func agentStick312New() string {
	return `var bailanLast="";` + agentSessionMemoryJS() + agentSyncJS() + agentStick312Old()
}

func agentPick312Old() string {
	return `ie=P(()=>n!==void 0&&g.some(e=>e.modelUid===n)?[n]:Qo,[g,n])`
}

func agentPick312New() string {
	return `ie=P(()=>{if(typeof n!="string")return Qo;let exact=g.find(e=>e.modelUid===n);if(exact)return[exact.modelUid];if(n.indexOf("@")<0)return Qo;let base=n.split("@")[0];let hit=g.find(e=>String(e.modelUid).split("@")[0]===base);return hit?[hit.modelUid]:Qo},[g,n])`
}

func agentShown312Old() string {
	return `{value:u??n?.value??c,onSelect:`
}

func agentShown312New() string {
	return `{value:(()=>{if(!(t&&t.category==="model"))return u??n?.value??c;let picked=n?.value;if(typeof picked=="string"&&picked.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,picked);bailanLast=picked;return picked}let g=globalThis.bailanLookup?globalThis.bailanLookup(a):"";if(g)return g;let v=u??c;if(typeof v=="string"&&v.indexOf("@")>=0){globalThis.bailanRemember&&globalThis.bailanRemember(a,v);bailanLast=v;return v}return typeof bailanLast=="string"&&bailanLast.indexOf("@")>=0?bailanLast:v})(),onSelect:`
}

func agentListMarker() string {
	return "startsWith(\x60devinhub-family-"
}

func patchAgentChoice(text string) (string, bool) {
	changed := false
	if next, ok := stripAgentModelSync(text); ok {
		text = next
		changed = true
	}
	for _, pair := range [][2]string{
		{agentChoice312Old(), agentChoice312New()},
		{agentChoice238Old(), agentChoice238New()},
		{agentChoiceOld(), agentChoiceNew()},
		{agentSendOld(), agentSendNew()},
		{agentSendPlain(agentModelAlias), agentSendNew()},
		{agentSendPlain(agentModelAliasPrev), agentSendNew()},
		{agentSendWith(agentModelAliasPrev), agentSendNew()},
		{agentConfigOld(), agentConfigNew()},
		{agentConfigWith(agentModelAliasPrev), agentConfigNew()},
		{agentKeep312Old(), agentKeep312New()},
		{agentKeep238Old(), agentKeep238New()},
		{agentKeepOld(), agentKeepNew()},
		{agentReplay312Old(), agentReplay312New()},
		{agentLive312Old(), agentLive312New()},
		{agentReplayOld(), agentReplayNew()},
		{agentStickPrev(), agentStickNew()},
		{agentStickMid(), agentStickNew()},
		{agentStick312Old(), agentStick312New()},
		{agentStick238Old(), agentStick238New()},
		{agentStickOld(), agentStickNew()},
		{agentForgetOld(), agentForgetNew()},
		{agentShown312Old(), agentShown312New()},
		{agentShownPrev(), agentShownNew()},
		{agentShownOld(), agentShownNew()},
		{agentShownKeepBroad(), agentShownKeep()},
		{agentShownNew(), agentShownKeep()},
		{agentNewSession312Old(), agentNewSession312New()},
		{agentNewSession238Old(), agentNewSession238New()},
		{agentNewSessionOld(), agentNewSessionNew()},
		{agentTrackOld(), agentTrackNew()},
		{agentPickMid(), agentPickNew()},
		{agentPick312Old(), agentPick312New()},
		{agentPick238Old(), agentPick238New()},
		{agentPickOld(), agentPickNew()},
		{agentSelect312Old(), agentSelect312New()},
		{agentSessionSend312Old(), agentSessionSend312New()},
		{agentSessionSend238Old(), agentSessionSend238New()},
		{agentSessionSendOld(), agentSessionSendNew()},
	} {
		if strings.Contains(text, pair[1]) || strings.Count(text, pair[0]) != 1 {
			continue
		}
		// 上下文用量会插进这段开头，整段就对不上了。函数已经在文件里时不能再插一次，否则页面解析失败。
		if pair[0] == agentStick312Old() && strings.Contains(text, "function bailanSyncModel(") {
			continue
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
	}
	return text, changed
}

func restoreAgentChoice(text string) string {
	for _, pair := range [][2]string{
		{agentStick312New(), agentStick312Old()},
		{agentChoice312New(), agentChoice312Old()},
		{agentNewSession312New(), agentNewSession312Old()},
		{agentKeep312New(), agentKeep312Old()},
		{agentPick312New(), agentPick312Old()},
		{agentReplay312New(), agentReplay312Old()},
		{agentLive312New(), agentLive312Old()},
		{agentShown312New(), agentShown312Old()},
		{agentSelect312New(), agentSelect312Old()},
		{agentSessionSend312New(), agentSessionSend312Old()},
		{agentStick238New(), agentStick238Old()},
		{agentChoice238New(), agentChoice238Old()},
		{agentNewSession238New(), agentNewSession238Old()},
		{agentKeep238New(), agentKeep238Old()},
		{agentPick238New(), agentPick238Old()},
		{agentSessionSend238New(), agentSessionSend238Old()},
		{agentChoiceNew(), agentChoiceOld()},
		{agentSendNew(), agentSendOld()},
		{agentSendPlain(agentModelAlias), agentSendOld()},
		{agentSendPlain(agentModelAliasPrev), agentSendOld()},
		{agentSendWith(agentModelAliasPrev), agentSendOld()},
		{agentConfigNew(), agentConfigOld()},
		{agentConfigWith(agentModelAliasPrev), agentConfigOld()},
		{agentKeepNew(), agentKeepOld()},
		{agentReplayNew(), agentReplayOld()},
		{agentTrackNew(), agentTrackOld()},
		{agentPickNew(), agentPickOld()},
		{agentPickMid(), agentPickOld()},
		{agentSessionSendNew(), agentSessionSendOld()},
		{agentShownKeep(), agentShownOld()},
		{agentShownKeepBroad(), agentShownOld()},
		{agentShownNew(), agentShownOld()},
		{agentShownPrev(), agentShownOld()},
		{agentNewSessionNew(), agentNewSessionOld()},
		{agentForgetNew(), agentForgetOld()},
		{agentStickNew(), agentStickOld()},
		{agentStickPrev(), agentStickOld()},
		{agentStickMid(), agentStickOld()},
	} {
		if strings.Contains(text, pair[0]) {
			text = strings.Replace(text, pair[0], pair[1], 1)
		}
	}
	return text
}

const agentContextOld = `function Dm(){let e=Ku(e=>e.status.contextUsage);return e?(0,Om.jsx)(Ui,{tokensUsed:e.used,contextLimit:e.size}):null}`

const agentContextNew = `function Dm(){let e=Ku(e=>e.status.contextUsage);if(!e)return null;let n=(()=>{let s=typeof globalThis.bailanLast=="string"?globalThis.bailanLast:"";let p=s.split("@");let c=p.length===3?Number(p[2]):0;return c>0?c:e.size})();return(0,Om.jsx)(Ui,{tokensUsed:e.used,contextLimit:n})}`

// 3.12.28 把圆环挪到了另一份脚本。Dm / Ku / Ui 换成了 ed / _r / Hr。
const agentContext312Old = `function ed(){let e=_r(e=>e.status.contextUsage);return e?(0,td.jsx)(Hr,{tokensUsed:e.used,contextLimit:e.size}):null}`

const agentContext312New = `function ed(){let e=_r(e=>e.status.contextUsage);if(!e)return null;let n=(()=>{let s=typeof globalThis.bailanLast=="string"?globalThis.bailanLast:"";let p=s.split("@");let c=p.length===3?Number(p[2]):0;return c>0?c:e.size})();return(0,td.jsx)(Hr,{tokensUsed:e.used,contextLimit:n})}`

// applyAgentContextWindow 让 Agent 界面的上下文用量跟选中档，而不是 swe-1-6-slow 的 200K。
func applyAgentContextWindow(resources string) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, ""
	}
	changed := false
	mirror := `var bailanLast="";` + agentContextMirror()
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
		fileChanged := false
		// 先用稳定结构定位上下文圈，放过压缩名字；对不上再退回按版本的精确锚点。
		if out, ok, bad := tryContextRingApply(text); bad {
			return false, "Agent 上下文用量对不上，没有改动"
		} else if ok {
			text = out
			fileChanged = true
		} else {
			for _, pair := range [][2]string{
				{agentContext312Old, agentContext312New},
				{agentContextOld, agentContextNew},
			} {
				if strings.Contains(text, pair[1]) {
					continue
				}
				n := strings.Count(text, pair[0])
				if n == 0 {
					continue
				}
				if n != 1 {
					return false, "Agent 上下文用量对不上，没有改动"
				}
				text = strings.Replace(text, pair[0], pair[1], 1)
				fileChanged = true
				break
			}
		}
		junction := `var bailanLast="";var bailanChat={`
		if strings.Contains(text, junction) && !strings.Contains(text, agentContextMirror()) {
			text = strings.Replace(text, junction, mirror+`var bailanChat={`, 1)
			fileChanged = true
		}
		if !fileChanged {
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return false, "写入 Agent 上下文用量失败"
		}
		changed = true
	}
	if changed {
		return true, "上下文用量按选中的档位显示"
	}
	return false, ""
}

func restoreAgentContextWindow(resources string) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, ""
	}
	changed := false
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
		fileChanged := false
		// 先还原正则版补丁；没有再按版本的精确锚点还原。
		if out, ok := tryContextRingRestore(text); ok {
			text = out
			fileChanged = true
		} else {
			for _, pair := range [][2]string{
				{agentContext312New, agentContext312Old},
				{agentContextNew, agentContextOld},
			} {
				n := strings.Count(text, pair[0])
				if n == 0 {
					continue
				}
				if n != 1 {
					return false, "Agent 上下文用量没能还原"
				}
				text = strings.Replace(text, pair[0], pair[1], 1)
				fileChanged = true
				break
			}
		}
		if !fileChanged {
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return false, "还原上下文用量失败"
		}
		changed = true
	}
	if !changed {
		return false, ""
	}
	return true, "已还原上下文用量"
}

// ApplyAgentModelList 在 Devin 关闭后，把后台模型补进 Agent 模式的列表。
// 已经写过，或这版 Devin 对不上时，不阻断启动。
func ApplyAgentModelList(executable string) (bool, string) {
	exe := DiscoverExecutable(executable)
	if exe == "" {
		return false, "未找到 Devin，Agent 模式的列表未改"
	}
	return applyAgentModelList(resourcesDir(exe))
}

func resourcesDir(executable string) string {
	if strings.HasSuffix(strings.ToLower(executable), ".app") {
		return filepath.Join(executable, "Contents", "Resources")
	}
	return filepath.Join(filepath.Dir(executable), "resources")
}

func applyAgentModelList(resources string) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, "未找到 Devin 的 Agent 界面"
	}
	already := false
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
		if strings.Contains(text, agentListMarker()) {
			oldInsert := agentListHubInsertOld()
			newInsert := agentListHubInsert()
			prevFlat := agentListFlatPrev()
			nextFlat := agentListFlatNew()
			changed := false
			if strings.Count(text, oldInsert) == 1 && !strings.Contains(text, newInsert) {
				text = strings.Replace(text, oldInsert, newInsert, 1)
				changed = true
			}
			if strings.Count(text, prevFlat) == 1 && !strings.Contains(text, nextFlat) {
				text = strings.Replace(text, prevFlat, nextFlat, 1)
				changed = true
			}
			if strings.Count(text, agentSelect312Old()) == 1 {
				text = strings.Replace(text, agentSelect312Old(), agentSelect312New(), 1)
				changed = true
			} else if strings.Count(text, agentSelectOld()) == 1 {
				text = strings.Replace(text, agentSelectOld(), agentSelectNew(), 1)
				changed = true
			}
			if next, ok := patchAgentChoice(text); ok {
				text = next
				changed = true
			}
			if next, ok := patchAgentSortDefault(text); ok {
				text = next
				changed = true
			}
			if changed {
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					return false, "写入 Agent 模型列表失败"
				}
				return true, "已按分类整理 Agent 模式的模型列表"
			}
			already = true
			continue
		}
		flatOld, flatNew := "", ""
		for _, pair := range [][2]string{{agentListFlat312Old, agentListFlat312New()}, {agentListFlat238Old, agentListFlat238New()}, {agentListFlatOld, agentListFlatNew()}} {
			if strings.Count(text, pair[0]) == 1 && strings.Count(text, agentListReturn) == 1 {
				flatOld, flatNew = pair[0], pair[1]
				break
			}
		}
		if flatOld == "" {
			continue
		}
		text = strings.Replace(text, flatOld, flatNew, 1)
		text = strings.Replace(text, agentListReturn, agentListHubInsert()+agentListReturn, 1)
		if strings.Count(text, agentSelect312Old()) == 1 {
			text = strings.Replace(text, agentSelect312Old(), agentSelect312New(), 1)
		} else if strings.Count(text, agentSelectOld()) == 1 {
			text = strings.Replace(text, agentSelectOld(), agentSelectNew(), 1)
		}
		if next, ok := patchAgentChoice(text); ok {
			text = next
		}
		if next, ok := patchAgentSortDefault(text); ok {
			text = next
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return false, "写入 Agent 模型列表失败"
		}
		return true, "已把你的模型写进 Agent 模式的列表"
	}
	if already {
		return false, "Agent 模式的模型列表已经写好"
	}
	return false, "这版 Devin 的 Agent 列表对不上，没有改动"
}

const (
	agentSortMenuOld = "h=(0,ut.useMemo)(()=>n.filter(e=>e!==t),[n,t])"
	agentSortMenuNew = "h=(0,ut.useMemo)(()=>n.slice(),[n])"
	// Devin 3.11.238 把 ut 换成了 ht。
	agentSortMenu238Old = "h=(0,ht.useMemo)(()=>n.filter(e=>e!==t),[n,t])"
	agentSortMenu238New = "h=(0,ht.useMemo)(()=>n.slice(),[n])"
	// Devin 3.12.28 把 useMemo 调用写成了 D(()=>。
	agentSortMenu312Old = "h=D(()=>n.filter(e=>e!==t),[n,t])"
	agentSortMenu312New = "h=D(()=>n.slice(),[n])"
)

func applyAgentSortMenu(resources string) (bool, string) {
	// 先用「认结构」的正则；改了就成，已是该形态就按已生效处理，都没有再退回精确锚点。
	if changed, already := applySortRegexInAssets(resources, true); changed {
		return true, "排序里可以按分组看"
	} else if already {
		return false, ""
	}
	return patchAgentSortMenu(resources, [][2]string{
		{agentSortMenu312Old, agentSortMenu312New},
		{agentSortMenu238Old, agentSortMenu238New},
		{agentSortMenuOld, agentSortMenuNew},
	}, "排序里可以按分组看")
}

func restoreAgentSortMenu(resources string) (bool, string) {
	if changed, _ := applySortRegexInAssets(resources, false); changed {
		return true, "已还原 Agent 的排序菜单"
	}
	return patchAgentSortMenu(resources, [][2]string{
		{agentSortMenu312New, agentSortMenu312Old},
		{agentSortMenu238New, agentSortMenu238Old},
		{agentSortMenuNew, agentSortMenuOld},
	}, "已还原 Agent 的排序菜单")
}

func patchAgentSortMenu(resources string, pairs [][2]string, done string) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, ""
	}
	sawOrder := false
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
		already := false
		for _, pair := range pairs {
			if strings.Contains(text, pair[1]) {
				already = true
				break
			}
		}
		if already {
			return false, ""
		}
		if strings.Contains(text, "modelSelect.orderBy") {
			sawOrder = true
		}
		replaced := false
		for _, pair := range pairs {
			if strings.Count(text, pair[0]) != 1 {
				continue
			}
			text = strings.Replace(text, pair[0], pair[1], 1)
			replaced = true
			break
		}
		if !replaced {
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return false, "写入 Agent 排序菜单失败"
		}
		return true, done
	}
	if sawOrder {
		return false, "Agent 的排序菜单对不上，没有改动"
	}
	return false, ""
}

func restoreAgentModelList(resources string) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, ""
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
		if !strings.Contains(text, agentListMarker()) {
			continue
		}
		flatNew, flatOld := agentListFlat312New(), agentListFlat312Old
		if strings.Count(text, flatNew) != 1 {
			flatNew, flatOld = agentListFlat238New(), agentListFlat238Old
		}
		if strings.Count(text, flatNew) != 1 {
			flatNew, flatOld = agentListFlatNew(), agentListFlatOld
		}
		if strings.Count(text, flatNew) != 1 {
			flatNew, flatOld = agentListFlatPrev(), agentListFlatOld
		}
		insert := agentListHubInsert()
		if strings.Count(text, insert) != 1 {
			insert = agentListHubInsertOld()
		}
		if strings.Count(text, flatNew) != 1 || strings.Count(text, insert) != 1 {
			return false, "Agent 模式的改动对不上，没能还原"
		}
		text = strings.Replace(text, flatNew, flatOld, 1)
		text = strings.Replace(text, insert, "", 1)
		if strings.Contains(text, agentSelect312New()) {
			text = strings.Replace(text, agentSelect312New(), agentSelect312Old(), 1)
		} else if strings.Contains(text, agentSelectNew()) {
			text = strings.Replace(text, agentSelectNew(), agentSelectOld(), 1)
		}
		text = restoreAgentChoice(text)
		if strings.Contains(text, agentSortDefaultNew()) {
			text = strings.Replace(text, agentSortDefaultNew(), agentSortDefaultOld(), 1)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return false, "还原 Agent 模型列表失败"
		}
		return true, "已还原 Agent 模式的模型列表"
	}
	return false, ""
}

// 新会话的模型按钮会显示 None selected，发送箭头却是亮的。
// 画出这句文案时记下 bailanHasModel。发送按钮排在模型按钮后面，同一次绘制读得到。
// 模型按钮没挂出来时把标记改回可以发送，避免上一页的状态留在别的输入框上。
const (
	requireModelPickerOld     = `k&&(0,Z.jsx)(ra,{disabled:ps||F,onClose:Id})`
	requireModelPickerNew     = `k?(0,Z.jsx)(ra,{disabled:ps||F,onClose:Id}):(globalThis.bailanHasModel=!0,null)`
	requireModelButtonOld     = `disabled:t,onDisabledPress:s,className:d,menu:_,menuVisible:v,onClick:w?m:i`
	requireModelButtonNew     = `disabled:t||globalThis.bailanHasModel===!1,onDisabledPress:s,className:d,menu:_,menuVisible:v,onClick:w?m:i`
	requireModelSubmitOld     = `Pf=K(e=>{Nf()||(cf.onSend(),z.guardSubmit(()=>{zu(!1),Md(e)}))})`
	requireModelSubmitNew     = `Pf=K(e=>{if(globalThis.bailanHasModel===!1)return;Nf()||(cf.onSend(),z.guardSubmit(()=>{zu(!1),Md(e)}))})`
	requireModelQueueOld      = `Wu=K((e,t=!1)=>{if(Qa)return;`
	requireModelQueueNew      = `Wu=K((e,t=!1)=>{if(globalThis.bailanHasModel===!1)return;if(Qa)return;`
	requireModelAutoOld       = `oa(!1),zc||Md()`
	requireModelAutoNew       = `oa(!1),zc||globalThis.bailanHasModel===!1||Md()`
	requireModelBackgroundOld = `fp=K(()=>{Nf()||(cf.onSend(),z.guardSubmit(()=>{zu(!1),Md({background:!0})}))})`
	requireModelBackgroundNew = `fp=K(()=>{if(globalThis.bailanHasModel===!1)return;Nf()||(cf.onSend(),z.guardSubmit(()=>{zu(!1),Md({background:!0})}))})`
)

func requireModelLabelOld() string {
	bt := "\x60"
	return "$e=Oe.length===0?Ee(" + bt + "modelSelect.noneSelected" + bt + "):At(Ze,re,Ee)"
}

func requireModelLabelNew() string {
	bt := "\x60"
	return "$e=(globalThis.bailanHasModel=Oe.length>0,Oe.length===0?Ee(" + bt + "modelSelect.noneSelected" + bt + "):At(Ze,re,Ee))"
}

func requireModelPairs() [][2]string {
	return [][2]string{
		{requireModelLabelOld(), requireModelLabelNew()},
		{requireModelPickerOld, requireModelPickerNew},
		{requireModelButtonOld, requireModelButtonNew},
		{requireModelSubmitOld, requireModelSubmitNew},
		{requireModelQueueOld, requireModelQueueNew},
		{requireModelAutoOld, requireModelAutoNew},
		{requireModelBackgroundOld, requireModelBackgroundNew},
	}
}

func applyRequireModel(resources string) (bool, string) {
	return patchRequireModel(resources, requireModelPairs(), "没选模型时不能发送")
}

func restoreRequireModel(resources string) (bool, string) {
	pairs := requireModelPairs()
	reversed := make([][2]string, len(pairs))
	for i, pair := range pairs {
		reversed[i] = [2]string{pair[1], pair[0]}
	}
	return patchRequireModel(resources, reversed, "已还原没选模型时的发送限制")
}

func patchRequireModel(resources string, pairs [][2]string, done string) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, ""
	}
	type fileText struct {
		path string
		text string
	}
	var files []fileText
	countsOld := make([]int, len(pairs))
	countsNew := make([]int, len(pairs))
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
		stripped := text
		for i, pair := range pairs {
			countsNew[i] += strings.Count(text, pair[1])
			stripped = strings.ReplaceAll(stripped, pair[1], "")
		}
		for i, pair := range pairs {
			countsOld[i] += strings.Count(stripped, pair[0])
		}
		files = append(files, fileText{path, text})
	}
	// 没有这句文案，就是还没把模型按钮改成这版的旧安装。
	if countsOld[0]+countsNew[0] == 0 {
		return false, ""
	}
	for i := range pairs {
		if countsOld[i] > 1 || countsNew[i] > 1 || countsOld[i]+countsNew[i] != 1 {
			return false, "没选模型时的发送对不上，没有改动"
		}
	}
	dirty := make([]int, 0)
	for fi := range files {
		fileChanged := false
		for _, pair := range pairs {
			if strings.Contains(files[fi].text, pair[1]) || strings.Count(files[fi].text, pair[0]) != 1 {
				continue
			}
			files[fi].text = strings.Replace(files[fi].text, pair[0], pair[1], 1)
			fileChanged = true
		}
		if fileChanged {
			dirty = append(dirty, fi)
		}
	}
	if len(dirty) == 0 {
		return false, ""
	}
	for _, fi := range dirty {
		if err := os.WriteFile(files[fi].path, []byte(files[fi].text), 0o644); err != nil {
			return false, "写入发送限制失败"
		}
	}
	return true, done
}

func clearDevinScriptCache() {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}
	_ = os.RemoveAll(filepath.Join(appData, "Devin", "Code Cache"))
}
