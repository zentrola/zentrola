package openaicodex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func token(payload string) string {
	return "x." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".x"
}

func TestRefreshKeepsFreshAccessToken(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour).Unix()
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + token(`{"exp":`+strconv.FormatInt(expires, 10)+`}`) + `","refresh_token":"refresh","account_id":"account-1"}}`)
	updated, changed, err := New("").RefreshIfNeeded(context.Background(), raw, nil)
	if err != nil || changed || len(updated) != 0 {
		t.Fatalf("updated=%q changed=%v err=%v", updated, changed, err)
	}
}

func TestNeedsRefreshUsesConfiguredAheadWindow(t *testing.T) {
	adapter := New("", WithRefreshAhead(30*time.Minute))
	tests := []struct {
		name       string
		expiresAt  *time.Time
		wantNeeded bool
	}{
		{name: "outside window", expiresAt: timePointer(time.Now().UTC().Add(31 * time.Minute)), wantNeeded: false},
		{name: "inside window", expiresAt: timePointer(time.Now().UTC().Add(29 * time.Minute)), wantNeeded: true},
		{name: "expired", expiresAt: timePointer(time.Now().UTC().Add(-time.Minute)), wantNeeded: true},
		{name: "missing expiration", wantNeeded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessToken := "access"
			if tt.expiresAt != nil {
				accessToken = token(`{"exp":` + strconv.FormatInt(tt.expiresAt.Unix(), 10) + `}`)
			}
			raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + accessToken + `","refresh_token":"refresh","account_id":"account-1"}}`)
			needed, err := adapter.NeedsRefresh(raw)
			if err != nil || needed != tt.wantNeeded {
				t.Fatalf("needed=%v want=%v err=%v", needed, tt.wantNeeded, err)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time { return &value }

func TestInspectAuthCache(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	refreshed := expires.Add(-time.Hour)
	raw := []byte(`{"auth_mode":"chatgpt","last_refresh":"` + refreshed.Format(time.RFC3339) + `","tokens":{"id_token":"` + token(`{"exp":`+strconv.FormatInt(expires.Unix(), 10)+`,"https://api.openai.com/auth":{"chatgpt_plan_type":"plus"}}`) + `","access_token":"` + token(`{"iat":`+strconv.FormatInt(refreshed.Unix(), 10)+`,"exp":`+strconv.FormatInt(expires.Unix(), 10)+`}`) + `","refresh_token":"refresh","account_id":"account-1"}}`)
	inspection, err := New("").Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.AccountRef != "account-1" || inspection.PlanCode != "plus" || inspection.ExpiresAt != nil ||
		inspection.CredentialRefreshedAt == nil || !inspection.CredentialRefreshedAt.Equal(refreshed) ||
		inspection.CredentialExpiresAt == nil || !inspection.CredentialExpiresAt.Equal(expires) {
		t.Fatalf("unexpected inspection: %+v", inspection)
	}
}

func TestExportCredentialAddsMissingLastRefreshFromAccessToken(t *testing.T) {
	issued := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + token(`{"iat":`+strconv.FormatInt(issued.Unix(), 10)+`}`) + `","refresh_token":"refresh","account_id":"account-1"}}`)

	exported, err := New("").ExportCredential(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(exported)
	var document map[string]any
	if err := json.Unmarshal(exported, &document); err != nil {
		t.Fatal(err)
	}
	if document["last_refresh"] != issued.Format(time.RFC3339) {
		t.Fatalf("last_refresh=%v; want %s", document["last_refresh"], issued.Format(time.RFC3339))
	}
}

func TestExportCredentialPreservesExistingAuthFile(t *testing.T) {
	raw := []byte("{\n  \"auth_mode\": \"chatgpt\",\n  \"last_refresh\": \"2026-09-13T01:02:03Z\",\n  \"tokens\": {\"id_token\":\"id\",\"access_token\":\"access\",\"refresh_token\":\"refresh\",\"account_id\":\"account-1\"}\n}\n")

	exported, err := New("").ExportCredential(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(exported)
	if !bytes.Equal(exported, raw) {
		t.Fatalf("exported auth file changed:\n%s", exported)
	}
}

func TestInspectRejectsIncompleteAuthCache(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"auth_mode":"apikey","tokens":{"access_token":"a","refresh_token":"r","account_id":"id"}}`,
		`{"auth_mode":"chatgpt","tokens":{"access_token":"a","account_id":"id"}}`,
	} {
		if _, err := New("").Inspect([]byte(raw)); err == nil {
			t.Fatalf("expected invalid credential for %s", raw)
		}
	}
}

func TestRateLimitsBecomeQuotas(t *testing.T) {
	reset := int64(1730947200)
	duration := int64(900)
	name := "Codex"
	allowed := true
	response := usageResponse{AdditionalRateLimits: []additionalRateLimit{{
		LimitName: &name, MeteredFeature: "codex_other",
		RateLimit: &directRateLimit{Allowed: &allowed, PrimaryWindow: &directRateWindow{
			UsedPercent: 95, LimitWindowSeconds: &duration, ResetAt: &reset,
		}},
	}}}
	quotas := response.quotas(time.Unix(100, 0).UTC())
	if len(quotas) != 1 || quotas[0].Status != "NEAR_LIMIT" || quotas[0].WindowDurationSeconds == nil || *quotas[0].WindowDurationSeconds != 900 {
		t.Fatalf("unexpected quotas: %+v", quotas)
	}
}

func TestProbeReadsUsageDirectly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer access" || r.Header.Get("ChatGPT-Account-Id") != "account-1" {
			t.Fatalf("unexpected request: method=%s authorization=%q account=%q", r.Method, r.Header.Get("Authorization"), r.Header.Get("ChatGPT-Account-Id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"account_id":"account-1","plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":25,"limit_window_seconds":18000,"reset_after_seconds":60}}}`))
	}))
	defer server.Close()

	adapter := New("missing-codex-executable")
	adapter.client = server.Client()
	adapter.usageEndpoint = server.URL
	raw := []byte(`{"auth_mode":"chatgpt","last_refresh":"2026-01-01T00:00:00Z","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	probe, err := adapter.Probe(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(probe.Credential)
	if probe.Inspection.PlanCode != "plus" || probe.Inspection.AccountRef != "account-1" || !bytes.Equal(probe.Credential, raw) {
		t.Fatalf("unexpected probe: %+v", probe.Inspection)
	}
	if len(probe.Quotas) != 1 || probe.Quotas[0].Status != "AVAILABLE" || probe.Quotas[0].ResetsAt == nil {
		t.Fatalf("unexpected quotas: %+v", probe.Quotas)
	}
}

func TestProbeRejectsUsageAccountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"account_id":"account-2","rate_limit":{"allowed":true}}`))
	}))
	defer server.Close()

	adapter := New("")
	adapter.client = server.Client()
	adapter.usageEndpoint = server.URL
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	if _, err := adapter.Probe(context.Background(), raw, nil); err == nil {
		t.Fatal("expected account mismatch")
	}
}

func TestProbeReadsUsageThroughProviderProxy(t *testing.T) {
	requests := make(chan *http.Request, 1)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"account_id":"account-1","plan_type":"plus","rate_limit":{"allowed":true}}`))
	}))
	defer proxyServer.Close()

	adapter := New("missing-codex-executable")
	adapter.usageEndpoint = "http://chatgpt.example/backend-api/wham/usage"
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	probe, err := adapter.Probe(context.Background(), raw, &catalog.OutboundProxy{URL: proxyServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(probe.Credential)
	select {
	case request := <-requests:
		if request.Method != http.MethodGet || request.Host != "chatgpt.example" || request.URL.Path != "/backend-api/wham/usage" {
			t.Fatalf("unexpected proxy request: method=%s host=%s path=%s", request.Method, request.Host, request.URL.Path)
		}
	case <-time.After(time.Second):
		t.Fatal("subscription usage request did not reach provider proxy")
	}
}
