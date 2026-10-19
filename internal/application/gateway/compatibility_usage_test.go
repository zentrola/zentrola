package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func openCompatibility(t *testing.T, route Route, request Request, upstreamResponse *Response) (*Response, string) {
	t.Helper()
	open := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		return upstreamResponse, nil
	})
	upstream := NewCompatibleUpstream(open, open)
	response, err := upstream.Open(context.Background(), route, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("converted body failed: %v", err)
	}
	return response, string(data)
}

func anthropicChatRequest(stream bool) Request {
	body := `{"model":"m","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`
	if stream {
		body = `{"model":"m","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	}
	return Request{Protocol: AnthropicProtocol, Path: "/v1/messages", Body: []byte(body)}
}

func TestAnthropicToOpenAIStreamRequestsUsage(t *testing.T) {
	for _, test := range []struct {
		stream bool
		want   bool
	}{{stream: true, want: true}, {stream: false}} {
		converted, err := convertGatewayRequest(anthropicChatRequest(test.stream), OpenAIEndpoint)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(converted.Body), `"stream_options":{"include_usage":true}`); got != test.want {
			t.Fatalf("stream=%v include_usage=%v body=%s", test.stream, got, converted.Body)
		}
	}
}

func TestCompatibilityPreservesCacheTokens(t *testing.T) {
	claudeUsage := `{"input_tokens":3,"cache_creation_input_tokens":1200,"cache_read_input_tokens":45000,"output_tokens":350}`
	tests := []struct {
		name     string
		route    Route
		request  Request
		response *Response
		want     []string
	}{
		{
			name: "Anthropic JSON to OpenAI Chat", route: Route{EndpointProtocol: AnthropicEndpoint},
			request:  Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)},
			response: compatibilityResponse(http.StatusOK, "application/json", `{"type":"message","id":"msg_1","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":`+claudeUsage+`}`),
			want:     []string{`"prompt_tokens":46203`, `"cached_tokens":45000`, `"completion_tokens":350`, `"total_tokens":46553`},
		},
		{
			name: "Anthropic JSON to OpenAI Responses", route: Route{EndpointProtocol: AnthropicEndpoint},
			request:  Request{Protocol: OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"m","input":"hi"}`)},
			response: compatibilityResponse(http.StatusOK, "application/json", `{"type":"message","id":"msg_1","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":`+claudeUsage+`}`),
			want:     []string{`"input_tokens":46203`, `"cached_tokens":45000`, `"output_tokens":350`},
		},
		{
			name: "Anthropic stream to OpenAI Chat", route: Route{EndpointProtocol: AnthropicEndpoint},
			request: Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"m","stream":true,"messages":[{"role":"user","content":"hi"}]}`)},
			response: compatibilityResponse(http.StatusOK, "text/event-stream",
				"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"m\",\"usage\":{\"input_tokens\":3,\"cache_creation_input_tokens\":1200,\"cache_read_input_tokens\":45000,\"output_tokens\":1}}}\n\n"+
					"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":350}}\n\n"+
					"data: {\"type\":\"message_stop\"}\n\n"),
			want: []string{`"prompt_tokens":46203`, `"cached_tokens":45000`, `"completion_tokens":350`},
		},
		{
			name: "Anthropic stream to OpenAI Responses", route: Route{EndpointProtocol: AnthropicEndpoint},
			request: Request{Protocol: OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"m","stream":true,"input":"hi"}`)},
			response: compatibilityResponse(http.StatusOK, "text/event-stream",
				"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"m\",\"usage\":{\"input_tokens\":3,\"cache_creation_input_tokens\":1200,\"cache_read_input_tokens\":45000}}}\n\n"+
					"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":350}}\n\n"+
					"data: {\"type\":\"message_stop\"}\n\n"),
			want: []string{`"input_tokens":46203`, `"cached_tokens":45000`, `"output_tokens":350`},
		},
		{
			name: "OpenAI JSON to Anthropic", route: Route{EndpointProtocol: OpenAIEndpoint}, request: anthropicChatRequest(false),
			response: compatibilityResponse(http.StatusOK, "application/json", `{"id":"c1","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":800}}}`),
			want:     []string{`"input_tokens":200`, `"cache_read_input_tokens":800`, `"output_tokens":20`},
		},
		{
			name: "OpenAI stream to Anthropic", route: Route{EndpointProtocol: OpenAIEndpoint}, request: anthropicChatRequest(true),
			response: compatibilityResponse(http.StatusOK, "text/event-stream",
				"data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"+
					"data: {\"id\":\"c1\",\"choices\":[],\"usage\":{\"prompt_tokens\":1000,\"completion_tokens\":20,\"prompt_cache_hit_tokens\":800}}\n\n"+
					"data: [DONE]\n\n"),
			want: []string{`"input_tokens":200`, `"cache_read_input_tokens":800`, `"output_tokens":20`, "event: message_stop"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, data := openCompatibility(t, test.route, test.request, test.response)
			for _, want := range test.want {
				if !strings.Contains(data, want) {
					t.Fatalf("converted response missing %s: %s", want, data)
				}
			}
		})
	}
}

func TestCompatibilityStreamTokensMatchNativeObserver(t *testing.T) {
	// OpenAI→Anthropic 转换后的流经 Anthropic observer 归一，总输入应等于上游 prompt_tokens。
	stream := "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"id\":\"c1\",\"choices\":[],\"usage\":{\"prompt_tokens\":1000,\"completion_tokens\":20,\"prompt_tokens_details\":{\"cached_tokens\":800}}}\n\n" +
		"data: [DONE]\n\n"
	_, data := openCompatibility(t, Route{EndpointProtocol: OpenAIEndpoint}, anthropicChatRequest(true),
		compatibilityResponse(http.StatusOK, "text/event-stream", stream))
	observer := NewUsageObserver(true)
	observer.Feed([]byte(data))
	input, output, cached := observer.Tokens(observer.Complete())
	if !observer.Complete() || input == nil || *input != 1000 || output == nil || *output != 20 || cached == nil || *cached != 800 {
		t.Fatalf("observer tokens mismatch: complete=%v diagnostics=%+v", observer.Complete(), observer.Diagnostics())
	}
}

func TestOpenAIToAnthropicStreamDoesNotFakeCompletionOnEarlyEOF(t *testing.T) {
	tests := []struct {
		name, stream string
		wantStop     bool
		wantError    bool
	}{
		{
			name:      "eof before finish",
			stream:    "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n",
			wantError: true,
		},
		{
			name:      "empty stream",
			stream:    "",
			wantError: true,
		},
		{
			name:     "finish reason without done",
			stream:   "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\n",
			wantStop: true,
		},
		{
			name:      "upstream error chunk",
			stream:    "data: {\"id\":\"c1\",\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: {\"error\":{\"message\":\"boom\",\"type\":\"server_error\"}}\n\n",
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, data := openCompatibility(t, Route{EndpointProtocol: OpenAIEndpoint}, anthropicChatRequest(true),
				compatibilityResponse(http.StatusOK, "text/event-stream", test.stream))
			if got := strings.Contains(data, "event: message_stop"); got != test.wantStop {
				t.Fatalf("message_stop=%v: %s", got, data)
			}
			if got := strings.Contains(data, "event: error"); got != test.wantError {
				t.Fatalf("error event=%v: %s", got, data)
			}
			observer := NewUsageObserver(true)
			observer.Feed([]byte(data))
			if observer.Complete() == test.wantError {
				t.Fatalf("observer complete=%v diagnostics=%+v", observer.Complete(), observer.Diagnostics())
			}
		})
	}
}

func TestCompatibilityWrapsNonStandardErrorBodies(t *testing.T) {
	tests := []struct {
		name        string
		route       Route
		request     Request
		status      int
		contentType string
		body        string
		wantType    string
		wantMessage string
	}{
		{
			name: "vLLM error to Anthropic", route: Route{EndpointProtocol: OpenAIEndpoint}, request: anthropicChatRequest(false),
			status: http.StatusBadRequest, contentType: "application/json",
			body:     `{"object":"error","message":"max_tokens is too large","type":"BadRequestError","param":null,"code":400}`,
			wantType: "invalid_request_error", wantMessage: "max_tokens is too large",
		},
		{
			name: "nginx HTML to Anthropic", route: Route{EndpointProtocol: OpenAIEndpoint}, request: anthropicChatRequest(true),
			status: http.StatusRequestEntityTooLarge, contentType: "text/html",
			body:     "<html><body><h1>413 Request Entity Too Large</h1></body></html>",
			wantType: "request_too_large", wantMessage: "Upstream request failed with HTTP 413.",
		},
		{
			name: "FastAPI detail to OpenAI", route: Route{EndpointProtocol: AnthropicEndpoint},
			request: Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)},
			status:  http.StatusUnprocessableEntity, contentType: "application/json", body: `{"detail":"bad field"}`,
			wantType: "invalid_request_error", wantMessage: "bad field",
		},
		{
			name: "SSE content type error to OpenAI Responses", route: Route{EndpointProtocol: AnthropicEndpoint},
			request: Request{Protocol: OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"m","stream":true,"input":"hi"}`)},
			status:  http.StatusBadGateway, contentType: "text/event-stream", body: "upstream connect error",
			wantType: "api_error", wantMessage: "Upstream request failed with HTTP 502.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, data := openCompatibility(t, test.route, test.request, compatibilityResponse(test.status, test.contentType, test.body))
			if response.Status != test.status || http.Header(response.Headers).Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d headers=%v", response.Status, response.Headers)
			}
			var envelope struct {
				Type  string `json:"type"`
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(data), &envelope); err != nil {
				t.Fatalf("error body is not JSON: %s", data)
			}
			if test.request.Protocol == AnthropicProtocol && envelope.Type != "error" {
				t.Fatalf("Anthropic error envelope missing type: %s", data)
			}
			if envelope.Error.Type != test.wantType || envelope.Error.Message != test.wantMessage {
				t.Fatalf("error=%+v body=%s", envelope.Error, data)
			}
		})
	}
}

func TestCompatibilityClientErrorDoesNotCooldownOrRetry(t *testing.T) {
	routes := []Route{
		{ModelID: 1, ProviderID: 10, ResourceID: 1, UpstreamModel: "m", EndpointProtocol: OpenAIEndpoint, AuthType: "API_KEY"},
		{ModelID: 1, ProviderID: 10, ResourceID: 2, UpstreamModel: "m", EndpointProtocol: OpenAIEndpoint, AuthType: "API_KEY"},
	}
	calls := 0
	openAI := compatibilityUpstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		calls++
		return compatibilityResponse(http.StatusBadRequest, "application/json", `{"object":"error","message":"bad request","code":400}`), nil
	})
	state := &failoverState{blocked: map[int64]bool{}}
	service := New(&failoverStore{routes: routes}, failoverCipher{}, NewCompatibleUpstream(nil, openAI), WithRouteState(state))
	got, err := service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}, anthropicChatRequest(false))
	if err != nil || got == nil || got.Status != http.StatusBadRequest {
		t.Fatalf("response=%v err=%v", got, err)
	}
	data, readErr := io.ReadAll(got.Body)
	got.Body.Close()
	if readErr != nil || !strings.Contains(string(data), "bad request") {
		t.Fatalf("body=%s err=%v", data, readErr)
	}
	if calls != 1 || len(state.cooldowns) != 0 {
		t.Fatalf("calls=%d cooldowns=%v", calls, state.cooldowns)
	}
}

func TestCompatibilityNumberConversion(t *testing.T) {
	tests := []struct {
		value any
		want  int64
		ok    bool
	}{
		{value: json.Number("9007199254740993"), want: 9007199254740993, ok: true},
		{value: json.Number("12.0"), want: 12, ok: true},
		{value: json.Number("1.5")},
		{value: float64(7), want: 7, ok: true},
		{value: "7"},
		{value: nil},
	}
	for _, test := range tests {
		got, ok := int64Value(test.value)
		if got != test.want || ok != test.ok {
			t.Fatalf("int64Value(%#v)=%d,%v want %d,%v", test.value, got, ok, test.want, test.ok)
		}
	}
	if intNumber(json.Number("3")) != 3 || intNumber("x") != 0 {
		t.Fatal("intNumber conversion failed")
	}
}

func TestCompatibilityRecordsAnthropicCacheCreation(t *testing.T) {
	jsonBody := func(usage string) string {
		return `{"type":"message","id":"msg_1","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":` + usage + `}`
	}
	streamBody := func(usage string) string {
		return "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"m\",\"usage\":" + usage + "}}\n\n" +
			"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":350}}\n\n" +
			"data: {\"type\":\"message_stop\"}\n\n"
	}
	withCache := `{"input_tokens":3,"cache_creation_input_tokens":1200,"cache_read_input_tokens":45000,"output_tokens":350}`
	withoutCache := `{"input_tokens":3,"output_tokens":350}`
	chat := Request{Protocol: OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)}
	responses := Request{Protocol: OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"m","input":"hi"}`)}
	cacheCreation := int64(1200)
	tests := []struct {
		name          string
		request       Request
		stream        bool
		body          string
		wantInput     int64
		wantCacheRead int64
		wantCreation  *int64
	}{
		{name: "chat JSON", request: chat, body: jsonBody(withCache), wantInput: 46203, wantCacheRead: 45000, wantCreation: &cacheCreation},
		{name: "responses JSON", request: responses, body: jsonBody(withCache), wantInput: 46203, wantCacheRead: 45000, wantCreation: &cacheCreation},
		{name: "chat stream", request: chat, stream: true, body: streamBody(withCache), wantInput: 46203, wantCacheRead: 45000, wantCreation: &cacheCreation},
		{name: "responses stream", request: responses, stream: true, body: streamBody(withCache), wantInput: 46203, wantCacheRead: 45000, wantCreation: &cacheCreation},
		{name: "chat JSON without cache creation", request: chat, body: jsonBody(withoutCache), wantInput: 3},
		{name: "responses stream without cache creation", request: responses, stream: true, body: streamBody(withoutCache), wantInput: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentType := "application/json"
			if test.stream {
				contentType = "text/event-stream"
			}
			response, data := openCompatibility(t, Route{EndpointProtocol: AnthropicEndpoint}, test.request,
				compatibilityResponse(http.StatusOK, contentType, test.body))
			observer := NewOpenAIUsageObserver(test.stream)
			if test.request.Protocol == OpenAIResponsesProtocol {
				observer = NewOpenAIResponsesUsageObserver(test.stream)
			}
			observer.UseCompatibilityUsage(response.CompatibilityUsage)
			observer.Feed([]byte(data))
			complete := observer.Complete()
			input, _, cached := observer.Tokens(complete)
			if !complete || input == nil || *input != test.wantInput {
				t.Fatalf("complete=%v input=%v diagnostics=%+v body=%s", complete, input, observer.Diagnostics(), data)
			}
			if test.wantCacheRead > 0 && (cached == nil || *cached != test.wantCacheRead) {
				t.Fatalf("cached=%v want %d", cached, test.wantCacheRead)
			}
			got := observer.CacheCreationTokens()
			if (got == nil) != (test.wantCreation == nil) || got != nil && *got != *test.wantCreation {
				t.Fatalf("cache creation=%v want %v", got, test.wantCreation)
			}
		})
	}
}

func TestNativeOpenAIObserverHasNoCacheCreation(t *testing.T) {
	observer := NewOpenAIUsageObserver(false)
	observer.UseCompatibilityUsage(nil)
	observer.Feed([]byte(`{"object":"chat.completion","usage":{"prompt_tokens":10,"completion_tokens":2}}`))
	if !observer.Complete() || observer.CacheCreationTokens() != nil {
		t.Fatalf("complete=%v cache creation=%v", observer.Complete(), observer.CacheCreationTokens())
	}
}

func TestCacheCreationTokensStayWithinUncachedInput(t *testing.T) {
	tests := []struct {
		name          string
		usage         string
		cacheCreation int64
		want          *int64
	}{
		{name: "fits uncached input", usage: `{"prompt_tokens":100,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":60}}`, cacheCreation: 40, want: ptrInt64(40)},
		// cache_creation + cached 超过总输入会违反 usage_record 约束，只能放弃缓存写入。
		{name: "exceeds uncached input", usage: `{"prompt_tokens":100,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":60}}`, cacheCreation: 41},
		{name: "no cached tokens", usage: `{"prompt_tokens":100,"completion_tokens":1}`, cacheCreation: 100, want: ptrInt64(100)},
		{name: "negative", usage: `{"prompt_tokens":100,"completion_tokens":1}`, cacheCreation: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compatibility := &CompatibilityUsage{}
			compatibility.record(anthropicUsageCounts{cacheCreation: test.cacheCreation, cacheCreationSeen: true})
			observer := NewOpenAIUsageObserver(false)
			observer.UseCompatibilityUsage(compatibility)
			observer.Feed([]byte(`{"object":"chat.completion","usage":` + test.usage + `}`))
			got := observer.CacheCreationTokens()
			if (got == nil) != (test.want == nil) || got != nil && *got != *test.want {
				t.Fatalf("cache creation=%v want %v", got, test.want)
			}
		})
	}
}

func ptrInt64(value int64) *int64 { return &value }
