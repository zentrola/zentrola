package anthropicclaude

import (
	"bytes"
	"context"
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

func TestRejectsNonSetupToken(t *testing.T) {
	for _, raw := range []string{"", "sk-ant-oat", "sk-ant-api03-test", `{"kind":"claude_code_setup_token","access_token":"bad"}`} {
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
