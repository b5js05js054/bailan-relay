// Package connectframe 解析 Connect/gRPC 帧并把 Chat protobuf 的 Field 21
// (模型名) 从 SWE-1.6 替换成指定模型。移植自旧版 proxy_addon.py。
package connectframe

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

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
		if shift > 35 {
			break
		}
	}
	return 0, offset, errors.New("truncated varint")
}

func encodeVarint(value uint64) []byte {
	var out []byte
	for value > 0x7F {
		out = append(out, byte(value&0x7F)|0x80)
		value >>= 7
	}
	out = append(out, byte(value&0x7F))
	return out
}

func encodeStringField(fieldNumber int, value string) []byte {
	raw := []byte(value)
	out := encodeVarint(uint64(fieldNumber<<3) | 2)
	out = append(out, encodeVarint(uint64(len(raw)))...)
	return append(out, raw...)
}

type field struct {
	num int
	wt  int
	raw []byte
}

func parseFields(data []byte) []field {
	var fields []field
	offset := 0
	for offset < len(data) {
		start := offset
		tag, next, err := readVarint(data, offset)
		if err != nil {
			break
		}
		offset = next
		fn := int(tag >> 3)
		wt := int(tag & 0x07)
		if fn == 0 {
			break
		}
		switch wt {
		case 0:
			_, offset, err = readVarint(data, offset)
			if err != nil {
				return fields
			}
		case 1:
			offset += 8
		case 2:
			var length uint64
			length, offset, err = readVarint(data, offset)
			if err != nil {
				return fields
			}
			offset += int(length)
		case 5:
			offset += 4
		default:
			return fields
		}
		if offset > len(data) {
			break
		}
		fields = append(fields, field{num: fn, wt: wt, raw: data[start:offset]})
	}
	return fields
}

func extractModelName(payload []byte) string {
	for _, f := range parseFields(payload) {
		if f.num == 21 && f.wt == 2 {
			offset := 0
			_, offset, err := readVarint(f.raw, offset)
			if err != nil {
				return ""
			}
			length, offset, err := readVarint(f.raw, offset)
			if err != nil {
				return ""
			}
			end := offset + int(length)
			if end > len(f.raw) {
				return ""
			}
			return strings.TrimSpace(string(f.raw[offset:end]))
		}
	}
	return ""
}

func protoString(payload []byte, num int) string {
	for _, f := range parseFields(payload) {
		if f.num == num && f.wt == 2 {
			return fieldString(f.raw)
		}
	}
	return ""
}

func fieldString(raw []byte) string {
	offset := 0
	_, offset, err := readVarint(raw, offset)
	if err != nil {
		return ""
	}
	length, offset, err := readVarint(raw, offset)
	if err != nil {
		return ""
	}
	end := offset + int(length)
	if end > len(raw) {
		return ""
	}
	return strings.TrimSpace(string(raw[offset:end]))
}

func replaceModelName(payload []byte, newModel string) []byte {
	var parts [][]byte
	replaced := false
	for _, f := range parseFields(payload) {
		if f.num == 21 && f.wt == 2 {
			parts = append(parts, encodeStringField(21, newModel))
			replaced = true
		} else {
			parts = append(parts, f.raw)
		}
	}
	if !replaced {
		parts = append(parts, encodeStringField(21, newModel))
	}
	return bytes.Join(parts, nil)
}

func isSWE16(modelName string) bool {
	name := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(modelName)), ".", "-")
	return name == "swe-1-6" || strings.HasPrefix(name, "swe-1-6-")
}

// IsSWE16 判断模型名是不是官方 SWE-1.6 系列。这类请求仍走 Hub 里保存的默认模型。
func IsSWE16(modelName string) bool { return isSWE16(modelName) }

// ReplaceChatModel 把请求里的模型名换成目录 ID。
// 输入：可能带 Connect 帧的请求体、目录模型 ID、内容类型是否标明 Connect。输出：替换后的请求体；解析失败时原样返回。
// Devin 选择器会把思考档和上下文编进模型名，中继只认目录里的基础 ID。
// 有的请求体是 Connect 帧，但 Content-Type 只写 application/proto，这时也要按帧替换，不能把帧头当成 protobuf。
func ReplaceChatModel(rawBody []byte, catalogID string, isConnect bool) []byte {
	catalogID = strings.TrimSpace(catalogID)
	if catalogID == "" {
		return rawBody
	}
	if isConnect || looksLikeConnectFrame(rawBody) {
		payload, isTrailer, compressed, leftover, err := decodeConnectFrame(rawBody)
		if err == nil && !isTrailer && (isConnect || extractModelName(payload) != "") {
			if extractModelName(payload) == catalogID {
				return rawBody
			}
			encoded, err := encodeConnectFrame(replaceModelName(payload, catalogID), compressed, leftover)
			if err != nil {
				return rawBody
			}
			return encoded
		}
		if isConnect {
			return rawBody
		}
	}
	if extractModelName(rawBody) == catalogID {
		return rawBody
	}
	return replaceModelName(rawBody, catalogID)
}

func looksLikeConnectFrame(raw []byte) bool {
	if len(raw) < 5 || raw[0] > 0x03 {
		return false
	}
	n := int(binary.BigEndian.Uint32(raw[1:5]))
	return n >= 0 && 5+n <= len(raw)
}

// ChatModel 读取 GetChatMessage 请求里的模型名。
// 输入：可能带 Connect 帧的 protobuf 请求体；输出：Field 21 的模型名，读不到时为空。
func ChatModel(raw []byte) string {
	if len(raw) >= 5 && raw[0] <= 0x03 {
		payload, isTrailer, _, _, err := decodeConnectFrame(raw)
		if err == nil && !isTrailer {
			return extractModelName(payload)
		}
	}
	return extractModelName(raw)
}

// MessagePayload 去掉 Connect 帧头。压缩帧会先解压，便于在正文里找会话名。
func MessagePayload(raw []byte) []byte {
	if len(raw) >= 5 && raw[0] <= 0x03 {
		if decoded, isTrailer, _, _, err := decodeConnectFrame(raw); err == nil && !isTrailer && len(decoded) > 0 {
			return decoded
		}
	}
	return raw
}

// ChatSessionIDs 收集请求里可能用来区分会话的字符串，包含 cascade_id、session_id，以及嵌套字段里的短编号。
func ChatSessionIDs(raw []byte) []string {
	payload := MessagePayload(raw)
	var ids []string
	seen := map[string]bool{}
	collectSessionIDs(payload, &ids, seen, 0)
	return ids
}

func collectSessionIDs(payload []byte, ids *[]string, seen map[string]bool, depth int) {
	if depth > 4 {
		return
	}
	for _, f := range parseFields(payload) {
		if f.wt != 2 {
			continue
		}
		text := fieldString(f.raw)
		if sessionHint(text) && !seen[text] {
			seen[text] = true
			*ids = append(*ids, text)
		}
		inner := fieldBytes(f.raw)
		if len(inner) > 0 {
			collectSessionIDs(inner, ids, seen, depth+1)
		}
	}
}

func fieldBytes(raw []byte) []byte {
	offset := 0
	_, offset, err := readVarint(raw, offset)
	if err != nil {
		return nil
	}
	length, offset, err := readVarint(raw, offset)
	if err != nil {
		return nil
	}
	end := offset + int(length)
	if end > len(raw) {
		return nil
	}
	return raw[offset:end]
}

func sessionHint(text string) bool {
	if len(text) < 6 || len(text) > 160 {
		return false
	}
	for _, r := range text {
		if r < 32 || r > 126 {
			return false
		}
	}
	return strings.Contains(text, "/") || strings.Count(text, "-") >= 1
}

func decodeConnectFrame(raw []byte) (payload []byte, isTrailer, compressed bool, leftover []byte, err error) {
	if len(raw) < 5 {
		return nil, false, false, nil, errors.New("frame too short")
	}
	flags := raw[0]
	msgLen := int(binary.BigEndian.Uint32(raw[1:5]))
	if 5+msgLen > len(raw) {
		return nil, false, false, nil, errors.New("frame length mismatch")
	}
	payload = raw[5 : 5+msgLen]
	leftover = raw[5+msgLen:]
	isTrailer = flags&0x02 != 0
	compressed = flags&0x01 != 0
	if compressed && !isTrailer {
		payload, err = gunzip(payload)
		if err != nil {
			return nil, false, false, nil, err
		}
	}
	return payload, isTrailer, compressed, leftover, nil
}

func encodeConnectFrame(protobuf []byte, compress bool, leftover []byte) ([]byte, error) {
	payload := protobuf
	var flags byte
	if compress {
		var err error
		payload, err = gzipBytes(protobuf)
		if err != nil {
			return nil, err
		}
		flags = 0x01
	}
	head := make([]byte, 5)
	head[0] = flags
	binary.BigEndian.PutUint32(head[1:5], uint32(len(payload)))
	out := append(head, payload...)
	return append(out, leftover...), nil
}

// ErrorTrailer 构造 Connect 错误 trailer 帧（flags=0x02，未压缩）：
// {"error":{"code":..,"message":..}}。与中继服务端返回给 IDE 的错误帧格式一致，
// IDE 会把 message 原样展示给用户。
func ErrorTrailer(message, code string) []byte {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
	head := make([]byte, 5)
	head[0] = 0x02
	binary.BigEndian.PutUint32(head[1:5], uint32(len(body)))
	return append(head, body...)
}

// OverrideChatModel 把 Chat protobuf Field 21 从 SWE-1.6 换成 newModel。
// 返回 (新 body, 原始模型名)。非 SWE-1.6 或解析失败时原样返回。
func OverrideChatModel(rawBody []byte, newModel string, isConnect bool) ([]byte, string) {
	if isConnect {
		payload, isTrailer, compressed, leftover, err := decodeConnectFrame(rawBody)
		if err != nil {
			return rawBody, ""
		}
		if isTrailer {
			return rawBody, ""
		}
		original := extractModelName(payload)
		if !isSWE16(original) {
			return rawBody, original
		}
		modified := replaceModelName(payload, newModel)
		encoded, err := encodeConnectFrame(modified, compressed, leftover)
		if err != nil {
			return rawBody, original
		}
		return encoded, original
	}
	original := extractModelName(rawBody)
	if !isSWE16(original) {
		return rawBody, original
	}
	return replaceModelName(rawBody, newModel), original
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
