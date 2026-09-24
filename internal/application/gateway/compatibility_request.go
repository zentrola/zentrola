package gateway

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
)

func convertGatewayRequest(request Request, endpointProtocol string) (Request, error) {
	var (
		body []byte
		err  error
	)
	switch endpointProtocol {
	case OpenAIEndpoint:
		if request.Protocol != AnthropicProtocol || request.Path == "/v1/messages/count_tokens" {
			return Request{}, ErrRoute
		}
		body, err = anthropicRequestToOpenAIChat(request.Body)
		request.Protocol = OpenAIProtocol
		request.Path = "/v1/chat/completions"
		request.Version, request.Beta = "", ""
		request.BetaQuery = false
		request.ProtocolHeaders = nil
	case AnthropicEndpoint:
		switch request.Protocol {
		case OpenAIProtocol:
			body, err = openAIChatRequestToAnthropic(request.Body)
		case OpenAIResponsesProtocol:
			body, err = openAIResponsesRequestToAnthropic(request.Body)
		default:
			return Request{}, ErrRoute
		}
		request.Protocol = AnthropicProtocol
		request.Path = "/v1/messages"
		request.BetaQuery = false
		request.ProtocolHeaders = nil
	default:
		return Request{}, ErrRoute
	}
	if err != nil {
		return Request{}, err
	}
	request.Body = body
	return request, nil
}

func decodeJSONObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, ErrInvalid
	}
	return value, nil
}

func encodeJSONObject(value map[string]any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func anthropicRequestToOpenAIChat(body []byte) ([]byte, error) {
	source, err := decodeJSONObject(body)
	if err != nil {
		return nil, err
	}
	target := map[string]any{"model": source["model"]}
	copyFields(target, source, "temperature", "top_p", "stream")
	if value, ok := source["max_tokens"]; ok {
		target["max_tokens"] = value
	}
	if value, ok := source["stop_sequences"]; ok {
		target["stop"] = value
	}

	messages := make([]any, 0)
	if system, ok := source["system"]; ok {
		content, convertErr := anthropicContentToOpenAI(system)
		if convertErr != nil {
			return nil, convertErr
		}
		messages = append(messages, map[string]any{"role": "system", "content": content})
	}
	for _, raw := range anySlice(source["messages"]) {
		message, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		converted, convertErr := anthropicMessageToOpenAI(message)
		if convertErr != nil {
			return nil, convertErr
		}
		messages = append(messages, converted...)
	}
	if len(messages) == 0 {
		return nil, ErrInvalid
	}
	target["messages"] = messages

	if tools := anySlice(source["tools"]); len(tools) > 0 {
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || stringValue(tool["name"]) == "" {
				return nil, ErrInvalid
			}
			function := map[string]any{"name": tool["name"], "parameters": tool["input_schema"]}
			if description, ok := tool["description"]; ok {
				function["description"] = description
			}
			converted = append(converted, map[string]any{"type": "function", "function": function})
		}
		target["tools"] = converted
	}
	if choice, ok := source["tool_choice"].(map[string]any); ok {
		switch stringValue(choice["type"]) {
		case "auto":
			target["tool_choice"] = "auto"
		case "any":
			target["tool_choice"] = "required"
		case "tool":
			target["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": choice["name"]}}
		case "none":
			target["tool_choice"] = "none"
		}
	}
	return encodeJSONObject(target)
}

func anthropicMessageToOpenAI(message map[string]any) ([]any, error) {
	role := stringValue(message["role"])
	if role != "user" && role != "assistant" {
		return nil, ErrInvalid
	}
	content := message["content"]
	if text, ok := content.(string); ok {
		return []any{map[string]any{"role": role, "content": text}}, nil
	}
	blocks := anySlice(content)
	if blocks == nil {
		return nil, ErrInvalid
	}
	if role == "assistant" {
		parts := make([]any, 0)
		toolCalls := make([]any, 0)
		for _, raw := range blocks {
			block, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrInvalid
			}
			switch stringValue(block["type"]) {
			case "text":
				parts = append(parts, map[string]any{"type": "text", "text": block["text"]})
			case "tool_use":
				arguments, marshalErr := json.Marshal(block["input"])
				if marshalErr != nil {
					return nil, ErrInvalid
				}
				toolCalls = append(toolCalls, map[string]any{
					"id": block["id"], "type": "function",
					"function": map[string]any{"name": block["name"], "arguments": string(arguments)},
				})
			}
		}
		result := map[string]any{"role": "assistant", "content": nil}
		if len(parts) == 1 {
			result["content"] = stringValue(parts[0].(map[string]any)["text"])
		} else if len(parts) > 0 {
			result["content"] = parts
		}
		if len(toolCalls) > 0 {
			result["tool_calls"] = toolCalls
		}
		return []any{result}, nil
	}

	result := make([]any, 0)
	parts := make([]any, 0)
	flushUser := func() {
		if len(parts) == 0 {
			return
		}
		content := any(parts)
		if len(parts) == 1 && stringValue(parts[0].(map[string]any)["type"]) == "text" {
			content = parts[0].(map[string]any)["text"]
		}
		result = append(result, map[string]any{"role": "user", "content": content})
		parts = nil
	}
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		switch stringValue(block["type"]) {
		case "text":
			parts = append(parts, map[string]any{"type": "text", "text": block["text"]})
		case "image":
			part, convertErr := anthropicImageToOpenAI(block)
			if convertErr != nil {
				return nil, convertErr
			}
			parts = append(parts, part)
		case "tool_result":
			flushUser()
			result = append(result, map[string]any{
				"role": "tool", "tool_call_id": block["tool_use_id"],
				"content": textContent(block["content"]),
			})
		}
	}
	flushUser()
	return result, nil
}

func anthropicContentToOpenAI(value any) (any, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	parts := make([]any, 0)
	for _, raw := range anySlice(value) {
		block, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		switch stringValue(block["type"]) {
		case "text":
			parts = append(parts, map[string]any{"type": "text", "text": block["text"]})
		case "image":
			part, err := anthropicImageToOpenAI(block)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
	}
	return parts, nil
}

func anthropicImageToOpenAI(block map[string]any) (map[string]any, error) {
	source, ok := block["source"].(map[string]any)
	if !ok {
		return nil, ErrInvalid
	}
	url := ""
	switch stringValue(source["type"]) {
	case "url":
		url = stringValue(source["url"])
	case "base64":
		mediaType, data := stringValue(source["media_type"]), stringValue(source["data"])
		if mediaType == "" || data == "" {
			return nil, ErrInvalid
		}
		url = "data:" + mediaType + ";base64," + data
	default:
		return nil, ErrInvalid
	}
	return map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}}, nil
}

func openAIChatRequestToAnthropic(body []byte) ([]byte, error) {
	source, err := decodeJSONObject(body)
	if err != nil {
		return nil, err
	}
	target := map[string]any{"model": source["model"], "max_tokens": firstValue(source, "max_completion_tokens", "max_tokens", json.Number("4096"))}
	copyFields(target, source, "temperature", "top_p", "stream")
	if stop, ok := source["stop"]; ok {
		target["stop_sequences"] = normalizeStrings(stop)
	}
	messages := make([]any, 0)
	system := make([]any, 0)
	for _, raw := range anySlice(source["messages"]) {
		message, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		role := stringValue(message["role"])
		if role == "system" || role == "developer" {
			system = append(system, map[string]any{"type": "text", "text": textContent(message["content"])})
			continue
		}
		if role == "tool" {
			messages = append(messages, map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "tool_result", "tool_use_id": message["tool_call_id"], "content": textContent(message["content"]),
			}}})
			continue
		}
		if role != "user" && role != "assistant" {
			return nil, ErrInvalid
		}
		content, convertErr := openAIContentToAnthropic(message["content"])
		if convertErr != nil {
			return nil, convertErr
		}
		if role == "assistant" {
			for _, rawCall := range anySlice(message["tool_calls"]) {
				call, ok := rawCall.(map[string]any)
				function, functionOK := call["function"].(map[string]any)
				if !ok || !functionOK {
					return nil, ErrInvalid
				}
				var input any
				if err := decodeJSONValue([]byte(stringValue(function["arguments"])), &input); err != nil {
					return nil, ErrInvalid
				}
				content = append(content, map[string]any{"type": "tool_use", "id": call["id"], "name": function["name"], "input": input})
			}
		}
		messages = append(messages, map[string]any{"role": role, "content": content})
	}
	if len(system) > 0 {
		target["system"] = system
	}
	if len(messages) == 0 {
		return nil, ErrInvalid
	}
	target["messages"] = messages
	convertOpenAITools(target, source)
	return encodeJSONObject(target)
}

func openAIResponsesRequestToAnthropic(body []byte) ([]byte, error) {
	source, err := decodeJSONObject(body)
	if err != nil {
		return nil, err
	}
	target := map[string]any{"model": source["model"], "max_tokens": firstValue(source, "max_output_tokens", "", json.Number("4096"))}
	copyFields(target, source, "temperature", "top_p", "stream")
	if instructions := stringValue(source["instructions"]); instructions != "" {
		target["system"] = instructions
	}
	messages := make([]any, 0)
	if input, ok := source["input"].(string); ok {
		messages = append(messages, map[string]any{"role": "user", "content": input})
	} else {
		for _, raw := range anySlice(source["input"]) {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrInvalid
			}
			switch stringValue(item["type"]) {
			case "message", "":
				role := stringValue(item["role"])
				if role == "developer" || role == "system" {
					target["system"] = textContent(item["content"])
					continue
				}
				content, convertErr := openAIContentToAnthropic(item["content"])
				if convertErr != nil {
					return nil, convertErr
				}
				messages = append(messages, map[string]any{"role": role, "content": content})
			case "function_call":
				var arguments any
				if err := decodeJSONValue([]byte(stringValue(item["arguments"])), &arguments); err != nil {
					return nil, ErrInvalid
				}
				messages = append(messages, map[string]any{"role": "assistant", "content": []any{map[string]any{
					"type": "tool_use", "id": item["call_id"], "name": item["name"], "input": arguments,
				}}})
			case "function_call_output":
				messages = append(messages, map[string]any{"role": "user", "content": []any{map[string]any{
					"type": "tool_result", "tool_use_id": item["call_id"], "content": textContent(item["output"]),
				}}})
			default:
				return nil, ErrInvalid
			}
		}
	}
	if len(messages) == 0 {
		return nil, ErrInvalid
	}
	target["messages"] = messages
	if tools := anySlice(source["tools"]); len(tools) > 0 {
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || stringValue(tool["type"]) != "function" {
				return nil, ErrInvalid
			}
			converted = append(converted, map[string]any{"name": tool["name"], "description": tool["description"], "input_schema": tool["parameters"]})
		}
		target["tools"] = converted
	}
	if choice, ok := source["tool_choice"]; ok {
		switch value := choice.(type) {
		case string:
			switch value {
			case "auto", "none":
				target["tool_choice"] = map[string]any{"type": value}
			case "required":
				target["tool_choice"] = map[string]any{"type": "any"}
			}
		case map[string]any:
			if stringValue(value["type"]) == "function" {
				target["tool_choice"] = map[string]any{"type": "tool", "name": value["name"]}
			}
		}
	}
	return encodeJSONObject(target)
}

func openAIContentToAnthropic(value any) ([]any, error) {
	if value == nil {
		return []any{}, nil
	}
	if text, ok := value.(string); ok {
		return []any{map[string]any{"type": "text", "text": text}}, nil
	}
	result := make([]any, 0)
	for _, raw := range anySlice(value) {
		part, ok := raw.(map[string]any)
		if !ok {
			return nil, ErrInvalid
		}
		switch stringValue(part["type"]) {
		case "text", "input_text", "output_text":
			result = append(result, map[string]any{"type": "text", "text": part["text"]})
		case "image_url", "input_image":
			url := stringValue(part["image_url"])
			if image, ok := part["image_url"].(map[string]any); ok {
				url = stringValue(image["url"])
			}
			if url == "" {
				url = stringValue(part["image_url"])
			}
			source := map[string]any{"type": "url", "url": url}
			if strings.HasPrefix(url, "data:") {
				media, data, ok := strings.Cut(strings.TrimPrefix(url, "data:"), ";base64,")
				if !ok || media == "" {
					return nil, ErrInvalid
				}
				if _, err := base64.StdEncoding.DecodeString(data); err != nil {
					return nil, ErrInvalid
				}
				source = map[string]any{"type": "base64", "media_type": media, "data": data}
			}
			result = append(result, map[string]any{"type": "image", "source": source})
		default:
			return nil, ErrInvalid
		}
	}
	return result, nil
}

func convertOpenAITools(target, source map[string]any) {
	tools := anySlice(source["tools"])
	if len(tools) > 0 {
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			function, _ := tool["function"].(map[string]any)
			if stringValue(tool["type"]) != "function" || function == nil {
				continue
			}
			converted = append(converted, map[string]any{"name": function["name"], "description": function["description"], "input_schema": function["parameters"]})
		}
		if len(converted) > 0 {
			target["tools"] = converted
		}
	}
	choice := source["tool_choice"]
	switch value := choice.(type) {
	case string:
		if value == "required" {
			target["tool_choice"] = map[string]any{"type": "any"}
		} else if value == "auto" || value == "none" {
			target["tool_choice"] = map[string]any{"type": value}
		}
	case map[string]any:
		if function, ok := value["function"].(map[string]any); ok {
			target["tool_choice"] = map[string]any{"type": "tool", "name": function["name"]}
		}
	}
}

func decodeJSONValue(body []byte, target *any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

func copyFields(target, source map[string]any, keys ...string) {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			target[key] = value
		}
	}
}

func anySlice(value any) []any {
	result, _ := value.([]any)
	return result
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func textContent(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	var builder strings.Builder
	for _, raw := range anySlice(value) {
		part, _ := raw.(map[string]any)
		if text := stringValue(part["text"]); text != "" {
			builder.WriteString(text)
		}
	}
	return builder.String()
}

func normalizeStrings(value any) []string {
	if text, ok := value.(string); ok {
		return []string{text}
	}
	result := make([]string, 0)
	for _, item := range anySlice(value) {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func firstValue(source map[string]any, first, second string, fallback any) any {
	if value, ok := source[first]; ok && first != "" {
		return value
	}
	if value, ok := source[second]; ok && second != "" {
		return value
	}
	return fallback
}
