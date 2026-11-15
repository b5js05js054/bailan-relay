package devin

import (
	"strings"
	"testing"
)

func roundTripAgentRate(t *testing.T, label, original string) {
	t.Helper()
	applied, changed, bad := tryAgentRateApply(original)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !changed {
		t.Fatalf("%s：没识别出倍率角标", label)
	}
	if !strings.Contains(applied, rateAgentMark) {
		t.Fatalf("%s：套完没有标记 %q", label, rateAgentMark)
	}
	// 幂等。
	if _, again, _ := tryAgentRateApply(applied); again {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, ok := tryAgentRateRestore(applied)
	if !ok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != original {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, original, restored)
	}
}

func TestAgentRateRoundTrip(t *testing.T) {
	roundTripAgentRate(t, "3.12.28", agentRate312Old())
	roundTripAgentRate(t, "3.11.238", agentRate238Old())
	roundTripAgentRate(t, "older", agentRateOld())

	// 模拟将来 Devin 重新压缩、换了一批名字（累加器 q、promo 源 w、下标 z、组件 Aa/Bb/Cc/Dd）。
	future := "q=[];return e.isBeta?q.push({...nn.beta,icon:e.betaWarningMessage?(0,ZZ.jsx)(Aa,{size:12,className:`flex-none`}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&q.push(nn.new),e.fastStatus?.isActive&&q.push({...nn.fast,tooltip:e.fastStatus.tooltip}),q.length===0?w?(0,ZZ.jsx)(Bb,{tooltip:Ee(w,t),children:(0,ZZ.jsx)(Dd,{size:12,className:`text-text-accent-primary flex-none`})}):null:(0,ZZ.jsx)(`div`,{className:`flex items-center gap-1`,children:q.map((e,z)=>(0,ZZ.jsx)(Cc,{config:e,promo:z===0?w:void 0,t},e.key))})"
	roundTripAgentRate(t, "future-renamed", future)
}

// 普通对话用双引号 key、promo 写成 0===r，Agent 正则不应把它改动。
func TestAgentRateLeavesChatAlone(t *testing.T) {
	if _, changed, _ := tryAgentRateApply(chatRateOld); changed {
		t.Error("Agent 倍率正则不应改动普通对话那段")
	}
}

func roundTripChatRate(t *testing.T, label, original string) {
	t.Helper()
	applied, changed, bad := tryChatRateApply(original)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !changed {
		t.Fatalf("%s：没识别出倍率角标", label)
	}
	if !strings.Contains(applied, rateChatMark) {
		t.Fatalf("%s：套完没有标记 %q", label, rateChatMark)
	}
	if _, again, _ := tryChatRateApply(applied); again {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, ok := tryChatRateRestore(applied)
	if !ok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != original {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, original, restored)
	}
}

func TestChatRateRoundTrip(t *testing.T) {
	roundTripChatRate(t, "chat-older", chatRateOld)
	roundTripChatRate(t, "chat-126", chatRate126Old)
	roundTripChatRate(t, "chat-238", chatRate238Old)

	// 模拟将来重新压缩、换名：累加器 b、promo 源 w、下标 r、组件 Mm/Nn/Pp/Qq。
	future := `b=[];return e.isBeta?b.push({...rr.beta,icon:e.betaWarningMessage?(0,QQ.jsx)(Mm,{size:12,className:"flex-none"}):void 0,tooltip:e.betaWarningMessage}):e.isNew&&b.push(rr.new),e.fastStatus?.isActive&&b.push({...rr.fast,tooltip:e.fastStatus.tooltip}),0===b.length?w?(0,QQ.jsx)(Nn,{tooltip:Oo(w,n),children:(0,QQ.jsx)(Pp,{size:12,className:"text-text-accent-primary flex-none"})}):null:(0,QQ.jsx)("div",{className:"flex items-center gap-1",children:b.map((e,r)=>(0,QQ.jsx)(Qq,{config:e,promo:0===r?w:void 0,t:n},e.key))})`
	roundTripChatRate(t, "chat-future", future)
}

// Agent 侧用反引号 key、promo 写成 n===0，普通对话正则不应改动它。
func TestChatRateLeavesAgentAlone(t *testing.T) {
	if _, changed, _ := tryChatRateApply(agentRate312Old()); changed {
		t.Error("普通对话倍率正则不应改动 Agent 那段")
	}
}
