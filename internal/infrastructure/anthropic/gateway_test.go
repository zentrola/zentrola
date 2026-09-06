package anthropic

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
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
	transport := client.client.Transport
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
