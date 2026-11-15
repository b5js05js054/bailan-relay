package proxysrv

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"testing"
)

func TestRememberSessionModelUpdatesShortNameOnSwitch(t *testing.T) {
	s := &Server{}
	s.RememberSessionModel("acp/devin-cli/maple-beechnut", "mimo-v2.6-pro", "low", 1000000)
	s.RememberSessionModel("acp/devin-cli/estimated-tree", "gpt-6.1-terra", "low", 1000000)
	s.RememberSessionModel("acp/devin-cli/maple-beechnut", "gpt-6.1-terra", "high", 200000)

	maple, _, ok := s.sessionChoice([]string{"maple-beechnut"})
	if !ok || maple.Model != "gpt-6.1-terra" || maple.Effort != "high" || maple.Context != 200000 {
		t.Fatalf("同会话换模型后短名仍是旧模型: %+v ok=%v", maple, ok)
	}
	other, _, ok := s.sessionChoice([]string{"estimated-tree"})
	if !ok || other.Model != "gpt-6.1-terra" || other.Effort != "low" {
		t.Fatalf("另一个会话被带跑了: %+v ok=%v", other, ok)
	}
	// 当前会话的短名先出现。后面即使跟着另一个会话的完整名字，也不能改用那一个。
	maple, key, ok := s.sessionChoice([]string{"maple-beechnut", "acp/devin-cli/estimated-tree"})
	if !ok || key != "maple-beechnut" || maple.Model != "gpt-6.1-terra" {
		t.Fatalf("更长的编号盖掉了当前会话: key=%s %+v", key, maple)
	}
}

func TestSessionChoiceFromBodyUsesUnderscoreName(t *testing.T) {
	s := &Server{}
	s.RememberSessionModel("devin-acp/devin-cli/whimsical-system", "deepseek-v4.1-flash-1017", "low", 1000000)
	s.RememberSessionModel("devin-acp/devin-cli/hilarious-slayer", "swe2flash", "max", 262000)

	got, _, ok := s.sessionChoice([]string{"devin-acp_devin-cli_whimsical-system"})
	if !ok || got.Model != "deepseek-v4.1-flash-1017" {
		t.Fatalf("下划线编号没有对上: %+v ok=%v", got, ok)
	}
	body := []byte("prefix devin-acp_devin-cli_hilarious-slayer suffix")
	got, _, ok = s.sessionChoiceFromBody(body)
	if !ok || got.Model != "swe2flash" || got.Effort != "max" {
		t.Fatalf("正文里的会话名没有对上: %+v ok=%v", got, ok)
	}
	mixed := []byte("devin-acp_devin-cli_whimsical-system and hilarious-slayer")
	if _, _, ok = s.sessionChoiceFromBody(mixed); ok {
		t.Fatal("两个会话的名字同时出现时不应猜一个")
	}
	frame := gzipConnectFrame([]byte("xx devin-acp_devin-cli_whimsical-system yy"))
	got, _, ok = s.sessionChoiceFromBody(frame)
	if !ok || got.Model != "deepseek-v4.1-flash-1017" {
		t.Fatalf("压缩帧里的会话名没有对上: %+v ok=%v", got, ok)
	}
}

func gzipConnectFrame(payload []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(payload)
	_ = w.Close()
	z := buf.Bytes()
	head := make([]byte, 5)
	head[0] = 1
	binary.BigEndian.PutUint32(head[1:], uint32(len(z)))
	return append(head, z...)
}
