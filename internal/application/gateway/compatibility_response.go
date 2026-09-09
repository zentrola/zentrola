package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const compatibilityResponseLimit = 32 << 20

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
			"usage": map[string]any{
				"input_tokens": numberValue(usage, "prompt_tokens"), "output_tokens": numberValue(usage, "completion_tokens"),
			},
		}, nil
	})
}

func anthropicJSONToOpenAIChat(reader io.Reader, writer io.Writer) error {
	return transformJSON(reader, writer, func(source map[string]any) (map[string]any, error) {
		if stringValue(source["type"]) == "error" {
			return anthropicErrorToOpenAI(source), nil
		}
		content, toolCalls := anthropicResponseContent(source["content"])
		message := map[string]any{"role": "assistant", "content": content}
		if len(toolCalls) > 0 {
			message["tool_calls"] = toolCalls
		}
		usage, _ := source["usage"].(map[string]any)
		prompt, completion := numberValue(usage, "input_tokens"), numberValue(usage, "output_tokens")
		return map[string]any{
			"id": openAIID(source["id"], "chatcmpl"), "object": "chat.completion", "created": time.Now().Unix(), "model": source["model"],
			"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": anthropicStopToOpenAI(stringValue(source["stop_reason"]))}},
			"usage":   map[string]any{"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": addNumbers(prompt, completion)},
		}, nil
	})
}

func anthropicJSONToOpenAIResponses(reader io.Reader, writer io.Writer) error {
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
		usage, _ := source["usage"].(map[string]any)
		inputTokens, outputTokens := numberValue(usage, "input_tokens"), numberValue(usage, "output_tokens")
		return map[string]any{
			"id": openAIID(source["id"], "resp"), "object": "response", "created_at": time.Now().Unix(),
			"status": "completed", "model": source["model"], "output": output, "error": nil,
			"usage": map[string]any{"input_tokens": inputTokens, "output_tokens": outputTokens, "total_tokens": addNumbers(inputTokens, outputTokens)},
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

func numberValue(source map[string]any, key string) any {
	if source != nil {
		if value, ok := source[key]; ok {
			return value
		}
	}
	return json.Number("0")
}

func addNumbers(left, right any) json.Number {
	var l, r int64
	fmt.Sscan(fmt.Sprint(left), &l)
	fmt.Sscan(fmt.Sprint(right), &r)
	return json.Number(fmt.Sprint(l + r))
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
