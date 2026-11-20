package openaicodex

import (
	"errors"
	"strings"
	"testing"
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
