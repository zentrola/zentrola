package anthropicclaude

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const testToken = "sk-ant-oat01-test-token"

func TestProbeDoesNotReadUsageWithSetupToken(t *testing.T) {
	_, err := New().Probe(context.Background(), []byte(testToken), nil)
	var connectionError interface{ ConnectionCode() string }
	if err == nil || errors.As(err, &connectionError) {
		t.Fatalf("Probe error=%v; want unsupported error without connection code", err)
	}
}

func TestRequestCredentialAcceptsRawAndStoredSetupToken(t *testing.T) {
	for _, raw := range []string{"  " + testToken + "\n", `{"kind":"claude_code_setup_token","access_token":"` + testToken + `"}`} {
		token, err := RequestCredential([]byte(raw))
		if err != nil || token != testToken {
			t.Fatalf("RequestCredential(%q) = %q, %v", raw, token, err)
		}
	}
}

func TestAcceptsOpaqueSetupTokenWithoutAssumingAnthropicPrefix(t *testing.T) {
	for _, raw := range []string{
		"future-token-format",
		"sk-ant-api03-test",
		`{"kind":"claude_code_setup_token","access_token":"future-token-format"}`,
	} {
		if _, err := New().Inspect([]byte(raw)); err != nil {
			t.Fatalf("expected opaque credential %q to be accepted: %v", raw, err)
		}
	}
}

func TestRejectsMalformedSetupToken(t *testing.T) {
	for _, raw := range []string{"", " ", "token with spaces", "token\x7f", `{"kind":"claude_code_setup_token","access_token":"bad token"}`} {
		if _, err := New().Inspect([]byte(raw)); err == nil {
			t.Fatalf("expected invalid credential for %q", raw)
		}
	}
}

func TestSupportsOnlyAnthropicOfficial(t *testing.T) {
	adapter := New()
	if !adapter.SupportsProvider(management.Provider{Code: catalog.AnthropicOfficialCode}) ||
		adapter.SupportsProvider(management.Provider{Code: catalog.OpenAIOfficialCode}) {
		t.Fatal("unexpected provider support")
	}
}

func TestClaudeCodeUserAgent(t *testing.T) {
	defaultAgent := "claude-cli/" + DefaultClaudeCodeVersion + " (external, cli)"
	for _, tt := range []struct{ version, want string }{
		{"", defaultAgent},
		{"2.1.300", "claude-cli/2.1.300 (external, cli)"},
		{"10.0.1", "claude-cli/10.0.1 (external, cli)"},
		{"2.1", defaultAgent},
		{"v2.1.300", defaultAgent},
		{"2.1.300-beta", defaultAgent},
		{"2.1.300\r\nX-Injected: 1", defaultAgent},
	} {
		if got := ClaudeCodeUserAgent(tt.version); got != tt.want {
			t.Errorf("ClaudeCodeUserAgent(%q) = %q, want %q", tt.version, got, tt.want)
		}
	}
	if !ValidClaudeCodeVersion(DefaultClaudeCodeVersion) {
		t.Fatalf("default version %q must be valid", DefaultClaudeCodeVersion)
	}
}

func TestUpstreamError(t *testing.T) {
	long := strings.Repeat("长", maxUpstreamErrorMessage+10)
	for _, tt := range []struct {
		name, body, wantType, wantMessage string
	}{
		{"anthropic error", `{"type":"error","error":{"type":"rate_limit_error","message":" Error "}}`, "rate_limit_error", "Error"},
		{"truncated message", `{"error":{"type":"invalid_request_error","message":"` + long + `"}}`, "invalid_request_error", strings.Repeat("长", maxUpstreamErrorMessage) + "…"},
		{"not JSON", `<html>bad gateway</html>`, "", ""},
		{"empty", ``, "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			errorType, message := UpstreamError([]byte(tt.body))
			if errorType != tt.wantType || message != tt.wantMessage {
				t.Fatalf("UpstreamError() = %q, %q; want %q, %q", errorType, message, tt.wantType, tt.wantMessage)
			}
		})
	}
}
