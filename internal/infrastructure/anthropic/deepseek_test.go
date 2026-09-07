package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
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
		resp, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.deepseek.com/anthropic/"}, gw.Request{Path: path, BetaQuery: true, Body: []byte(`{}`)}, []byte("upstream-secret"))
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

func TestDeepSeekConnectionUsesOfficialModels(t *testing.T) {
	tester := NewConnectionTester()
	var headers http.Header
	tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		headers = r.Header
		if r.URL.String() != "https://api.deepseek.com/models" || r.Header.Get("Authorization") != "Bearer upstream-secret" || r.Header.Get("x-api-key") != "" {
			t.Fatal("wrong account probe")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"deepseek-v4-flash"}]}`))}, nil
	})
	if result := tester.Test(context.Background(), "ANTHROPIC", "https://api.deepseek.com/anthropic", []byte("upstream-secret"), nil); !result.OK {
		t.Fatal(result)
	}
	if headers.Get("Authorization") != "" {
		t.Fatal("completed probe retained credential")
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
		if result := tester.Test(context.Background(), "ANTHROPIC", base, []byte("secret"), nil); result.Code != "UPSTREAM_URL_REJECTED" {
			t.Fatal("unsafe probe URL accepted")
		}
	}
}
