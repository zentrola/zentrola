package gateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type sseEvent struct {
	event string
	data  string
}

func scanSSE(reader io.Reader, consume func(sseEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	current := sseEvent{}
	flush := func() error {
		if current.event == "" && current.data == "" {
			return nil
		}
		err := consume(current)
		current = sseEvent{}
		return err
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			current.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			if current.data != "" {
				current.data += "\n"
			}
			current.data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func writeSSE(writer io.Writer, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return ErrUpstream
	}
	if event != "" {
		if _, err = fmt.Fprintf(writer, "event: %s\n", event); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(writer, "data: %s\n\n", data)
	return err
}

func writeSSEData(writer io.Writer, data string) error {
	_, err := fmt.Fprintf(writer, "data: %s\n\n", data)
	return err
}

type anthropicStreamState struct {
	started      bool
	stopped      bool
	done         bool
	failed       bool
	id           string
	model        any
	nextBlock    int
	textBlock    int
	textOpen     bool
	toolBlocks   map[int]int
	openedBlocks []int
	usage        openAIUsageCounts
	stopReason   any
}

func openAIStreamToAnthropic(reader io.Reader, writer io.Writer) error {
	state := &anthropicStreamState{textBlock: -1, toolBlocks: map[int]int{}}
	start := func(source map[string]any) error {
		if state.started {
			return nil
		}
		state.started = true
		state.id = fallbackString(stringValue(source["id"]), "msg_compat")
		state.model = source["model"]
		if usage, ok := source["usage"].(map[string]any); ok {
			state.usage = newOpenAIUsageCounts(usage)
		}
		usage := state.usage.anthropicUsage()
		usage["output_tokens"] = 0
		return writeSSE(writer, "message_start", map[string]any{"type": "message_start", "message": map[string]any{
			"id": state.id, "type": "message", "role": "assistant", "model": state.model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": usage,
		}})
	}
	finish := func() error {
		if state.stopped {
			return nil
		}
		if !state.started {
			if err := start(map[string]any{}); err != nil {
				return err
			}
		}
		for _, index := range state.openedBlocks {
			if err := writeSSE(writer, "content_block_stop", map[string]any{"type": "content_block_stop", "index": index}); err != nil {
				return err
			}
		}
		if err := writeSSE(writer, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{
			"stop_reason": state.stopReason, "stop_sequence": nil,
		}, "usage": state.usage.anthropicUsage()}); err != nil {
			return err
		}
		state.stopped = true
		return writeSSE(writer, "message_stop", map[string]any{"type": "message_stop"})
	}
	err := scanSSE(reader, func(event sseEvent) error {
		if event.data == "[DONE]" {
			state.done = true
			if state.failed {
				return nil
			}
			return finish()
		}
		source, err := decodeJSONObject([]byte(event.data))
		if err != nil {
			return ErrUpstream
		}
		if upstreamError, ok := source["error"].(map[string]any); ok {
			state.failed = true
			return writeSSE(writer, "error", map[string]any{"type": "error", "error": map[string]any{
				"type":    openAIErrorToAnthropic(stringValue(upstreamError["type"]), stringValue(upstreamError["code"])),
				"message": fallbackString(stringValue(upstreamError["message"]), "Upstream request failed."),
			}})
		}
		if err := start(source); err != nil {
			return err
		}
		if usage, ok := source["usage"].(map[string]any); ok {
			state.usage = newOpenAIUsageCounts(usage)
		}
		choices := anySlice(source["choices"])
		if len(choices) == 0 {
			return nil
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if text := stringValue(delta["content"]); text != "" {
			if !state.textOpen {
				state.textBlock = state.nextBlock
				state.nextBlock++
				state.textOpen = true
				state.openedBlocks = append(state.openedBlocks, state.textBlock)
				if err := writeSSE(writer, "content_block_start", map[string]any{"type": "content_block_start", "index": state.textBlock, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
					return err
				}
			}
			if err := writeSSE(writer, "content_block_delta", map[string]any{"type": "content_block_delta", "index": state.textBlock, "delta": map[string]any{"type": "text_delta", "text": text}}); err != nil {
				return err
			}
		}
		for _, raw := range anySlice(delta["tool_calls"]) {
			call, _ := raw.(map[string]any)
			toolIndex := intNumber(call["index"])
			blockIndex, exists := state.toolBlocks[toolIndex]
			function, _ := call["function"].(map[string]any)
			if !exists {
				blockIndex = state.nextBlock
				state.nextBlock++
				state.toolBlocks[toolIndex] = blockIndex
				state.openedBlocks = append(state.openedBlocks, blockIndex)
				if err := writeSSE(writer, "content_block_start", map[string]any{"type": "content_block_start", "index": blockIndex, "content_block": map[string]any{
					"type": "tool_use", "id": fallbackString(stringValue(call["id"]), fmt.Sprintf("call_%d", toolIndex)), "name": function["name"], "input": map[string]any{},
				}}); err != nil {
					return err
				}
			}
			if arguments := stringValue(function["arguments"]); arguments != "" {
				if err := writeSSE(writer, "content_block_delta", map[string]any{"type": "content_block_delta", "index": blockIndex, "delta": map[string]any{"type": "input_json_delta", "partial_json": arguments}}); err != nil {
					return err
				}
			}
		}
		if reason := stringValue(choice["finish_reason"]); reason != "" {
			state.stopReason = openAIFinishToAnthropic(reason)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if state.failed || state.stopped {
		// 上游错误已写出 Anthropic error 事件，不能再合成正常结束。
		return nil
	}
	if !state.done && state.stopReason == nil {
		// 上游在 [DONE] 与 finish_reason 之前提前结束：写出错误事件而不是伪造
		// message_stop，避免客户端与计量把截断响应当作完整结果。
		return writeSSE(writer, "error", map[string]any{"type": "error", "error": map[string]any{
			"type": "api_error", "message": "The upstream model provider ended the stream before completion.",
		}})
	}
	return finish()
}

type openAIStreamState struct {
	id, model     string
	created       int64
	usage         anthropicUsageCounts
	stopReason    any
	toolCalls     map[int]map[string]any
	toolArguments map[int]*strings.Builder
	textByBlock   map[int]*strings.Builder
	sequence      int
}

func newOpenAIStreamState() *openAIStreamState {
	return &openAIStreamState{
		created:   time.Now().Unix(),
		toolCalls: map[int]map[string]any{}, toolArguments: map[int]*strings.Builder{}, textByBlock: map[int]*strings.Builder{},
	}
}

func anthropicStreamToOpenAIChat(reader io.Reader, writer io.Writer, sink *CompatibilityUsage) error {
	state := newOpenAIStreamState()
	return scanSSE(reader, func(event sseEvent) error {
		if event.data == "" {
			return nil
		}
		source, err := decodeJSONObject([]byte(event.data))
		if err != nil {
			return ErrUpstream
		}
		typeName := fallbackString(stringValue(source["type"]), event.event)
		if typeName == "error" {
			encoded, _ := json.Marshal(anthropicErrorToOpenAI(source))
			return writeSSEData(writer, string(encoded))
		}
		switch typeName {
		case "message_start":
			message, _ := source["message"].(map[string]any)
			state.id, state.model = openAIID(message["id"], "chatcmpl"), stringValue(message["model"])
			state.usage.apply(message["usage"])
			sink.record(state.usage)
			return writeOpenAIChatChunk(writer, state, map[string]any{"role": "assistant", "content": ""}, nil, nil)
		case "content_block_start":
			index := intNumber(source["index"])
			block, _ := source["content_block"].(map[string]any)
			if stringValue(block["type"]) == "tool_use" {
				state.toolCalls[index] = block
				delta := map[string]any{"tool_calls": []any{map[string]any{
					"index": index, "id": block["id"], "type": "function", "function": map[string]any{"name": block["name"], "arguments": ""},
				}}}
				return writeOpenAIChatChunk(writer, state, delta, nil, nil)
			}
		case "content_block_delta":
			index := intNumber(source["index"])
			delta, _ := source["delta"].(map[string]any)
			switch stringValue(delta["type"]) {
			case "text_delta":
				return writeOpenAIChatChunk(writer, state, map[string]any{"content": delta["text"]}, nil, nil)
			case "input_json_delta":
				return writeOpenAIChatChunk(writer, state, map[string]any{"tool_calls": []any{map[string]any{
					"index": index, "function": map[string]any{"arguments": delta["partial_json"]},
				}}}, nil, nil)
			}
		case "message_delta":
			delta, _ := source["delta"].(map[string]any)
			state.stopReason = anthropicStopToOpenAI(stringValue(delta["stop_reason"]))
			state.usage.apply(source["usage"])
			sink.record(state.usage)
			return writeOpenAIChatChunk(writer, state, map[string]any{}, state.stopReason, nil)
		case "message_stop":
			if err := writeOpenAIChatChunk(writer, state, map[string]any{}, nil, state.usage.chatUsage()); err != nil {
				return err
			}
			return writeSSEData(writer, "[DONE]")
		}
		return nil
	})
}

func writeOpenAIChatChunk(writer io.Writer, state *openAIStreamState, delta map[string]any, finish, usage any) error {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": finish}
	chunk := map[string]any{"id": fallbackString(state.id, "chatcmpl_compat"), "object": "chat.completion.chunk", "created": state.created, "model": state.model, "choices": []any{choice}}
	if usage != nil {
		chunk["usage"] = usage
		chunk["choices"] = []any{}
	}
	return writeSSEData(writer, mustJSON(chunk))
}

func anthropicStreamToOpenAIResponses(reader io.Reader, writer io.Writer, sink *CompatibilityUsage) error {
	state := newOpenAIStreamState()
	responseID := "resp_compat"
	return scanSSE(reader, func(event sseEvent) error {
		if event.data == "" {
			return nil
		}
		source, err := decodeJSONObject([]byte(event.data))
		if err != nil {
			return ErrUpstream
		}
		typeName := fallbackString(stringValue(source["type"]), event.event)
		emit := func(name string, value map[string]any) error {
			state.sequence++
			value["type"] = name
			value["sequence_number"] = state.sequence
			return writeSSE(writer, name, value)
		}
		switch typeName {
		case "error":
			converted := anthropicErrorToOpenAI(source)
			return emit("error", converted)
		case "message_start":
			message, _ := source["message"].(map[string]any)
			responseID, state.model = openAIID(message["id"], "resp"), stringValue(message["model"])
			state.usage.apply(message["usage"])
			sink.record(state.usage)
			return emit("response.created", map[string]any{"response": responseEnvelope(responseID, state.model, "in_progress", nil, state)})
		case "content_block_start":
			index := intNumber(source["index"])
			block, _ := source["content_block"].(map[string]any)
			if stringValue(block["type"]) == "tool_use" {
				state.toolCalls[index] = block
				state.toolArguments[index] = &strings.Builder{}
				return emit("response.output_item.added", map[string]any{"output_index": index, "item": map[string]any{
					"type": "function_call", "id": openAIID(block["id"], "fc"), "call_id": block["id"], "name": block["name"], "arguments": "", "status": "in_progress",
				}})
			}
			state.textByBlock[index] = &strings.Builder{}
			itemID := responseStreamItemID(responseID, "msg", index)
			if err := emit("response.output_item.added", map[string]any{"output_index": index, "item": map[string]any{
				"type": "message", "id": itemID, "role": "assistant", "status": "in_progress", "content": []any{},
			}}); err != nil {
				return err
			}
			return emit("response.content_part.added", map[string]any{"item_id": itemID, "output_index": index, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
		case "content_block_delta":
			index := intNumber(source["index"])
			delta, _ := source["delta"].(map[string]any)
			if stringValue(delta["type"]) == "input_json_delta" {
				if arguments := state.toolArguments[index]; arguments != nil {
					arguments.WriteString(stringValue(delta["partial_json"]))
				}
				return emit("response.function_call_arguments.delta", map[string]any{"item_id": openAIID(state.toolCalls[index]["id"], "fc"), "output_index": index, "delta": delta["partial_json"]})
			}
			if builder := state.textByBlock[index]; builder != nil {
				builder.WriteString(stringValue(delta["text"]))
			}
			return emit("response.output_text.delta", map[string]any{"item_id": responseStreamItemID(responseID, "msg", index), "output_index": index, "content_index": 0, "delta": delta["text"]})
		case "content_block_stop":
			index := intNumber(source["index"])
			if tool := state.toolCalls[index]; tool != nil {
				arguments := state.toolArguments[index].String()
				itemID := openAIID(tool["id"], "fc")
				if err := emit("response.function_call_arguments.done", map[string]any{"item_id": itemID, "output_index": index, "arguments": arguments}); err != nil {
					return err
				}
				return emit("response.output_item.done", map[string]any{"output_index": index, "item": responseToolItem(tool, arguments, "completed")})
			}
			if text := state.textByBlock[index]; text != nil {
				itemID := responseStreamItemID(responseID, "msg", index)
				if err := emit("response.output_text.done", map[string]any{"item_id": itemID, "output_index": index, "content_index": 0, "text": text.String()}); err != nil {
					return err
				}
				if err := emit("response.content_part.done", map[string]any{"item_id": itemID, "output_index": index, "content_index": 0, "part": map[string]any{"type": "output_text", "text": text.String(), "annotations": []any{}}}); err != nil {
					return err
				}
				return emit("response.output_item.done", map[string]any{"output_index": index, "item": responseTextItem(responseID, index, text.String(), "completed")})
			}
		case "message_delta":
			state.usage.apply(source["usage"])
			sink.record(state.usage)
		case "message_stop":
			return emit("response.completed", map[string]any{"response": responseEnvelope(responseID, state.model, "completed", responseStreamOutput(responseID, state), state)})
		}
		return nil
	})
}

func responseStreamOutput(responseID string, state *openAIStreamState) []any {
	maxIndex := -1
	for index := range state.textByBlock {
		maxIndex = max(maxIndex, index)
	}
	for index := range state.toolCalls {
		maxIndex = max(maxIndex, index)
	}
	output := make([]any, 0, maxIndex+1)
	for index := 0; index <= maxIndex; index++ {
		if text := state.textByBlock[index]; text != nil {
			output = append(output, responseTextItem(responseID, index, text.String(), "completed"))
		} else if tool := state.toolCalls[index]; tool != nil {
			arguments := ""
			if builder := state.toolArguments[index]; builder != nil {
				arguments = builder.String()
			}
			output = append(output, responseToolItem(tool, arguments, "completed"))
		}
	}
	return output
}

func responseTextItem(responseID string, index int, text, status string) map[string]any {
	return map[string]any{
		"type": "message", "id": responseStreamItemID(responseID, "msg", index), "role": "assistant", "status": status,
		"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}},
	}
}

func responseToolItem(tool map[string]any, arguments, status string) map[string]any {
	return map[string]any{
		"type": "function_call", "id": openAIID(tool["id"], "fc"), "call_id": tool["id"],
		"name": tool["name"], "arguments": arguments, "status": status,
	}
}

func responseStreamItemID(responseID, prefix string, index int) string {
	return fmt.Sprintf("%s_%s_%d", prefix, strings.TrimPrefix(responseID, "resp_"), index)
}

func responseEnvelope(id, model, status string, output any, state *openAIStreamState) map[string]any {
	if output == nil {
		output = []any{}
	}
	return map[string]any{
		"id": id, "object": "response", "created_at": state.created, "status": status, "model": model, "output": output,
		"usage": state.usage.responsesUsage(),
	}
}

func intNumber(value any) int {
	number, ok := int64Value(value)
	if !ok || number < math.MinInt32 || number > math.MaxInt32 {
		return 0
	}
	return int(number)
}

func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
