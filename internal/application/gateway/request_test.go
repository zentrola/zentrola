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
