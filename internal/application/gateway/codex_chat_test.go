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

func TestCodexChatSubscriptionConvertsHistoryAndNonStreamResponse(t *testing.T) {
	upstream := compatibilityUpstreamFunc(func(_ context.Context, _ Route, request Request, _ []byte) (*Response, error) {
		if request.Protocol != OpenAIResponsesProtocol || request.Path != "/v1/responses" {
			t.Fatalf("wrong upstream protocol: %+v", request)
		}
		body, err := decodeJSONObject(request.Body)
		if err != nil || body["stream"] != true || body["store"] != false || body["model"] != "gpt-test" {
			t.Fatalf("wrong upstream body: %s, %v", request.Body, err)
		}
		input := anySlice(body["input"])
		if len(input) != 6 || input[0].(map[string]any)["role"] != "system" ||
			input[3].(map[string]any)["type"] != "function_call" ||
			input[4].(map[string]any)["type"] != "function_call_output" {
			t.Fatalf("history was not preserved: %s", request.Body)
		}
		tools := anySlice(body["tools"])
		if len(tools) != 1 || tools[0].(map[string]any)["name"] != "weather" {
			t.Fatalf("tools were not converted: %s", request.Body)
		}
		return codexTestResponse(http.StatusOK, `event: response.created
data: {"response":{"id":"resp_1","model":"gpt-test","created_at":1780000000}}

event: response.output_text.delta
data: {"delta":"晴天"}

event: response.completed
data: {"response":{"id":"resp_1","model":"gpt-test","output":[{"type":"message","content":[{"type":"output_text","text":"晴天"}]}],"usage":{"input_tokens":18,"output_tokens":3,"input_tokens_details":{"cached_tokens":4}}}}

`), nil
	})
	request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{
"model":"gpt-test","stream":false,
"messages":[
 {"role":"system","content":"简洁回答"},
 {"role":"user","content":"天气怎样"},
 {"role":"assistant","content":"我来查","tool_calls":[{"id":"call_1","type":"function","function":{"name":"weather","arguments":"{\"city\":\"杭州\"}"}}]},
 {"role":"tool","tool_call_id":"call_1","content":"晴天"},
 {"role":"user","content":"总结一下"}
],
"tools":[{"type":"function","function":{"name":"weather","description":"查询天气","parameters":{"type":"object"}}}],
"tool_choice":"auto"
}`)}
	response, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{
		AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX", EndpointProtocol: OpenAIEndpoint,
	}, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.Headers["Content-Type"][0] != "application/json" {
		t.Fatalf("invalid converted response: %s, %v", data, err)
	}
	var chat struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &chat); err != nil || len(chat.Choices) != 1 ||
		chat.Choices[0].Message.Content != "晴天" || chat.Usage.PromptTokens != 18 || chat.Usage.CompletionTokens != 3 {
		t.Fatalf("wrong Chat response: %s, %v", data, err)
	}
}

func TestCodexChatSubscriptionConvertsStreamingToolCall(t *testing.T) {
	upstream := compatibilityUpstreamFunc(func(_ context.Context, _ Route, request Request, _ []byte) (*Response, error) {
		if request.Protocol != OpenAIResponsesProtocol {
			t.Fatalf("request was not converted: %+v", request)
		}
		return codexTestResponse(http.StatusOK, `event: response.created
data: {"response":{"id":"resp_2","model":"gpt-test"}}

event: response.output_item.added
data: {"output_index":0,"item":{"type":"function_call","call_id":"call_2","name":"weather","arguments":""}}

event: response.function_call_arguments.delta
data: {"output_index":0,"delta":"{\"city\":"}

event: response.function_call_arguments.delta
data: {"output_index":0,"delta":"\"杭州\"}"}

event: response.output_item.done
data: {"output_index":0,"item":{"type":"function_call","call_id":"call_2","name":"weather","arguments":"{\"city\":\"杭州\"}"}}

event: response.completed
data: {"response":{"id":"resp_2","model":"gpt-test","output":[{"type":"function_call","call_id":"call_2","name":"weather","arguments":"{\"city\":\"杭州\"}"}],"usage":{"input_tokens":8,"output_tokens":5}}}

`), nil
	})
	request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"杭州天气"}]}`)}
	response, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{
		AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX", EndpointProtocol: OpenAIEndpoint,
	}, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	stream := string(data)
	for _, expected := range []string{`"role":"assistant"`, `"name":"weather"`, `"arguments":"{\"city\":"`, `"finish_reason":"tool_calls"`, `"prompt_tokens":8`, "data: [DONE]"} {
		if !strings.Contains(stream, expected) {
			t.Fatalf("missing %s in stream: %s", expected, stream)
		}
	}
}

func TestCodexChatSubscriptionRejectsUnsupportedRequest(t *testing.T) {
	upstream := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		t.Fatal("unsupported request reached upstream")
		return nil, nil
	})
	for _, body := range []string{
		`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"stop":["END"]}`,
		`{"model":"gpt-test","stream":true,"stream_options":{"future":true},"messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"gpt-test","stream":false,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hello"}]}`,
		`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"max_tokens":0}`,
		`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"max_tokens":1.5}`,
		`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"temperature":0.7}`,
		`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"top_p":0.9}`,
	} {
		request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(body)}
		_, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{
			AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX",
		}, request, nil)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("unexpected error for %s: %v", body, err)
		}
	}
}

func TestCodexChatConversionStageDoesNotExposeRequestContent(t *testing.T) {
	upstream := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		t.Fatal("invalid request reached upstream")
		return nil, nil
	})
	for _, test := range []struct {
		name, body, stage string
	}{
		{"unsupported field", `{"model":"gpt-test","messages":[{"role":"user","content":"private prompt"}],"stop":"private prompt"}`, "unsupported_field"},
		{"message", `{"model":"gpt-test","messages":[{"role":"user","content":[{"type":"future","text":"private prompt"}]}]}`, "message"},
		{"tools", `{"model":"gpt-test","messages":[{"role":"user","content":"private prompt"}],"tools":[{"type":"future"}]}`, "tools"},
		{"options", `{"model":"gpt-test","messages":[{"role":"user","content":"private prompt"}],"temperature":0.7}`, "options"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := Request{Protocol: OpenAIProtocol, Body: []byte(test.body)}
			_, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"}, request, nil)
			if !errors.Is(err, ErrInvalid) || CodexChatConversionStage(err) != test.stage || strings.Contains(err.Error(), "private prompt") {
				t.Fatalf("unexpected conversion error: stage=%q err=%v", CodexChatConversionStage(err), err)
			}
		})
	}
}

func TestCodexChatSubscriptionOmitsDefaultSamplingOptions(t *testing.T) {
	request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"Hi"}],"temperature":1,"top_p":1.0}`)}
	converted, _, err := codexChatToResponses(request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := decodeJSONObject(converted.Body)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["temperature"]; exists {
		t.Fatalf("unsupported temperature was forwarded: %s", converted.Body)
	}
	if _, exists := payload["top_p"]; exists {
		t.Fatalf("unsupported top_p was forwarded: %s", converted.Body)
	}
}

func TestCodexChatSubscriptionRaisesSubMinimumOutputLimit(t *testing.T) {
	for _, test := range []struct {
		field string
		value string
		want  string
	}{
		{"max_tokens", "1", "16"},
		{"max_completion_tokens", "15", "16"},
		{"max_tokens", "16", "16"},
		{"max_completion_tokens", "32", "32"},
	} {
		body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"Hi"}],"` + test.field + `":` + test.value + `}`)
		converted, _, err := codexChatToResponses(Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: body})
		if err != nil {
			t.Fatalf("%s=%s: %v", test.field, test.value, err)
		}
		payload, err := decodeJSONObject(converted.Body)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := payload["max_output_tokens"].(json.Number)
		if !ok || string(got) != test.want {
			t.Fatalf("%s=%s: max_output_tokens=%v, want %s", test.field, test.value, payload["max_output_tokens"], test.want)
		}
	}
}

func TestCodexChatSubscriptionDoesNotAcceptIncompleteResponse(t *testing.T) {
	upstream := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		return codexTestResponse(http.StatusOK, `event: response.output_text.delta
data: {"delta":"partial"}

`), nil
	})
	request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`)}
	_, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{
		AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX",
	}, request, nil)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("incomplete response was accepted: %v", err)
	}
}

func TestCodexChatConversionLeavesAPIKeyRouteUnchanged(t *testing.T) {
	const body = `{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`
	upstream := compatibilityUpstreamFunc(func(_ context.Context, _ Route, request Request, _ []byte) (*Response, error) {
		if request.Protocol != OpenAIProtocol || request.Path != "/v1/chat/completions" || string(request.Body) != body {
			t.Fatalf("API-key request changed: %+v", request)
		}
		return &Response{Status: http.StatusOK, Headers: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`))}, nil
	})
	response, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{
		AuthType: "API_KEY", EndpointProtocol: OpenAIEndpoint,
	}, Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(body)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || !strings.Contains(string(data), `"content":"ok"`) {
		t.Fatalf("API-key response changed: %s, %v", data, err)
	}
}

func TestCodexChatConversionPreservesUpstreamErrorStatus(t *testing.T) {
	upstream := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		return &Response{Status: http.StatusTooManyRequests,
			Headers: http.Header{"Content-Type": {"application/json"}},
			Body:    io.NopCloser(strings.NewReader(`{"error":{"message":"rate limit","type":"rate_limit_error"}}`))}, nil
	})
	request := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hi"}]}`)}
	response, err := NewCompatibleUpstream(nil, upstream).Open(context.Background(), Route{
		AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX",
	}, request, nil)
	if err != nil || response.Status != http.StatusTooManyRequests {
		t.Fatalf("error status changed: %+v, %v", response, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || !strings.Contains(string(data), `"rate limit"`) {
		t.Fatalf("error body changed: %s, %v", data, err)
	}
}

func codexTestResponse(status int, body string) *Response {
	return &Response{Status: status, Headers: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(body))}
}
