package openaicodex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestSanitizeDiagnostic(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      []string
		forbidden []string
	}{
		{
			name:  "keeps ordinary upstream reason",
			input: "codex app-server request failed (-32603): unexpected status 401 Unauthorized",
			want:  []string{"unexpected status 401 Unauthorized", "-32603"},
		},
		{
			name:      "redacts jwt",
			input:     "token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl rejected",
			want:      []string{"token ****** rejected"},
			forbidden: []string{"eyJhbGci", "c2lnbmF0dXJl"},
		},
		{
			name:      "redacts bearer token",
			input:     "Authorization: Bearer abc.def-ghi",
			forbidden: []string{"abc.def-ghi"},
		},
		{
			name:      "redacts json secret fields",
			input:     `{"refresh_token":"rt-secret-value","access_token": "at-secret"}`,
			want:      []string{`"refresh_token":"******"`},
			forbidden: []string{"rt-secret-value", "at-secret"},
		},
		{
			name:      "redacts proxy userinfo and query",
			input:     "proxy http://alice:pa55@proxy.example:8080 failed; GET https://chatgpt.com/backend-api/wham?token=abc",
			want:      []string{"http://******@proxy.example:8080", "https://chatgpt.com/backend-api/wham?******"},
			forbidden: []string{"alice", "pa55", "token=abc"},
		},
		{
			name:      "redacts email and api keys",
			input:     "account user@example.com key sk-proj-ABCDEFGHIJKLMNOP1234",
			forbidden: []string{"user@example.com", "sk-proj-ABCDEFGHIJKLMNOP1234"},
		},
		{
			name:      "redacts long opaque tokens but keeps identifiers",
			input:     "rt_" + strings.Repeat("a1", 30) + " codex_app_server_protocol_rate_limits_read",
			want:      []string{"codex_app_server_protocol_rate_limits_read"},
			forbidden: []string{strings.Repeat("a1", 30)},
		},
		{
			name:      "strips terminal escapes",
			input:     "\x1b[31mERROR\x1b[0m failed\r",
			want:      []string{"ERROR failed"},
			forbidden: []string{"\x1b", "\r"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeDiagnostic(tt.input)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("got %q, want substring %q", got, want)
				}
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(got, forbidden) {
					t.Fatalf("got %q, must not contain %q", got, forbidden)
				}
			}
		})
	}
}

func TestErrorForLogTruncatesToSingleLine(t *testing.T) {
	got := errorForLog(errors.New("first\nsecond " + strings.Repeat("x ", maxErrorLogRunes)))
	if strings.Contains(got, "\n") || !strings.HasPrefix(got, "first second") || len([]rune(got)) != maxErrorLogRunes+1 {
		t.Fatalf("unexpected log text %q", got)
	}
}

func TestStderrTailKeepsBoundedSuffix(t *testing.T) {
	tests := []struct {
		name      string
		writes    []string
		want      string
		forbidden string
	}{
		{name: "empty", want: ""},
		{name: "keeps last lines", writes: []string{"one\n\n", "two\nthree\n"}, want: "one\ntwo\nthree"},
		{
			name:      "drops partial first line after truncation",
			writes:    []string{"secret-" + strings.Repeat("y", maxStderrCapture), "\nlast reason\n"},
			want:      "last reason",
			forbidden: "secret-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tail stderrTail
			for _, value := range tt.writes {
				if n, err := tail.Write([]byte(value)); err != nil || n != len(value) {
					t.Fatalf("n=%d err=%v", n, err)
				}
			}
			if len(tail.buf) > maxStderrCapture {
				t.Fatalf("buffer grew to %d bytes", len(tail.buf))
			}
			got := tail.summary()
			if got != tt.want || (tt.forbidden != "" && strings.Contains(got, tt.forbidden)) {
				t.Fatalf("summary=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestProbeLogsSanitizedFailure(t *testing.T) {
	var logs bytes.Buffer
	adapter := New("", WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))))
	adapter.runSession = func(context.Context, []byte, *catalog.OutboundProxy, func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
		return nil, mgmt.SubscriptionInspection{}, &sessionFailure{
			cause:  errors.New("codex app-server request failed (-32603): unexpected status 401 Unauthorized; Bearer leaked-token"),
			stderr: "ERROR refresh failed",
		}
	}
	_, err := adapter.Probe(context.Background(), []byte("{}"), nil)
	var connectionError mgmt.SubscriptionConnectionError
	if !errors.As(err, &connectionError) || connectionError.ConnectionCode() != "SUBSCRIPTION_UNAVAILABLE" {
		t.Fatalf("unexpected error: %v", err)
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("log=%q err=%v", logs.String(), err)
	}
	if entry["level"] != "WARN" || entry["error_code"] != "SUBSCRIPTION_UNAVAILABLE" ||
		entry["app_server_stderr"] != "ERROR refresh failed" ||
		!strings.Contains(entry["error"].(string), "unexpected status 401 Unauthorized") {
		t.Fatalf("unexpected log entry %v", entry)
	}
	if strings.Contains(logs.String(), "leaked-token") {
		t.Fatalf("log leaked credential: %s", logs.String())
	}
}

func TestProbeCapturesAppServerStderr(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	adapter := New(executable, WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))))
	expires := time.Now().UTC().Add(time.Hour).Unix()
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + token(`{"exp":`+strconv.FormatInt(expires, 10)+`}`) + `","refresh_token":"refresh","account_id":"account-1"}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = adapter.Probe(ctx, raw, nil)
	var failure *sessionFailure
	if !errors.As(err, &failure) || !strings.Contains(failure.stderr, "token refresh failed: Bearer ******") {
		t.Fatalf("stderr summary was not attached: %v", err)
	}
	if !strings.Contains(err.Error(), "unexpected status 401 Unauthorized") {
		t.Fatalf("unexpected error text: %v", err)
	}
	if strings.Contains(logs.String(), "eyJhbGci") || !strings.Contains(logs.String(), "codex subscription probe failed") {
		t.Fatalf("unexpected log output: %s", logs.String())
	}
}
