package devincatalog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"bailan-relay/internal/config"
)

func testModels() []config.Model {
	return []config.Model{{
		ID: "glm-5-2", Label: "GLM-5.2", ContextWindow: 128000, MaxOutput: 8192, SupportsThinking: true,
	}}
}

func officialConfig() []byte {
	raw := appendString(nil, 1, "Official")
	return appendString(raw, 22, "official-model")
}

func TestAugmentProtoAddsModelAndDropsOfficial(t *testing.T) {
	official := officialConfig()
	list := appendBytes(nil, 1, official)
	path := "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs"
	out, added, err := Augment(list, "application/proto", path, testModels())
	if err != nil || added == 0 {
		t.Fatalf("augment: added=%d err=%v", added, err)
	}
	if bytes.Contains(out, official) || bytes.Contains(out, []byte("official-model")) {
		t.Fatal("official model was kept")
	}
	for _, want := range []string{"glm-5-2", "GLM 5.2", uncategorizedName, groupSortName, "Provider", "Cost", inferenceURL} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("missing %s", want)
		}
	}
	again, added, err := Augment(out, "application/proto", path, testModels())
	if err != nil || added != 0 || again != nil {
		t.Fatalf("second augment added=%d out=%d err=%v", added, len(again), err)
	}
}

func TestAugmentSkipsModelAlreadyInOfficialList(t *testing.T) {
	existing := appendString(nil, 1, "GLM")
	existing = appendString(existing, 22, "glm-5-2@-@128000")
	list := appendBytes(nil, 1, existing)
	_, added, err := Augment(list, "application/proto", "/exa.language_server_pb.LanguageServerService/GetCascadeModelConfigs", testModels())
	if err != nil || added != 0 {
		t.Fatalf("added=%d err=%v", added, err)
	}
}

func TestAugmentKeepsEffortVariantsWhenBaseModelExists(t *testing.T) {
	existing := appendString(nil, 1, "Claude Opus 4.7")
	existing = appendString(existing, 22, "claude-opus-4-7")
	list := appendBytes(nil, 1, existing)
	model := config.Model{
		ID: "claude-opus-4-7", ReasoningEfforts: []string{"low", "high"},
		ContextOptions: []int{200000, 1000000},
	}
	path := "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs"
	out, added, err := Augment(list, "application/proto", path, []config.Model{model})
	if err != nil || added != 5 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	if !bytes.Contains(out, []byte("claude-opus-4-7@high@1000000")) {
		t.Fatal("effort variant missing")
	}
}

func TestAugmentUserStatusNestsModels(t *testing.T) {
	status := appendBytes(nil, 33, nil)
	body := appendBytes(nil, 1, status)
	out, added, err := Augment(body, "application/proto", "/exa.language_server_pb.LanguageServerService/GetUserStatus", testModels())
	if err != nil || added != 1 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	fields, err := parseFields(out)
	if err != nil || len(fields) != 1 || fields[0].num != 1 {
		t.Fatalf("outer fields: %+v %v", fields, err)
	}
	statusFields, err := parseFields(fields[0].val)
	if err != nil {
		t.Fatal(err)
	}
	var list []byte
	for _, field := range statusFields {
		if field.num == 33 {
			list = field.val
		}
	}
	uid, ok := fieldString(list, 22)
	if !ok {
		// field 22 lives inside the first model config, not the list itself.
		configs, err := parseFields(list)
		if err != nil {
			t.Fatal(err)
		}
		uid, ok = fieldString(configs[0].val, 22)
	}
	if !ok || uid != "glm-5-2@-@128000" {
		t.Fatalf("uid=%q ok=%v", uid, ok)
	}
}

func TestAugmentConnectFramePreservesTrailer(t *testing.T) {
	list := appendBytes(nil, 1, officialConfig())
	var framed []byte
	framed = append(framed, encodeFrame(0, list)...)
	framed = append(framed, encodeFrame(0x02, []byte("{}"))...)
	out, added, err := Augment(framed, "application/connect+proto", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", testModels())
	if err != nil || added == 0 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	frames, err := splitFrames(out)
	if err != nil || len(frames) != 2 || frames[1].flags != 0x02 || string(frames[1].data) != "{}" {
		t.Fatalf("frames=%v err=%v", frames, err)
	}
	if !bytes.Contains(frames[0].data, []byte("glm-5-2")) || bytes.Contains(frames[0].data, []byte("official-model")) {
		t.Fatal("framed payload kept the official model or lost the catalog model")
	}
}

func TestAugmentJSONList(t *testing.T) {
	body := []byte(`{"clientModelConfigs":[{"label":"Official","modelUid":"official-model"}],"clientModelSorts":[{"name":"Featured","groups":[{"groupName":"Featured","modelLabels":["Official"]}]}]}`)
	out, added, err := Augment(body, "application/json", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", testModels())
	if err != nil || added == 0 {
		t.Fatalf("added=%d err=%v body=%s", added, err, out)
	}
	var doc struct {
		Configs []struct {
			Label    string `json:"label"`
			ModelUID string `json:"modelUid"`
		} `json:"clientModelConfigs"`
		Sorts []struct {
			Name   string `json:"name"`
			Groups []struct {
				Name   string   `json:"groupName"`
				Labels []string `json:"modelLabels"`
			} `json:"groups"`
		} `json:"clientModelSorts"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Configs) != 1 || doc.Configs[0].ModelUID != "glm-5-2@-@128000" {
		t.Fatalf("configs=%+v", doc.Configs)
	}
	if len(doc.Sorts) != 3 || doc.Sorts[0].Name != groupSortName || doc.Sorts[1].Name != "Provider" || doc.Sorts[2].Name != "Cost" || doc.Sorts[0].Groups[0].Name != uncategorizedName || !strings.Contains(strings.Join(doc.Sorts[0].Groups[0].Labels, ","), "GLM 5.2") {
		t.Fatalf("sorts=%+v", doc.Sorts)
	}
}

func TestAugmentJSONDropsAdaptiveAndFusion(t *testing.T) {
	body := []byte(`{"clientModelConfigs":[{"label":"Adaptive","modelUid":"adaptive"},{"label":"Fusion","modelUid":"fusion"},{"label":"Official","modelUid":"official-model"}],"clientModelSorts":[{"name":"All","groups":[{"groupName":"Routers","modelLabels":["Adaptive","Fusion","Official"]}]}]}`)
	out, _, err := Augment(body, "application/json", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", testModels())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("official-model")) || bytes.Contains(out, []byte(`"adaptive"`)) || bytes.Contains(out, []byte(`"fusion"`)) || bytes.Contains(out, []byte("Adaptive")) || bytes.Contains(out, []byte("Fusion")) {
		t.Fatalf("official or routers kept: %s", out)
	}
	if !bytes.Contains(out, []byte("glm-5-2")) {
		t.Fatalf("catalog model missing: %s", out)
	}
}

func TestAugmentStatusBase64(t *testing.T) {
	status := appendBytes(nil, 33, nil)
	body, err := json.Marshal(map[string]any{
		"userStatusProtoBinaryBase64": base64.StdEncoding.EncodeToString(status),
		"keep":                        "yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	out, added, err := Augment(body, "application/json", "/exa.seat_management_pb.SeatManagementService/GetUserStatus", testModels())
	if err != nil || added != 1 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	var doc struct {
		Encoded string `json:"userStatusProtoBinaryBase64"`
		Keep    string `json:"keep"`
	}
	if json.Unmarshal(out, &doc) != nil || doc.Keep != "yes" {
		t.Fatalf("doc=%+v", doc)
	}
	raw, err := base64.StdEncoding.DecodeString(doc.Encoded)
	if err != nil || !bytes.Contains(raw, []byte("glm-5-2")) {
		t.Fatalf("decoded missing model: %v %q", err, raw)
	}
}

func TestSelectedRelayChoiceMapsOfficialThinkingName(t *testing.T) {
	config.SetSelectableModels([]config.Model{
		{ID: "claude-opus-4-6", ReasoningEfforts: []string{"low", "medium", "high"}},
		{ID: "claude-sonnet-4-6"},
	})
	t.Cleanup(func() { config.SetSelectableModels(nil) })

	choice, ok := SelectedRelayChoice("claude-opus-4-6-thinking")
	if !ok || choice.Model != "claude-opus-4-6" || choice.Effort != "" {
		t.Fatalf("slug: %+v %v", choice, ok)
	}
	choice, ok = SelectedRelayChoice("claude-opus-4.6-medium")
	if !ok || choice.Model != "claude-opus-4-6" || choice.Effort != "medium" {
		t.Fatalf("medium: %+v %v", choice, ok)
	}
	rememberOfficialModel("private-opus", "Claude Opus 4.6 Thinking")
	choice, ok = SelectedRelayChoice("private-opus")
	if !ok || choice.Model != "claude-opus-4-6" {
		t.Fatalf("alias: %+v %v", choice, ok)
	}
	if _, ok := SelectedRelayChoice("swe-1.6"); ok {
		t.Fatal("SWE-1.6 must stay on the hub model")
	}
}

func TestSonnetEffortLevelsReplaceOfficialThinkingRow(t *testing.T) {
	official := appendString(nil, 1, "Claude Sonnet 4.6 Thinking 1M")
	official = appendString(official, 22, "claude-sonnet-4-6-thinking-1m")
	other := appendString(nil, 1, "Keep Me")
	other = appendString(other, 22, "keep-me")
	list := appendBytes(nil, 1, official)
	list = appendBytes(list, 1, other)
	model := config.Model{
		ID:               "claude-sonnet-4-6",
		ReasoningEfforts: []string{"none", "low", "medium", "high"},
		ContextOptions:   []int{256000, 1_000_000},
		ContextWindow:    1_000_000,
	}
	out, added, err := Augment(list, "application/proto", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", []config.Model{model})
	if err != nil || added == 0 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	if bytes.Contains(out, []byte("claude-sonnet-4-6-thinking-1m")) {
		t.Fatal("official thinking row should be replaced by catalog effort levels")
	}
	if (!bytes.Contains(out, []byte("Reasoning Effort")) && !bytes.Contains(out, []byte("Effort"))) || !bytes.Contains(out, []byte("claude-sonnet-4-6@high@1000000")) {
		t.Fatal("catalog effort levels missing")
	}
	if bytes.Contains(out, []byte("Keep Me")) {
		t.Fatal("unrelated official model was kept")
	}
}

func TestAugmentUnlocksProPlanGate(t *testing.T) {
	reason := appendString(nil, 1, "Pro")
	reason = appendString(reason, 2, "Upgrade to Pro to access this model")
	locked := appendString(nil, 1, "Claude Opus 4.6 Thinking")
	locked = appendString(locked, 22, "claude-opus-4-6-thinking")
	locked = appendVarintField(locked, 4, 1)
	locked = appendBytes(locked, 33, reason)
	kept := appendString(nil, 1, "Broken Model")
	kept = appendString(kept, 22, "broken-model")
	kept = appendVarintField(kept, 4, 1)
	kept = appendBytes(kept, 33, appendString(nil, 2, "This model is currently experiencing some issues."))
	list := appendBytes(nil, 1, locked)
	list = appendBytes(list, 1, kept)
	out, added, err := Augment(list, "application/proto", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", testModels())
	if err != nil || added < 2 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	if bytes.Contains(out, []byte("Upgrade to Pro")) || bytes.Contains(out, []byte("experiencing some issues")) || bytes.Contains(out, []byte("Claude Opus 4.6 Thinking")) {
		t.Fatal("official model was kept")
	}
	if !bytes.Contains(out, []byte("glm-5-2")) {
		t.Fatal("catalog model was not added")
	}
}

func TestAugmentTeamSettingsKeepsEmptyAllowListOpen(t *testing.T) {
	path := "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings"
	body := appendString(nil, 1, "keep")
	out, added, err := Augment(body, "application/proto", path, testModels())
	if err != nil || added != 0 || out != nil {
		t.Fatalf("auth payload changed: added=%d out=%d err=%v", added, len(out), err)
	}
	restricted := appendString(nil, 7, "official-model")
	out, added, err = Augment(restricted, "application/proto", path, testModels())
	if err != nil || added != 0 || out != nil {
		t.Fatalf("restricted team settings must stay untouched: added=%d out=%d err=%v", added, len(out), err)
	}
}

func TestAugmentUserStatusKeepsLoginIdentity(t *testing.T) {
	status := appendString(nil, 7, "user@qq.com")
	status = appendString(status, 36, "user-123")
	status = append(status, appendBytes(nil, 13, appendString(nil, 1, "Pro"))...)
	status = append(status, appendBytes(nil, 33, nil)...)
	body := appendBytes(nil, 1, status)
	body = append(body, appendBytes(nil, 2, appendString(nil, 1, "keep-plan"))...)

	model := config.Model{
		ID: "glm-5-2", ReasoningEfforts: []string{"low", "high"},
		ContextOptions: []int{128000}, PriceIn: 1.5, PriceOut: 3,
	}
	out, added, err := Augment(body, "application/proto", "/exa.language_server_pb.LanguageServerService/GetUserStatus", []config.Model{model})
	if err != nil || added != 2 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	for _, want := range []string{"user@qq.com", "user-123", "Pro", "keep-plan", "glm-5-2@high@128000"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("missing %s", want)
		}
	}
	fields, err := parseFields(out)
	if err != nil {
		t.Fatal(err)
	}
	var plan []byte
	for _, field := range fields {
		if field.num == 2 {
			plan = field.val
		}
	}
	if string(plan) == "" || !bytes.Contains(plan, []byte("keep-plan")) {
		t.Fatal("plan_info was rewritten")
	}
	if !priceUsesFloat32(out) {
		t.Fatal("model price must be float32 so user status still parses")
	}
}

func priceUsesFloat32(body []byte) bool {
	fields, err := parseFields(body)
	if err != nil {
		return false
	}
	for _, field := range fields {
		if field.wt != 2 {
			continue
		}
		if priceUsesFloat32(field.val) {
			return true
		}
		// ModelDimension.value is field 2, float32 wire type 5, tag byte 0x15.
		if bytes.Contains(field.val, []byte("Input")) && bytes.Contains(field.raw, []byte{0x15}) && !bytes.Contains(field.raw, []byte{0x11}) {
			return true
		}
	}
	return false
}

func TestOfficialVariantsExposeEffortContextAndPrice(t *testing.T) {
	model := config.Model{
		ID: "claude-opus-4-7", ReasoningEfforts: []string{"low", "high"},
		ContextOptions: []int{200000, 1000000}, ContextWindow: 1000000,
		PriceIn: 15, PriceOut: 75, PriceCache: 1.5, Ratio: 1.3,
	}
	config.SetSelectableModels([]config.Model{model})
	t.Cleanup(func() { config.SetSelectableModels(nil) })

	prepared := prepare([]config.Model{model})
	if len(prepared) != 4 {
		t.Fatalf("variants: %d", len(prepared))
	}
	defaults := 0
	for _, item := range prepared {
		if item.Family != "Claude Opus 4.7" || item.BaseID != "claude-opus-4-7" || !strings.HasPrefix(item.Label, "Claude Opus 4.7 ") {
			t.Fatalf("label: %+v", item)
		}
		if item.Default {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults: %d", defaults)
	}

	list := appendBytes(nil, 1, officialConfig())
	path := "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs"
	out, added, err := Augment(list, "application/proto", path, []config.Model{model})
	if err != nil || added != 5 {
		t.Fatalf("augment added=%d err=%v", added, err)
	}
	for _, want := range []string{"Claude Opus 4.7", "Effort", "Low", "High", "Context", "200K", "1M", "Input", "Cached input", "claude-opus-4-7@high@1000000"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("missing %s", want)
		}
	}
	again, added, err := Augment(out, "application/proto", path, []config.Model{model})
	if err != nil || added != 0 || again != nil {
		t.Fatalf("second augment added=%d err=%v", added, err)
	}

	choice, ok := SelectedRelayChoice("claude-opus-4-7@high@1000000")
	if !ok || choice.Model != "claude-opus-4-7" || choice.Effort != "high" || choice.Context != 1000000 {
		t.Fatalf("choice: %+v %v", choice, ok)
	}
	if _, ok := SelectedRelayChoice("claude-opus-4-7@high"); ok {
		t.Fatal("truncated variant must be ignored")
	}
}

func TestSelectedRelayModelUsesCatalogOnly(t *testing.T) {
	if got, ok := SelectedRelayModel("glm-5-2"); !ok || got != "glm-5-2" {
		t.Fatalf("catalog model: %q %v", got, ok)
	}
	if _, ok := SelectedRelayModel("SWE-1.6"); ok {
		t.Fatal("SWE-1.6 must stay on the hub default")
	}
	if _, ok := SelectedRelayModel("not-in-catalog"); ok {
		t.Fatal("unknown model must not be selected")
	}
}

func TestCategoryGroupsKeepCatalogOrder(t *testing.T) {
	models := []config.Model{
		{ID: "claude-sonnet-5-5", Label: "Claude Sonnet 5.5", CategoryName: "CCMAX"},
		{ID: "gpt-6.1-terra", Label: "GPT-6.1 Terra", CategoryName: "免费"},
		{ID: "mimo-v2.6-pro", Label: "MiMo V2.6 Pro", CategoryName: "MIMO"},
		{ID: "plain-model", Label: "Plain"},
	}
	groups := categoryGroups(prepare(models))
	if len(groups) != 4 {
		t.Fatalf("groups=%+v", groups)
	}
	want := []string{"CCMAX", "免费", "MIMO", uncategorizedName}
	for i, name := range want {
		if groups[i].Name != name {
			t.Fatalf("group %d = %s, want %s", i, groups[i].Name, name)
		}
	}
	if groups[0].Labels[0] != "Claude Sonnet 5.5" || groups[3].Name != uncategorizedName || len(groups[3].Labels) != 1 {
		t.Fatalf("labels=%+v", groups)
	}
}

func TestRateBadge(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "Free"},
		{1.3, "x1.3"},
		{0.25, "x0.25"},
		{1, "x1"},
	}
	for _, item := range cases {
		if got := rateBadge(item.in); got != item.want {
			t.Fatalf("rate %v = %s, want %s", item.in, got, item.want)
		}
	}
	raw := encodeModel(preparedModel{ID: "mimo", BaseID: "mimo", Label: "MiMo", Context: 1000, Rate: "x0.3"})
	if !bytes.Contains(raw, []byte("x0.3")) {
		t.Fatal("proto model missing rate label")
	}
	if providerOf("gpt-6.1-terra") != 2 || providerOf("mimo-v2.6-pro") != 0 || providerOf("claude-sonnet") != 3 {
		t.Fatalf("providers gpt=%d mimo=%d claude=%d", providerOf("gpt-6.1-terra"), providerOf("mimo-v2.6-pro"), providerOf("claude-sonnet"))
	}
	if modelJSON(preparedModel{ID: "mimo", BaseID: "mimo", Label: "MiMo", Context: 1000})["provider"] != 0 {
		t.Fatal("unknown provider was labeled as a known vendor")
	}
	doc := modelJSON(preparedModel{ID: "claude", BaseID: "claude", Label: "Claude", Context: 1000, Rate: "x1.3"})
	promo, _ := doc["promoStatus"].(map[string]any)
	if promo["label"] != "x1.3" || promo["isActive"] != true {
		t.Fatalf("json promo: %+v", promo)
	}
}

func TestAgentFallbackModelStays(t *testing.T) {
	fallback := appendString(nil, 1, "SWE-1.6 Fast")
	fallback = appendString(fallback, 22, "swe-1-6-fast")
	list := appendBytes(appendBytes(nil, 1, officialConfig()), 1, fallback)
	out, _, err := Augment(list, "application/proto", "/exa.api_server_pb.ApiServerService/GetCascadeModelConfigs", testModels())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("official-model")) {
		t.Fatal("unrelated official model was kept")
	}
	if !bytes.Contains(out, []byte("swe-1-6-fast")) || !bytes.Contains(out, []byte("glm-5-2")) {
		t.Fatal("fallback or catalog model missing")
	}
}
