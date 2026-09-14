package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type compatibilityUpstreamFunc func(context.Context, Route, Request, []byte) (*Response, error)

func (f compatibilityUpstreamFunc) Open(ctx context.Context, route Route, request Request, credential []byte) (*Response, error) {
	return f(ctx, route, request, credential)
}

func TestAnthropicFallsBackToOpenAIAndReturnsAnthropic(t *testing.T) {
	openAI := compatibilityUpstreamFunc(func(_ context.Context, _ Route, request Request, _ []byte) (*Response, error) {
		if request.Protocol != OpenAIProtocol || request.Path != "/v1/chat/completions" {
			t.Fatalf("request was not converted to OpenAI: %+v", request)
		}
		var body map[string]any
		if err := json.Unmarshal(request.Body, &body); err != nil {
			t.Fatal(err)
		}
		messages := body["messages"].([]any)
		if body["model"] != "upstream-model" || messages[0].(map[string]any)["role"] != "user" {
			t.Fatalf("unexpected converted request: %s", request.Body)
		}
		response := `{"id":"chatcmpl_1","model":"upstream-model","choices":[{"message":{"role":"assistant","content":"hello","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"line\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`
		converted := compatibilityResponse(http.StatusOK, "application/json", response)
		converted.Headers["X-Ratelimit-Remaining-Requests"] = []string{"9"}
		return converted, nil
	})
	upstream := NewCompatibleUpstream(nil, openAI)
	request := Request{Protocol: AnthropicProtocol, Path: "/v1/messages", Body: []byte(`{"model":"upstream-model","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`)}
	response, err := upstream.Open(context.Background(), Route{EndpointProtocol: OpenAIEndpoint}, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	result, err := decodeJSONObject(data)
	if err != nil {
		t.Fatal(err)
	}
	content := result["content"].([]any)
	tool := content[1].(map[string]any)
	line := tool["input"].(map[string]any)["line"]
	headers := http.Header(response.Headers)
	if result["type"] != "message" || result["stop_reason"] != "tool_use" || line != json.Number("9007199254740993") || headers.Get("Anthropic-Ratelimit-Requests-Remaining") != "9" || headers.Get("X-Ratelimit-Remaining-Requests") != "" {
		t.Fatalf("response was not converted to Anthropic: %s", data)
	}
}

func TestOpenAIChatFallsBackToAnthropicAndReturnsOpenAI(t *testing.T) {
	anthropic := compatibilityUpstreamFunc(func(_ context.Context, _ Route, request Request, _ []byte) (*Response, error) {
		if request.Protocol != AnthropicProtocol || request.Path != "/v1/messages" {
			t.Fatalf("request was not converted to Anthropic: %+v", request)
		}
		body := string(request.Body)
		for _, expected := range []string{`"max_tokens":128`, `"type":"tool_use"`, `"tool_use_id":"call_1"`} {
			if !strings.Contains(body, expected) {
				t.Fatalf("converted request missing %s: %s", expected, body)
			}
		}
		response := `{"id":"msg_1","type":"message","role":"assistant","model":"upstream-model","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":2}}`
		return compatibilityResponse(http.StatusOK, "application/json", response), nil
	})
	upstream := NewCompatibleUpstream(anthropic, nil)
	request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"upstream-model","max_tokens":128,"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"path\":\"a\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"ok"}]}`)}
	response, err := upstream.Open(context.Background(), Route{EndpointProtocol: AnthropicEndpoint}, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !strings.Contains(string(data), `"object":"chat.completion"`) || !strings.Contains(string(data), `"finish_reason":"stop"`) {
		t.Fatalf("response was not converted to OpenAI Chat: %s, %v", data, err)
	}
}

func TestOpenAIResponsesFallsBackToAnthropicAndReturnsResponses(t *testing.T) {
	anthropic := compatibilityUpstreamFunc(func(_ context.Context, _ Route, request Request, _ []byte) (*Response, error) {
		if !strings.Contains(string(request.Body), `"content":"hello"`) {
			t.Fatalf("Responses input was not converted: %s", request.Body)
		}
		response := `{"id":"msg_2","type":"message","role":"assistant","model":"upstream-model","content":[{"type":"text","text":"world"},{"type":"tool_use","id":"call_2","name":"lookup","input":{"q":"x"}}],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":6}}`
		return compatibilityResponse(http.StatusOK, "application/json", response), nil
	})
	upstream := NewCompatibleUpstream(anthropic, nil)
	request := Request{Protocol: OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"upstream-model","input":"hello","max_output_tokens":64}`)}
	response, err := upstream.Open(context.Background(), Route{EndpointProtocol: AnthropicEndpoint}, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !strings.Contains(string(data), `"object":"response"`) || !strings.Contains(string(data), `"type":"function_call"`) {
		t.Fatalf("response was not converted to Responses: %s, %v", data, err)
	}
}

func TestOpenAIImagesOnlyUsesOpenAIEndpoint(t *testing.T) {
	body := []byte(`{"model":"gpt-image","prompt":"draw an otter","future":{"value":9007199254740993}}`)
	openAICalls := 0
	openAI := upstreamFunc(func(_ context.Context, route Route, request Request, _ []byte) (*Response, error) {
		openAICalls++
		if route.EndpointProtocol != OpenAIEndpoint || request.Protocol != OpenAIImagesProtocol || request.Path != "/v1/images/generations" || string(request.Body) != string(body) {
			t.Fatalf("Images request was not passed through unchanged: route=%+v request=%+v", route, request)
		}
		return compatibilityResponse(http.StatusOK, "application/json", `{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`), nil
	})
	upstream := NewCompatibleUpstream(nil, openAI)
	response, err := upstream.Open(context.Background(), Route{EndpointProtocol: OpenAIEndpoint}, Request{
		Protocol: OpenAIImagesProtocol, Path: "/v1/images/generations", Body: body,
	}, nil)
	if err != nil || response.Status != http.StatusOK || openAICalls != 1 {
		t.Fatalf("Images pass-through failed: response=%+v calls=%d err=%v", response, openAICalls, err)
	}
	response.Body.Close()

	anthropicCalls := 0
	upstream = NewCompatibleUpstream(upstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		anthropicCalls++
		return nil, nil
	}), openAI)
	_, err = upstream.Open(context.Background(), Route{EndpointProtocol: AnthropicEndpoint}, Request{
		Protocol: OpenAIImagesProtocol, Path: "/v1/images/generations", Body: body,
	}, nil)
	if !errors.Is(err, ErrRoute) || anthropicCalls != 0 {
		t.Fatalf("Images request reached Anthropic endpoint: calls=%d err=%v", anthropicCalls, err)
	}
}

func TestCompatibilityConvertsStreamingResponses(t *testing.T) {
	t.Run("OpenAI to Anthropic", func(t *testing.T) {
		openAI := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
			stream := "data: {\"id\":\"chatcmpl_1\",\"model\":\"m\",\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hel\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"id\":\"chatcmpl_1\",\"model\":\"m\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\n" +
				"data: [DONE]\n\n"
			return compatibilityResponse(http.StatusOK, "text/event-stream", stream), nil
		})
		upstream := NewCompatibleUpstream(nil, openAI)
		response, err := upstream.Open(context.Background(), Route{EndpointProtocol: OpenAIEndpoint}, Request{Protocol: AnthropicProtocol, Path: "/v1/messages", Body: []byte(`{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || !strings.Contains(string(data), "event: message_start") || !strings.Contains(string(data), `"text":"lo"`) || !strings.Contains(string(data), "event: message_stop") {
			t.Fatalf("invalid Anthropic stream: %s, %v", data, err)
		}
	})

	t.Run("Anthropic to OpenAI", func(t *testing.T) {
		anthropic := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
			stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"m\",\"usage\":{\"input_tokens\":2}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			return compatibilityResponse(http.StatusOK, "text/event-stream", stream), nil
		})
		upstream := NewCompatibleUpstream(anthropic, nil)
		response, err := upstream.Open(context.Background(), Route{EndpointProtocol: AnthropicEndpoint}, Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || !strings.Contains(string(data), `"object":"chat.completion.chunk"`) || !strings.Contains(string(data), `"content":"hello"`) || !strings.Contains(string(data), "data: [DONE]") {
			t.Fatalf("invalid OpenAI stream: %s, %v", data, err)
		}
	})

	t.Run("Anthropic to OpenAI Responses", func(t *testing.T) {
		anthropic := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
			stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_2\",\"model\":\"m\",\"usage\":{\"input_tokens\":3}}}\n\n" +
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
			return compatibilityResponse(http.StatusOK, "text/event-stream", stream), nil
		})
		upstream := NewCompatibleUpstream(anthropic, nil)
		response, err := upstream.Open(context.Background(), Route{EndpointProtocol: AnthropicEndpoint}, Request{Protocol: OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"m","stream":true,"input":"hi"}`)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		for _, expected := range []string{"event: response.created", "event: response.output_item.added", "event: response.output_text.delta", "event: response.output_item.done", "event: response.completed", `"input_tokens":3`, `"output_tokens":2`} {
			if err != nil || !strings.Contains(string(data), expected) {
				t.Fatalf("Responses stream missing %q: %s, %v", expected, data, err)
			}
		}
	})
}

func TestAnthropicCountTokensDoesNotGuessThroughOpenAI(t *testing.T) {
	called := false
	openAI := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		called = true
		return nil, nil
	})
	upstream := NewCompatibleUpstream(nil, openAI)
	_, err := upstream.Open(context.Background(), Route{EndpointProtocol: OpenAIEndpoint}, Request{Protocol: AnthropicProtocol, Path: "/v1/messages/count_tokens", Body: []byte(`{"model":"m","messages":[]}`)}, nil)
	if !errors.Is(err, ErrRoute) || called {
		t.Fatalf("count_tokens was guessed or sent to OpenAI: called=%v err=%v", called, err)
	}
}

func TestCompatibleUpstreamKeepsMatchingProtocolNative(t *testing.T) {
	want := compatibilityResponse(http.StatusOK, "application/json", `{"native":true}`)
	anthropic := compatibilityUpstreamFunc(func(_ context.Context, route Route, request Request, _ []byte) (*Response, error) {
		if route.EndpointProtocol != AnthropicEndpoint || request.Protocol != AnthropicProtocol {
			t.Fatal("matching protocol was changed")
		}
		return want, nil
	})
	upstream := NewCompatibleUpstream(anthropic, nil)
	got, err := upstream.Open(context.Background(), Route{EndpointProtocol: AnthropicEndpoint}, Request{Protocol: AnthropicProtocol}, nil)
	if err != nil || got != want {
		t.Fatal("matching protocol did not pass through unchanged")
	}
}

func TestCompatibilityConvertsErrorEnvelope(t *testing.T) {
	openAI := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		return compatibilityResponse(http.StatusTooManyRequests, "application/json", `{"error":{"message":"slow down","type":"rate_limit_error"}}`), nil
	})
	upstream := NewCompatibleUpstream(nil, openAI)
	response, err := upstream.Open(context.Background(), Route{EndpointProtocol: OpenAIEndpoint}, Request{
		Protocol: AnthropicProtocol, Path: "/v1/messages", Body: []byte(`{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.Status != http.StatusTooManyRequests || !strings.Contains(string(data), `"type":"rate_limit_error"`) || !strings.Contains(string(data), `"type":"error"`) {
		t.Fatalf("error envelope not converted: %s, %v", data, readErr)
	}
}

func compatibilityResponse(status int, contentType, body string) *Response {
	return &Response{Status: status, Headers: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
}
