package gateway

import (
	"encoding/json"
	"errors"
	"strings"
)

type codexChatConversionError struct {
	stage string
	field string
	cause error
}

func (e *codexChatConversionError) Error() string { return e.cause.Error() }
func (e *codexChatConversionError) Unwrap() error { return e.cause }

func codexChatStage(stage string, err error) error {
	return &codexChatConversionError{stage: stage, cause: err}
}

func codexChatField(stage, field string, err error) error {
	return &codexChatConversionError{stage: stage, field: safeCodexChatField(field), cause: err}
}

// CodexChatConversionStage returns a fixed diagnostic label without request content.
func CodexChatConversionStage(err error) string {
	var conversion *codexChatConversionError
	if errors.As(err, &conversion) {
		return conversion.stage
	}
	return ""
}

// CodexChatConversionField returns only short, ordinary field names for dev diagnostics.
func CodexChatConversionField(err error) string {
	var conversion *codexChatConversionError
	if errors.As(err, &conversion) {
		return conversion.field
	}
	return ""
}

func safeCodexChatField(field string) string {
	if len(field) == 0 || len(field) > 48 {
		return ""
	}
	for i := 0; i < len(field); i++ {
		char := field[i]
		if i == 0 && !('a' <= char && char <= 'z' || 'A' <= char && char <= 'Z') ||
			i > 0 && !('a' <= char && char <= 'z' || 'A' <= char && char <= 'Z' || '0' <= char && char <= '9' || char == '_') {
			return ""
		}
	}
	lower := strings.ToLower(field)
	for _, sensitive := range []string{"secret", "password", "token", "credential", "private", "api_key", "access_key", "authorization"} {
		if strings.Contains(lower, sensitive) {
			return ""
		}
	}
	return field
}

// codexChatToResponses converts the Chat Completions subset supported by the
// Codex subscription endpoint. Unknown fields fail closed instead of silently
// changing a client's request semantics.
func codexChatToResponses(request Request) (Request, bool, error) {
	source, err := decodeJSONObject(request.Body)
	if err != nil {
		return Request{}, false, codexChatStage("json", err)
	}
	allowed := map[string]bool{
		"model": true, "messages": true, "stream": true, "stream_options": true,
		"temperature": true, "top_p": true, "max_tokens": true,
		"max_completion_tokens": true, "tools": true, "tool_choice": true,
		"parallel_tool_calls": true, "n": true,
	}
	for field := range source {
		if !allowed[field] {
			return Request{}, false, codexChatStage("unsupported_field", ErrInvalid)
		}
	}
	model := stringValue(source["model"])
	if !validModel(model) {
		return Request{}, false, ErrInvalid
	}
	clientStream, _ := source["stream"].(bool)
	if source["stream"] != nil {
		if _, ok := source["stream"].(bool); !ok {
			return Request{}, false, ErrInvalid
		}
	}
	if options, exists := source["stream_options"]; exists {
		values, ok := options.(map[string]any)
		if !ok || !clientStream {
			return Request{}, false, ErrInvalid
		}
		for field, value := range values {
			if field != "include_usage" {
				return Request{}, false, ErrInvalid
			}
			if _, ok := value.(bool); !ok {
				return Request{}, false, ErrInvalid
			}
		}
	}
	if n, ok := source["n"]; ok {
		number, numberOK := n.(json.Number)
		if !numberOK || number.String() != "1" {
			return Request{}, false, ErrInvalid
		}
	}
	if _, first := source["max_tokens"]; first {
		if _, second := source["max_completion_tokens"]; second {
			return Request{}, false, ErrInvalid
		}
	}
	for _, field := range []string{"temperature", "top_p"} {
		if value, exists := source[field]; exists && value != nil {
			number, ok := value.(json.Number)
			if !ok {
				return Request{}, false, ErrInvalid
			}
			defaultValue, err := number.Float64()
			if err != nil || defaultValue != 1 {
				return Request{}, false, ErrInvalid
			}
		}
	}
	input := make([]any, 0)
	messages := anySlice(source["messages"])
	if len(messages) == 0 {
		return Request{}, false, ErrInvalid
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			return Request{}, false, ErrInvalid
		}
		if rawSkip, exists := message["skipRun"]; exists {
			skip, ok := rawSkip.(bool)
			if !ok {
				return Request{}, false, codexChatStage("message_skip_run", ErrInvalid)
			}
			if skip {
				continue
			}
			delete(message, "skipRun")
		}
		items, convertErr := codexChatMessage(message)
		if convertErr != nil {
			if CodexChatConversionStage(convertErr) == "" {
				convertErr = codexChatStage("message", convertErr)
			}
			return Request{}, false, convertErr
		}
		input = append(input, items...)
	}
	if len(input) == 0 {
		return Request{}, false, codexChatStage("messages_skipped", ErrInvalid)
	}
	target := map[string]any{"model": model, "input": input, "stream": true, "store": false}
	copyFields(target, source, "parallel_tool_calls")
	if value, ok := source["max_tokens"]; ok {
		limit, err := codexChatOutputLimit(value)
		if err != nil {
			return Request{}, false, err
		}
		target["max_output_tokens"] = limit
	} else if value, ok := source["max_completion_tokens"]; ok {
		limit, err := codexChatOutputLimit(value)
		if err != nil {
			return Request{}, false, err
		}
		target["max_output_tokens"] = limit
	}
	if raw, ok := source["tools"]; ok {
		tools, convertErr := codexChatTools(raw)
		if convertErr != nil {
			return Request{}, false, codexChatStage("tools", convertErr)
		}
		target["tools"] = tools
	}
	if raw, ok := source["tool_choice"]; ok {
		choice, convertErr := codexChatToolChoice(raw)
		if convertErr != nil {
			return Request{}, false, codexChatStage("tool_choice", convertErr)
		}
		target["tool_choice"] = choice
	}
	body, err := encodeJSONObject(target)
	if err != nil {
		return Request{}, false, err
	}
	request.Protocol, request.Path, request.Body = OpenAIResponsesProtocol, "/v1/responses", body
	return request, clientStream, nil
}

// Codex subscriptions require at least 16 output tokens. Clients commonly use
// a one-token request to test connectivity; raise only sub-minimum limits.
func codexChatOutputLimit(value any) (json.Number, error) {
	number, ok := value.(json.Number)
	if !ok {
		return "", ErrInvalid
	}
	limit, err := number.Int64()
	if err != nil || limit <= 0 {
		return "", ErrInvalid
	}
	if limit < 16 {
		return json.Number("16"), nil
	}
	return number, nil
}

func codexChatMessage(message map[string]any) ([]any, error) {
	role := stringValue(message["role"])
	if role == "tool" {
		if len(message) != 3 || stringValue(message["tool_call_id"]) == "" {
			return nil, codexChatStage("tool_message_shape", ErrInvalid)
		}
		content, ok := message["content"].(string)
		if !ok {
			return nil, codexChatStage("tool_message_content", ErrInvalid)
		}
		return []any{map[string]any{"type": "function_call_output", "call_id": message["tool_call_id"], "output": content}}, nil
	}
	if role != "system" && role != "developer" && role != "user" && role != "assistant" {
		return nil, codexChatStage("message_role", ErrInvalid)
	}
	for field := range message {
		if field != "role" && field != "content" && field != "tool_calls" {
			return nil, codexChatField(codexChatMessageFieldStage(field), field, ErrInvalid)
		}
	}
	parts := make([]any, 0)
	if content, ok := message["content"]; ok && content != nil {
		var err error
		parts, err = codexChatContent(content, role)
		if err != nil {
			if CodexChatConversionStage(err) == "" {
				err = codexChatStage("message_content", err)
			}
			return nil, err
		}
	}
	result := make([]any, 0, 1)
	if len(parts) > 0 {
		result = append(result, map[string]any{"type": "message", "role": role, "content": parts})
	} else if role != "assistant" {
		return nil, codexChatStage("message_content_missing", ErrInvalid)
	}
	if rawCalls, ok := message["tool_calls"]; ok {
		if role != "assistant" || len(anySlice(rawCalls)) == 0 {
			return nil, codexChatStage("assistant_tool_calls", ErrInvalid)
		}
		for _, raw := range anySlice(rawCalls) {
			call, ok := raw.(map[string]any)
			if !ok || len(call) != 3 || stringValue(call["type"]) != "function" || stringValue(call["id"]) == "" {
				return nil, codexChatStage("assistant_tool_call_shape", ErrInvalid)
			}
			function, ok := call["function"].(map[string]any)
			if !ok || len(function) != 2 || stringValue(function["name"]) == "" || !json.Valid([]byte(stringValue(function["arguments"]))) {
				return nil, codexChatStage("assistant_tool_call_function", ErrInvalid)
			}
			result = append(result, map[string]any{
				"type": "function_call", "call_id": call["id"],
				"name": function["name"], "arguments": function["arguments"],
			})
		}
	}
	if len(result) == 0 {
		return nil, ErrInvalid
	}
	return result, nil
}

func codexChatMessageFieldStage(field string) string {
	switch field {
	case "name", "function_call", "tool_call_id", "reasoning_content", "refusal", "audio", "annotations", "metadata", "cache_control", "id", "status":
		return "message_field_" + field
	default:
		return "message_field"
	}
}

func codexChatContent(content any, role string) ([]any, error) {
	textType := "input_text"
	if role == "assistant" {
		textType = "output_text"
	}
	if text, ok := content.(string); ok {
		return []any{map[string]any{"type": textType, "text": text}}, nil
	}
	items := anySlice(content)
	if len(items) == 0 {
		return nil, codexChatStage("content_type", ErrInvalid)
	}
	parts := make([]any, 0, len(items))
	for _, raw := range items {
		part, ok := raw.(map[string]any)
		if !ok {
			return nil, codexChatStage("content_part_shape", ErrInvalid)
		}
		switch stringValue(part["type"]) {
		case "text":
			if _, ok := part["text"].(string); !ok || len(part) != 2 {
				return nil, codexChatStage("text_part_shape", ErrInvalid)
			}
			parts = append(parts, map[string]any{"type": textType, "text": part["text"]})
		case "image_url":
			image, ok := part["image_url"].(map[string]any)
			if !ok || role != "user" || len(part) != 2 || stringValue(image["url"]) == "" {
				return nil, codexChatStage("image_part_shape", ErrInvalid)
			}
			converted := map[string]any{"type": "input_image", "image_url": image["url"]}
			for field := range image {
				if field != "url" && field != "detail" {
					return nil, codexChatStage("image_part_field", ErrInvalid)
				}
			}
			copyFields(converted, image, "detail")
			parts = append(parts, converted)
		default:
			return nil, codexChatStage("content_part_type", ErrInvalid)
		}
	}
	return parts, nil
}

func codexChatTools(value any) ([]any, error) {
	rawTools := anySlice(value)
	if rawTools == nil {
		return nil, ErrInvalid
	}
	tools := make([]any, 0, len(rawTools))
	for _, raw := range rawTools {
		tool, ok := raw.(map[string]any)
		if !ok || len(tool) != 2 || stringValue(tool["type"]) != "function" {
			return nil, ErrInvalid
		}
		function, ok := tool["function"].(map[string]any)
		if !ok || stringValue(function["name"]) == "" {
			return nil, ErrInvalid
		}
		for field := range function {
			if field != "name" && field != "description" && field != "parameters" && field != "strict" {
				return nil, ErrInvalid
			}
		}
		converted := map[string]any{"type": "function", "name": function["name"]}
		copyFields(converted, function, "description", "parameters", "strict")
		tools = append(tools, converted)
	}
	return tools, nil
}

func codexChatToolChoice(value any) (any, error) {
	if choice, ok := value.(string); ok {
		if choice == "auto" || choice == "none" || choice == "required" {
			return choice, nil
		}
		return nil, ErrInvalid
	}
	choice, ok := value.(map[string]any)
	if !ok || len(choice) != 2 || stringValue(choice["type"]) != "function" {
		return nil, ErrInvalid
	}
	function, ok := choice["function"].(map[string]any)
	if !ok || len(function) != 1 || stringValue(function["name"]) == "" {
		return nil, ErrInvalid
	}
	return map[string]any{"type": "function", "name": function["name"]}, nil
}
