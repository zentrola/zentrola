package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConnectionProtocol(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		body, code string
	}{
		{"valid", 200, `{"data":[{"id":"model"}]}`, "OK"},
		{"empty list", 200, `{"data":[]}`, "OK"},
		{"wrong body", 200, `{"token":"test-secret"}`, "UPSTREAM_INVALID_RESPONSE"},
		{"malformed", 200, `{`, "UPSTREAM_INVALID_RESPONSE"},
		{"oversized", 200, strings.Repeat("a", 65537), "UPSTREAM_INVALID_RESPONSE"},
		{"invalid key", 401, `{"error":"test-secret"}`, "UPSTREAM_AUTH_FAILED"},
		{"denied", 403, "", "UPSTREAM_AUTH_FAILED"},
		{"rate limit", 429, "", "UPSTREAM_RATE_LIMITED"},
		{"error", 503, "", "UPSTREAM_UNAVAILABLE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tester := NewConnectionTester()
			tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.String() != "https://api.anthropic.com/v1/models?limit=1" || r.Header.Get("x-api-key") != "test-secret" || r.Header.Get("anthropic-version") != "2023-06-01" {
					t.Error("invalid upstream request")
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})
			result := tester.Test(context.Background(), "https://api.anthropic.com", []byte("test-secret"))
			if result.Code != tt.code || result.OK != (tt.code == "OK") || result.HTTPStatus != tt.status {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestConnectionRejectsURLAndHeaderInjection(t *testing.T) {
	tester := NewConnectionTester()
	tester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid destination or key reached transport")
		return nil, nil
	})
	for _, base := range []string{"http://api.anthropic.com", "https://api.anthropic.com.evil.test", "http://127.0.0.1", "https://api.anthropic.com@evil.test", "https://api.anthropic.com/path"} {
		if result := tester.Test(context.Background(), base, []byte("test-key")); result.Code != "UPSTREAM_URL_REJECTED" {
			t.Fatal("invalid URL accepted")
		}
	}
	if result := tester.Test(context.Background(), "https://api.anthropic.com", []byte("key\r\nInjected: value")); result.Code != "CREDENTIAL_INVALID" {
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
	target, _ := url.Parse(slow.URL)
	tester := NewConnectionTester()
	original := tester.client.Transport
	slowMode := false
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		copy.URL.Scheme = target.Scheme
		copy.URL.Host = target.Host
		if slowMode {
			copy.URL.RawQuery = "slow=1"
		}
		return original.RoundTrip(copy)
	})
	if result := tester.Test(context.Background(), "https://api.anthropic.com", []byte("test-key")); result.OK || result.HTTPStatus != 302 || redirectHits.Load() != 0 {
		t.Fatal("credential redirect was followed")
	}
	slowMode = true
	tester.client.Timeout = 30 * time.Millisecond
	if result := tester.Test(context.Background(), "https://api.anthropic.com", []byte("test-key")); result.Code != "UPSTREAM_TIMEOUT" {
		t.Fatalf("expected timeout: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := tester.Test(ctx, "https://api.anthropic.com", []byte("test-key")); result.Code != "REQUEST_CANCELLED" {
		t.Fatal("cancellation lost")
	}
}
