package connectframe

import "testing"

func TestChatModelReadsRawAndConnectFrame(t *testing.T) {
	payload := encodeStringField(21, "glm-5-2")
	if got := ChatModel(payload); got != "glm-5-2" {
		t.Fatalf("raw=%q", got)
	}
	frame, err := encodeConnectFrame(payload, false, []byte{0x02, 0, 0, 0, 2, '{', '}'})
	if err != nil {
		t.Fatal(err)
	}
	if got := ChatModel(frame); got != "glm-5-2" {
		t.Fatalf("frame=%q", got)
	}
	if !IsSWE16("SWE-1.6") || !IsSWE16("swe-1-6-fast") || IsSWE16("glm-5-2") {
		t.Fatal("SWE-1.6 detection mismatch")
	}
}

func TestReplaceChatModelStripsEffortVariant(t *testing.T) {
	payload := encodeStringField(21, "gpt-5.6@high@400000")
	frame, err := encodeConnectFrame(payload, false, []byte{0x02, 0, 0, 0, 2, '{', '}'})
	if err != nil {
		t.Fatal(err)
	}
	out := ReplaceChatModel(frame, "gpt-5.6", true)
	if got := ChatModel(out); got != "gpt-5.6" {
		t.Fatalf("model=%q", got)
	}
	if len(out) < 7 || string(out[len(out)-2:]) != "{}" {
		t.Fatal("connect trailer was dropped")
	}
	raw := ReplaceChatModel(payload, "gpt-5.6", false)
	if got := ChatModel(raw); got != "gpt-5.6" {
		t.Fatalf("raw=%q", got)
	}
	// Content-Type 未标明 connect 时，帧也不能被当成裸 protobuf 改坏。
	unmarked := ReplaceChatModel(frame, "gpt-5.6", false)
	if got := ChatModel(unmarked); got != "gpt-5.6" {
		t.Fatalf("unmarked frame=%q", got)
	}
	if len(unmarked) < 7 || string(unmarked[len(unmarked)-2:]) != "{}" {
		t.Fatal("unmarked frame dropped the trailer")
	}
}
