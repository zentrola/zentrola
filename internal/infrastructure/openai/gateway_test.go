package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestNativeOpenAIForwarding(t *testing.T) {
	c := NewGatewayClient(time.Second)
	calls := 0
	var headers http.Header
	const body = `{"model":"deepseek-v4-flash","tools":[{"function":{"parameters":{"const":9007199254740993}}}],"messages":[{"role":"tool","tool_call_id":"call_a","content":"answer"}]}`
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		headers = r.Header
		if r.URL.String() != "https://api.deepseek.com/chat/completions" || r.Method != "POST" || r.GetBody != nil {
			t.Fatal("incorrect native URL or replay policy")
		}
		if r.Header.Get("Authorization") != "Bearer upstream-only" || r.Header.Get("x-api-key") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("anthropic-version") != "" {
			t.Fatal("credential isolation failed")
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != body {
			t.Fatal("opaque tool content modified")
		}
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"2"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`))}, nil
	})
	resp, err := c.Open(context.Background(), gw.Route{BaseURL: "https://api.deepseek.com/"}, gw.Request{Protocol: gw.OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(body), ProtocolHeaders: map[string][]string{"Cookie": {"private"}, "Authorization": {"Bearer virtual-key"}}}, []byte("upstream-only"))
	if err != nil || resp.Status != 429 || calls != 1 {
		t.Fatal("response changed or retried")
	}
	resp.Body.Close()
	if headers.Get("Authorization") != "" {
		t.Fatal("credential retained after Close")
	}
	for _, base := range []string{"http://api.deepseek.com", "https://127.0.0.1", "https://localhost", "https://api.deepseek.com@evil.test", "https://api.deepseek.com?x=1", "https://api.deepseek.com/path/../escape"} {
		if _, err := c.Open(context.Background(), gw.Route{BaseURL: base}, gw.Request{Protocol: gw.OpenAIProtocol, Path: "/v1/chat/completions"}, []byte("key")); !errors.Is(err, gw.ErrRoute) {
			t.Fatal("untrusted destination accepted")
		}
	}
	if calls != 1 {
		t.Fatal("rejected URL reached transport")
	}
}

func TestResponsesForwarding(t *testing.T) {
	c := NewGatewayClient(time.Second)
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.example.com/v1/responses" {
			t.Fatalf("unexpected Responses URL: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_1"}`))}, nil
	})
	response, err := c.Open(context.Background(), gw.Route{BaseURL: "https://api.example.com/v1"}, gw.Request{
		Protocol: gw.OpenAIResponsesProtocol,
		Path:     "/v1/responses",
		Body:     []byte(`{"model":"system-model","input":"hello"}`),
	}, []byte("upstream-only"))
	if err != nil || response.Status != 200 {
		t.Fatalf("Responses forwarding failed: response=%+v err=%v", response, err)
	}
	response.Body.Close()
}

func TestImageGenerationForwarding(t *testing.T) {
	c := NewGatewayClient(time.Second)
	const body = `{"model":"gpt-image","prompt":"draw an otter","stream":true,"partial_images":2}`
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.example.com/v1/images/generations" || r.Method != http.MethodPost {
			t.Fatalf("unexpected Images URL: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer upstream-only" || r.Header.Get("Accept") != "application/json, text/event-stream" {
			t.Fatalf("unexpected Images headers: %+v", r.Header)
		}
		data, _ := io.ReadAll(r.Body)
		if string(data) != body {
			t.Fatalf("Images request changed: %s", data)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"aW1hZ2U=\"}\n\n")),
		}, nil
	})
	response, err := c.Open(context.Background(), gw.Route{BaseURL: "https://api.example.com/v1"}, gw.Request{
		Protocol: gw.OpenAIImagesProtocol,
		Path:     "/v1/images/generations",
		Body:     []byte(body),
	}, []byte("upstream-only"))
	if err != nil || response.Status != http.StatusOK {
		t.Fatalf("Images forwarding failed: response=%+v err=%v", response, err)
	}
	data, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || !strings.Contains(string(data), `"b64_json":"aW1hZ2U="`) {
		t.Fatalf("Images response changed: %s, %v", data, readErr)
	}
}

func TestImageGenerationRejectsSubscriptionCredential(t *testing.T) {
	c := NewGatewayClient(time.Second)
	_, err := c.Open(context.Background(), gw.Route{AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"}, gw.Request{
		Protocol: gw.OpenAIImagesProtocol,
		Path:     "/v1/images/generations",
		Body:     []byte(`{"model":"gpt-image","prompt":"draw an otter"}`),
	}, []byte("subscription"))
	if !errors.Is(err, gw.ErrRoute) {
		t.Fatalf("Images accepted a subscription credential: %v", err)
	}
}

func TestCodexSubscriptionResponsesForwarding(t *testing.T) {
	c := NewGatewayClient(time.Second)
	var headers http.Header
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		headers = r.Header
		if r.URL.String() != "https://chatgpt.com/backend-api/codex/responses" || r.Method != http.MethodPost {
			t.Fatalf("unexpected subscription URL: %s", r.URL)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer x.") || strings.Contains(r.Header.Get("Authorization"), "refresh") || r.Header.Get("ChatGPT-Account-Id") != "account-1" || r.Header.Get("Originator") != "zentrola" {
			t.Fatalf("unexpected subscription headers: %+v", r.Header)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_1"}`))}, nil
	})
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix())))
	auth := []byte(fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"x.%s.x","refresh_token":"refresh","account_id":"account-1"}}`, payload))
	response, err := c.Open(context.Background(), gw.Route{AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"}, gw.Request{
		Protocol: gw.OpenAIResponsesProtocol, Path: "/v1/responses", Body: []byte(`{"model":"system-model","input":"hello"}`),
	}, auth)
	clear(auth)
	if err != nil || response.Status != 200 {
		t.Fatalf("subscription forwarding failed: response=%+v err=%v", response, err)
	}
	response.Body.Close()
	if headers.Get("Authorization") != "" || headers.Get("ChatGPT-Account-Id") != "" {
		t.Fatal("subscription credential retained after Close")
	}
}

func TestRedirectAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) == "slow" {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Location", "https://example.invalid/leak")
		w.WriteHeader(307)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	c := NewGatewayClient(30 * time.Millisecond)
	defer c.CloseIdleConnections()
	transport := c.client.Transport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: time.Second}).DialContext
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		copy.URL.Scheme = target.Scheme
		copy.URL.Host = target.Host
		return transport.RoundTrip(copy)
	})
	for _, tc := range []struct {
		body string
		err  error
	}{{"redirect", gw.ErrUpstream}, {"slow", gw.ErrTimeout}} {
		_, err := c.Open(context.Background(), gw.Route{BaseURL: "https://api.deepseek.com"}, gw.Request{Protocol: gw.OpenAIProtocol, Path: "/v1/chat/completions", Body: []byte(tc.body)}, []byte("key"))
		if !errors.Is(err, tc.err) {
			t.Fatal("timeout/redirect policy incorrect")
		}
	}
}

func TestGatewayTransportDiagnosticsRespectEnvironment(t *testing.T) {
	for _, test := range []struct {
		environment string
		wantSecret  bool
	}{
		{environment: "dev", wantSecret: true},
		{environment: "test", wantSecret: true},
		{environment: "prod", wantSecret: false},
	} {
		t.Run(test.environment, func(t *testing.T) {
			provider.ConfigureLogEnvironment(test.environment)
			defer provider.ConfigureLogEnvironment("prod")
			var logs bytes.Buffer
			client := NewGatewayClient(time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
			client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("transport failed with credential=network-secret")
			})
			_, err := client.Open(context.Background(), gw.Route{
				BaseURL: "https://api.example.com/v1", ProviderID: 11, ResourceID: 22,
			}, gw.Request{
				Protocol: gw.OpenAIResponsesProtocol, Path: "/v1/responses", RequestID: "req_diagnostic",
			}, []byte("provider-secret"))
			if !errors.Is(err, gw.ErrUpstream) {
				t.Fatalf("unexpected error: %v", err)
			}
			output := logs.String()
			for _, expected := range []string{"provider upstream request failed", "req_diagnostic", `"provider_id":11`, `"resource_id":22`} {
				if !strings.Contains(output, expected) {
					t.Fatalf("missing diagnostic %q: %s", expected, output)
				}
			}
			hasSecret := strings.Contains(output, "network-secret") && strings.Contains(output, "provider-secret")
			if hasSecret != test.wantSecret {
				t.Fatalf("secret logging mismatch: want=%v output=%s", test.wantSecret, output)
			}
			if test.wantSecret && !strings.Contains(output, `"redacted":false`) {
				t.Fatalf("development diagnostic was marked redacted: %s", output)
			}
			if !test.wantSecret && (!strings.Contains(output, `"redacted":true`) || !strings.Contains(output, "******")) {
				t.Fatalf("production diagnostic was not redacted: %s", output)
			}
		})
	}
}
