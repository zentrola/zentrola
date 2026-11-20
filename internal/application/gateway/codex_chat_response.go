package gateway

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxCodexChatOutput = 8 << 20

type codexChatCall struct {
	id, name, arguments string
	index               int
	announced           bool
	emittedArguments    int
}

type codexChatResult struct {
	id, model   string
	created     int64
	text        []byte
	textSeen    map[int]bool
	calls       []*codexChatCall
	byOutput    map[int]*codexChatCall
	usage       map[string]any
	complete    bool
	started     bool
	outputBytes int
}

func codexResponsesToChat(ctx context.Context, response *Response, clientStream, includeUsage bool, model string) (*Response, error) {
	if response == nil || response.Body == nil {
		return nil, ErrUpstream
	}
	switch encoding := strings.ToLower(strings.TrimSpace(http.Header(response.Headers).Get("Content-Encoding"))); encoding {
	case "", "identity":
	case "gzip", "x-gzip":
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			response.Body.Close()
			return nil, ErrUpstream
		}
		response.Body = &codexGzipBody{Reader: reader, source: response.Body}
	default:
		response.Body.Close()
		return nil, ErrUpstream
	}
	if response.Status >= 400 {
		headers := cloneHeaders(response.Headers)
		headers.Del("Content-Length")
		headers.Del("Content-Encoding")
		headers.Set("Content-Type", "application/json")
		return &Response{Status: response.Status, Headers: headers,
			Body: newConvertedBody(response.Body, compatibilityErrorTransform(OpenAIProtocol, response.Status))}, nil
	}
	headers := cloneHeaders(response.Headers)
	headers.Del("Content-Length")
	headers.Del("Content-Encoding")
	result := &Response{Status: response.Status, Headers: headers}
	if clientStream {
		headers.Set("Content-Type", "text/event-stream")
		result.Body = newConvertedBody(response.Body, func(reader io.Reader, writer io.Writer) error {
			return convertCodexChatSSE(reader, writer, model, true, includeUsage)
		})
		return result, nil
	}
	// The subscription upstream is SSE-only. Consume it before opening a JSON
	// response so an incomplete stream can still be reported as an upstream error.
	var output bytes.Buffer
	if err := convertCodexChatSSE(response.Body, &output, model, false, false); err != nil {
		response.Body.Close()
		if ctx.Err() == context.Canceled {
			return nil, ErrCancelled
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, ErrTimeout
		}
		return nil, ErrUpstream
	}
	response.Body.Close()
	// The converter writes its final Chat JSON for non-streaming clients.
	result.Body = io.NopCloser(bytes.NewReader(output.Bytes()))
	headers.Set("Content-Type", "application/json")
	return result, nil
}

type codexGzipBody struct {
	*gzip.Reader
	source io.ReadCloser
}

func (b *codexGzipBody) Close() error {
	_ = b.Reader.Close()
	return b.source.Close()
}

func convertCodexChatSSE(reader io.Reader, writer io.Writer, model string, stream, includeUsage bool) error {
	state := &codexChatResult{model: model, created: time.Now().Unix(), byOutput: map[int]*codexChatCall{}, textSeen: map[int]bool{}}
	err := scanSSELimit(reader, 16<<20, func(event sseEvent) error {
		if event.data == "" || event.data == "[DONE]" {
			return nil
		}
		source, err := decodeJSONObject([]byte(event.data))
		if err != nil {
			return ErrUpstream
		}
		kind := event.event
		if kind == "" {
			kind = stringValue(source["type"])
		}
		if kind == "response.failed" || kind == "error" {
			return ErrUpstream
		}
		if err := state.consume(kind, source, writer, stream, includeUsage); err != nil {
			return err
		}
		return nil
	})
	if err != nil || !state.complete {
		return ErrUpstream
	}
	if !stream {
		body, err := state.chatJSON()
		if err != nil {
			return err
		}
		_, err = writer.Write(body)
		return err
	}
	return writeSSEData(writer, "[DONE]")
}

func (s *codexChatResult) consume(kind string, source map[string]any, writer io.Writer, stream, includeUsage bool) error {
	switch kind {
	case "response.created", "response.in_progress":
		if response, ok := source["response"].(map[string]any); ok {
			s.metadata(response)
		}
		if stream {
			return s.start(writer)
		}
	case "response.output_text.delta":
		delta := stringValue(source["delta"])
		if !s.addOutput(len(delta)) {
			return ErrUpstream
		}
		s.text = append(s.text, delta...)
		s.textSeen[intNumber(source["output_index"])] = true
		if stream && delta != "" {
			if err := s.start(writer); err != nil {
				return err
			}
			return s.chunk(writer, map[string]any{"content": delta}, nil)
		}
	case "response.output_item.added", "response.output_item.done":
		item, ok := source["item"].(map[string]any)
		if !ok || stringValue(item["type"]) != "function_call" {
			return nil
		}
		index := intNumber(source["output_index"])
		call := s.call(index)
		call.id = fallbackString(stringValue(item["call_id"]), call.id)
		call.name = fallbackString(stringValue(item["name"]), call.name)
		if stream {
			if err := s.start(writer); err != nil {
				return err
			}
			if !call.announced {
				call.announced = true
				if err := s.chunk(writer, map[string]any{"tool_calls": []any{map[string]any{
					"index": call.index, "id": call.id, "type": "function",
					"function": map[string]any{"name": call.name, "arguments": ""},
				}}}, nil); err != nil {
					return err
				}
			}
		}
		if kind == "response.output_item.done" {
			return s.finishCall(writer, call, stringValue(item["arguments"]), stream)
		}
	case "response.function_call_arguments.delta":
		index := intNumber(source["output_index"])
		call := s.call(index)
		delta := stringValue(source["delta"])
		if !s.addOutput(len(delta)) {
			return ErrUpstream
		}
		call.arguments += delta
		if stream && delta != "" && call.announced {
			if err := s.start(writer); err != nil {
				return err
			}
			if err := s.chunk(writer, map[string]any{"tool_calls": []any{map[string]any{
				"index": call.index, "function": map[string]any{"arguments": delta},
			}}}, nil); err != nil {
				return err
			}
			call.emittedArguments += len(delta)
		}
	case "response.completed":
		response, ok := source["response"].(map[string]any)
		if !ok {
			return ErrUpstream
		}
		s.metadata(response)
		if err := s.completedOutput(response, writer, stream); err != nil {
			return err
		}
		for _, call := range s.calls {
			if call.id == "" || call.name == "" || !json.Valid([]byte(call.arguments)) {
				return ErrUpstream
			}
		}
		s.usage = codexChatUsage(response["usage"])
		s.complete = true
		if stream {
			if err := s.start(writer); err != nil {
				return err
			}
			finish := "stop"
			if len(s.calls) > 0 {
				finish = "tool_calls"
			}
			if err := s.chunk(writer, map[string]any{}, finish); err != nil {
				return err
			}
			if includeUsage && s.usage != nil {
				return s.usageChunk(writer)
			}
		}
	}
	return nil
}

func (s *codexChatResult) metadata(response map[string]any) {
	s.id = fallbackString(stringValue(response["id"]), s.id)
	s.model = fallbackString(stringValue(response["model"]), s.model)
	if created, ok := response["created_at"].(json.Number); ok {
		if value, err := created.Int64(); err == nil && value > 0 {
			s.created = value
		}
	}
}

func (s *codexChatResult) call(outputIndex int) *codexChatCall {
	if call := s.byOutput[outputIndex]; call != nil {
		return call
	}
	call := &codexChatCall{index: len(s.calls)}
	s.calls = append(s.calls, call)
	s.byOutput[outputIndex] = call
	return call
}

func (s *codexChatResult) completedOutput(response map[string]any, writer io.Writer, stream bool) error {
	for index, raw := range anySlice(response["output"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			return ErrUpstream
		}
		switch stringValue(item["type"]) {
		case "message":
			if s.textSeen[index] {
				continue
			}
			for _, rawPart := range anySlice(item["content"]) {
				part, ok := rawPart.(map[string]any)
				if !ok || stringValue(part["type"]) != "output_text" {
					return ErrUpstream
				}
				text, ok := part["text"].(string)
				if !ok {
					return ErrUpstream
				}
				if !s.addOutput(len(text)) {
					return ErrUpstream
				}
				s.text = append(s.text, text...)
				if stream && text != "" {
					if err := s.chunk(writer, map[string]any{"content": text}, nil); err != nil {
						return err
					}
				}
			}
		case "function_call":
			call := s.call(index)
			call.id = fallbackString(stringValue(item["call_id"]), call.id)
			call.name = fallbackString(stringValue(item["name"]), call.name)
			if stream && !call.announced {
				if err := s.chunk(writer, map[string]any{"tool_calls": []any{map[string]any{
					"index": call.index, "id": call.id, "type": "function",
					"function": map[string]any{"name": call.name, "arguments": ""},
				}}}, nil); err != nil {
					return err
				}
				call.announced = true
			}
			if err := s.finishCall(writer, call, stringValue(item["arguments"]), stream); err != nil {
				return err
			}
		case "reasoning":
			// Reasoning is not part of a Chat Completions assistant message.
		default:
			return ErrUpstream
		}
	}
	return nil
}

func (s *codexChatResult) finishCall(writer io.Writer, call *codexChatCall, full string, stream bool) error {
	if full != "" {
		if !strings.HasPrefix(full, call.arguments) || !s.addOutput(len(full)-len(call.arguments)) {
			return ErrUpstream
		}
		call.arguments = full
	}
	if !stream || call.emittedArguments == len(call.arguments) {
		return nil
	}
	delta := call.arguments[call.emittedArguments:]
	if err := s.chunk(writer, map[string]any{"tool_calls": []any{map[string]any{
		"index": call.index, "function": map[string]any{"arguments": delta},
	}}}, nil); err != nil {
		return err
	}
	call.emittedArguments = len(call.arguments)
	return nil
}

func (s *codexChatResult) addOutput(size int) bool {
	if size < 0 || size > maxCodexChatOutput-s.outputBytes {
		return false
	}
	s.outputBytes += size
	return true
}

func (s *codexChatResult) chatID() string { return "chatcmpl_" + fallbackString(s.id, "compat") }

func (s *codexChatResult) start(writer io.Writer) error {
	if s.started {
		return nil
	}
	s.started = true
	return s.chunk(writer, map[string]any{"role": "assistant"}, nil)
}

func (s *codexChatResult) chunk(writer io.Writer, delta map[string]any, finish any) error {
	return writeSSE(writer, "", map[string]any{
		"id": s.chatID(), "object": "chat.completion.chunk", "created": s.created,
		"model": s.model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
	})
}

func (s *codexChatResult) usageChunk(writer io.Writer) error {
	return writeSSE(writer, "", map[string]any{
		"id": s.chatID(), "object": "chat.completion.chunk", "created": s.created,
		"model": s.model, "choices": []any{}, "usage": s.usage,
	})
}

func (s *codexChatResult) chatJSON() ([]byte, error) {
	message := map[string]any{"role": "assistant", "content": string(s.text)}
	finish := "stop"
	if len(s.calls) > 0 {
		finish = "tool_calls"
		if len(s.text) == 0 {
			message["content"] = nil
		}
		calls := make([]any, 0, len(s.calls))
		for _, call := range s.calls {
			calls = append(calls, map[string]any{"id": call.id, "type": "function",
				"function": map[string]any{"name": call.name, "arguments": call.arguments}})
		}
		message["tool_calls"] = calls
	}
	return encodeJSONObject(map[string]any{
		"id": s.chatID(), "object": "chat.completion", "created": s.created,
		"model": s.model, "choices": []any{map[string]any{
			"index": 0, "message": message, "finish_reason": finish,
		}}, "usage": s.usage,
	})
}

func codexChatUsage(value any) map[string]any {
	source, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	input, inputOK := usageCount(source, "input_tokens")
	output, outputOK := usageCount(source, "output_tokens")
	if !inputOK || !outputOK {
		return nil
	}
	usage := map[string]any{"prompt_tokens": input, "completion_tokens": output,
		"total_tokens": addCounts(input, output)}
	if details, ok := source["input_tokens_details"].(map[string]any); ok {
		if cached, ok := usageCount(details, "cached_tokens"); ok {
			usage["prompt_tokens_details"] = map[string]any{"cached_tokens": cached}
		}
	}
	return usage
}
