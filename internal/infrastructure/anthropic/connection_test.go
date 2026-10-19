package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAnthropicConnectionUsesRealInference(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		body, code string
	}{
		{"valid", 200, `{"type":"message","content":[{"type":"text","text":"OK"}]}`, "OK"},
		{"thinking only", 200, `{"type":"message","content":[{"type":"thinking","thinking":"","signature":"sig"}],"stop_reason":"max_tokens"}`, "OK"},
		{"redacted thinking only", 200, `{"type":"message","content":[{"type":"redacted_thinking","data":"encrypted"}]}`, "OK"},
		{"unsigned thinking", 200, `{"type":"message","content":[{"type":"thinking","thinking":""}]}`, "UPSTREAM_INVALID_RESPONSE"},
		{"empty content", 200, `{"type":"message","content":[]}`, "UPSTREAM_INVALID_RESPONSE"},
		{"wrong body", 200, `{"data":[]}`, "UPSTREAM_INVALID_RESPONSE"},
		{"malformed", 200, `{`, "UPSTREAM_INVALID_RESPONSE"},
		{"oversized", 200, strings.Repeat("a", 65537), "UPSTREAM_INVALID_RESPONSE"},
		{"invalid key", 401, `{"error":"invalid key"}`, "UPSTREAM_AUTH_FAILED"},
		{"model denied", 403, `{"error":{"message":"permission denied"}}`, "UPSTREAM_MODEL_UNAVAILABLE"},
		{"billing", 400, `{"error":{"message":"insufficient balance"}}`, "UPSTREAM_BILLING_BLOCKED"},
		{"invalid request", 400, `{"error":{"type":"invalid_request_error","message":"unsupported parameter"}}`, "UPSTREAM_REQUEST_REJECTED"},
		{"expired subscription", 403, `{"error":{"message":"subscription expired"}}`, "UPSTREAM_BILLING_BLOCKED"},
		{"suspended", 403, `{"error":{"message":"account suspended"}}`, "UPSTREAM_ACCOUNT_SUSPENDED"},
		{"rate limit", 429, `{}`, "UPSTREAM_RATE_LIMITED"},
		{"error", 503, `{}`, "UPSTREAM_UNAVAILABLE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tester := NewConnectionTester()
			tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.String() != "https://api.anthropic.com/v1/messages" || r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") != "2023-06-01" ||
					r.Header.Get("anthropic-beta") != "" || r.Header.Get("X-App") != "" || r.Header.Get("User-Agent") == anthropicclaude.ClaudeCodeUserAgent("") {
					t.Fatal("invalid upstream request")
				}
				var payload struct {
					Model       string   `json:"model"`
					MaxTokens   int      `json:"max_tokens"`
					Stream      bool     `json:"stream"`
					Thinking    any      `json:"thinking"`
					Temperature *float64 `json:"temperature"`
					System      any      `json:"system"`
				}
				if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "claude-sonnet-5-5" || payload.MaxTokens != 5 || payload.Stream || payload.Thinking != nil || payload.Temperature != nil || payload.System != nil {
					t.Fatal("invalid inference probe body")
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})
			result := tester.Test(context.Background(), mgmt.ConnectionTarget{Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-sonnet-5-5", AuthType: mgmt.AuthTypeAPIKey}, []byte("test-secret"), nil)
			if result.Code != tt.code || result.OK != (tt.code == "OK") || result.HTTPStatus != tt.status {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestConnectionTestReportsProxyAuthenticationRejection(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxyServer.Close()
	proxyURL := strings.Replace(proxyServer.URL, "://", "://user:wrong-password@", 1)
	tester := NewConnectionTester()
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{
		Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-test", AuthType: mgmt.AuthTypeAPIKey,
	}, []byte("provider-secret"), &catalog.OutboundProxy{URL: proxyURL})
	if result.OK || result.Code != "PROXY_AUTH_REJECTED" {
		t.Fatalf("proxy authentication rejection was not reported: %+v", result)
	}
}

func TestClaudeSubscriptionConnectionUsesOAuthBearer(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://api.anthropic.com/v1/messages" {
			t.Fatalf("unexpected Claude inference request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer sk-ant-oat01-test" || r.Header.Get("x-api-key") != "" ||
			r.Header.Get("anthropic-beta") != anthropicclaude.ClaudeCodeBeta+","+anthropicclaude.OAuthBeta ||
			r.Header.Get("User-Agent") != anthropicclaude.ClaudeCodeUserAgent("") || r.Header.Get("X-App") != "cli" {
			t.Fatalf("unexpected Claude OAuth headers: %+v", r.Header)
		}
		var payload struct {
			Model       string   `json:"model"`
			MaxTokens   int      `json:"max_tokens"`
			Temperature *float64 `json:"temperature"`
			System      []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"system"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Model != "claude-sonnet-5-5" ||
			payload.MaxTokens != 5 || payload.Temperature != nil || len(payload.Messages) != 1 || payload.Messages[0].Content != "Hi, how are you?" ||
			len(payload.System) != 1 || payload.System[0].Type != "text" || payload.System[0].Text != anthropicclaude.ClaudeCodeSystemPrompt {
			t.Fatalf("unexpected Claude inference body: %+v err=%v", payload, err)
		}
		// 上游实测：默认思考模型在 5 个输出 Token 内只返回带签名的空思考块。
		body := `{"type":"message","content":[{"type":"thinking","thinking":"","signature":"sig"}],"stop_reason":"max_tokens"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	// 端点被配置为第三方中转时，订阅 Token 仍只发往 Anthropic 官方主机。
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{
		Protocol: "ANTHROPIC", BaseURL: "https://relay.example.com", UpstreamModelCode: "claude-sonnet-5-5",
		AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode,
	}, []byte(`{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`), nil)
	if !result.OK {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestClaudeSubscriptionConnectionObservesUnifiedRateLimits(t *testing.T) {
	success := `{"type":"message","content":[{"type":"text","text":"OK"}]}`
	tests := []struct {
		name       string
		status     int
		body       string
		headers    map[string]string
		wantOK     bool
		wantQuotas []string
	}{
		{
			name: "successful inference with unified headers", status: 200, body: success, wantOK: true,
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":         "allowed",
				"anthropic-ratelimit-unified-5h-utilization": "0.5",
				"anthropic-ratelimit-unified-5h-reset":       "1792000000",
				"anthropic-ratelimit-unified-7d-utilization": "0.56",
				"anthropic-ratelimit-unified-7d-reset":       "1792500000",
			},
			wantQuotas: []string{"subscription", "five_hour", "seven_day"},
		},
		{
			name: "successful inference without unified headers", status: 200, body: success, wantOK: true,
			wantQuotas: []string{"subscription"},
		},
		{
			name: "rate limited inference keeps rejected window", status: 429,
			body: `{"type":"error","error":{"type":"rate_limit_error","message":"limited"}}`,
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":               "rejected",
				"anthropic-ratelimit-unified-reset":                "1792000000",
				"anthropic-ratelimit-unified-representative-claim": "five_hour",
			},
			wantQuotas: []string{"subscription", "five_hour"},
		},
		{name: "rate limited inference without unified headers", status: 429, body: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tester := NewConnectionTester()
			tester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				header := make(http.Header)
				for name, value := range test.headers {
					header.Set(name, value)
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: header}, nil
			})
			result := tester.Test(context.Background(), mgmt.ConnectionTarget{
				Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-sonnet-5-5",
				AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode,
			}, []byte("sk-ant-oat01-test"), nil)
			if result.OK != test.wantOK || len(result.ObservedQuotas) != len(test.wantQuotas) {
				t.Fatalf("result = %+v quotas=%+v", result, result.ObservedQuotas)
			}
			for index, code := range test.wantQuotas {
				if result.ObservedQuotas[index].Code != code {
					t.Fatalf("quota[%d] = %+v, want %s", index, result.ObservedQuotas[index], code)
				}
			}
		})
	}
}

func TestClaudeQuotaProbeReadsOnlyHeaders(t *testing.T) {
	unified := map[string]string{
		"anthropic-ratelimit-unified-status":         "allowed",
		"anthropic-ratelimit-unified-5h-utilization": "0.01",
		"anthropic-ratelimit-unified-5h-reset":       "1792000000",
	}
	tests := []struct {
		name       string
		target     mgmt.ConnectionTarget
		status     int
		body       string
		headers    map[string]string
		wantCalls  int
		wantOK     bool
		wantCode   string
		wantQuotas int
	}{
		{
			name:   "reply content is not validated",
			target: mgmt.ConnectionTarget{AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode},
			status: 200, body: `not json`, headers: unified,
			wantCalls: 1, wantOK: true, wantCode: "OK", wantQuotas: 2,
		},
		{
			name:   "successful probe without unified headers",
			target: mgmt.ConnectionTarget{AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode},
			status: 200, body: `{}`,
			wantCalls: 1, wantOK: true, wantCode: "OK",
		},
		{
			name:   "rate limited probe keeps quota headers",
			target: mgmt.ConnectionTarget{AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode},
			status: 429, body: `{"type":"error"}`, headers: unified,
			wantCalls: 1, wantCode: "UPSTREAM_RATE_LIMITED", wantQuotas: 2,
		},
		{
			name:     "API key target is rejected",
			target:   mgmt.ConnectionTarget{NetworkScope: "PUBLIC"},
			wantCode: "SUBSCRIPTION_ADAPTER_UNAVAILABLE",
		},
		{
			name:     "Codex subscription target is rejected",
			target:   mgmt.ConnectionTarget{AuthType: mgmt.AuthTypeSubscription, AuthAdapter: "OPENAI_CODEX"},
			wantCode: "SUBSCRIPTION_ADAPTER_UNAVAILABLE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			tester := NewConnectionTester()
			tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				var payload struct {
					MaxTokens int `json:"max_tokens"`
					System    []struct {
						Text string `json:"text"`
					} `json:"system"`
				}
				if r.URL.String() != anthropicclaude.APIBaseURL+"/v1/messages" || r.Header.Get("User-Agent") != anthropicclaude.ClaudeCodeUserAgent("") ||
					json.NewDecoder(r.Body).Decode(&payload) != nil || payload.MaxTokens != 1 ||
					len(payload.System) != 1 || payload.System[0].Text != anthropicclaude.ClaudeCodeSystemPrompt {
					t.Fatalf("unexpected quota probe request: %s %+v", r.URL, payload)
				}
				header := make(http.Header)
				for name, value := range test.headers {
					header.Set(name, value)
				}
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: header}, nil
			})
			target := test.target
			target.Protocol, target.BaseURL, target.UpstreamModelCode, target.QuotaProbe = "ANTHROPIC", "https://api.anthropic.com", "claude-haiku", true
			result := tester.Test(context.Background(), target, []byte("sk-ant-oat01-test"), nil)
			if calls != test.wantCalls || result.OK != test.wantOK || result.Code != test.wantCode || len(result.ObservedQuotas) != test.wantQuotas {
				t.Fatalf("calls=%d result=%+v quotas=%+v", calls, result, result.ObservedQuotas)
			}
		})
	}
}

func TestAPIKeyConnectionIgnoresUnifiedRateLimits(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		header := http.Header{"Anthropic-Ratelimit-Unified-Status": {"allowed"}}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"text","text":"OK"}]}`)), Header: header}, nil
	})
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{
		Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-sonnet-5-5", NetworkScope: "PUBLIC",
	}, []byte("sk-ant-api-test"), nil)
	if !result.OK || result.ObservedQuotas != nil {
		t.Fatalf("API key result = %+v quotas=%+v", result, result.ObservedQuotas)
	}
}

func TestOpenAIConnectionUsesRealInference(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://gateway.example.com/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("x-api-key") != "" {
			t.Fatal("invalid OpenAI-compatible probe")
		}
		var payload struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Thinking  any    `json:"thinking"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "gpt-test" || payload.MaxTokens != 5 || payload.Thinking != nil {
			t.Fatal("invalid inference probe body")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"OK"}}]}`)), Header: make(http.Header)}, nil
	})
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{Protocol: "OPENAI", BaseURL: "https://gateway.example.com/v1", UpstreamModelCode: "gpt-test", AuthType: mgmt.AuthTypeAPIKey}, []byte("test-secret"), nil)
	if !result.OK {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGoogleConnectionUsesNativeGenerateContent(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost ||
			r.URL.String() != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" ||
			r.Header.Get("X-Goog-Api-Key") != "test-secret" || r.Header.Get("Authorization") != "" ||
			r.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid Google probe request: %s %s %+v", r.Method, r.URL.String(), r.Header)
		}
		var payload struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			GenerationConfig struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.Contents) != 1 ||
			payload.Contents[0].Role != "user" || len(payload.Contents[0].Parts) != 1 ||
			payload.Contents[0].Parts[0].Text != "Reply only with OK." ||
			payload.GenerationConfig.MaxOutputTokens != googleProbeMaxOutputTokens {
			t.Fatalf("invalid Google probe body: %+v", payload)
		}
		body := `{"candidates":[{"content":{"parts":[{"text":"OK"}],"role":"model"},"finishReason":"STOP"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{
		ProviderCode:      catalog.GoogleOfficialCode,
		Protocol:          "OPENAI",
		BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai",
		UpstreamModelCode: "gemini-2.5-flash",
		AuthType:          mgmt.AuthTypeAPIKey,
	}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || result.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected Google connection result: %+v", result)
	}
}

func TestConnectionRejectsURLAndHeaderInjection(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid destination or key reached transport")
		return nil, nil
	})
	for _, base := range []string{"http://api.anthropic.com", "http://127.0.0.1", "https://127.0.0.1", "https://localhost", "https://api.anthropic.com@evil.test", "https://api.anthropic.com?target=evil", "https://api.anthropic.com#fragment", "https://api.anthropic.com/path/../escape"} {
		result := tester.Test(context.Background(), mgmt.ConnectionTarget{Protocol: "ANTHROPIC", BaseURL: base, UpstreamModelCode: "model", AuthType: mgmt.AuthTypeAPIKey}, []byte("test-key"), nil)
		if result.Code != "UPSTREAM_URL_REJECTED" {
			t.Fatal("invalid URL accepted")
		}
	}
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "model", AuthType: mgmt.AuthTypeAPIKey}, []byte("key\r\nInjected: value"), nil)
	if result.Code != "CREDENTIAL_INVALID" {
		t.Fatal("invalid credential accepted")
	}
}

func TestConnectionTimeoutCancellationAndRedirect(t *testing.T) {
	var redirectHits atomic.Int32
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectHits.Add(1) }))
	defer redirect.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slow") == "1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Location", redirect.URL)
		w.WriteHeader(302)
	}))
	defer slow.Close()
	targetURL, _ := url.Parse(slow.URL)
	tester := NewConnectionTester()
	original := tester.client.Transport.(*http.Transport).Clone()
	original.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	slowMode := false
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		copy.URL.Scheme = targetURL.Scheme
		copy.URL.Host = targetURL.Host
		if slowMode {
			copy.URL.RawQuery = "slow=1"
		}
		return original.RoundTrip(copy)
	})
	target := mgmt.ConnectionTarget{Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "model", AuthType: mgmt.AuthTypeAPIKey}
	if result := tester.Test(context.Background(), target, []byte("test-key"), nil); result.OK || result.HTTPStatus != 302 || redirectHits.Load() != 0 {
		t.Fatal("credential redirect was followed")
	}
	slowMode = true
	tester.client.Timeout = 30 * time.Millisecond
	if result := tester.Test(context.Background(), target, []byte("test-key"), nil); result.Code != "UPSTREAM_TIMEOUT" {
		t.Fatalf("expected timeout: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := tester.Test(ctx, target, []byte("test-key"), nil); result.Code != "REQUEST_CANCELLED" {
		t.Fatal("cancellation lost")
	}
}
