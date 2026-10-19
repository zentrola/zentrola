package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
)

func TestGatewayClientProtocol(t *testing.T) {
	client := NewGatewayClient(time.Second)
	var calls int
	var requestHeaders http.Header
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestHeaders = r.Header
		calls++
		if r.Method != "POST" || r.URL.String() != "https://api.anthropic.com/v1/messages/count_tokens?beta=true" {
			t.Fatal("incorrect native endpoint")
		}
		if r.Header.Get("x-api-key") != "provider-only-key" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("anthropic-beta") != "test-beta" || r.Header.Get("Authorization") != "" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatal("incorrect upstream headers")
		}
		if r.GetBody != nil {
			t.Fatal("POST body must not be replayable")
		}
		if r.Header.Get("Anthropic-Future-Capability") != "opaque-value" || r.Header.Get("Cookie") != "" {
			t.Fatal("native extension Header lost or foreign Header forwarded")
		}
		data, _ := io.ReadAll(r.Body)
		if !bytes.Equal(data, []byte(`{"model":"upstream"}`)) {
			t.Fatal("body modified")
		}
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"5"}}, Body: io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error"}}`))}, nil
	})
	resp, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.anthropic.com"}, gw.Request{Path: "/v1/messages/count_tokens", Beta: "test-beta", BetaQuery: true, Body: []byte(`{"model":"upstream"}`), ProtocolHeaders: map[string][]string{"Anthropic-Future-Capability": {"opaque-value"}, "Cookie": {"must-not-forward"}}}, []byte("provider-only-key"))
	if err != nil || resp.Status != 429 || calls != 1 {
		t.Fatal("native error response changed or retried")
	}
	resp.Body.Close()
	if requestHeaders.Get("x-api-key") != "" {
		t.Fatal("closed HTTP transaction retained credential Header")
	}
	if _, err := client.Open(context.Background(), gw.Route{BaseURL: "http://127.0.0.1"}, gw.Request{Path: "/v1/messages"}, []byte("key")); !errors.Is(err, gw.ErrRoute) {
		t.Fatal("arbitrary host accepted")
	}
	if calls != 1 {
		t.Fatal("invalid destination reached transport")
	}
}

func TestGatewayClientUsesBearerForClaudeSubscription(t *testing.T) {
	client := NewGatewayClient(time.Second)
	var requestHeaders http.Header
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestHeaders = r.Header
		if r.URL.String() != anthropicclaude.APIBaseURL+"/v1/messages" {
			t.Fatalf("Claude subscription token was sent to %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer sk-ant-oat01-test" || r.Header.Get("x-api-key") != "" {
			t.Fatal("Claude subscription did not use Bearer authentication")
		}
		beta := r.Header.Get("anthropic-beta")
		if !strings.Contains(beta, "test-beta") || !strings.Contains(beta, anthropicclaude.OAuthBeta) {
			t.Fatalf("missing OAuth beta header: %q", beta)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"type":"message"}`))}, nil
	})
	credential := []byte(`{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`)
	response, err := client.Open(context.Background(), gw.Route{
		// 端点被配置为第三方中转时，订阅 Token 仍只发往 Anthropic 官方主机。
		BaseURL: "https://relay.example.com", AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode,
	}, gw.Request{Path: "/v1/messages", Beta: "test-beta", Body: []byte(`{}`)}, credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if requestHeaders.Get("Authorization") != "" {
		t.Fatal("closed HTTP transaction retained Authorization header")
	}
}

type subscriptionObserverStub struct {
	routes   []gw.Route
	statuses []int
	headers  []http.Header
}

func (o *subscriptionObserverStub) ObserveResponse(_ context.Context, route gw.Route, status int, header http.Header) {
	o.routes = append(o.routes, route)
	o.statuses = append(o.statuses, status)
	o.headers = append(o.headers, header)
}

func TestGatewayClientObservesClaudeSubscriptionResponses(t *testing.T) {
	tests := []struct {
		name   string
		route  gw.Route
		status int
		want   bool
	}{
		{name: "Claude subscription success", route: gw.Route{ResourceID: 9, BaseURL: "https://api.anthropic.com", NetworkScope: "PUBLIC", AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode}, status: 200, want: true},
		{name: "Claude subscription rate limited", route: gw.Route{ResourceID: 9, BaseURL: "https://api.anthropic.com", NetworkScope: "PUBLIC", AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode}, status: 429, want: true},
		{name: "API key", route: gw.Route{ResourceID: 9, AuthType: "API_KEY", BaseURL: "https://api.anthropic.com", NetworkScope: "PUBLIC"}, status: 200},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewGatewayClient(time.Second)
			observer := &subscriptionObserverStub{}
			client.ObserveSubscriptionResponses(observer)
			client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				header := http.Header{"Anthropic-Ratelimit-Unified-5h-Utilization": {"0.5"}}
				return &http.Response{StatusCode: test.status, Header: header, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})
			credential := []byte("sk-ant-oat01-test")
			response, err := client.Open(context.Background(), test.route, gw.Request{Path: "/v1/messages", Body: []byte(`{}`)}, credential)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if !test.want {
				if len(observer.routes) != 0 {
					t.Fatalf("unexpected observation: %+v", observer.routes)
				}
				return
			}
			if len(observer.routes) != 1 || observer.routes[0].ResourceID != 9 || observer.statuses[0] != test.status ||
				observer.headers[0].Get("anthropic-ratelimit-unified-5h-utilization") != "0.5" {
				t.Fatalf("observations = %+v statuses=%v headers=%v", observer.routes, observer.statuses, observer.headers)
			}
		})
	}
}

func TestGatewayClientTimeoutAndRedirect(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		hits.Add(1)
		if r.URL.Path == "/v1/messages/count_tokens" {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Location", "http://127.0.0.1/credential-leak")
		w.WriteHeader(307)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client := NewGatewayClient(30 * time.Millisecond)
	defer client.CloseIdleConnections()
	transport := client.client.Transport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: time.Second}).DialContext
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		copy.URL.Scheme = target.Scheme
		copy.URL.Host = target.Host
		return transport.RoundTrip(copy)
	})
	_, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.anthropic.com"}, gw.Request{Path: "/v1/messages", Body: []byte(`{}`)}, []byte("key"))
	if !errors.Is(err, gw.ErrUpstream) || hits.Load() != 1 {
		t.Fatal("redirect followed or incorrect failure")
	}
	_, err = client.Open(context.Background(), gw.Route{BaseURL: "https://api.anthropic.com"}, gw.Request{Path: "/v1/messages/count_tokens", Body: []byte(`{}`)}, []byte("key"))
	if !errors.Is(err, gw.ErrTimeout) || hits.Load() != 2 {
		t.Fatal("response header timeout not enforced")
	}
}

func TestGatewayClientClaudeCodeIdentity(t *testing.T) {
	prompt := anthropicclaude.ClaudeCodeSystemPrompt
	identity := `{"type":"text","text":"` + prompt + `"}`
	subscription := gw.Route{BaseURL: "https://api.anthropic.com", AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode}
	subscriptionCredential := []byte(`{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`)
	tests := []struct {
		name       string
		route      gw.Route
		credential []byte
		path       string
		body       string
		want       string
	}{
		{
			name: "system missing", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"claude-sonnet-4-5","messages":[]}`,
			want: `{"system":[` + identity + `],"model":"claude-sonnet-4-5","messages":[]}`,
		},
		{
			name: "empty body object", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{}`,
			want: `{"system":[` + identity + `]}`,
		},
		{
			name: "system null", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system":null}`,
			want: `{"model":"m","system":[` + identity + `]}`,
		},
		{
			name: "system string", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system":"Be brief.","max_tokens":1.0}`,
			want: `{"model":"m","system":[` + identity + `,{"type":"text","text":"Be brief."}],"max_tokens":1.0}`,
		},
		{
			name: "system empty string", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system":""}`,
			want: `{"model":"m","system":[` + identity + `]}`,
		},
		{
			name: "system blocks keep cache_control", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system": [ {"type":"text","text":"Rules","cache_control":{"type":"ephemeral","ttl":"1h"}} ]}`,
			want: `{"model":"m","system": [` + identity + `, {"type":"text","text":"Rules","cache_control":{"type":"ephemeral","ttl":"1h"}} ]}`,
		},
		{
			name: "system empty blocks", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system":[]}`,
			want: `{"model":"m","system":[` + identity + `]}`,
		},
		{
			name: "Claude Code string unchanged", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system":"` + prompt + `\n\nMore."}`,
			want: `{"model":"m","system":"` + prompt + `\n\nMore."}`,
		},
		{
			name: "Claude Code blocks unchanged", route: subscription, credential: subscriptionCredential, path: "/v1/messages",
			body: `{"model":"m","system":[{"type":"text","text":"` + prompt + `","cache_control":{"type":"ephemeral"}},{"type":"text","text":"Rules"}]}`,
			want: `{"model":"m","system":[{"type":"text","text":"` + prompt + `","cache_control":{"type":"ephemeral"}},{"type":"text","text":"Rules"}]}`,
		},
		{
			name: "count_tokens", route: subscription, credential: subscriptionCredential, path: "/v1/messages/count_tokens",
			body: `{"model":"m","system":"Be brief."}`,
			want: `{"model":"m","system":[` + identity + `,{"type":"text","text":"Be brief."}]}`,
		},
		{
			name: "API key route unchanged", route: gw.Route{BaseURL: "https://api.anthropic.com"}, credential: []byte("provider-only-key"), path: "/v1/messages",
			body: `{"model":"m","system":"Be brief."}`,
			want: `{"model":"m","system":"Be brief."}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewGatewayClient(time.Second)
			var body []byte
			var headers http.Header
			client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body, _ = io.ReadAll(r.Body)
				headers = r.Header.Clone()
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})
			response, err := client.Open(context.Background(), tt.route, gw.Request{
				Path: tt.path, Body: []byte(tt.body),
				ProtocolHeaders: map[string][]string{"Anthropic-Beta": {"interleaved-thinking-2025-05-14," + anthropicclaude.ClaudeCodeBeta}},
			}, tt.credential)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if string(body) != tt.want {
				t.Fatalf("body = %s\nwant  %s", body, tt.want)
			}
			if !json.Valid(body) {
				t.Fatalf("invalid JSON body: %s", body)
			}
			beta := headers.Get("anthropic-beta")
			if tt.route.AuthType != "SUBSCRIPTION" {
				if headers.Get("X-App") != "" || strings.HasPrefix(headers.Get("User-Agent"), "claude-cli/") || strings.Contains(beta, anthropicclaude.OAuthBeta) {
					t.Fatalf("API key route received Claude Code identity: %v", headers)
				}
				return
			}
			want := "interleaved-thinking-2025-05-14," + anthropicclaude.ClaudeCodeBeta + "," + anthropicclaude.OAuthBeta
			if beta != want {
				t.Fatalf("anthropic-beta = %q, want %q", beta, want)
			}
			if headers.Get("User-Agent") != anthropicclaude.ClaudeCodeUserAgent || headers.Get("X-App") != "cli" {
				t.Fatalf("missing Claude Code client headers: %v", headers)
			}
		})
	}
}

func TestGatewayClientClaudeCodeIdentityAddsBeta(t *testing.T) {
	client := NewGatewayClient(time.Second)
	var beta string
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		beta = r.Header.Get("anthropic-beta")
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	response, err := client.Open(context.Background(), gw.Route{
		BaseURL: "https://api.anthropic.com", AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode,
	}, gw.Request{Path: "/v1/messages", Body: []byte(`{"model":"m"}`)}, []byte("sk-ant-oat01-test"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if beta != anthropicclaude.ClaudeCodeBeta+","+anthropicclaude.OAuthBeta {
		t.Fatalf("anthropic-beta = %q", beta)
	}
}

func TestGatewayClientRejectsDuplicateSystem(t *testing.T) {
	client := NewGatewayClient(time.Second)
	client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached transport")
		return nil, nil
	})
	_, err := client.Open(context.Background(), gw.Route{
		BaseURL: "https://api.anthropic.com", AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode,
	}, gw.Request{Path: "/v1/messages", Body: []byte(`{"system":"a","system":"b"}`)}, []byte("sk-ant-oat01-test"))
	if !errors.Is(err, gw.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
