package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

type Parsed struct {
	Model              string
	Stream             bool
	PreviousResponseID string
	// IncludeUsage 表示客户端自己通过 stream_options.include_usage 请求了 usage chunk。
	IncludeUsage                         bool
	objectStart                          int
	modelStart, modelEnd                 int
	toolsStart, toolsEnd                 int
	streamOptionsStart, streamOptionsEnd int
}

// skipJSONValue 只校验并跳过 messages 等路由无关字段，避免为大字段额外复制 RawMessage。
type skipJSONValue struct{}

func (*skipJSONValue) UnmarshalJSON([]byte) error { return nil }

func validModel(model string) bool {
	return len(model) > 0 && len(model) <= 128 && strings.TrimSpace(model) == model && !strings.ContainsAny(model, "\x00\r\n\t ")
}

// Parse 仅检查路由必需字段。其余 JSON 原文保留，避免工具输入数字或未知字段被重新编码。
func Parse(body []byte) (Parsed, error) {
	return parse(body, false)
}
func ParseOpenAI(body []byte) (Parsed, error) { return parse(body, true) }
func parse(body []byte, nullableStream bool) (Parsed, error) {
	var result Parsed
	if !utf8.Valid(body) {
		return result, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return result, ErrInvalid
	}
	result.objectStart = int(d.InputOffset())
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return result, ErrInvalid
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return result, ErrInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		switch key {
		case "model", "stream", "previous_response_id", "tools", "stream_options":
			if err := d.Decode(&raw); err != nil {
				return result, ErrInvalid
			}
		default:
			if err := d.Decode(&skipJSONValue{}); err != nil {
				return result, ErrInvalid
			}
			continue
		}
		switch key {
		case "model":
			if err := json.Unmarshal(raw, &result.Model); err != nil || !validModel(result.Model) {
				return result, ErrInvalid
			}
			result.modelEnd = int(d.InputOffset())
			result.modelStart = result.modelEnd - len(raw)
		case "stream":
			if nullableStream && bytes.Equal(raw, []byte("null")) {
				continue
			}
			if !bytes.Equal(raw, []byte("true")) && !bytes.Equal(raw, []byte("false")) {
				return result, ErrInvalid
			}
			result.Stream = bytes.Equal(raw, []byte("true"))
		case "previous_response_id":
			if !bytes.Equal(raw, []byte("null")) {
				if err := json.Unmarshal(raw, &result.PreviousResponseID); err != nil {
					return result, ErrInvalid
				}
			}
		case "tools":
			result.toolsEnd = int(d.InputOffset())
			result.toolsStart = result.toolsEnd - len(raw)
		case "stream_options":
			result.streamOptionsEnd = int(d.InputOffset())
			result.streamOptionsStart = result.streamOptionsEnd - len(raw)
			var options struct {
				IncludeUsage *bool `json:"include_usage"`
			}
			if json.Unmarshal(raw, &options) == nil && options.IncludeUsage != nil {
				result.IncludeUsage = *options.IncludeUsage
			}
		}
	}
	if _, err := d.Token(); err != nil {
		return result, ErrInvalid
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return result, ErrInvalid
	}
	if !seen["model"] {
		return result, ErrInvalid
	}
	return result, nil
}
func (p Parsed) Rewrite(body []byte, model string) []byte {
	encoded, _ := json.Marshal(model)
	out := make([]byte, 0, len(body)+len(encoded))
	out = append(out, body[:p.modelStart]...)
	out = append(out, encoded...)
	return append(out, body[p.modelEnd:]...)
}

// injectsStreamUsage 判断网关是否需要代客户端开启 Chat Completions 流式 usage。
func (p Parsed) injectsStreamUsage(protocol string) bool {
	return protocol == OpenAIProtocol && p.Stream && !p.IncludeUsage
}

type bodySegment struct {
	start, end  int
	replacement []byte
}

// upstreamBody 生成发往上游的请求体：model 与路由映射不同才改写；OpenAI Chat
// 流式请求强制开启 stream_options.include_usage，保留客户端已有的其他选项。
// 无需改写时直接复用原始请求体，避免热路径复制。
func (p Parsed) upstreamBody(body []byte, protocol, model string) []byte {
	segments := make([]bodySegment, 0, 2)
	if model != p.Model {
		encoded, _ := json.Marshal(model)
		segments = append(segments, bodySegment{start: p.modelStart, end: p.modelEnd, replacement: encoded})
	}
	if p.injectsStreamUsage(protocol) {
		if p.streamOptionsEnd == 0 {
			// 请求必然包含 model 字段，插入到对象开头后补逗号仍是合法 JSON。
			segments = append(segments, bodySegment{
				start: p.objectStart, end: p.objectStart,
				replacement: []byte(`"stream_options":{"include_usage":true},`),
			})
		} else if options, ok := streamOptionsWithUsage(body[p.streamOptionsStart:p.streamOptionsEnd]); ok {
			segments = append(segments, bodySegment{start: p.streamOptionsStart, end: p.streamOptionsEnd, replacement: options})
		}
	}
	if len(segments) == 0 {
		return body
	}
	sort.Slice(segments, func(i, j int) bool { return segments[i].start < segments[j].start })
	size := len(body)
	for _, segment := range segments {
		size += len(segment.replacement) - (segment.end - segment.start)
	}
	out := make([]byte, 0, size)
	cursor := 0
	for _, segment := range segments {
		out = append(out, body[cursor:segment.start]...)
		out = append(out, segment.replacement...)
		cursor = segment.end
	}
	return append(out, body[cursor:]...)
}

// streamOptionsWithUsage 在 stream_options 中写入 include_usage=true。
// 非对象且非 null 的取值保持原样，由上游按原协议校验。
func streamOptionsWithUsage(raw []byte) ([]byte, bool) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []byte(`{"include_usage":true}`), true
	}
	var options map[string]json.RawMessage
	if json.Unmarshal(raw, &options) != nil || options == nil {
		return nil, false
	}
	options["include_usage"] = json.RawMessage("true")
	encoded, err := json.Marshal(options)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

// RemoveToolTypes 删除上游明确不支持的服务端工具类型，同时保留请求中其他 JSON 原文。
func (p Parsed) RemoveToolTypes(body []byte, deniedTypes ...string) ([]byte, int, error) {
	if p.toolsEnd == 0 || len(deniedTypes) == 0 {
		return body, 0, nil
	}
	denied := make(map[string]struct{}, len(deniedTypes))
	for _, toolType := range deniedTypes {
		denied[toolType] = struct{}{}
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(body[p.toolsStart:p.toolsEnd], &tools); err != nil {
		return nil, 0, ErrInvalid
	}
	kept := make([]json.RawMessage, 0, len(tools))
	removed := 0
	for _, raw := range tools {
		var tool struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			return nil, 0, ErrInvalid
		}
		if _, blocked := denied[tool.Type]; blocked {
			removed++
			continue
		}
		kept = append(kept, raw)
	}
	if removed == 0 {
		return body, 0, nil
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		return nil, 0, ErrInvalid
	}
	out := make([]byte, 0, len(body)-p.toolsEnd+p.toolsStart+len(encoded))
	out = append(out, body[:p.toolsStart]...)
	out = append(out, encoded...)
	return append(out, body[p.toolsEnd:]...), removed, nil
}
