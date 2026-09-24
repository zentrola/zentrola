package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAccessLogHeadersForEnvironmentRedactsSensitiveHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization":   {"Bearer secret"},
		"Cookie":          {"session=secret"},
		"X-Api-Key":       {"api-secret"},
		"X-Request-ID":    {"req-1"},
		"X-Custom-Header": {strings.Repeat("x", accessLogHeaderValueLimit+10)},
		"X-Auth-Token":    {"token-secret"},
	}
	got := accessLogHeadersForEnvironment(headers, requestHeaderAllowlist, true)
	for _, name := range []string{"authorization", "cookie", "x-api-key", "x-auth-token"} {
		if got[name] != "******" {
			t.Fatalf("header %s was not redacted: %q", name, got[name])
		}
	}
	if got["x-request-id"] != "req-1" {
		t.Fatalf("safe header was unexpectedly redacted: %q", got["x-request-id"])
	}
	if len([]rune(got["x-custom-header"])) != accessLogHeaderValueLimit+3 {
		t.Fatalf("header was not bounded: length=%d", len([]rune(got["x-custom-header"])))
	}
}

func TestBodyCaptureBoundsAndRedactsJSON(t *testing.T) {
	capture := &bodyCapture{}
	payload := `{"password":"secret","nested":{"access_token":"token"},"proxyHeaders":{"Authorization":"secret"},"safe":"ok"}`
	if n, err := capture.Write([]byte(payload)); err != nil || n != len(payload) {
		t.Fatalf("capture write = %d, %v", n, err)
	}
	logged, ok := bodyLogValue(capture).(json.RawMessage)
	if !ok {
		t.Fatalf("expected JSON log body, got %T", bodyLogValue(capture))
	}
	text := string(logged)
	if strings.Contains(text, "secret") || !strings.Contains(text, "\"safe\":\"ok\"") {
		t.Fatalf("sensitive JSON was not redacted: %s", text)
	}

	large := &bodyCapture{}
	data := []byte(strings.Repeat("x", accessLogBodyLimit+10))
	if n, err := large.Write(data); err != nil || n != len(data) {
		t.Fatalf("large capture write = %d, %v", n, err)
	}
	if large.total != int64(len(data)) || large.data.Len() != accessLogBodyLimit || !large.truncated {
		t.Fatalf("unexpected bounded capture: total=%d stored=%d truncated=%v", large.total, large.data.Len(), large.truncated)
	}
}
