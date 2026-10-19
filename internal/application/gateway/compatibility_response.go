package gateway

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	compatibilityResponseLimit = 32 << 20
	compatibilityErrorLimit    = 64 << 10
)

func transformJSON(reader io.Reader, writer io.Writer, transform func(map[string]any) (map[string]any, error)) error {
	data, err := io.ReadAll(io.LimitReader(reader, compatibilityResponseLimit+1))
	if err != nil || len(data) > compatibilityResponseLimit {
		return ErrUpstream
	}
	source, err := decodeJSONObject(data)
	if err != nil {
		return ErrUpstream
	}
	target, err := transform(source)
	if err != nil {
		return err
	}
	return json.NewEncoder(writer).Encode(target)
}

func openAIJSONToAnthropic(reader io.Reader, writer io.Writer) error {
	return transformJSON(reader, writer, func(source map[string]any) (map[string]any, error) {
		if upstreamError, ok := source["error"].(map[string]any); ok {
			return map[string]any{"type": "error", "error": map[string]any{
				"type":    openAIErrorToAnthropic(stringValue(upstreamError["type"]), stringValue(upstreamError["code"])),
				"message": fallbackString(stringValue(upstreamError["message"]), "Upstream request failed."),
			}}, nil
		}
		choices := anySlice(source["choices"])
		if len(choices) == 0 {
			return nil, ErrUpstream
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			return nil, ErrUpstream
		}
		message, ok := choice["message"].(map[string]any)
		if !ok {
			return nil, ErrUpstream
		}
		content := make([]any, 0)
		if text := stringValue(message["content"]); text != "" {
			content = append(content, map[string]any{"type": "text", "text": text})
		} else if parts := anySlice(message["content"]); len(parts) > 0 {
			for _, raw := range parts {
				part, _ := raw.(map[string]any)
				if text := stringValue(part["text"]); text != "" {
					content = append(content, map[string]any{"type": "text", "text": text})
				}
			}
		}
		for _, raw := range anySlice(message["tool_calls"]) {
			call, ok := raw.(map[string]any)
			function, functionOK := call["function"].(map[string]any)
			if !ok || !functionOK {
				return nil, ErrUpstream
			}
			var input any
			if err := decodeJSONValue([]byte(fallbackString(stringValue(function["arguments"]), "{}")), &input); err != nil {
				return nil, ErrUpstream
			}
			content = append(content, map[string]any{"type": "tool_use", "id": call["id"], "name": function["name"], "input": input})
		}
		usage, _ := source["usage"].(map[string]any)
		return map[string]any{
			"id": fallbackString(stringValue(source["id"]), "msg_compat"), "type": "message", "role": "assistant",
			"model": source["model"], "content": content,
			"stop_reason": openAIFinishToAnthropic(stringValue(choice["finish_reason"])), "stop_sequence": nil,
			"usage": newOpenAIUsageCounts(usage).anthropicUsage(),
		}, nil
	})
}

func anthropicJSONToOpenAIChat(reader io.Reader, writer io.Writer, sink *CompatibilityUsage) error {
	return transformJSON(reader, writer, func(source map[string]any) (map[string]any, error) {
		if stringValue(source["type"]) == "error" {
			return anthropicErrorToOpenAI(source), nil
		}
		content, toolCalls := anthropicResponseContent(source["content"])
		message := map[string]any{"role": "assistant", "content": content}
		if len(toolCalls) > 0 {
			message["tool_calls"] = toolCalls
		}
		var counts anthropicUsageCounts
		counts.apply(source["usage"])
		sink.record(counts)
		return map[string]any{
			"id": openAIID(source["id"], "chatcmpl"), "object": "chat.completion", "created": time.Now().Unix(), "model": source["model"],
			"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": anthropicStopToOpenAI(stringValue(source["stop_reason"]))}},
			"usage":   counts.chatUsage(),
		}, nil
	})
}

func anthropicJSONToOpenAIResponses(reader io.Reader, writer io.Writer, sink *CompatibilityUsage) error {
	return transformJSON(reader, writer, func(source map[string]any) (map[string]any, error) {
		if stringValue(source["type"]) == "error" {
			return anthropicErrorToOpenAI(source), nil
		}
		output := make([]any, 0)
		textParts := make([]any, 0)
		for _, raw := range anySlice(source["content"]) {
			block, _ := raw.(map[string]any)
			switch stringValue(block["type"]) {
			case "text":
				textParts = append(textParts, map[string]any{"type": "output_text", "text": block["text"], "annotations": []any{}})
			case "tool_use":
				arguments, _ := json.Marshal(block["input"])
				output = append(output, map[string]any{
					"type": "function_call", "id": openAIID(block["id"], "fc"), "call_id": block["id"],
					"name": block["name"], "arguments": string(arguments), "status": "completed",
				})
			}
		}
		if len(textParts) > 0 {
			output = append([]any{map[string]any{"type": "message", "id": openAIID(source["id"], "msg"), "role": "assistant", "status": "completed", "content": textParts}}, output...)
		}
		var counts anthropicUsageCounts
		counts.apply(source["usage"])
		sink.record(counts)
		return map[string]any{
			"id": openAIID(source["id"], "resp"), "object": "response", "created_at": time.Now().Unix(),
			"status": "completed", "model": source["model"], "output": output, "error": nil,
			"usage": counts.responsesUsage(),
		}, nil
	})
}

func anthropicResponseContent(value any) (any, []any) {
	var text strings.Builder
	toolCalls := make([]any, 0)
	for _, raw := range anySlice(value) {
		block, _ := raw.(map[string]any)
		switch stringValue(block["type"]) {
		case "text":
			text.WriteString(stringValue(block["text"]))
		case "tool_use":
			arguments, _ := json.Marshal(block["input"])
			toolCalls = append(toolCalls, map[string]any{
				"id": block["id"], "type": "function", "function": map[string]any{"name": block["name"], "arguments": string(arguments)},
			})
		}
	}
	if text.Len() == 0 {
		return nil, toolCalls
	}
	return text.String(), toolCalls
}

func anthropicErrorToOpenAI(source map[string]any) map[string]any {
	errorValue, _ := source["error"].(map[string]any)
	message := fallbackString(stringValue(errorValue["message"]), "Upstream request failed.")
	errorType := fallbackString(stringValue(errorValue["type"]), "api_error")
	return map[string]any{"error": map[string]any{"message": message, "type": errorType, "code": errorType}}
}

func openAIErrorToAnthropic(errorType, code string) string {
	value := strings.ToLower(errorType + " " + code)
	switch {
	case strings.Contains(value, "rate"):
		return "rate_limit_error"
	case strings.Contains(value, "auth") || strings.Contains(value, "permission"):
		return "authentication_error"
	case strings.Contains(value, "invalid"):
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

func openAIFinishToAnthropic(reason string) any {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls", "function_call":
		return "tool_use"
	case "stop":
		return "end_turn"
	case "content_filter":
		return "refusal"
	case "":
		return nil
	default:
		return "end_turn"
	}
}

func anthropicStopToOpenAI(reason string) any {
	switch reason {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "end_turn", "stop_sequence", "pause_turn":
		return "stop"
	case "refusal":
		return "content_filter"
	case "":
		return nil
	default:
		return "stop"
	}
}

// int64Value 将 decodeJSONObject（UseNumber）产生的数字转换为整数。
func int64Value(value any) (int64, bool) {
	switch number := value.(type) {
	case json.Number:
		if parsed, err := number.Int64(); err == nil {
			return parsed, true
		}
		parsed, err := number.Float64()
		if err != nil {
			return 0, false
		}
		return int64Value(parsed)
	case float64:
		if number != math.Trunc(number) || number < math.MinInt64 || number >= math.MaxInt64 {
			return 0, false
		}
		return int64(number), true
	case int:
		return int64(number), true
	case int64:
		return number, true
	}
	return 0, false
}

// usageCount 读取非负 Token 计数；缺失或非法时返回 ok=false。
func usageCount(source map[string]any, key string) (int64, bool) {
	value, exists := source[key]
	if !exists {
		return 0, false
	}
	count, ok := int64Value(value)
	if !ok || count < 0 {
		return 0, false
	}
	return count, true
}

func addCounts(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}

// anthropicUsageCounts 保存 Anthropic 互不包含的三类输入计数。message_delta 的
// 计数为累计值，出现的字段直接覆盖。
type anthropicUsageCounts struct {
	input, cacheCreation, cacheRead, output int64
	cacheCreationSeen, cacheReadSeen        bool
}

func (c *anthropicUsageCounts) apply(raw any) {
	usage, ok := raw.(map[string]any)
	if !ok {
		return
	}
	if value, ok := usageCount(usage, "input_tokens"); ok {
		c.input = value
	}
	if value, ok := usageCount(usage, "cache_creation_input_tokens"); ok {
		c.cacheCreation, c.cacheCreationSeen = value, true
	}
	if value, ok := usageCount(usage, "cache_read_input_tokens"); ok {
		c.cacheRead, c.cacheReadSeen = value, true
	}
	if value, ok := usageCount(usage, "output_tokens"); ok {
		c.output = value
	}
}

// prompt 按 OpenAI 语义返回总输入：包含缓存写入与缓存读取。
func (c anthropicUsageCounts) prompt() int64 {
	return addCounts(addCounts(c.input, c.cacheCreation), c.cacheRead)
}

func (c anthropicUsageCounts) chatUsage() map[string]any {
	prompt := c.prompt()
	usage := map[string]any{"prompt_tokens": prompt, "completion_tokens": c.output, "total_tokens": addCounts(prompt, c.output)}
	if c.cacheReadSeen {
		usage["prompt_tokens_details"] = map[string]any{"cached_tokens": c.cacheRead}
	}
	return usage
}

func (c anthropicUsageCounts) responsesUsage() map[string]any {
	prompt := c.prompt()
	usage := map[string]any{"input_tokens": prompt, "output_tokens": c.output, "total_tokens": addCounts(prompt, c.output)}
	if c.cacheReadSeen {
		usage["input_tokens_details"] = map[string]any{"cached_tokens": c.cacheRead}
	}
	return usage
}

// openAIUsageCounts 保存 OpenAI 语义的 usage：prompt 为总输入，cached 包含在其中。
type openAIUsageCounts struct {
	prompt, completion, cached int64
	cachedSeen                 bool
}

func newOpenAIUsageCounts(usage map[string]any) openAIUsageCounts {
	var counts openAIUsageCounts
	counts.prompt, _ = usageCount(usage, "prompt_tokens")
	counts.completion, _ = usageCount(usage, "completion_tokens")
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		counts.cached, counts.cachedSeen = usageCount(details, "cached_tokens")
	}
	if !counts.cachedSeen {
		counts.cached, counts.cachedSeen = usageCount(usage, "prompt_cache_hit_tokens")
	}
	counts.cached = min(counts.cached, counts.prompt)
	return counts
}

// anthropicUsage 转换为 Anthropic 语义：input_tokens 不含缓存读取部分。
func (c openAIUsageCounts) anthropicUsage() map[string]any {
	usage := map[string]any{"input_tokens": c.prompt - c.cached, "output_tokens": c.completion}
	if c.cachedSeen {
		usage["cache_read_input_tokens"] = c.cached
	}
	return usage
}

// compatibilityErrorTransform 将上游错误响应统一包装为目标协议的标准错误体。
// vLLM、nginx 等返回的非标准错误体不能被当作传输失败，否则会触发冷却与重试。
func compatibilityErrorTransform(targetProtocol string, status int) func(io.Reader, io.Writer) error {
	return func(reader io.Reader, writer io.Writer) error {
		// 读取失败时仍用已读部分生成错误体，HTTP 状态码保持不变。
		data, _ := io.ReadAll(io.LimitReader(reader, compatibilityErrorLimit))
		return json.NewEncoder(writer).Encode(compatibilityErrorBody(targetProtocol, status, data))
	}
}

func compatibilityErrorBody(targetProtocol string, status int, data []byte) map[string]any {
	errorType, message := "", ""
	if source, err := decodeJSONObject(data); err == nil {
		errorType, message = upstreamErrorDetails(source)
	}
	if message == "" {
		message = "Upstream request failed with HTTP " + strconv.Itoa(status) + "."
	}
	if targetProtocol == AnthropicProtocol {
		return map[string]any{"type": "error", "error": map[string]any{
			"type": anthropicErrorType(errorType, status), "message": message,
		}}
	}
	if errorType == "" || errorType == "error" {
		errorType = openAIErrorTypeForStatus(status)
	}
	return map[string]any{"error": map[string]any{"message": message, "type": errorType, "code": errorType}}
}

// upstreamErrorDetails 兼容 OpenAI、Anthropic、vLLM 与 FastAPI 风格的错误体。
func upstreamErrorDetails(source map[string]any) (string, string) {
	switch value := source["error"].(type) {
	case map[string]any:
		return fallbackString(stringValue(value["type"]), stringValue(value["code"])), stringValue(value["message"])
	case string:
		return stringValue(source["type"]), value
	}
	if message := stringValue(source["message"]); message != "" {
		return stringValue(source["type"]), message
	}
	return "", stringValue(source["detail"])
}

func anthropicErrorType(upstreamType string, status int) string {
	if mapped := openAIErrorToAnthropic(upstreamType, ""); mapped != "api_error" {
		return mapped
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	}
	if status >= 400 && status < 500 {
		return "invalid_request_error"
	}
	return "api_error"
}

func openAIErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	}
	if status >= 400 && status < 500 {
		return "invalid_request_error"
	}
	return "api_error"
}

func openAIID(value any, prefix string) string {
	id := stringValue(value)
	if id == "" {
		return prefix + "_compat"
	}
	if strings.HasPrefix(id, prefix+"_") {
		return id
	}
	return prefix + "_" + id
}

func fallbackString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
