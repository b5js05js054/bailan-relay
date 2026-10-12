package bodydelta

import (
	"bytes"
	"math/rand"
	"testing"
)

func randomBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	r.Read(b)
	return b
}

func TestEncodeApplyRoundTripTypicalConversationGrowth(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	base := randomBytes(r, 3<<20)

	// 模拟下一轮请求：开头元数据变了几个字节，中间插入新消息，末尾的工具定义不变
	target := append([]byte(nil), base[:100]...)
	target = append(target, []byte("new-request-id-123")...)
	target = append(target, base[118:2<<20]...)
	target = append(target, randomBytes(r, 40<<10)...)
	target = append(target, base[2<<20:]...)

	ix := NewIndex(base)
	payload, literal, ok := ix.Encode(target, 0)
	if !ok || literal > 50<<10 {
		t.Fatalf("增量过大: literal=%d", literal)
	}
	if len(payload) > 60<<10 {
		t.Fatalf("payload=%d", len(payload))
	}
	got, err := Apply(base, payload, MaxSize)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, target) {
		t.Fatal("还原结果与原始请求不一致")
	}
}

func TestEncodeApplyUnrelatedAndEdgeInputs(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	base := randomBytes(r, 200<<10)
	for _, target := range [][]byte{
		randomBytes(r, 150<<10), // 完全不同
		{},                      // 空
		base[:blockSize-1],      // 短于一个块
		append(append([]byte(nil), base...), base...), // 重复引用
	} {
		payload, _, _ := NewIndex(base).Encode(target, 0)
		got, err := Apply(base, payload, MaxSize)
		if err != nil || !bytes.Equal(got, target) {
			t.Fatalf("len=%d err=%v", len(target), err)
		}
	}
}

func TestBestPicksRelatedBaseAndRejectsUnrelated(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	convA := randomBytes(r, 1<<20)
	convB := randomBytes(r, 1<<20)
	next := append(append([]byte(nil), convA...), randomBytes(r, 10<<10)...)
	bases := []*Index{NewIndex(convB), NewIndex(convA)}

	ix, payload := Best(bases, next)
	if ix == nil || ix.Hash != Sum(convA) {
		t.Fatal("应选中同一对话的基准")
	}
	if got, err := Apply(ix.Base, payload, MaxSize); err != nil || !bytes.Equal(got, next) {
		t.Fatalf("还原失败: %v", err)
	}
	if ix, _ := Best(bases[:1], next); ix != nil {
		t.Fatal("不相关的基准不应被使用")
	}
}

func TestApplyRejectsCorruptOrOversized(t *testing.T) {
	base := []byte("0123456789")
	bad := [][]byte{
		[]byte("XXXX"),
		append(append([]byte(nil), magic...), opCopy, 5, 20), // 越界
		append(append([]byte(nil), magic...), opInsert, 9, 'a'),
		append(append([]byte(nil), magic...), 7),
	}
	for i, p := range bad {
		if _, err := Apply(base, p, MaxSize); err == nil {
			t.Fatalf("case %d 应报错", i)
		}
	}
	ok := append(append([]byte(nil), magic...), opCopy, 0, 10, opCopy, 0, 10)
	if _, err := Apply(base, ok, 15); err == nil {
		t.Fatal("超过 maxSize 应报错")
	}
}

func TestGzipRoundTripAndLimit(t *testing.T) {
	src := bytes.Repeat([]byte("hello "), 100000)
	z := Gzip(src)
	got, err := Gunzip(z, MaxSize)
	if err != nil || !bytes.Equal(got, src) {
		t.Fatalf("err=%v", err)
	}
	if _, err := Gunzip(z, 1000); err == nil {
		t.Fatal("超过上限应报错")
	}
}
