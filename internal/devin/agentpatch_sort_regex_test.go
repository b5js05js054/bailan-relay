package devin

import (
	"strings"
	"testing"
)

func roundTripSort(t *testing.T, label, original string) {
	t.Helper()
	applied, changed, bad := trySortApply(original)
	if bad {
		t.Fatalf("%s：套补丁被判成对不上", label)
	}
	if !changed {
		t.Fatalf("%s：没识别出排序 filter", label)
	}
	if !strings.Contains(applied, "||!0)") {
		t.Fatalf("%s：套完没有恒真标记", label)
	}
	if _, again, _ := trySortApply(applied); again {
		t.Fatalf("%s：重复套补丁又改了一次", label)
	}
	restored, ok := trySortRestore(applied)
	if !ok {
		t.Fatalf("%s：没能还原", label)
	}
	if restored != original {
		t.Fatalf("%s：还原没回到原文\n原文=%s\n还原=%s", label, original, restored)
	}
}

func TestSortRoundTrip(t *testing.T) {
	roundTripSort(t, "agent-useMemo", `h=(0,ut.useMemo)(()=>n.filter(e=>e!==t),[n,t])`)
	roundTripSort(t, "agent-238", `h=(0,ht.useMemo)(()=>n.filter(e=>e!==t),[n,t])`)
	roundTripSort(t, "agent-312", `h=D(()=>n.filter(e=>e!==t),[n,t])`)
	roundTripSort(t, "chat", `f=(0,tK.useMemo)(()=>r.filter(e=>e!==n),[r,n])`)
	// 模拟将来重新压缩、换名。
	roundTripSort(t, "future", `qq=(0,Zx.useMemo)(()=>aa.filter(bb=>bb!==cc),[aa,cc])`)
}

// 不是「排除当前项」这种 filter 的，不应被改动。
func TestSortLeavesOtherFiltersAlone(t *testing.T) {
	cases := []string{
		`x=(0,k.useMemo)(()=>a.filter(e=>e.active),[a])`,
		`y=(0,k.useMemo)(()=>a.filter(e=>e!==t),[a])`, // 依赖数组只有一个元素，结构不符
		`z=a.filter(e=>e!==t)`,                        // 不在 ()=> 工厂里
	}
	for _, c := range cases {
		if _, changed, _ := trySortApply(c); changed {
			t.Errorf("不该改动：%s", c)
		}
	}
}
