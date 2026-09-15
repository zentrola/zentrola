package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestDeepSeekNativeEndpoint(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
		client := NewGatewayClient(time.Second)
		client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://api.deepseek.com/anthropic"+path+"?beta=true" || r.Header.Get("x-api-key") != "upstream-secret" || r.Header.Get("Authorization") != "" || r.GetBody != nil {
				t.Fatal("incorrect DeepSeek endpoint, credential or replay policy")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: message_stop\ndata: {}\n\n"))}, nil
		})
		resp, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.deepseek.com/anthropic/"}, gw.Request{Path: path, BetaQuery: true, Body: []byte(`{"model":"upstream"}`)}, []byte("upstream-secret"))
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(data) != "event: message_stop\ndata: {}\n\n" {
			t.Fatal("native SSE modified")
		}
	}
}

func TestDeepSeekRemovesUnsupportedAdvisorCapability(t *testing.T) {
	client := NewGatewayClient(time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		if bytes.Contains(data, []byte(`"type":"advisor_20260301"`)) {
			t.Fatalf("advisor tool reached DeepSeek: %s", data)
		}
		for _, preserved := range [][]byte{[]byte(`"name":"read"`), []byte(`"type":"web_search_20260209"`), []byte(`9007199254740993`)} {
			if !bytes.Contains(data, preserved) {
				t.Fatalf("supported request content lost: %s", data)
			}
		}
		if got := r.Header.Get("anthropic-beta"); got != "other-beta" {
			t.Fatalf("got beta %q, want other-beta", got)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"message"}`))}, nil
	})
	body := []byte(`{"model":"deepseek-v4-flash","future":9007199254740993,"tools":[{"name":"read","input_schema":{"type":"object"}},{"type":"advisor_20260301","name":"advisor","model":"claude-opus-5"},{"type":"web_search_20260209","name":"web_search"}],"messages":[{"role":"user","content":"test"}]}`)
	resp, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.deepseek.com/anthropic"}, gw.Request{
		Path: "/v1/messages", Body: body, Beta: "advisor-tool-2026-03-01,other-beta",
		ProtocolHeaders: map[string][]string{"Anthropic-Beta": {"advisor-tool-2026-03-01, other-beta"}},
	}, []byte("upstream-secret"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestAnthropicPreservesAdvisorCapability(t *testing.T) {
	client := NewGatewayClient(time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		if !bytes.Contains(data, []byte(`"type":"advisor_20260301"`)) || !strings.Contains(r.Header.Get("anthropic-beta"), "advisor-tool-2026-03-01") {
			t.Fatal("native Anthropic advisor capability was modified")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"message"}`))}, nil
	})
	body := []byte(`{"model":"claude-sonnet-5","tools":[{"type":"advisor_20260301","name":"advisor","model":"claude-opus-5"}],"messages":[{"role":"user","content":"test"}]}`)
	resp, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.anthropic.com"}, gw.Request{Path: "/v1/messages", Body: body, Beta: "advisor-tool-2026-03-01"}, []byte("upstream-secret"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestUnknownProviderRemovesAdvisorForSafety(t *testing.T) {
	// 未知服务商默认移除 advisor，避免向不支持的端点发送导致报错
	client := NewGatewayClient(time.Second)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(r.Body)
		if bytes.Contains(data, []byte(`"type":"advisor_20260301"`)) {
			t.Fatalf("advisor tool reached unknown provider: %s", data)
		}
		if !bytes.Contains(data, []byte(`"name":"read"`)) {
			t.Fatalf("other tools were lost: %s", data)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"type":"message"}`))}, nil
	})
	body := []byte(`{"model":"some-model","tools":[{"name":"read","input_schema":{"type":"object"}},{"type":"advisor_20260301","name":"advisor","model":"claude-opus-5"}],"messages":[{"role":"user","content":"test"}]}`)
	resp, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.unknown-provider.com/v1"}, gw.Request{
		Path: "/v1/messages", Body: body, Beta: "advisor-tool-2026-03-01",
	}, []byte("test-key"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestDeepSeekConnectionUsesRealInference(t *testing.T) {
	for _, tt := range []struct {
		name, protocol, baseURL, wantURL, response string
	}{
		{
			name: "anthropic", protocol: "ANTHROPIC", baseURL: "https://api.deepseek.com/anthropic",
			wantURL:  "https://api.deepseek.com/anthropic/v1/messages",
			response: `{"type":"message","content":[{"type":"text","text":"OK"}]}`,
		},
		{
			name: "openai", protocol: "OPENAI", baseURL: "https://api.deepseek.com",
			wantURL:  "https://api.deepseek.com/chat/completions",
			response: `{"choices":[{"message":{"content":"OK"}}]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tester := NewConnectionTester()
			var headers http.Header
			tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				headers = r.Header
				if r.URL.String() != tt.wantURL {
					t.Fatalf("probe URL=%q; want %q", r.URL.String(), tt.wantURL)
				}
				if tt.protocol == "ANTHROPIC" {
					if r.Header.Get("x-api-key") != "upstream-secret" || r.Header.Get("Authorization") != "" {
						t.Fatal("wrong Anthropic inference credential")
					}
				} else if r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("x-api-key") != "" {
					t.Fatal("wrong OpenAI inference credential")
				}
				var payload struct {
					Model     string `json:"model"`
					MaxTokens int    `json:"max_tokens"`
					Thinking  struct {
						Type string `json:"type"`
					} `json:"thinking"`
				}
				if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "deepseek-chat" ||
					payload.MaxTokens != deepSeekProbeMaxOutputTokens || payload.Thinking.Type != "disabled" {
					t.Fatalf("wrong DeepSeek inference body: %+v", payload)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tt.response))}, nil
			})
			target := mgmt.ConnectionTarget{
				ProviderCode: catalog.DeepSeekOfficialCode, Protocol: tt.protocol, BaseURL: tt.baseURL,
				UpstreamModelCode: "deepseek-chat", AuthType: mgmt.AuthTypeAPIKey,
			}
			if result := tester.Test(context.Background(), target, []byte("upstream-secret"), nil); !result.OK {
				t.Fatal(result)
			}
			if headers.Get("Authorization") != "" || headers.Get("x-api-key") != "" {
				t.Fatal("completed probe retained credential")
			}
		})
	}
}

func TestDeepSeekRejectsUntrustedURLs(t *testing.T) {
	client := NewGatewayClient(time.Second)
	tester := NewConnectionTester()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("untrusted URL reached transport")
		return nil, nil
	})
	client.client.Transport = transport
	tester.client.Transport = transport
	for _, base := range []string{"http://api.deepseek.com/anthropic", "https://127.0.0.1/anthropic", "https://localhost/anthropic", "https://api.deepseek.com/anthropic/../", "https://api.deepseek.com@evil.test/anthropic", "https://api.deepseek.com/anthropic?target=evil", "https://api.deepseek.com/anthropic#fragment", "https://api.deepseek.com/anthropic//"} {
		if _, err := client.Open(context.Background(), gw.Route{BaseURL: base}, gw.Request{Path: "/v1/messages"}, []byte("secret")); !errors.Is(err, gw.ErrRoute) {
			t.Fatal("unsafe Gateway URL accepted")
		}
		target := mgmt.ConnectionTarget{Protocol: "ANTHROPIC", BaseURL: base, UpstreamModelCode: "model", AuthType: mgmt.AuthTypeAPIKey}
		if result := tester.Test(context.Background(), target, []byte("secret"), nil); result.Code != "UPSTREAM_URL_REJECTED" {
			t.Fatal("unsafe probe URL accepted")
		}
	}
}
