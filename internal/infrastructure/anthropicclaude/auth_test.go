package anthropicclaude

import (
	"context"
	"errors"
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
