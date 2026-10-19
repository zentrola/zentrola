package gateway

import (
	"strings"
	"testing"
)

func TestRewritePreservesNativePayload(t *testing.T) {
	body := []byte(" {\n  \"model\" : \"claude-sonnet\", \"stream\": true, \"future\": {\"id\":9007199254740993},\n\"messages\":[{\"role\":\"user\",\"content\":[{\"type\":\"tool_result\",\"tool_use_id\":\"toolu_123\",\"content\":\"claude-sonnet\"}]}],\"tools\":[{\"name\":\"read\",\"input_schema\":{\"type\":\"object\"}}]} ")
	p, err := Parse(body)
	if err != nil || p.Model != "claude-sonnet" || !p.Stream {
		t.Fatal("parse failed", err)
	}
	want := strings.Replace(string(body), `"claude-sonnet"`, `"actual-upstream-model"`, 1)
	if got := string(p.Rewrite(body, "actual-upstream-model")); got != want {
		t.Fatal("fields outside top-level model were altered")
	}
	p, err = Parse([]byte(`{"m\u006fdel":"claude-sonnet"}`))
	if err != nil || string(p.Rewrite([]byte(`{"m\u006fdel":"claude-sonnet"}`), "upstream")) != `{"m\u006fdel":"upstream"}` {
		t.Fatal("escaped key handling failed")
	}
}
func TestRejectAmbiguousGatewayJSON(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{}`, `{"model":null}`, `{"model":1}`, `{"model":""}`, `{"model":"a b"}`, `{"model":"a","model":"b"}`, `{"model":"a","m\u006fdel":"b"}`, `{"model":"a","stream":null}`, `{"model":"a","stream":"true"}`, `{"model":"a","stream":false,"stream":true}`, `{"model":"a"} {}`, `{"model":"a",}`, "{\"model\":\"a\",\"content\":\"\xff\"}"} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Fatalf("invalid input accepted: %q", body)
		}
	}
}

func TestRemoveUnsupportedToolTypesPreservesOtherJSON(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-5","future":9007199254740993,"tools":[{"name":"read","description":"advisor_20260301","input_schema":{"type":"object"}},{"type":"advisor_20260301","name":"advisor","model":"claude-opus-5"},{"type":"web_search_20260209","name":"web_search"}],"messages":[{"role":"user","content":"keep verbatim"}]}`)
	parsed, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	filtered, removed, err := parsed.RemoveToolTypes(body, "advisor_20260301")
	if err != nil || removed != 1 {
		t.Fatalf("removed=%d err=%v", removed, err)
	}
	got := string(filtered)
	for _, preserved := range []string{"9007199254740993", `"description":"advisor_20260301"`, `"type":"web_search_20260209"`, `"content":"keep verbatim"`} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("filtered request lost %s: %s", preserved, got)
		}
	}
	if strings.Contains(got, `"type":"advisor_20260301"`) {
		t.Fatalf("advisor tool was not removed: %s", got)
	}
	unchanged, removed, err := parsed.RemoveToolTypes(body, "unsupported_elsewhere")
	if err != nil || removed != 0 || string(unchanged) != string(body) {
		t.Fatal("unmatched tool filtering changed request")
	}
}

func TestUpstreamBodyForcesChatStreamUsage(t *testing.T) {
	tests := []struct {
		name, protocol, body, model, want string
		includeUsage, reuse               bool
	}{
		{name: "insert", protocol: OpenAIProtocol, model: "alias",
			body: ` {"model":"alias","stream":true,"messages":[]}`,
			want: ` {"stream_options":{"include_usage":true},"model":"alias","stream":true,"messages":[]}`},
		{name: "insert with model rewrite", protocol: OpenAIProtocol, model: "upstream",
			body: `{"model":"alias","stream":true,"messages":[]}`,
			want: `{"stream_options":{"include_usage":true},"model":"upstream","stream":true,"messages":[]}`},
		{name: "null options", protocol: OpenAIProtocol, model: "upstream",
			body: `{"stream":true,"stream_options":null,"model":"alias"}`,
			want: `{"stream":true,"stream_options":{"include_usage":true},"model":"upstream"}`},
		{name: "merge other options", protocol: OpenAIProtocol, model: "alias",
			body: `{"model":"alias","stream":true,"stream_options":{"continuous_usage_stats":true,"include_usage":false}}`,
			want: `{"model":"alias","stream":true,"stream_options":{"continuous_usage_stats":true,"include_usage":true}}`},
		{name: "client requested usage", protocol: OpenAIProtocol, model: "alias", includeUsage: true, reuse: true,
			body: `{"model":"alias","stream":true,"stream_options":{"include_usage":true}}`,
			want: `{"model":"alias","stream":true,"stream_options":{"include_usage":true}}`},
		{name: "non stream", protocol: OpenAIProtocol, model: "alias", reuse: true,
			body: `{"model":"alias","stream":false}`, want: `{"model":"alias","stream":false}`},
		{name: "responses untouched", protocol: OpenAIResponsesProtocol, model: "alias", reuse: true,
			body: `{"model":"alias","stream":true}`, want: `{"model":"alias","stream":true}`},
		{name: "anthropic untouched", protocol: AnthropicProtocol, model: "upstream",
			body: `{"model":"alias","stream":true}`, want: `{"model":"upstream","stream":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(test.body)
			parsed, err := ParseOpenAI(body)
			if err != nil || parsed.IncludeUsage != test.includeUsage {
				t.Fatalf("parse err=%v includeUsage=%v", err, parsed.IncludeUsage)
			}
			got := parsed.upstreamBody(body, test.protocol, test.model)
			if string(got) != test.want {
				t.Fatalf("got %s want %s", got, test.want)
			}
			if test.reuse && &got[0] != &body[0] {
				t.Fatal("unchanged body was copied")
			}
			if _, err := ParseOpenAI(got); err != nil {
				t.Fatalf("rewritten body is invalid: %v", err)
			}
		})
	}
}

func TestParseSkipsUnusedFieldsButStillValidatesThem(t *testing.T) {
	for _, body := range []string{
		`{"model":"a","messages":[{"role":"user","content":"x"}`,
		`{"model":"a","messages":[1,]}`,
	} {
		if _, err := ParseOpenAI([]byte(body)); err == nil {
			t.Fatalf("invalid unused field accepted: %s", body)
		}
	}
	if _, err := ParseOpenAI([]byte(`{"model":"a","messages":null,"metadata":{"k":"v"}}`)); err != nil {
		t.Fatalf("valid body rejected: %v", err)
	}
}
