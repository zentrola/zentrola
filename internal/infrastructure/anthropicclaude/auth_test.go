package anthropicclaude

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const testToken = "sk-ant-oat01-test-token"

func TestProbeNormalizesSetupTokenAndReadsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer "+testToken || r.Header.Get("anthropic-beta") != OAuthBeta {
			t.Fatalf("unexpected request: method=%s authorization=%q beta=%q", r.Method, r.Header.Get("Authorization"), r.Header.Get("anthropic-beta"))
		}
		_, _ = w.Write([]byte(`{"subscription_type":"max","five_hour":{"utilization":25},"seven_day":{"utilization":95}}`))
	}))
	defer server.Close()

	adapter := New()
	adapter.client = server.Client()
	adapter.usageEndpoint = server.URL
	probe, err := adapter.Probe(context.Background(), []byte("  "+testToken+"\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(probe.Credential)
	if probe.Inspection.PlanCode != "max" || bytes.Equal(probe.Credential, []byte(testToken)) {
		t.Fatalf("unexpected probe: inspection=%+v credential=%q", probe.Inspection, probe.Credential)
	}
	token, err := RequestCredential(probe.Credential)
	if err != nil || token != testToken {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if len(probe.Quotas) != 3 || probe.Quotas[0].Status != management.QuotaAvailable ||
		probe.Quotas[1].Status != management.QuotaAvailable || probe.Quotas[2].Status != management.QuotaNearLimit {
		t.Fatalf("unexpected quotas: %+v", probe.Quotas)
	}
}

func TestProbeClassifiesUsageFailures(t *testing.T) {
	tests := []struct {
		name, code string
		status     int
		body       string
	}{
		{name: "authentication", code: "UPSTREAM_AUTH_FAILED", status: http.StatusUnauthorized},
		{name: "forbidden", code: "UPSTREAM_AUTH_FAILED", status: http.StatusForbidden},
		{name: "billing", code: "UPSTREAM_BILLING_BLOCKED", status: http.StatusPaymentRequired},
		{name: "rate limited", code: "UPSTREAM_RATE_LIMITED", status: http.StatusTooManyRequests},
		{name: "unavailable", code: "UPSTREAM_UNAVAILABLE", status: http.StatusBadGateway},
		{name: "invalid response", code: "UPSTREAM_INVALID_RESPONSE", status: http.StatusOK, body: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			adapter := New()
			adapter.client = server.Client()
			adapter.usageEndpoint = server.URL

			_, err := adapter.Probe(context.Background(), []byte(testToken), nil)
			var connectionError interface{ ConnectionCode() string }
			if !errors.As(err, &connectionError) || connectionError.ConnectionCode() != tt.code {
				t.Fatalf("error=%v code=%v; want %s", err, connectionError, tt.code)
			}
		})
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
