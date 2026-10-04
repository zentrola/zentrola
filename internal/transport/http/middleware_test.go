package http

import (
	"net/http"
	"strings"
	"testing"
)

func TestSafeAccessLogHeadersRedactsUnknownHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization":   {"Bearer secret"},
		"Cookie":          {"session=secret"},
		"X-Api-Key":       {"api-secret"},
		"X-Request-ID":    {"req-1"},
		"X-Custom-Header": {strings.Repeat("x", accessLogHeaderValueLimit+10)},
		"X-Auth-Token":    {"token-secret"},
	}
	got := safeAccessLogHeaders(headers, requestHeaderAllowlist)
	for _, name := range []string{"authorization", "cookie", "x-api-key", "x-auth-token"} {
		if got[name] != "******" {
			t.Fatalf("header %s was not redacted: %q", name, got[name])
		}
	}
	if got["x-request-id"] != "******" {
		t.Fatalf("untrusted request ID was not redacted: %q", got["x-request-id"])
	}
	if got["x-custom-header"] != "******" {
		t.Fatalf("unknown header was not redacted: %q", got["x-custom-header"])
	}
}
