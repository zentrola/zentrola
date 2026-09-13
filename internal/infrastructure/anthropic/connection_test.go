package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
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
		{"empty content", 200, `{"type":"message","content":[]}`, "UPSTREAM_INVALID_RESPONSE"},
		{"wrong body", 200, `{"data":[]}`, "UPSTREAM_INVALID_RESPONSE"},
		{"malformed", 200, `{`, "UPSTREAM_INVALID_RESPONSE"},
		{"oversized", 200, strings.Repeat("a", 65537), "UPSTREAM_INVALID_RESPONSE"},
		{"invalid key", 401, `{"error":"invalid key"}`, "UPSTREAM_AUTH_FAILED"},
		{"model denied", 403, `{"error":{"message":"permission denied"}}`, "UPSTREAM_MODEL_UNAVAILABLE"},
		{"billing", 400, `{"error":{"message":"insufficient balance"}}`, "UPSTREAM_BILLING_BLOCKED"},
		{"expired subscription", 403, `{"error":{"message":"subscription expired"}}`, "UPSTREAM_BILLING_BLOCKED"},
		{"suspended", 403, `{"error":{"message":"account suspended"}}`, "UPSTREAM_ACCOUNT_SUSPENDED"},
		{"rate limit", 429, `{}`, "UPSTREAM_RATE_LIMITED"},
		{"error", 503, `{}`, "UPSTREAM_UNAVAILABLE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tester := NewConnectionTester()
			tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost || r.URL.String() != "https://api.anthropic.com/v1/messages" || r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") != "2023-06-01" {
					t.Fatal("invalid upstream request")
				}
				var payload struct {
					Model     string `json:"model"`
					MaxTokens int    `json:"max_tokens"`
					Stream    bool   `json:"stream"`
				}
				if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "claude-test" || payload.MaxTokens != 5 || payload.Stream {
					t.Fatal("invalid inference probe body")
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})
			result := tester.Test(context.Background(), mgmt.ConnectionTarget{Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-test", AuthType: mgmt.AuthTypeAPIKey}, []byte("test-secret"), nil)
			if result.Code != tt.code || result.OK != (tt.code == "OK") || result.HTTPStatus != tt.status {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestClaudeSubscriptionConnectionUsesOAuthBearer(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer sk-ant-oat01-test" || r.Header.Get("x-api-key") != "" ||
			r.Header.Get("anthropic-beta") != anthropicclaude.OAuthBeta {
			t.Fatalf("unexpected Claude OAuth headers: %+v", r.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"text","text":"OK"}]}`)), Header: make(http.Header)}, nil
	})
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{
		Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-test",
		AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode,
	}, []byte(`{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`), nil)
	if !result.OK {
		t.Fatalf("unexpected result: %+v", result)
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
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "gpt-test" || payload.MaxTokens != 5 {
			t.Fatal("invalid inference probe body")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"OK"}}]}`)), Header: make(http.Header)}, nil
	})
	result := tester.Test(context.Background(), mgmt.ConnectionTarget{Protocol: "OPENAI", BaseURL: "https://gateway.example.com/v1", UpstreamModelCode: "gpt-test", AuthType: mgmt.AuthTypeAPIKey}, []byte("test-secret"), nil)
	if !result.OK {
		t.Fatalf("unexpected result: %+v", result)
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
	original := tester.client.Transport
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
