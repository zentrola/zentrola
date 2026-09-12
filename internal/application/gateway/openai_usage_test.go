package gateway

import (
	"strings"
	"testing"
)

func TestOpenAIUsageFormats(t *testing.T) {
	for _, body := range []string{
		`{"object":"chat.completion","choices":[{"message":{"content":"` + strings.Repeat("large response", 10000) + `"}}],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":7,"prompt_cache_hit_tokens":3}}`,
		`{"usage":{"prompt_tokens":9007199254740993,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":3}},"object":"chat.completion"}`,
	} {
		for _, step := range []int{1, 23, 32768} {
			o := NewOpenAIUsageObserver(false)
			for i := 0; i < len(body); i += step {
				o.Feed([]byte(body[i:min(i+step, len(body))]))
			}
			in, out, cache := o.Tokens(o.Complete())
			if !o.Complete() || in == nil || *in != 9007199254740993 || out == nil || *out != 7 || cache == nil || *cache != 3 {
				t.Fatal("OpenAI usage lost or rounded")
			}
		}
	}
	for _, body := range []string{`{"object":"chat.completion","usage":null}`, `{"object":"chat.completion"}`} {
		o := NewOpenAIUsageObserver(false)
		o.Feed([]byte(body))
		in, out, _ := o.Tokens(o.Complete())
		if !o.Complete() || in != nil || out != nil {
			t.Fatal("missing usage fabricated")
		}
	}
}
func TestOpenAIStreamUsageAndEnd(t *testing.T) {
	for _, final := range []string{
		`{"object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":9,"prompt_tokens_details":{"cached_tokens":0}}}`,
		`{"object":"chat.completion.chunk","choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":9,"prompt_cache_hit_tokens":0}}`,
	} {
		body := "data: {\"object\":\"chat.completion.chunk\",\"usage\":null,\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_a\"}]}}]}\n\ndata: " + final + "\n\ndata: [DONE]\n\n"
		o := NewOpenAIUsageObserver(true)
		for _, b := range []byte(body) {
			o.Feed([]byte{b})
		}
		in, out, c := o.Tokens(o.Complete())
		if !o.Complete() || in == nil || *in != 12 || out == nil || *out != 9 || c == nil || *c != 0 {
			t.Fatal("final usage chunk not collected")
		}
	}
	o := NewOpenAIUsageObserver(true)
	o.Feed([]byte("data: {\"object\":\"chat.completion.chunk\",\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\n"))
	in, out, _ := o.Tokens(o.Complete())
	if o.Complete() || in == nil || out != nil {
		t.Fatal("truncated stream reported final output")
	}
	o.Feed([]byte("data: {\"error\":{\"message\":\"private\"}}\n\ndata: [DONE]\n\n"))
	if o.Complete() {
		t.Fatal("stream error treated as success")
	}
}
func TestOpenAIRequestNullStreamAndOpaqueRewrite(t *testing.T) {
	body := []byte(`{"model":"alias","stream":null,"messages":[{"role":"tool","tool_call_id":"call_x","content":"9007199254740993"}]}`)
	p, err := ParseOpenAI(body)
	if err != nil || p.Stream {
		t.Fatal("nullable stream rejected")
	}
	if string(p.Rewrite(body, "deepseek-v4-flash")) != strings.Replace(string(body), `"alias"`, `"deepseek-v4-flash"`, 1) {
		t.Fatal("non-model fields changed")
	}
	if _, err := Parse(body); err == nil {
		t.Fatal("Anthropic null-stream validation regressed")
	}
}

func TestOpenAIResponsesUsageFormats(t *testing.T) {
	jsonBody := `{"object":"response","usage":{"input_tokens":12,"input_tokens_details":{"cached_tokens":4},"output_tokens":9,"total_tokens":21}}`
	observer := NewOpenAIResponsesUsageObserver(false)
	observer.Feed([]byte(jsonBody))
	input, output, cached := observer.Tokens(observer.Complete())
	if !observer.Complete() || input == nil || *input != 12 || output == nil || *output != 9 || cached == nil || *cached != 4 {
		t.Fatal("Responses JSON usage not collected")
	}

	stream := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":4},\"output_tokens\":9,\"total_tokens\":21}}}\n\n"
	observer = NewOpenAIResponsesUsageObserver(true)
	observer.Feed([]byte(stream))
	input, output, cached = observer.Tokens(observer.Complete())
	if !observer.Complete() || input == nil || *input != 12 || output == nil || *output != 9 || cached == nil || *cached != 4 {
		t.Fatal("Responses stream usage not collected")
	}
	diagnostics := observer.Diagnostics()
	if !diagnostics.Stream || !diagnostics.Started || !diagnostics.Stopped || diagnostics.Failed || diagnostics.BadUsage ||
		diagnostics.EventCount != 2 || diagnostics.LastEvent != "response.completed" ||
		!diagnostics.InputSeen || !diagnostics.OutputSeen || !diagnostics.CachedSeen {
		t.Fatalf("Responses stream diagnostics incomplete: %+v", diagnostics)
	}
}

func TestOpenAIResponsesUsageAutoDetectsSSEAndJSON(t *testing.T) {
	for _, test := range []struct {
		name           string
		fallbackStream bool
		body           string
		wantStream     bool
	}{
		{
			name:           "SSE overrides JSON fallback",
			fallbackStream: false,
			body: " \r\nevent: response.created\n" +
				"data: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
				"event: response.completed\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":9}}}\n\n",
			wantStream: true,
		},
		{
			name:           "JSON overrides SSE fallback",
			fallbackStream: true,
			body:           " \r\n{\"object\":\"response\",\"usage\":{\"input_tokens\":12,\"output_tokens\":9}}",
			wantStream:     false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			observer := NewAutoOpenAIResponsesUsageObserver(test.fallbackStream)
			for _, b := range []byte(test.body) {
				observer.Feed([]byte{b})
			}
			input, output, _ := observer.Tokens(observer.Complete())
			if !observer.Complete() || observer.Diagnostics().Stream != test.wantStream ||
				input == nil || *input != 12 || output == nil || *output != 9 {
				t.Fatalf("auto-detected usage incorrectly: diagnostics=%+v", observer.Diagnostics())
			}
		})
	}
}
