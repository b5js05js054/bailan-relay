package devin

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bailan-relay/ui"
)

// 顺序和页面上的厂商图标一致：先对上的用 Devin 自带图标，对不上的才用这里的图。
var vendorIcons = []struct {
	file     string
	prefixes []string
}{
	{"claude.svg", []string{"claude", "anthropic"}},
	{"openai.svg", []string{"openai", "chatgpt", "gpt-", "gpt", "o1-", "o3-", "o4-", "dall-e", "dalle"}},
	{"gemini.svg", []string{"gemini", "gemma"}},
	{"google.svg", []string{"google", "palm"}},
	{"deepseek.svg", []string{"deepseek"}},
	{"chatglm.svg", []string{"chatglm"}},
	{"zhipu.svg", []string{"zhipu", "glm"}},
	{"kimi.svg", []string{"kimi"}},
	{"moonshot.svg", []string{"moonshot"}},
	{"mimo.svg", []string{"mimo", "xiaomi"}},
	{"grok.svg", []string{"grok"}},
	{"xai.svg", []string{"xai"}},
	{"qwen.svg", []string{"qwen", "qwq", "qvq", "tongyi"}},
	{"doubao.svg", []string{"doubao", "skylark"}},
	{"bytedance.svg", []string{"bytedance", "seedance", "seedream"}},
	{"baichuan.svg", []string{"baichuan"}},
	{"minimax.svg", []string{"minimax", "abab"}},
	{"hailuo.svg", []string{"hailuo"}},
	{"mistral.svg", []string{"mistral", "mixtral", "pixtral", "codestral"}},
	{"meta.svg", []string{"meta-llama", "llama", "meta"}},
	{"cohere.svg", []string{"cohere", "command-r", "command-a", "c4ai-aya", "aya-"}},
	{"perplexity.svg", []string{"perplexity", "sonar", "pplx"}},
	{"yi.svg", []string{"yi-", "yi/", "01-ai"}},
	{"zeroone.svg", []string{"zeroone", "ling-"}},
	{"wenxin.svg", []string{"wenxin", "ernie"}},
	{"baidu.svg", []string{"baidu"}},
	{"hunyuan.svg", []string{"hunyuan"}},
	{"spark.svg", []string{"spark", "iflytek"}},
	{"nvidia.svg", []string{"nvidia", "nemotron"}},
	{"microsoft.svg", []string{"microsoft", "phi-", "phi3", "phi4"}},
	{"ai21.svg", []string{"ai21", "jamba"}},
	{"stepfun.svg", []string{"stepfun", "step-"}},
	{"sensenova.svg", []string{"sensenova", "sensechat"}},
	{"internlm.svg", []string{"internlm", "intern-"}},
	{"skywork.svg", []string{"skywork"}},
	{"longcat.svg", []string{"longcat"}},
	{"qingyan.svg", []string{"qingyan"}},
	{"kling.svg", []string{"kling"}},
	{"vidu.svg", []string{"vidu"}},
	{"jimeng.svg", []string{"jimeng"}},
	{"nova.svg", []string{"nova-"}},
	{"groq.svg", []string{"groq"}},
	{"huggingface.svg", []string{"huggingface"}},
	{"ollama.svg", []string{"ollama"}},
	{"together.svg", []string{"together"}},
	{"fireworks.svg", []string{"fireworks"}},
	{"openrouter.svg", []string{"openrouter"}},
	{"alibaba.svg", []string{"alibaba", "alibabacloud"}},
	{"stability.svg", []string{"stability", "sd3", "stable-diffusion"}},
	{"midjourney.svg", []string{"midjourney"}},
	{"jina.svg", []string{"jina"}},
	{"voyage.svg", []string{"voyage"}},
	{"runway.svg", []string{"runway"}},
	{"luma.svg", []string{"luma"}},
	{"suno.svg", []string{"suno"}},
}

const (
	chatIconFnOld   = `function rwX(e,n=16){if(void 0===e)return null;`
	chatIconRowOld  = `(e.icon||e.provider)&&(0,tY.jsx)("span",{className:"flex size-[18px] shrink-0 items-center justify-center",children:e.icon??rwX(e.provider)})`
	chatIconRowNew  = `(e.icon||e.provider||bailanSrc(e))&&(0,tY.jsx)("span",{className:"flex size-[18px] shrink-0 items-center justify-center",children:e.icon??rwX(e.provider)??(bailanSrc(e)?(0,tY.jsx)("img",{src:bailanSrc(e),alt:"",draggable:!1,width:16,height:16,style:{display:"block",objectFit:"contain"}}):null)})`
	chatCardIconOld = `r=R.icon??(n?rwr(e.modelUid)??rwX(R.provider):rwX(R.provider))`

	// Devin 1.126：rwX 改成画 SVG，厂商图标是 rw0，按模型找图标是 rwo。
	chatIconFn126     = `function rw0(e,n=16){if(void 0===e)return null;`
	chatIconRow126Old = `(e.icon||e.provider)&&(0,tY.jsx)("span",{className:"flex size-[18px] shrink-0 items-center justify-center",children:e.icon??rw0(e.provider)})`
	chatIconRow126New = `(e.icon||e.provider||bailanSrc(e))&&(0,tY.jsx)("span",{className:"flex size-[18px] shrink-0 items-center justify-center",children:e.icon??rw0(e.provider)??(bailanSrc(e)?(0,tY.jsx)("img",{src:bailanSrc(e),alt:"",draggable:!1,width:16,height:16,style:{display:"block",objectFit:"contain"}}):null)})`
	chatCardIcon126   = `r=R.icon??(n?rwo(e.modelUid)??rw0(R.provider):rw0(R.provider))`

	// Devin 3.11.238：厂商图标是 rkv，按模型找图标是 rwO。
	chatIconFn238     = `function rkv(e,n=16){if(void 0===e)return null;`
	chatIconRow238Old = `(e.icon||e.provider)&&(0,tY.jsx)("span",{className:"flex size-[18px] shrink-0 items-center justify-center",children:e.icon??rkv(e.provider)})`
	chatIconRow238New = `(e.icon||e.provider||bailanSrc(e))&&(0,tY.jsx)("span",{className:"flex size-[18px] shrink-0 items-center justify-center",children:e.icon??rkv(e.provider)??(bailanSrc(e)?(0,tY.jsx)("img",{src:bailanSrc(e),alt:"",draggable:!1,width:16,height:16,style:{display:"block",objectFit:"contain"}}):null)})`
	chatCardIcon238   = `r=R.icon??(n?rwO(e.modelUid)??rkv(R.provider):rkv(R.provider))`

	chatSnapOld     = `o=(0,tK.useMemo)(()=>null==n||!r||0===r.length||r.includes(n)?n:r[0],[n,r])`
	chatSnapNew     = `o=(0,tK.useMemo)(()=>null==n||!r||0===r.length||r.includes(n)||e==="model"?n:r[0],[n,r,e])`
	shownSnapOld    = `r.includes(n)||e==="model"?n:r[0]`
	shownSnapNew    = `r.includes(n)||e==="model"||typeof n=="string"&&n.indexOf("@")>=0?n:r[0]`
	chatShownOld    = `J=rEs({configId:U,rawValue:j,availableValues:q.length>0?q:null,activeSessionId:c})??j,z=(0,tK.useMemo)(()=>J?[J]:[],[J]),[G,H]=(0,tK.useState)(!1)`
	chatShownNew    = `J=bailanChatModel(c,rEs({configId:U,rawValue:j,availableValues:q.length>0?q:null,activeSessionId:c})??j),z=(0,tK.useMemo)(()=>J?[J]:[],[J]),[G,H]=(0,tK.useState)(!1)`
	chatShown126    = `J=rEu({configId:U,rawValue:j,availableValues:q.length>0?q:null,activeSessionId:c})??j,z=(0,tK.useMemo)(()=>J?[J]:[],[J]),[G,H]=(0,tK.useState)(!1)`
	chatShown126New = `J=bailanChatModel(c,rEu({configId:U,rawValue:j,availableValues:q.length>0?q:null,activeSessionId:c})??j),z=(0,tK.useMemo)(()=>J?[J]:[],[J]),[G,H]=(0,tK.useState)(!1)`
	chatShown238    = `H=rEh({configId:J,rawValue:z,availableValues:G.length>0?G:null,activeSessionId:c})??z,V=(0,tK.useMemo)(()=>H?[H]:[],[H]),[W,$]=(0,tK.useState)(!1)`
	chatShown238New = `H=bailanChatModel(c,rEh({configId:J,rawValue:z,availableValues:G.length>0?G:null,activeSessionId:c})??z),V=(0,tK.useMemo)(()=>H?[H]:[],[H]),[W,$]=(0,tK.useState)(!1)`
	chatSyncOld     = `V=(0,tK.useRef)(null);(0,tK.useEffect)(()=>u.toggleModelSelector`
	chatSyncNew     = chatSyncOld
	chatBindOld     = `t.send(JSON.stringify([p[0],p[1]==="-"?"":p[1],Number(p[2])||0]))}catch(e){}})(e)`
	chatBindNew     = `t.send(JSON.stringify([p[0],p[1]==="-"?"":p[1],Number(p[2])||0]));let s=new XMLHttpRequest;s.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);s.setRequestHeader("content-type","text/plain");s.send(JSON.stringify([c||"",u]))}catch(e){}})(e)`

	// IDE 页面上正在显示的模型，切过来直接发送时也要告诉 Hub。
	chatShownSyncFn     = `function bailanSyncShown(s,u){if(typeof u!="string"||u.indexOf("@")<0)return;bailanSyncModel(u);if(!s)return;try{let n=new XMLHttpRequest;n.open("POST","http://127.0.0.1:5099/api/set_session_model",!1);n.setRequestHeader("content-type","text/plain");n.send(JSON.stringify([s,u]))}catch(e){}}`
	chatShownSyncOld    = `V=(0,tK.useRef)(null);(0,tK.useEffect)(()=>u.toggleModelSelector.on(()=>(V.current?.click(),!0)),[u]);`
	chatShownSyncNew    = `V=(0,tK.useRef)(null);(0,tK.useEffect)(()=>u.toggleModelSelector.on(()=>(V.current?.click(),!0)),[u]);(0,tK.useEffect)(()=>{bailanSyncShown(c,J)},[c,J]);`
	chatShownSync238Old = `Y=(0,tK.useRef)(null);(0,tK.useEffect)(()=>u.toggleModelSelector.on(()=>(Y.current?.click(),!0)),[u]);`
	chatShownSync238New = `Y=(0,tK.useRef)(null);(0,tK.useEffect)(()=>u.toggleModelSelector.on(()=>(Y.current?.click(),!0)),[u]);(0,tK.useEffect)(()=>{bailanSyncShown(c,H)},[c,H]);`

	agentIconFnOld = `function Vn(e,t=16){if(e===void 0)return null;`
)

func agentIconRowOld() string {
	bt := "`"
	return "(e.icon||e.provider)&&(0,q.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:e.icon??Vn(e.provider)})"
}

// Devin 3.11.238 的 Agent 列表把厂商图标函数换成了 Kn。
func agentIconFn238() string {
	return `function Kn(e,t=16){if(e===void 0)return null;`
}

func agentIconRow238Old() string {
	bt := "`"
	return "(e.icon||e.provider)&&(0,q.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:e.icon??Kn(e.provider)})"
}

func agentIconRow238New() string {
	bt := "`"
	// 带 provider 时 bailanSrc 会直接返回空，这里只拿编号和名字去配图。
	lookup := "bailanSrc({modelUid:e.modelUid,label:e.label})"
	return "(e.icon||e.provider||" + lookup + ")&&(0,q.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:e.icon??Kn(e.provider)??(" + lookup + "?(0,q.jsx)(" + bt + "img" + bt + ",{src:" + lookup + ",alt:" + bt + bt + ",draggable:!1,width:16,height:16,style:{display:" + bt + "block" + bt + ",objectFit:" + bt + "contain" + bt + "}}):null)})"
}

// 下拉列表真正画图标的是这一行，不是上面的 ModelSelectOptionItem。
func agentListIcon238Old() string {
	bt := "`"
	return "n=C.icon??(t?Dt(e.modelUid)??Kn(C.provider):Kn(C.provider));return n?(0,J.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:n}):null"
}

func agentListIcon238New() string {
	bt := "`"
	lookup := "bailanSrc({modelUid:C.modelUid,label:C.label})"
	img := "(" + lookup + "?(0,J.jsx)(" + bt + "img" + bt + ",{src:" + lookup + ",alt:" + bt + bt + ",draggable:!1,width:16,height:16,style:{display:" + bt + "block" + bt + ",objectFit:" + bt + "contain" + bt + "}}):null)"
	return "n=C.icon??(t?Dt(e.modelUid)??Kn(C.provider):Kn(C.provider))??" + img + ";return n?(0,J.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:n}):null"
}

func agentIconRow312Old() string {
	bt := "`"
	return "(e.icon||e.provider)&&(0,J.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:e.icon??Kn(e.provider)})"
}

func agentIconRow312New() string {
	bt := "`"
	lookup := "bailanSrc({modelUid:e.modelUid,label:e.label})"
	return "(e.icon||e.provider||" + lookup + ")&&(0,J.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:e.icon??Kn(e.provider)??(" + lookup + "?(0,J.jsx)(" + bt + "img" + bt + ",{src:" + lookup + ",alt:" + bt + bt + ",draggable:!1,width:16,height:16,style:{display:" + bt + "block" + bt + ",objectFit:" + bt + "contain" + bt + "}}):null)})"
}

func agentListIcon312Old() string {
	bt := "`"
	return "n=S.icon??(t?Nt(e.modelUid)??Kn(S.provider):Kn(S.provider));return n?(0,Y.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:n}):null"
}

func agentListIcon312New() string {
	bt := "`"
	lookup := "bailanSrc({modelUid:S.modelUid,label:S.label})"
	img := "(" + lookup + "?(0,Y.jsx)(" + bt + "img" + bt + ",{src:" + lookup + ",alt:" + bt + bt + ",draggable:!1,width:16,height:16,style:{display:" + bt + "block" + bt + ",objectFit:" + bt + "contain" + bt + "}}):null)"
	return "n=S.icon??(t?Nt(e.modelUid)??Kn(S.provider):Kn(S.provider))??" + img + ";return n?(0,Y.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:n}):null"
}

func agentIconRowNew() string {
	bt := "`"
	return "(e.icon||e.provider||bailanSrc(e))&&(0,q.jsx)(" + bt + "span" + bt + ",{className:" + bt + "flex size-[18px] shrink-0 items-center justify-center" + bt + ",children:e.icon??Vn(e.provider)??(bailanSrc(e)?(0,q.jsx)(" + bt + "img" + bt + ",{src:bailanSrc(e),alt:" + bt + bt + ",draggable:!1,width:16,height:16,style:{display:" + bt + "block" + bt + ",objectFit:" + bt + "contain" + bt + "}}):null)})"
}

func agentCardIconOld() string {
	return `n=S.icon??(t?St(e.modelUid)??Vn(S.provider):Vn(S.provider))`
}

func agentCardIconNew() string {
	bt := "`"
	return agentCardIconOld() + `??(bailanSrc(S)?(0,J.jsx)(` + bt + `img` + bt + `,{src:bailanSrc(S),alt:` + bt + bt + `,draggable:!1,width:16,height:16,style:{display:` + bt + `block` + bt + `,objectFit:` + bt + `contain` + bt + `}}):null)`
}

func chatSessionJS() string {
	return `var bailanChat={};function bailanChatModel(s,v){if(typeof v=="string"&&v.indexOf("@")>=0){if(s)bailanChat[s]=v;bailanChat._=v;return v}return s&&bailanChat[s]||bailanChat._||v}` + agentSyncJS()
}

func patchChatSession(text string) (string, bool, string) {
	changed := false
	const broken = `,(0,tK.useEffect)(()=>{bailanSyncModel(J)},[J]),[G,H]=(0,tK.useState)(!1)`
	if strings.Contains(text, broken) {
		text = strings.Replace(text, broken, `,[G,H]=(0,tK.useState)(!1)`, 1)
		changed = true
	}
	js := chatSessionJS()
	if !strings.Contains(text, "function bailanChatModel(") {
		anchor := ""
		for _, candidate := range []string{chatIconFn238, chatIconFn126, chatIconFnOld} {
			if strings.Count(text, candidate) == 1 {
				anchor = candidate
				break
			}
		}
		if anchor == "" {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, anchor, js+anchor, 1)
		changed = true
	}
	if !strings.Contains(text, chatSnapNew) && strings.Contains(text, chatSnapOld) {
		if strings.Count(text, chatSnapOld) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, chatSnapOld, chatSnapNew, 1)
		changed = true
	}
	for _, pair := range [][2]string{{chatShown238, chatShown238New}, {chatShown126, chatShown126New}, {chatShownOld, chatShownNew}} {
		if strings.Contains(text, pair[1]) || !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if !strings.Contains(text, chatSyncNew) && strings.Contains(text, chatSyncOld) && chatSyncNew != chatSyncOld {
		if strings.Count(text, chatSyncOld) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, chatSyncOld, chatSyncNew, 1)
		changed = true
	}
	const ideSync = `(0,tK.useEffect)(()=>{bailanSyncModel(J)},[J]);`
	if strings.Contains(text, ideSync) {
		text = strings.Replace(text, ideSync, "", 1)
		changed = true
	}
	if !strings.Contains(text, "function bailanSyncShown(") && strings.Contains(text, "function bailanSyncModel(") {
		const mark = `catch(e){bailanSynced=""}}`
		if strings.Count(text, mark) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, mark, mark+chatShownSyncFn, 1)
		changed = true
	}
	for _, pair := range [][2]string{{chatShownSync238Old, chatShownSync238New}, {chatShownSyncOld, chatShownSyncNew}} {
		if strings.Contains(text, pair[1]) || !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if !strings.Contains(text, chatBindNew) && strings.Contains(text, chatBindOld) {
		if strings.Count(text, chatBindOld) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, chatBindOld, chatBindNew, 1)
		changed = true
	}
	if strings.Contains(text, shownSnapOld) && !strings.Contains(text, shownSnapNew) {
		if strings.Count(text, shownSnapOld) != 1 {
			return text, false, "普通对话的会话模型对不上，没有改动"
		}
		text = strings.Replace(text, shownSnapOld, shownSnapNew, 1)
		changed = true
	}
	return text, changed, ""
}

func restoreChatSession(text string) (string, bool, string) {
	changed := false
	if strings.Contains(text, shownSnapNew) {
		if strings.Count(text, shownSnapNew) != 1 {
			return text, false, "普通对话的会话模型对不上，没能还原"
		}
		text = strings.Replace(text, shownSnapNew, shownSnapOld, 1)
		changed = true
	}
	if chatSyncNew != chatSyncOld && strings.Contains(text, chatSyncNew) {
		if strings.Count(text, chatSyncNew) != 1 {
			return text, false, "普通对话的会话模型对不上，没能还原"
		}
		text = strings.Replace(text, chatSyncNew, chatSyncOld, 1)
		changed = true
	}
	for _, pair := range [][2]string{{chatShown238New, chatShown238}, {chatShown126New, chatShown126}, {chatShownNew, chatShownOld}} {
		if !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return text, false, "普通对话的会话模型对不上，没能还原"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if strings.Contains(text, chatSnapNew) {
		if strings.Count(text, chatSnapNew) != 1 {
			return text, false, "普通对话的会话模型对不上，没能还原"
		}
		text = strings.Replace(text, chatSnapNew, chatSnapOld, 1)
		changed = true
	}
	for _, pair := range [][2]string{{chatShownSync238New, chatShownSync238Old}, {chatShownSyncNew, chatShownSyncOld}} {
		if !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return text, false, "普通对话的会话模型对不上，没能还原"
		}
		text = strings.Replace(text, pair[0], pair[1], 1)
		changed = true
		break
	}
	if strings.Contains(text, chatShownSyncFn) {
		text = strings.Replace(text, chatShownSyncFn, "", 1)
		changed = true
	}
	js := chatSessionJS()
	if strings.Contains(text, js) {
		text = strings.Replace(text, js, "", 1)
		changed = true
	}
	if strings.Contains(text, chatBindNew) {
		if strings.Count(text, chatBindNew) != 1 {
			return text, false, "普通对话的会话模型对不上，没能还原"
		}
		text = strings.Replace(text, chatBindNew, chatBindOld, 1)
		changed = true
	}
	return text, changed, ""
}

func chatCardIconNew() string {
	return chatCardIconOld + `??(bailanSrc(R)?(0,tY.jsx)("img",{src:bailanSrc(R),alt:"",draggable:!1,width:16,height:16,style:{display:"block",objectFit:"contain"}}):null)`
}

func chatCardIcon126New() string {
	return chatCardIcon126 + `??(bailanSrc(R)?(0,tY.jsx)("img",{src:bailanSrc(R),alt:"",draggable:!1,width:16,height:16,style:{display:"block",objectFit:"contain"}}):null)`
}

func chatCardIcon238New() string {
	return chatCardIcon238 + `??(bailanSrc(R)?(0,tY.jsx)("img",{src:bailanSrc(R),alt:"",draggable:!1,width:16,height:16,style:{display:"block",objectFit:"contain"}}):null)`
}

// patchChatCards 补上 IDE 列表里真正画出来的那一行。上一行 ModelSelectOptionItem 并不负责这里。
func patchChatCards(text string) (string, bool, string) {
	for _, pair := range [][2]string{{chatCardIcon238, chatCardIcon238New()}, {chatCardIcon126, chatCardIcon126New()}, {chatCardIconOld, chatCardIconNew()}} {
		if strings.Contains(text, pair[1]) || !strings.Contains(text, pair[0]) {
			continue
		}
		if strings.Count(text, pair[0]) != 1 {
			return text, false, "普通对话的图标位置对不上，没有改动"
		}
		return strings.Replace(text, pair[0], pair[1], 1), true, ""
	}
	return text, false, ""
}

func restoreChatCards(text string) (string, bool, string) {
	for _, pair := range [][2]string{{chatCardIcon238New(), chatCardIcon238}, {chatCardIcon126New(), chatCardIcon126}, {chatCardIconNew(), chatCardIconOld}} {
		n := strings.Count(text, pair[0])
		if n == 0 {
			continue
		}
		if n != 1 {
			return text, false, "普通对话的图标对不上，没能还原"
		}
		return strings.Replace(text, pair[0], pair[1], 1), true, ""
	}
	return text, false, ""
}

func patchChatIcons(text string) (string, bool, string) {
	for _, pair := range [][3]string{
		{chatIconFn238, chatIconRow238Old, chatIconRow238New},
		{chatIconFn126, chatIconRow126Old, chatIconRow126New},
		{chatIconFnOld, chatIconRowOld, chatIconRowNew},
	} {
		if strings.Contains(text, pair[2]) && strings.Contains(text, "function bailanSrc(e){") {
			return text, false, ""
		}
		if !strings.Contains(text, pair[0]) || !strings.Contains(text, pair[1]) {
			continue
		}
		return patchVendorIcons(text, pair[0], pair[1], pair[2])
	}
	return text, false, ""
}

func restoreChatIcons(text string) (string, bool) {
	for _, pair := range [][3]string{
		{chatIconFn238, chatIconRow238Old, chatIconRow238New},
		{chatIconFn126, chatIconRow126Old, chatIconRow126New},
		{chatIconFnOld, chatIconRowOld, chatIconRowNew},
	} {
		if next, ok := restoreVendorIcons(text, pair[0], pair[1], pair[2]); ok {
			return next, true
		}
	}
	return text, false
}

var (
	bailanSrcOnce sync.Once
	bailanSrcJS   string
	bailanSrcErr  error
)

func bailanSrcFunction() (string, error) {
	bailanSrcOnce.Do(func() {
		var b strings.Builder
		b.WriteString(`function bailanSrc(e){if(!e||e.provider)return"";let id=String(e.modelUid||"").toLowerCase(),label=String(e.label||"").toLowerCase(),rows=[`)
		for i, item := range vendorIcons {
			raw, err := ui.Icons.ReadFile("icons/" + item.file)
			if err != nil {
				bailanSrcErr = err
				return
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`["data:image/svg+xml;base64,`)
			b.WriteString(base64.StdEncoding.EncodeToString(raw))
			b.WriteByte('"')
			for _, prefix := range item.prefixes {
				b.WriteString(`,"`)
				b.WriteString(prefix)
				b.WriteByte('"')
			}
			b.WriteByte(']')
		}
		b.WriteString(`];for(let r of rows)for(let i=1;i<r.length;i++){let p=r[i];if(id.startsWith(p)||label.startsWith(p))return r[0]}return""}`)
		bailanSrcJS = b.String()
	})
	return bailanSrcJS, bailanSrcErr
}

func patchVendorIcons(text, fnOld, rowOld, rowNew string) (string, bool, string) {
	src, err := bailanSrcFunction()
	if err != nil {
		return text, false, "厂商图标读不出来"
	}
	if strings.Contains(text, src) && strings.Contains(text, rowNew) {
		return text, false, ""
	}
	if strings.Count(text, fnOld) != 1 || strings.Count(text, rowOld) != 1 {
		return text, false, "图标位置对不上，没有改动"
	}
	text = strings.Replace(text, fnOld, src+fnOld, 1)
	text = strings.Replace(text, rowOld, rowNew, 1)
	return text, true, ""
}

func restoreVendorIcons(text, fnOld, rowOld, rowNew string) (string, bool) {
	src, err := bailanSrcFunction()
	if err != nil {
		return text, false
	}
	changed := false
	if strings.Contains(text, src+fnOld) {
		text = strings.Replace(text, src+fnOld, fnOld, 1)
		changed = true
	}
	if strings.Contains(text, rowNew) {
		if strings.Count(text, rowNew) != 1 {
			return text, changed
		}
		text = strings.Replace(text, rowNew, rowOld, 1)
		changed = true
	}
	return text, changed
}

func applyAgentIcons(resources string) (bool, string) {
	return writeAgentIcons(resources, true)
}

func restoreAgentIcons(resources string) (bool, string) {
	return writeAgentIcons(resources, false)
}

func writeAgentIcons(resources string, apply bool) (bool, string) {
	assets := filepath.Join(resources, "app", "out", "vs", "workbench", "devin-webapp-host", "assets")
	entries, err := os.ReadDir(assets)
	if err != nil {
		return false, ""
	}
	type iconPair struct{ fn, rowOld, rowNew string }
	pairs := []iconPair{
		{agentIconFn238(), agentIconRow312Old(), agentIconRow312New()},
		{agentIconFn238(), agentIconRow238Old(), agentIconRow238New()},
		{agentIconFnOld, agentIconRowOld(), agentIconRowNew()},
	}
	cardOld, cardNew := agentCardIconOld(), agentCardIconNew()
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
		seen := strings.Contains(text, cardOld) || strings.Contains(text, cardNew) || strings.Contains(text, agentListIcon312Old()) || strings.Contains(text, agentListIcon312New())
		for _, pair := range pairs {
			if strings.Contains(text, pair.rowOld) || strings.Contains(text, pair.rowNew) {
				seen = true
				break
			}
		}
		if !seen {
			continue
		}
		if apply {
			next := text
			changed := false
			// 先用「认结构」的正则套图标行并插入 bailanSrc；没套上再退回按版本精确锚点。
			if out, ok, _ := tryAgentIconRowApply(next); ok {
				next = out
				changed = true
			} else {
				for _, pair := range pairs {
					if strings.Contains(next, pair.rowNew) && strings.Contains(next, "function bailanSrc(e){") {
						break
					}
					if !strings.Contains(next, pair.rowOld) || !strings.Contains(next, pair.fn) {
						continue
					}
					patched, ok, note := patchVendorIcons(next, pair.fn, pair.rowOld, pair.rowNew)
					if note != "" && !ok && !strings.Contains(next, "function bailanSrc(e){") {
						return false, "Agent 的" + note
					}
					if ok {
						next = patched
						changed = true
						break
					}
				}
			}
			if out, ok, bad := tryAgentIconListApply(next); bad {
				return false, "Agent 的图标位置对不上，没有改动"
			} else if ok {
				next = out
				changed = true
			} else if strings.Contains(next, agentListIcon312Old()) {
				if strings.Count(next, agentListIcon312Old()) != 1 {
					return false, "Agent 的图标位置对不上，没有改动"
				}
				next = strings.Replace(next, agentListIcon312Old(), agentListIcon312New(), 1)
				changed = true
			}
			if strings.Contains(next, agentListIcon238Old()) {
				if strings.Count(next, agentListIcon238Old()) != 1 {
					return false, "Agent 的图标位置对不上，没有改动"
				}
				next = strings.Replace(next, agentListIcon238Old(), agentListIcon238New(), 1)
				changed = true
			}
			if strings.Contains(next, cardNew) {
				// 真正画出来的那一行已经接上。
			} else if strings.Contains(next, cardOld) {
				if strings.Count(next, cardOld) != 1 {
					return false, "Agent 的图标位置对不上，没有改动"
				}
				next = strings.Replace(next, cardOld, cardNew, 1)
				changed = true
			}
			if !changed {
				return false, ""
			}
			if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
				return false, "写入 Agent 图标失败"
			}
			return true, "对不上的模型用上了 Hub 的图标"
		}
		next := text
		changed := false
		// 先还原正则版图标行（顺带移除 bailanSrc）；卡片/列表那几处仍按精确还原。
		if out, ok := tryAgentIconRowRestore(next); ok {
			next = out
			changed = true
		}
		if strings.Contains(next, cardNew) {
			if strings.Count(next, cardNew) != 1 {
				return false, "Agent 的图标对不上，没能还原"
			}
			next = strings.Replace(next, cardNew, cardOld, 1)
			changed = true
		}
		for _, pair := range pairs {
			restored, ok := restoreVendorIcons(next, pair.fn, pair.rowOld, pair.rowNew)
			if ok {
				next = restored
				changed = true
				break
			}
		}
		if out, ok := tryAgentIconListRestore(next); ok {
			next = out
			changed = true
		} else if strings.Contains(next, agentListIcon312New()) {
			if strings.Count(next, agentListIcon312New()) != 1 {
				return false, "Agent 的图标对不上，没能还原"
			}
			next = strings.Replace(next, agentListIcon312New(), agentListIcon312Old(), 1)
			changed = true
		}
		if strings.Contains(next, agentListIcon238New()) {
			if strings.Count(next, agentListIcon238New()) != 1 {
				return false, "Agent 的图标对不上，没能还原"
			}
			next = strings.Replace(next, agentListIcon238New(), agentListIcon238Old(), 1)
			changed = true
		}
		if !changed {
			continue
		}
		if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
			return false, "还原 Agent 图标失败"
		}
		return true, "已去掉 Hub 填上的模型图标"
	}
	return false, ""
}
