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
		if i == nil || *i != 9007199254740993 || out == nil || *out != 0 || c == nil || *c != 2 {
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
