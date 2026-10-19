package gateway

import (
	"strings"
	"testing"
)

func TestUsageJSONProjection(t *testing.T) {
	body := `{"content":[{"text":"` + strings.Repeat(`text \\" escaped `, 100000) + `","usage":{"input_tokens":999}}],"type":"message","usage":{"input_tokens":9007199254740993,"output_tokens":0,"cache_read_input_tokens":2}}`
	for _, chunk := range []int{1, 13, 32768} {
		o := NewUsageObserver(false)
		for i := 0; i < len(body); i += chunk {
			o.Feed([]byte(body[i:min(i+chunk, len(body))]))
		}
		if !o.Complete() {
			t.Fatal("valid large JSON did not complete")
		}
		i, out, c := o.Tokens(true)
		// Anthropic input_tokens 不含缓存，总输入 = input + cache_read。
		if i == nil || *i != 9007199254740995 || out == nil || *out != 0 || c == nil || *c != 2 {
			t.Fatal("usage corrupted or nested usage extracted")
		}
		if cap(o.json.capture) > 64<<10 || len(o.json.stack) > 256 {
			t.Fatal("unbounded JSON buffer")
		}
	}
}
func TestUsageSSECumulative(t *testing.T) {
	body := "event: message_start\r\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":1,\"cache_read_input_tokens\":0}}}\r\n\r\n" +
		"event: ping\ndata: {\"type\":\"ping\"}\n\n" +
		"data: {\"type\":\"message_delta\",\n" + "data: \"usage\":{\"output_tokens\":5}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":12}}\n\n" +
		"data: {\"type\":\"message_stop\"}\n\n"
	for _, chunk := range []int{1, 7, 32768} {
		o := NewUsageObserver(true)
		for i := 0; i < len(body); i += chunk {
			o.Feed([]byte(body[i:min(i+chunk, len(body))]))
		}
		in, out, c := o.Tokens(o.Complete())
		if !o.Complete() || in == nil || *in != 10 || out == nil || *out != 12 || c == nil || *c != 0 {
			t.Fatal("cumulative usage or chunk boundaries broken")
		}
	}
}

func TestUsageAnthropicContentBlockSequence(t *testing.T) {
	valid := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"ok\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	for _, chunk := range []int{1, 17, 32768} {
		o := NewUsageObserver(true)
		for i := 0; i < len(valid); i += chunk {
			o.Feed([]byte(valid[i:min(i+chunk, len(valid))]))
		}
		if !o.Complete() || o.ProtocolInvalid() {
			t.Fatalf("valid content block stream rejected with chunk=%d: %+v", chunk, o.Diagnostics())
		}
	}

	start := "data: {\"type\":\"message_start\",\"message\":{}}\n\n"
	block0 := "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"
	stop0 := "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"
	for name, invalid := range map[string]string{
		"sparse start":        start + block0 + stop0 + "data: {\"type\":\"content_block_start\",\"index\":4,\"content_block\":{\"type\":\"tool_use\"}}\n\n",
		"overlapping start":   start + block0 + "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\"}}\n\n",
		"stop without start":  start + "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"delta without start": start + "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n\n",
		"open block at stop":  start + block0 + "data: {\"type\":\"message_stop\"}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			o := NewUsageObserver(true)
			o.Feed([]byte(invalid))
			if !o.ProtocolInvalid() || o.ErrorCode() != "UPSTREAM_INVALID_RESPONSE" || o.Complete() {
				t.Fatalf("invalid content block sequence accepted: diagnostics=%+v code=%s", o.Diagnostics(), o.ErrorCode())
			}
		})
	}
}
func TestUsageIncompleteAndUnknown(t *testing.T) {
	for _, body := range []string{
		`{"type":"message","usage":{"input_tokens":-1,"output_tokens":3}}`,
		`{"type":"message","usage":{"input_tokens":1.5,"output_tokens":3}}`,
		`{"type":"message","usage":{"input_tokens":9223372036854775808}}`,
		`{"type":"message","usage":{"input_tokens":1,"input_tokens":2}}`,
		`{"type":"message","usage":{"input_tokens":1},"usage":{"input_tokens":2}}`,
	} {
		o := NewUsageObserver(false)
		o.Feed([]byte(body))
		in, out, c := o.Tokens(o.Complete())
		if in != nil || out != nil || c != nil {
			t.Fatal("unreliable values fabricated")
		}
	}
	o := NewUsageObserver(true)
	o.Feed([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":7,\"output_tokens\":1}}}\n\n"))
	in, out, _ := o.Tokens(o.Complete())
	if o.Complete() || in == nil || *in != 7 || out != nil {
		t.Fatal("truncated stream final tokens fabricated")
	}
	o.Feed([]byte("data: {\"type\":\"error\"}\n\ndata: {\"type\":\"message_stop\"}\n\n"))
	if o.Complete() {
		t.Fatal("error event treated as success")
	}
	json := NewUsageObserver(false)
	json.Feed([]byte(`{"type":"message","content":[]}`))
	in, out, c := json.Tokens(json.Complete())
	if !json.Complete() || in != nil || out != nil || c != nil {
		t.Fatal("missing usage became zero")
	}
}

func TestUsageOversizedSSEDoesNotGrowOrRecoverFalseCounts(t *testing.T) {
	o := NewUsageObserver(true)
	o.Feed([]byte("data: " + strings.Repeat("x", 300<<10) + "\n\ndata: {\"type\":\"message_stop\"}\n\n"))
	in, out, c := o.Tokens(true)
	if in != nil || out != nil || c != nil || len(o.line) > 256<<10 || len(o.data) > 256<<10 {
		t.Fatal("oversized event not bounded")
	}
}

func TestUsageAnthropicCacheTokensNormalizeToTotalInput(t *testing.T) {
	// Claude Code 典型样本：input_tokens 很小，绝大部分输入来自缓存读取。
	stream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3,\"cache_creation_input_tokens\":1200,\"cache_read_input_tokens\":45000,\"output_tokens\":1}}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":3,\"cache_creation_input_tokens\":1200,\"cache_read_input_tokens\":45000,\"output_tokens\":350}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	tests := []struct {
		name                  string
		stream                bool
		body                  string
		input, output, cached int64
		cacheCreation         int64
		complete              bool
	}{
		{name: "stream", stream: true, body: stream, input: 46203, output: 350, cached: 45000, cacheCreation: 1200, complete: true},
		{name: "stream without cumulative input in delta", stream: true, body: strings.Replace(stream,
			`"usage":{"input_tokens":3,"cache_creation_input_tokens":1200,"cache_read_input_tokens":45000,"output_tokens":350}`,
			`"usage":{"output_tokens":350}`, 1), input: 46203, output: 350, cached: 45000, cacheCreation: 1200, complete: true},
		{name: "json", body: `{"type":"message","usage":{"input_tokens":3,"cache_creation_input_tokens":1200,"cache_read_input_tokens":45000,"output_tokens":350}}`,
			input: 46203, output: 350, cached: 45000, cacheCreation: 1200, complete: true},
		{name: "json without cache fields", body: `{"type":"message","usage":{"input_tokens":12,"output_tokens":5}}`,
			input: 12, output: 5, cached: -1, cacheCreation: -1, complete: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, chunk := range []int{1, 19, 32768} {
				o := NewUsageObserver(test.stream)
				for i := 0; i < len(test.body); i += chunk {
					o.Feed([]byte(test.body[i:min(i+chunk, len(test.body))]))
				}
				in, out, cached := o.Tokens(o.Complete())
				if o.Complete() != test.complete || in == nil || *in != test.input || out == nil || *out != test.output {
					t.Fatalf("chunk=%d complete=%v input=%v output=%v", chunk, o.Complete(), in, out)
				}
				if test.cached < 0 && cached != nil || test.cached >= 0 && (cached == nil || *cached != test.cached) {
					t.Fatalf("chunk=%d cached=%v", chunk, cached)
				}
				if *cached0(cached) > *in {
					t.Fatalf("cached exceeds total input: cached=%d input=%d", *cached, *in)
				}
				cacheCreation := o.CacheCreationTokens()
				if test.cacheCreation < 0 && cacheCreation != nil ||
					test.cacheCreation >= 0 && (cacheCreation == nil || *cacheCreation != test.cacheCreation) {
					t.Fatalf("chunk=%d cacheCreation=%v", chunk, cacheCreation)
				}
				if *cached0(cached)+*cached0(cacheCreation) > *in {
					t.Fatalf("cache tokens exceed total input: input=%d", *in)
				}
			}
		})
	}
}

func cached0(value *int64) *int64 {
	if value == nil {
		zero := int64(0)
		return &zero
	}
	return value
}

func TestUsageAnthropicCacheRegressionAndOverflowAreUnreliable(t *testing.T) {
	for name, body := range map[string]string{
		"cache read regressed": "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3,\"cache_read_input_tokens\":100}}}\n\n" +
			"data: {\"type\":\"message_delta\",\"usage\":{\"cache_read_input_tokens\":10,\"output_tokens\":2}}\n\n" +
			"data: {\"type\":\"message_stop\"}\n\n",
		"cache creation regressed": "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3,\"cache_creation_input_tokens\":100}}}\n\n" +
			"data: {\"type\":\"message_delta\",\"usage\":{\"cache_creation_input_tokens\":10,\"output_tokens\":2}}\n\n" +
			"data: {\"type\":\"message_stop\"}\n\n",
		"total overflow": "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":9223372036854775807,\"cache_read_input_tokens\":1}}}\n\n" +
			"data: {\"type\":\"message_stop\"}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			o := NewUsageObserver(true)
			o.Feed([]byte(body))
			in, out, cached := o.Tokens(o.Complete())
			if in != nil || out != nil || cached != nil || o.CacheCreationTokens() != nil {
				t.Fatalf("unreliable usage reported: input=%v output=%v cached=%v cacheCreation=%v", in, out, cached, o.CacheCreationTokens())
			}
		})
	}
}

func TestOpenAIChatStreamWithoutUsageIsIncomplete(t *testing.T) {
	body := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	o := NewOpenAIUsageObserver(true)
	o.Feed([]byte(body))
	if o.Complete() || o.ErrorCode() != "UPSTREAM_RESPONSE_INCOMPLETE" {
		t.Fatalf("stream without usage completed: complete=%v code=%s", o.Complete(), o.ErrorCode())
	}
}

func TestOpenAIResponsesOversizedCompletedEventKeepsUsage(t *testing.T) {
	image := strings.Repeat("QUJD", 3<<20/4)
	completed := "{\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"image_generation_call\",\"result\":\"" + image +
		"\",\"note\":\"escaped \\\" quote\",\"usage\":{\"input_tokens\":999}}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":4},\"output_tokens\":9}}}"
	created := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n"
	// 多行 data 以 "\n" 拼接，只能在 JSON 空白位置拆分。
	split := strings.Index(completed, `],"usage"`) + 1
	partial := "event: response.image_generation_call.partial_image\ndata: {\"type\":\"response.image_generation_call.partial_image\",\"partial_image_b64\":\"" + image + "\"}\n\n"
	tests := []struct {
		name string
		body string
	}{
		{name: "named completed", body: created + partial + "event: response.completed\ndata: " + completed + "\n\n"},
		{name: "data only completed", body: created + "data: " + completed + "\n\n"},
		{name: "multi line data", body: created + "data: " + completed[:split] + "\ndata: " + completed[split:] + "\n\n"},
		{name: "crlf", body: strings.ReplaceAll(created, "\n", "\r\n") + "event: response.completed\r\ndata: " + completed + "\r\n\r\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, chunk := range []int{4093, 32 << 10, 1 << 20} {
				o := NewOpenAIResponsesUsageObserver(true)
				for i := 0; i < len(test.body); i += chunk {
					o.Feed([]byte(test.body[i:min(i+chunk, len(test.body))]))
				}
				in, out, cached := o.Tokens(o.Complete())
				if !o.Complete() || in == nil || *in != 12 || out == nil || *out != 9 || cached == nil || *cached != 4 {
					t.Fatalf("chunk=%d diagnostics=%+v", chunk, o.Diagnostics())
				}
				if len(o.line) > sseEventLimit || len(o.data) > sseEventLimit {
					t.Fatalf("oversized event buffered: line=%d data=%d", len(o.line), len(o.data))
				}
			}
		})
	}
}

func TestOpenAIResponsesOversizedTerminalEventsAreObserved(t *testing.T) {
	large := strings.Repeat("x", sseEventLimit+1)
	for name, test := range map[string]struct {
		body             string
		failed, badUsage bool
		complete         bool
	}{
		"failed without event name": {
			body:   "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"" + large + "\"}}}\n\n",
			failed: true,
		},
		"invalid json": {
			body:     "data: {\"type\":\"response.completed\",\"response\":{\"x\":\"" + large + "\"\n\n",
			badUsage: true,
		},
		"non object usage": {
			body:     "data: {\"type\":\"response.completed\",\"response\":{\"x\":\"" + large + "\",\"usage\":7}}\n\n",
			badUsage: true, complete: true,
		},
		"named non terminal event": {
			body: "event: response.output_text.done\ndata: {\"type\":\"response.output_text.done\",\"text\":\"" + large + "\"}\n\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := NewOpenAIResponsesUsageObserver(true)
			o.Feed([]byte(test.body))
			diagnostics := o.Diagnostics()
			if diagnostics.Failed != test.failed || diagnostics.BadUsage != test.badUsage || diagnostics.Stopped != test.complete {
				t.Fatalf("diagnostics=%+v", diagnostics)
			}
		})
	}
}
