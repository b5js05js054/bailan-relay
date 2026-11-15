// Package devincatalog 把后台下发的模型写进 Devin 的模型目录响应，
// 让用户在 Devin 自己的模型列表里切换。对话仍由现有中继转发。
package devincatalog

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"bailan-relay/internal/config"
	"bailan-relay/internal/connectframe"
)

const (
	// 未分类 是后台没填分类名时，写进 Devin 列表的组名。
	uncategorizedName = "未分类"
	// 分组 是按后台分类看列表。它要出现在 Order by 里，并且是打开列表时的默认排序。
	groupSortName = "分组"
	// 注入模型的推理地址仍指向被本机拦截的官方域名，GetChatMessage 会回到现有转发。
	inferenceURL     = "https://server.codeium.com"
	defaultContext   = 272000
	defaultMaxOutput = 131072
	// DisplayOption.MODERN_AGENT_CHAT。Devin 的模型选择器只把这一档和官方模型放进同一张列表。
	displayModernAgentChat = 8
)

// IsCatalog 判断这个路径是不是 Devin 用来取模型目录的接口。
func IsCatalog(path string) bool {
	switch lastSegment(path) {
	// GetCliTeamSettings / GetUserJwt 是登录和 ACP 认证，必须原样回传。
	// 改写或整包缓冲会让 Devin 在认证时限内读不到完整会话，表现为登录态失效。
	case "GetUserStatus", "GetCascadeModelConfigs", "GetCliModelConfigs", "GetCommandModelConfigs":
		return true
	default:
		return false
	}
}

// RelayChoice 是 Devin 选择器里点中的一档：基础模型，以及这一档的思考强度和上下文。
type RelayChoice struct {
	Model   string
	Effort  string
	Context int
}

// SelectedRelayModel 在请求里的模型属于当前目录、且不是 SWE-1.6 时，返回应转发给中继的模型 ID。
func SelectedRelayModel(requested string) (string, bool) {
	choice, ok := SelectedRelayChoice(requested)
	if !ok {
		return "", false
	}
	return choice.Model, true
}

// SelectedRelayChoice 解析 Devin 发来的模型。带 @ 的是同一模型的思考/上下文档位。
// 官方列表里的名字常带 Thinking / Medium，和目录 ID 不完全一样，也能对上目录模型。
func SelectedRelayChoice(requested string) (RelayChoice, bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" || connectframe.IsSWE16(requested) {
		return RelayChoice{}, false
	}
	base, effort, context := requested, "", 0
	if strings.Contains(requested, "@") {
		parts := strings.Split(requested, "@")
		if len(parts) != 3 || parts[0] == "" {
			return RelayChoice{}, false
		}
		base = parts[0]
		if parts[1] != "-" {
			effort = parts[1]
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil || n < 0 {
			return RelayChoice{}, false
		}
		context = n
	}
	if model, ok := config.ModelByID(base); ok && !connectframe.IsSWE16(model.ID) {
		return RelayChoice{Model: model.ID, Effort: effort, Context: context}, true
	}
	if id, ok := lookupAlias(base); ok {
		return RelayChoice{Model: id, Effort: effort, Context: context}, true
	}
	if id, foldedEffort, ok := matchFolded(base); ok {
		if effort == "" {
			effort = foldedEffort
		}
		return RelayChoice{Model: id, Effort: effort, Context: context}, true
	}
	return RelayChoice{}, false
}

var (
	aliasMu sync.Mutex
	aliases = map[string]string{}
)

func rememberOfficialModel(uid, label string) {
	uid = strings.TrimSpace(uid)
	if uid == "" || connectframe.IsSWE16(uid) {
		return
	}
	if _, ok := config.ModelByID(uid); ok {
		return
	}
	id, _, ok := matchFolded(label)
	if !ok {
		id, _, ok = matchFolded(uid)
	}
	if !ok {
		return
	}
	aliasMu.Lock()
	aliases[strings.ToLower(uid)] = id
	aliasMu.Unlock()
}

func lookupAlias(uid string) (string, bool) {
	aliasMu.Lock()
	id, ok := aliases[strings.ToLower(strings.TrimSpace(uid))]
	aliasMu.Unlock()
	if !ok || connectframe.IsSWE16(id) {
		return "", false
	}
	if _, known := config.ModelByID(id); !known {
		return "", false
	}
	return id, true
}

func matchFolded(raw string) (id, effort string, ok bool) {
	base, effort := foldKey(raw)
	if base == "" {
		return "", "", false
	}
	if effort == "thinking" {
		effort = ""
	}
	hit := ""
	for _, model := range config.Models() {
		if connectframe.IsSWE16(model.ID) {
			continue
		}
		folded, _ := foldKey(model.ID)
		if folded != base {
			continue
		}
		if hit != "" && hit != model.ID {
			return "", "", false
		}
		hit = model.ID
	}
	if hit == "" {
		return "", "", false
	}
	return hit, effort, true
}

func foldKey(raw string) (string, string) {
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	key := b.String()
	for _, suffix := range []string{"minimal", "thinking", "xhigh", "medium", "high", "low", "max", "none"} {
		if strings.HasSuffix(key, suffix) && len(key) > len(suffix)+2 {
			return key[:len(key)-len(suffix)], suffix
		}
	}
	return key, ""
}

// Augment 在模型目录响应里追加当前目录的模型。
// 输入：响应体、Content-Type、请求路径、要露出的模型；输出：改写后的响应体与新增数量。
// 不需要改写时 out 为 nil。解析失败时返回错误，调用方应原样回传官方响应。
func Augment(body []byte, contentType, path string, models []config.Model) ([]byte, int, error) {
	switch lastSegment(path) {
	case "GetCliTeamSettings", "GetUserJwt":
		return nil, 0, nil
	}
	prepared := prepare(models)
	if len(prepared) == 0 || !IsCatalog(path) || len(body) == 0 {
		return nil, 0, nil
	}
	ct := mediaType(contentType)
	if ct == "" {
		if body[0] == '{' {
			ct = "application/json"
		} else {
			ct = "application/proto"
		}
	}
	kind := catalogKind(path)
	if isFramed(ct) {
		return augmentFramed(body, ct, kind, prepared)
	}
	if isJSON(ct) {
		return augmentJSON(body, kind, prepared)
	}
	out, added, err := augmentProto(body, kind, prepared)
	// 登录时有的 GetUserStatus 内容类型写的是 proto，正文却是 Connect 帧。
	// 按普通 protobuf 解析会加不进模型，Devin 选择器里就只剩官方列表。
	if (err != nil || added == 0) && completeFrames(body) {
		if framed, n, ferr := augmentFramed(body, "application/proto", kind, prepared); ferr == nil && n > 0 {
			return framed, n, nil
		}
	}
	return out, added, err
}

func completeFrames(body []byte) bool {
	frames, err := splitFrames(body)
	return err == nil && len(frames) > 0
}

type preparedModel struct {
	ID        string
	BaseID    string
	Label     string
	Context   int
	MaxOutput int
	Thinking  bool
	Family    string
	Entries   []familyEntry
	Costs     []costLine
	Default   bool
	Rate      string
	Category  string
}

func prepare(models []config.Model) []preparedModel {
	return expandModels(models)
}

func catalogKind(path string) string {
	switch lastSegment(path) {
	case "GetUserStatus":
		return "status"
	case "GetCascadeModelConfigs":
		return "cascade"
	default:
		return "list"
	}
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func mediaType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

func isJSON(ct string) bool { return strings.Contains(ct, "json") }

func isFramed(ct string) bool {
	return strings.HasPrefix(ct, "application/connect+") || strings.HasPrefix(ct, "application/grpc")
}

func augmentProto(body []byte, kind string, models []preparedModel) ([]byte, int, error) {
	if kind != "status" {
		return augmentProtoList(body, models, kind == "cascade")
	}
	fields, err := parseFields(body)
	if err != nil {
		return nil, 0, err
	}
	var status []byte
	matches := 0
	for _, field := range fields {
		if field.num != 1 {
			continue
		}
		matches++
		if field.wt != 2 {
			return nil, 0, errors.New("ambiguous protobuf field")
		}
		status = field.val
	}
	if matches != 1 {
		return nil, 0, nil
	}
	next, added, err := augmentUserStatus(status, models)
	if err != nil || added == 0 {
		return nil, 0, err
	}
	var out []byte
	for _, field := range fields {
		if field.num == 1 {
			out = appendBytes(out, 1, next)
			continue
		}
		out = append(out, field.raw...)
	}
	return out, added, nil
}

func augmentUserStatus(status []byte, models []preparedModel) ([]byte, int, error) {
	var added int
	next, _, err := rewriteField(status, 33, func(list []byte) ([]byte, error) {
		out, n, err := augmentProtoList(list, models, true)
		added = n
		if n == 0 {
			return list, err
		}
		return out, err
	}, true)
	if err != nil {
		return nil, 0, err
	}
	if added == 0 {
		return status, 0, nil
	}
	return next, added, nil
}

func augmentProtoList(data []byte, models []preparedModel, hasSorts bool) ([]byte, int, error) {
	if len(models) == 0 {
		return nil, 0, nil
	}
	fields, err := parseFields(data)
	if err != nil {
		return nil, 0, err
	}
	ours := map[string]bool{}
	for _, model := range models {
		ours[strings.ToLower(model.ID)] = true
	}
	present := map[string]bool{}
	for _, field := range fields {
		if field.num == 1 && field.wt == 2 {
			if uid, ok := fieldString(field.val, 22); ok && ours[strings.ToLower(uid)] {
				present[strings.ToLower(uid)] = true
			}
		}
	}
	var adding []preparedModel
	for _, model := range models {
		if present[strings.ToLower(model.ID)] {
			continue
		}
		adding = append(adding, model)
	}
	keepSorts := hasSorts && protoSortsMatch(fields, models)
	out := make([]byte, 0, len(data)+len(adding)*256)
	for _, model := range adding {
		out = appendBytes(out, 1, encodeModel(model))
	}
	removed := 0
	for _, field := range fields {
		if field.num == 1 && field.wt == 2 {
			uid, ok := fieldString(field.val, 22)
			if ok && (ours[strings.ToLower(uid)] || agentFallback(uid)) {
				out = append(out, field.raw...)
				continue
			}
			removed++
			continue
		}
		if hasSorts && field.num == 2 && field.wt == 2 {
			if keepSorts {
				out = append(out, field.raw...)
				continue
			}
			removed++
			continue
		}
		out = append(out, field.raw...)
	}
	if hasSorts && !keepSorts && len(models) > 0 {
		out = appendSorts(out, models)
	}
	if len(adding) == 0 && removed == 0 {
		return nil, 0, nil
	}
	return out, len(adding) + removed, nil
}

// sortMatchesCategories 判断这份排序是不是按后台分类分成的组。组名或顺序变了就要重写。
// agentFallback 是 Agent 启动会话时认得的官方模型。
// 选择器里不展示它们，但目录里必须留着，否则 Agent 会报没有可用模型，选模型的入口也不出现。
func agentFallback(uid string) bool {
	uid = strings.ToLower(strings.TrimSpace(uid))
	switch uid {
	case "swe-1-6", "swe-1-6-fast", "swe-1-6-slow", "swe-1-6-medium", "claude-opus-4-7-medium", "gpt-5.4":
		return true
	default:
		return strings.HasPrefix(uid, "swe-1-6-")
	}
}

func protoSortsMatch(fields []protoField, models []preparedModel) bool {
	var got []namedSort
	for _, field := range fields {
		if field.num != 2 || field.wt != 2 {
			continue
		}
		sort, ok := parseSort(field.val)
		if !ok {
			return false
		}
		got = append(got, sort)
	}
	return sortsEqual(got, buildSorts(models))
}

func parseSort(sort []byte) (namedSort, bool) {
	fields, err := parseFields(sort)
	if err != nil {
		return namedSort{}, false
	}
	var name string
	var groups []namedGroup
	for _, field := range fields {
		if field.num == 1 && field.wt == 2 {
			name = string(field.val)
		}
		if field.num != 2 || field.wt != 2 {
			continue
		}
		parts, err := parseFields(field.val)
		if err != nil {
			return namedSort{}, false
		}
		var groupName string
		var labels []string
		for _, part := range parts {
			if part.num == 1 && part.wt == 2 {
				groupName = string(part.val)
			}
			if part.num == 2 && part.wt == 2 {
				labels = append(labels, string(part.val))
			}
		}
		if groupName == "" || len(labels) == 0 {
			return namedSort{}, false
		}
		groups = append(groups, namedGroup{Name: groupName, Labels: labels})
	}
	if name == "" || len(groups) == 0 {
		return namedSort{}, false
	}
	return namedSort{Name: name, Groups: groups}, true
}

func sortsEqual(got, want []namedSort) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Name != want[i].Name || len(got[i].Groups) != len(want[i].Groups) {
			return false
		}
		for j := range want[i].Groups {
			if got[i].Groups[j].Name != want[i].Groups[j].Name || strings.Join(got[i].Groups[j].Labels, "\n") != strings.Join(want[i].Groups[j].Labels, "\n") {
				return false
			}
		}
	}
	return len(want) > 0
}

// sortOnlyHasLabels 判断这个分组是不是只含后台模型。官方分组会被丢掉。
func sortOnlyHasLabels(sort []byte, allowed map[string]bool) bool {
	fields, err := parseFields(sort)
	if err != nil {
		return false
	}
	found := false
	for _, field := range fields {
		if field.num != 2 || field.wt != 2 {
			continue
		}
		groups, err := parseFields(field.val)
		if err != nil {
			return false
		}
		for _, group := range groups {
			if group.num != 2 || group.wt != 2 {
				continue
			}
			if !allowed[string(group.val)] {
				return false
			}
			found = true
		}
	}
	return found
}

// replacedByCatalog 后台已经给这个模型配了思考档或上下文时，去掉 Devin 自带的同名行。
// 自带行只有 Thinking 开关，不会显示后台的等级。
func replacedByCatalog(uid string, models []preparedModel) bool {
	uid = strings.ToLower(strings.TrimSpace(uid))
	if uid == "" {
		return false
	}
	counts := map[string]int{}
	for _, model := range models {
		counts[strings.ToLower(model.BaseID)]++
	}
	for base, n := range counts {
		if n < 2 || base == "" {
			continue
		}
		if uid == base {
			return true
		}
		if !strings.HasPrefix(uid, base+"-") {
			continue
		}
		switch strings.TrimPrefix(uid, base+"-") {
		case "thinking", "1m", "thinking-1m", "none", "minimal", "low", "medium", "high", "xhigh", "max":
			return true
		}
	}
	return false
}

// hideRouter 判断这是不是不在列表里展示的 Adaptive / Fusion。
// 这两个是官方自动挑选开关，不是普通模型，也不能改成后台目录里的模型。
func hideRouter(uid, label string) bool {
	return routerName(uid) || routerName(label)
}

func routerName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case name == "adaptive" || name == "fusion":
		return true
	case strings.HasPrefix(name, "adaptive-") || strings.HasPrefix(name, "adaptive "):
		return true
	case strings.HasPrefix(name, "fusion-") || strings.HasPrefix(name, "fusion "):
		return true
	default:
		return false
	}
}

func stripRouterSort(sort []byte) ([]byte, int, error) {
	fields, err := parseFields(sort)
	if err != nil {
		return nil, 0, err
	}
	removed := 0
	var out []byte
	for _, field := range fields {
		if field.num == 2 && field.wt == 2 {
			next, n, keep, err := stripRouterGroup(field.val)
			if err != nil {
				return nil, 0, err
			}
			removed += n
			if !keep {
				continue
			}
			out = appendBytes(out, 2, next)
			continue
		}
		out = append(out, field.raw...)
	}
	return out, removed, nil
}

func stripRouterGroup(group []byte) ([]byte, int, bool, error) {
	fields, err := parseFields(group)
	if err != nil {
		return nil, 0, false, err
	}
	removed := 0
	labels := 0
	var out []byte
	for _, field := range fields {
		if field.num == 2 && field.wt == 2 && routerName(string(field.val)) {
			removed++
			continue
		}
		if field.num == 2 && field.wt == 2 {
			labels++
		}
		out = append(out, field.raw...)
	}
	return out, removed, labels > 0, nil
}

// unlockPlanGate 去掉「升级到 Pro 才能用」这类套餐锁。
// 模型因故障、下线等原因被禁用时保持原样。
func unlockPlanGate(cfg []byte) ([]byte, bool) {
	fields, err := parseFields(cfg)
	if err != nil {
		return nil, false
	}
	disabled := false
	var reason []byte
	for _, field := range fields {
		if field.num == 4 && field.wt == 0 && varintValue(field.raw) == 1 {
			disabled = true
		}
		if field.num == 33 && field.wt == 2 {
			reason = field.val
		}
	}
	if !disabled || !planGateReason(reason) {
		return nil, false
	}
	var out []byte
	for _, field := range fields {
		switch field.num {
		case 4, 7, 12, 33:
			continue
		}
		out = append(out, field.raw...)
	}
	return out, true
}

func planGateReason(reason []byte) bool {
	fields, err := parseFields(reason)
	if err != nil {
		return false
	}
	for _, field := range fields {
		if field.wt != 2 || (field.num != 1 && field.num != 2) {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(string(field.val)))
		if text == "pro" || strings.Contains(text, "upgrade") {
			return true
		}
	}
	return false
}

func varintValue(raw []byte) uint64 {
	_, n, err := readVarint(raw, 0)
	if err != nil {
		return 0
	}
	v, _, err := readVarint(raw, n)
	if err != nil {
		return 0
	}
	return v
}

func encodeModel(model preparedModel) []byte {
	var features []byte
	features = appendVarintField(features, 8, 1)
	features = appendVarintField(features, 12, 1)
	if model.Thinking {
		features = appendVarintField(features, 15, 1)
	}
	features = appendVarintField(features, 21, 1)

	var info []byte
	info = appendVarintField(info, 3, 2)
	info = appendVarintField(info, 4, uint64(model.Context))
	info = appendBytes(info, 6, features)
	info = appendVarintField(info, 13, uint64(model.MaxOutput))
	info = appendString(info, 17, model.ID)
	info = appendString(info, 18, inferenceURL)
	info = appendString(info, 20, "swe-1p6")
	info = appendString(info, 20, "swe-1p5")
	info = appendVarintField(info, 22, displayModernAgentChat)
	info = appendString(info, 23, "devinhub-family-"+model.BaseID)

	var cfg []byte
	cfg = appendString(cfg, 1, model.Label)
	cfg = appendVarintField(cfg, 10, uint64(providerOf(model.BaseID)))
	cfg = appendVarintField(cfg, 18, uint64(model.Context))
	cfg = appendString(cfg, 22, model.ID)
	cfg = appendBytes(cfg, 23, info)
	cfg = appendBytes(cfg, 30, encodeFamily(model))
	if model.Default {
		cfg = appendVarintField(cfg, 31, 1)
	}
	for _, cost := range model.Costs {
		cfg = appendBytes(cfg, 32, encodeCost(cost))
	}
	if model.Rate != "" {
		cfg = appendBytes(cfg, 19, encodePromo(model.Rate))
	}
	return cfg
}

func encodePromo(label string) []byte {
	var msg []byte
	msg = appendVarintField(msg, 1, 1)
	msg = appendString(msg, 3, label)
	return msg
}

func encodeFamily(model preparedModel) []byte {
	var family []byte
	family = appendString(family, 1, model.Family)
	for _, entry := range model.Entries {
		var value []byte
		value = appendVarintField(value, 1, uint64(entry.Order))
		value = appendString(value, 2, entry.Name)
		value = appendVarintField(value, 3, uint64(entry.Control))
		var item []byte
		item = appendString(item, 1, entry.Key)
		item = appendBytes(item, 2, value)
		family = appendBytes(family, 2, item)
	}
	return family
}

func encodeCost(cost costLine) []byte {
	var msg []byte
	msg = appendString(msg, 1, cost.Label)
	// ModelDimension 的 value/min/max 是 float32（wire type 5）。
	// 写成 double 时，嵌在 GetUserStatus 里的模型列表会让整段登录状态解析失败。
	msg = appendFloatField(msg, 2, cost.Value)
	msg = appendString(msg, 3, "1M")
	msg = appendFloatField(msg, 4, cost.Min)
	msg = appendFloatField(msg, 5, cost.Max)
	msg = appendVarintField(msg, 6, 1)
	if cost.Info != "" {
		msg = appendString(msg, 7, cost.Info)
	}
	return msg
}

func appendFloatField(dst []byte, field int, v float64) []byte {
	dst = appendKey(dst, field, 5)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(float32(v)))
	return append(dst, buf[:]...)
}

func groupMessage(name string, labels []string) []byte {
	var group []byte
	group = appendString(group, 1, name)
	for _, label := range labels {
		group = appendString(group, 2, label)
	}
	return group
}

func insertGroup(sort []byte, name string, labels []string) ([]byte, error) {
	fields, err := parseFields(sort)
	if err != nil {
		return nil, err
	}
	group := groupMessage(name, labels)
	var out []byte
	placed := false
	for _, field := range fields {
		if !placed && field.num == 2 && field.wt == 2 {
			out = appendBytes(out, 2, group)
			placed = true
		}
		out = append(out, field.raw...)
	}
	if !placed {
		out = appendBytes(out, 2, group)
	}
	return out, nil
}

func newSort(models []preparedModel) []byte {
	return appendSorts(nil, models)
}

func appendSorts(dst []byte, models []preparedModel) []byte {
	for _, sort := range buildSorts(models) {
		var msg []byte
		msg = appendString(msg, 1, sort.Name)
		for _, group := range sort.Groups {
			msg = appendBytes(msg, 2, groupMessage(group.Name, group.Labels))
		}
		dst = appendBytes(dst, 2, msg)
	}
	return dst
}

type namedSort struct {
	Name   string
	Groups []namedGroup
}

func buildSorts(models []preparedModel) []namedSort {
	return []namedSort{
		{Name: groupSortName, Groups: categoryGroups(models)},
		{Name: "Provider", Groups: providerGroups(models)},
		{Name: "Cost", Groups: costGroups(models)},
	}
}

type namedGroup struct {
	Name   string
	Labels []string
}

func modelCategory(model preparedModel) string {
	name := strings.TrimSpace(model.Category)
	if name == "" {
		return uncategorizedName
	}
	return name
}

// categoryGroups 按后台分类名分组，组内保持目录顺序。没填分类的归到「未分类」，排在最后。
func categoryGroups(models []preparedModel) []namedGroup {
	var order []string
	labels := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, model := range models {
		name := modelCategory(model)
		if seen[name] == nil {
			seen[name] = map[string]bool{}
			if name != uncategorizedName {
				order = append(order, name)
			}
		}
		if model.Label == "" || seen[name][model.Label] {
			continue
		}
		seen[name][model.Label] = true
		labels[name] = append(labels[name], model.Label)
	}
	var out []namedGroup
	for _, name := range order {
		out = append(out, namedGroup{Name: name, Labels: labels[name]})
	}
	if len(labels[uncategorizedName]) > 0 {
		out = append(out, namedGroup{Name: uncategorizedName, Labels: labels[uncategorizedName]})
	}
	return out
}

var vendorSeries = []struct {
	Title    string
	Prefixes []string
}{
	{"Claude", []string{"claude", "anthropic"}},
	{"Gemini", []string{"gemini", "google"}},
	{"OpenAI", []string{"openai", "gpt", "chatgpt", "o1", "o3"}},
	{"Kimi", []string{"kimi", "moonshot"}},
	{"MiMo", []string{"mimo", "xiaomi"}},
	{"Zhipu", []string{"zhipu", "glm", "chatglm"}},
	{"DeepSeek", []string{"deepseek"}},
	{"Grok", []string{"grok", "xai"}},
}

func vendorOf(model preparedModel) string {
	id := strings.ToLower(model.BaseID)
	label := strings.ToLower(model.Label)
	for _, series := range vendorSeries {
		for _, prefix := range series.Prefixes {
			if strings.HasPrefix(id, prefix) || strings.HasPrefix(label, prefix) {
				return series.Title
			}
		}
	}
	return "其他"
}

func providerGroups(models []preparedModel) []namedGroup {
	return groupedBy(models, vendorOf, func(name string) bool { return name == "其他" })
}

func costGroups(models []preparedModel) []namedGroup {
	groups := groupedBy(models, func(model preparedModel) string {
		if model.Rate == "" {
			return "Free"
		}
		return model.Rate
	}, func(string) bool { return false })
	sortGroupsByRate(groups)
	return groups
}

func groupedBy(models []preparedModel, nameOf func(preparedModel) string, last func(string) bool) []namedGroup {
	var order []string
	var tail []string
	labels := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, model := range models {
		name := nameOf(model)
		if seen[name] == nil {
			seen[name] = map[string]bool{}
			if last(name) {
				tail = append(tail, name)
			} else {
				order = append(order, name)
			}
		}
		if model.Label == "" || seen[name][model.Label] {
			continue
		}
		seen[name][model.Label] = true
		labels[name] = append(labels[name], model.Label)
	}
	var out []namedGroup
	for _, name := range append(order, tail...) {
		out = append(out, namedGroup{Name: name, Labels: labels[name]})
	}
	return out
}

func sortGroupsByRate(groups []namedGroup) {
	for i := 1; i < len(groups); i++ {
		item := groups[i]
		j := i
		for j > 0 && rateValue(groups[j-1].Name) > rateValue(item.Name) {
			groups[j] = groups[j-1]
			j--
		}
		groups[j] = item
	}
}

func rateValue(rate string) float64 {
	if rate == "Free" || rate == "" {
		return 0
	}
	value, err := strconv.ParseFloat(strings.TrimPrefix(rate, "x"), 64)
	if err != nil {
		return 0
	}
	return value
}

type protoField struct {
	num int
	wt  int
	raw []byte
	val []byte
}

func parseFields(data []byte) ([]protoField, error) {
	var fields []protoField
	i := 0
	for i < len(data) {
		start := i
		tag, next, err := readVarint(data, i)
		if err != nil {
			return nil, err
		}
		i = next
		num := int(tag >> 3)
		wt := int(tag & 7)
		if num <= 0 {
			return nil, errors.New("invalid protobuf field")
		}
		switch wt {
		case 0:
			_, next, err = readVarint(data, i)
		case 1:
			next, err = skip(data, i, 8)
		case 5:
			next, err = skip(data, i, 4)
		case 2:
			var length uint64
			length, next, err = readVarint(data, i)
			if err != nil {
				return nil, err
			}
			end, err := skip(data, next, int(length))
			if err != nil {
				return nil, err
			}
			fields = append(fields, protoField{num: num, wt: wt, raw: bytes.Clone(data[start:end]), val: bytes.Clone(data[next:end])})
			i = end
			continue
		default:
			return nil, errors.New("unsupported protobuf wire type")
		}
		if err != nil {
			return nil, err
		}
		fields = append(fields, protoField{num: num, wt: wt, raw: bytes.Clone(data[start:next])})
		i = next
	}
	return fields, nil
}

func skip(data []byte, i, n int) (int, error) {
	if n < 0 || i+n > len(data) {
		return 0, errors.New("truncated protobuf")
	}
	return i + n, nil
}

func readVarint(data []byte, offset int) (uint64, int, error) {
	var result uint64
	var shift uint
	for offset < len(data) {
		b := data[offset]
		result |= uint64(b&0x7F) << shift
		offset++
		if b&0x80 == 0 {
			return result, offset, nil
		}
		shift += 7
		if shift > 63 {
			break
		}
	}
	return 0, offset, errors.New("truncated varint")
}

func appendVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

func appendKey(dst []byte, field, wire int) []byte {
	return appendVarint(dst, uint64(field<<3|wire))
}

func appendVarintField(dst []byte, field int, v uint64) []byte {
	return appendVarint(appendKey(dst, field, 0), v)
}

func appendBytes(dst []byte, field int, b []byte) []byte {
	dst = appendVarint(appendKey(dst, field, 2), uint64(len(b)))
	return append(dst, b...)
}

func appendString(dst []byte, field int, s string) []byte {
	return appendBytes(dst, field, []byte(s))
}

func fieldString(msg []byte, number int) (string, bool) {
	fields, err := parseFields(msg)
	if err != nil {
		return "", false
	}
	for _, field := range fields {
		if field.num == number && field.wt == 2 {
			return string(field.val), true
		}
	}
	return "", false
}

func rewriteField(data []byte, number int, transform func([]byte) ([]byte, error), create bool) ([]byte, int, error) {
	fields, err := parseFields(data)
	if err != nil {
		return nil, 0, err
	}
	matches := 0
	for _, field := range fields {
		if field.num != number {
			continue
		}
		matches++
		if field.wt != 2 {
			return nil, 0, errors.New("ambiguous protobuf field")
		}
	}
	if matches > 1 {
		return nil, 0, errors.New("ambiguous protobuf field")
	}
	if matches == 0 {
		if !create {
			return data, 0, nil
		}
		next, err := transform(nil)
		if err != nil {
			return nil, 0, err
		}
		return append(bytes.Clone(data), appendBytes(nil, number, next)...), 1, nil
	}
	var out []byte
	var changed bool
	for _, field := range fields {
		if field.num != number {
			out = append(out, field.raw...)
			continue
		}
		next, err := transform(field.val)
		if err != nil {
			return nil, 0, err
		}
		if bytes.Equal(next, field.val) {
			out = append(out, field.raw...)
			continue
		}
		changed = true
		out = appendBytes(out, number, next)
	}
	if !changed {
		return data, 0, nil
	}
	return out, 1, nil
}

func augmentFramed(body []byte, contentType, kind string, models []preparedModel) ([]byte, int, error) {
	frames, err := splitFrames(body)
	if err != nil {
		return nil, 0, err
	}
	var payload []byte
	var flags byte
	found := false
	rest := make([]frame, 0, len(frames))
	for _, item := range frames {
		if !found && item.flags&0x02 == 0 {
			payload = item.data
			flags = item.flags
			if item.flags&0x01 != 0 {
				payload, err = gunzip(item.data)
				if err != nil {
					return nil, 0, err
				}
				flags &^= 0x01
			}
			found = true
			continue
		}
		rest = append(rest, item)
	}
	if !found {
		return nil, 0, errors.New("missing connect message")
	}
	var next []byte
	var added int
	if isJSON(contentType) {
		next, added, err = augmentJSON(payload, kind, models)
	} else {
		next, added, err = augmentProto(payload, kind, models)
	}
	if err != nil || added == 0 {
		return nil, added, err
	}
	out := append(encodeFrame(flags, next), joinFrames(rest)...)
	return out, added, nil
}

type frame struct {
	flags byte
	data  []byte
}

func splitFrames(body []byte) ([]frame, error) {
	var frames []frame
	i := 0
	for i < len(body) {
		if i+5 > len(body) {
			return nil, errors.New("truncated connect frame")
		}
		flags := body[i]
		if flags&^0x03 != 0 {
			return nil, errors.New("invalid connect frame")
		}
		n := int(binary.BigEndian.Uint32(body[i+1 : i+5]))
		i += 5
		if n < 0 || i+n > len(body) {
			return nil, errors.New("truncated connect frame")
		}
		frames = append(frames, frame{flags: flags, data: bytes.Clone(body[i : i+n])})
		i += n
	}
	return frames, nil
}

func encodeFrame(flags byte, payload []byte) []byte {
	head := make([]byte, 5)
	head[0] = flags
	binary.BigEndian.PutUint32(head[1:], uint32(len(payload)))
	return append(head, payload...)
}

func joinFrames(frames []frame) []byte {
	var out []byte
	for _, item := range frames {
		out = append(out, encodeFrame(item.flags, item.data)...)
	}
	return out
}

func gunzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func augmentJSON(body []byte, kind string, models []preparedModel) ([]byte, int, error) {
	if kind == "status" {
		return augmentStatusJSON(body, models)
	}
	return augmentJSONList(body, models, kind == "cascade")
}

func augmentStatusJSON(body []byte, models []preparedModel) ([]byte, int, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, 0, err
	}
	if doc == nil {
		return nil, 0, errors.New("invalid catalog json")
	}
	added := rewriteBase64(doc, models)
	statusKey := jsonKey(doc, "userStatus", "user_status")
	if raw, ok := doc[statusKey]; ok {
		var status map[string]json.RawMessage
		if err := json.Unmarshal(raw, &status); err != nil {
			return nil, 0, err
		}
		added += rewriteBase64(status, models)
		configKey := jsonKey(status, "cascadeModelConfigData", "cascade_model_config_data")
		next, n, err := augmentJSONList(status[configKey], models, true)
		if err != nil {
			return nil, 0, err
		}
		if n > 0 {
			status[configKey] = next
			encoded, err := json.Marshal(status)
			if err != nil {
				return nil, 0, err
			}
			doc[statusKey] = encoded
			added += n
		}
	}
	if added == 0 {
		return nil, 0, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, 0, err
	}
	return out, added, nil
}

func augmentJSONList(raw json.RawMessage, models []preparedModel, hasSorts bool) (json.RawMessage, int, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = []byte("{}")
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, 0, err
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	modelsKey := jsonKey(doc, "clientModelConfigs", "client_model_configs")
	var existing []json.RawMessage
	if value, ok := doc[modelsKey]; ok && len(bytes.TrimSpace(value)) > 0 && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		if err := json.Unmarshal(value, &existing); err != nil {
			return nil, 0, err
		}
	}
	if len(models) == 0 {
		return raw, 0, nil
	}
	ours := map[string]bool{}
	encoded := make([]json.RawMessage, 0, len(models))
	for _, model := range models {
		ours[strings.ToLower(model.ID)] = true
		item, err := json.Marshal(modelJSON(model))
		if err != nil {
			return nil, 0, err
		}
		encoded = append(encoded, item)
	}
	present := map[string]bool{}
	extra := false
	for _, item := range existing {
		var probe struct {
			ModelUID  string `json:"modelUid"`
			ModelUID2 string `json:"model_uid"`
		}
		if json.Unmarshal(item, &probe) != nil {
			extra = true
			continue
		}
		uid := probe.ModelUID
		if uid == "" {
			uid = probe.ModelUID2
		}
		if agentFallback(uid) {
			encoded = append(encoded, item)
			continue
		}
		if uid == "" || !ours[strings.ToLower(uid)] {
			extra = true
			continue
		}
		present[strings.ToLower(uid)] = true
	}
	missing := false
	for _, model := range models {
		if !present[strings.ToLower(model.ID)] {
			missing = true
			break
		}
	}
	if !extra && !missing && jsonSortsMatch(doc, models) {
		return raw, 0, nil
	}
	merged, err := json.Marshal(encoded)
	if err != nil {
		return nil, 0, err
	}
	doc[modelsKey] = merged
	if hasSorts {
		sorts := make([]any, 0, 3)
		for _, sort := range buildSorts(models) {
			groups := make([]any, 0, len(sort.Groups))
			for _, group := range sort.Groups {
				groups = append(groups, map[string]any{
					"groupName":   group.Name,
					"modelLabels": group.Labels,
				})
			}
			sorts = append(sorts, map[string]any{"name": sort.Name, "groups": groups})
		}
		encodedSorts, err := json.Marshal(sorts)
		if err != nil {
			return nil, 0, err
		}
		doc[jsonKey(doc, "clientModelSorts", "client_model_sorts")] = encodedSorts
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, 0, err
	}
	return out, len(models) + len(existing), nil
}

func jsonSortsMatch(doc map[string]json.RawMessage, models []preparedModel) bool {
	key := jsonKey(doc, "clientModelSorts", "client_model_sorts")
	raw, ok := doc[key]
	if !ok || len(bytes.TrimSpace(raw)) == 0 {
		return false
	}
	var sorts []struct {
		Name   string `json:"name"`
		Groups []struct {
			Name   string   `json:"groupName"`
			Labels []string `json:"modelLabels"`
		} `json:"groups"`
	}
	if json.Unmarshal(raw, &sorts) != nil {
		return false
	}
	got := make([]namedSort, 0, len(sorts))
	for _, sort := range sorts {
		item := namedSort{Name: sort.Name}
		for _, group := range sort.Groups {
			item.Groups = append(item.Groups, namedGroup{Name: group.Name, Labels: group.Labels})
		}
		got = append(got, item)
	}
	return sortsEqual(got, buildSorts(models))
}

func unlockPlanGateJSON(raw json.RawMessage) (json.RawMessage, bool) {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil || doc == nil {
		return nil, false
	}
	if !jsonTruthy(doc["disabled"]) || !jsonPlanGate(doc["disabledReason"], doc["disabled_reason"]) {
		return nil, false
	}
	for _, key := range []string{"disabled", "disabledReason", "disabled_reason", "isPremium", "is_premium", "allowedTiers", "allowed_tiers"} {
		delete(doc, key)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	return out, true
}

func jsonTruthy(v any) bool {
	on, ok := v.(bool)
	return ok && on
}

func jsonPlanGate(values ...any) bool {
	for _, value := range values {
		switch item := value.(type) {
		case string:
			if planGateText(item) {
				return true
			}
		case map[string]any:
			for _, key := range []string{"shortReason", "short_reason", "description"} {
				if text, ok := item[key].(string); ok && planGateText(text) {
					return true
				}
			}
		}
	}
	return false
}

func planGateText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	return text == "pro" || strings.Contains(text, "upgrade")
}

func modelJSON(model preparedModel) map[string]any {
	features := map[string]any{
		"zeroShotCapable":           true,
		"supportsToolCalls":         true,
		"supportsParallelToolCalls": true,
	}
	if model.Thinking {
		features["supportsThinking"] = true
	}
	item := map[string]any{
		"label":     model.Label,
		"modelUid":  model.ID,
		"provider":  providerOf(model.BaseID),
		"maxTokens": model.Context,
		"modelInfo": map[string]any{
			"modelUid":           model.ID,
			"modelType":          2,
			"maxTokens":          model.Context,
			"maxOutputTokens":    model.MaxOutput,
			"inferenceServerUrl": inferenceURL,
			"harnessUids":        []string{"swe-1p6", "swe-1p5"},
			"displayOption":      "DISPLAY_OPTION_MODERN_AGENT_CHAT",
			"modelFamilyUid":     "devinhub-family-" + model.BaseID,
			"modelFeatures":      features,
		},
		"isDefaultModelInFamily": model.Default,
		"modelFamilyMetadata":    familyJSON(model),
		"modelDimensions":        costJSON(model.Costs),
	}
	if model.Rate != "" {
		item["promoStatus"] = map[string]any{"isActive": true, "label": model.Rate}
	}
	return item
}

func familyJSON(model preparedModel) map[string]any {
	entries := make([]any, 0, len(model.Entries))
	for _, entry := range model.Entries {
		entries = append(entries, map[string]any{
			"key":   entry.Key,
			"value": map[string]any{"order": entry.Order, "name": entry.Name, "controlType": entry.Control},
		})
	}
	return map[string]any{"modelFamilyLabel": model.Family, "entries": entries}
}

func costJSON(costs []costLine) []any {
	out := make([]any, 0, len(costs))
	for _, cost := range costs {
		item := map[string]any{
			"label": cost.Label, "value": cost.Value, "denominator": "1M",
			"minRange": cost.Min, "maxRange": cost.Max, "kind": 1,
		}
		if cost.Info != "" {
			item["info"] = cost.Info
		}
		out = append(out, item)
	}
	return out
}

func rewriteBase64(doc map[string]json.RawMessage, models []preparedModel) int {
	raw, ok := doc["userStatusProtoBinaryBase64"]
	if !ok {
		return 0
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) != nil || encoded == "" {
		return 0
	}
	binaryStatus, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return 0
	}
	next, added, err := augmentUserStatus(binaryStatus, models)
	if err != nil || added == 0 {
		return 0
	}
	replacement, err := json.Marshal(base64.StdEncoding.EncodeToString(next))
	if err != nil {
		return 0
	}
	doc["userStatusProtoBinaryBase64"] = replacement
	return added
}

func jsonKey(doc map[string]json.RawMessage, camel, snake string) string {
	if _, ok := doc[snake]; ok {
		if _, hasCamel := doc[camel]; !hasCamel {
			return snake
		}
	}
	return camel
}
