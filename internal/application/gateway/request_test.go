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
