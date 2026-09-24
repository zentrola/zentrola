package openaicodex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
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

func TestRefreshRejectsInvalidCredentialAsPermanent(t *testing.T) {
	_, _, err := New("").RefreshIfNeeded(context.Background(), []byte("invalid"), nil)
	var connectionError mgmt.SubscriptionConnectionError
	if !errors.As(err, &connectionError) || connectionError.ConnectionCode() != "CREDENTIAL_INVALID" {
		t.Fatalf("unexpected error classification: %v", err)
	}
	if code, permanent := New("").ClassifyRefreshError(err); code != "CREDENTIAL_INVALID" || !permanent {
		t.Fatalf("code=%q permanent=%v", code, permanent)
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
	duration := int64(15)
	name := "Codex"
	response := rateLimitsResponse{RateLimitsByLimitID: map[string]codexRateLimit{
		"codex_other": {LimitID: "codex_other", LimitName: &name, Primary: &rateLimitWindow{
			UsedPercent: 95, WindowDurationMins: &duration, ResetsAt: &reset,
		}},
	}}
	quotas := response.quotas(time.Unix(100, 0).UTC())
	if len(quotas) != 1 || quotas[0].Status != "NEAR_LIMIT" || quotas[0].WindowDurationSeconds == nil || *quotas[0].WindowDurationSeconds != 900 {
		t.Fatalf("unexpected quotas: %+v", quotas)
	}
}

type accountRPCFunc func(context.Context, int64, string, any, any) error

func (f accountRPCFunc) call(ctx context.Context, id int64, method string, params, target any) error {
	return f(ctx, id, method, params, target)
}

func TestProbeReadsOfficialRateLimitsAndResetCredits(t *testing.T) {
	reset := int64(1730947200)
	duration := int64(300)
	expires := int64(1730950800)
	title := "Rate-limit reset"
	adapter := New("missing-codex-executable")
	adapter.runSession = func(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
		if proxy != nil {
			t.Fatal("unexpected proxy")
		}
		rpc := accountRPCFunc(func(_ context.Context, id int64, method string, params, target any) error {
			if id != 2 || method != "account/rateLimits/read" || params != nil {
				t.Fatalf("unexpected RPC: id=%d method=%s params=%v", id, method, params)
			}
			result := target.(*rateLimitsResponse)
			result.RateLimits = &codexRateLimit{LimitID: "codex", PlanType: "plus", Primary: &rateLimitWindow{
				UsedPercent: 25, WindowDurationMins: &duration, ResetsAt: &reset,
			}}
			result.RateLimitResetCredits = &rateLimitResetCreditsWire{AvailableCount: 1, Credits: []rateLimitResetCreditWire{{
				ID: "credit-1", ResetType: "codexRateLimits", Status: "available", GrantedAt: reset - 60, ExpiresAt: &expires, Title: &title,
			}}}
			return nil
		})
		if err := action(rpc); err != nil {
			return nil, mgmt.SubscriptionInspection{}, err
		}
		return bytes.Clone(raw), mgmt.SubscriptionInspection{AccountRef: "account-1"}, nil
	}
	raw := []byte(`{"auth_mode":"chatgpt","last_refresh":"2026-01-01T00:00:00Z","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	probe, err := adapter.Probe(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(probe.Credential)
	if probe.Inspection.PlanCode != "plus" || probe.Inspection.AccountRef != "account-1" || !bytes.Equal(probe.Credential, raw) {
		t.Fatalf("unexpected probe: %+v", probe.Inspection)
	}
	if len(probe.Quotas) != 1 || probe.Quotas[0].Status != "AVAILABLE" || probe.Quotas[0].ResetsAt == nil ||
		probe.ResetCredits == nil || probe.ResetCredits.AvailableCount != 1 || len(probe.ResetCredits.Credits) != 1 ||
		probe.ResetCredits.Credits[0].ID != "credit-1" || probe.ResetCredits.Credits[0].ExpiresAt == nil {
		t.Fatalf("unexpected quotas: %+v", probe.Quotas)
	}
}

func TestProbePassesProviderProxyToOfficialAppServer(t *testing.T) {
	adapter := New("missing-codex-executable")
	proxySeen := false
	adapter.runSession = func(_ context.Context, raw []byte, proxy *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
		proxySeen = proxy != nil && proxy.URL == "http://proxy.example:8080"
		rpc := accountRPCFunc(func(_ context.Context, _ int64, _ string, _ any, target any) error {
			target.(*rateLimitsResponse).RateLimits = &codexRateLimit{LimitID: "codex"}
			return nil
		})
		if err := action(rpc); err != nil {
			return nil, mgmt.SubscriptionInspection{}, err
		}
		return bytes.Clone(raw), mgmt.SubscriptionInspection{}, nil
	}
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	probe, err := adapter.Probe(context.Background(), raw, &catalog.OutboundProxy{URL: "http://proxy.example:8080"})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(probe.Credential)
	if !proxySeen {
		t.Fatal("subscription rate-limit request did not receive provider proxy")
	}
}

func TestProbeClassifiesMissingAppServerExecutable(t *testing.T) {
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	_, err := New(filepath.Join(t.TempDir(), "missing-codex.exe")).Probe(context.Background(), raw, nil)
	var connectionError mgmt.SubscriptionConnectionError
	if !errors.As(err, &connectionError) || connectionError.ConnectionCode() != "CODEX_APP_SERVER_UNAVAILABLE" {
		t.Fatalf("unexpected error classification: %v", err)
	}
}

func TestConsumeResetCreditRefreshesOfficialRateLimits(t *testing.T) {
	adapter := New("missing-codex-executable")
	var methods []string
	adapter.runSession = func(_ context.Context, raw []byte, _ *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
		rpc := accountRPCFunc(func(_ context.Context, _ int64, method string, params, target any) error {
			methods = append(methods, method)
			switch method {
			case "account/rateLimitResetCredit/consume":
				values := params.(map[string]string)
				if values["idempotencyKey"] != "request-1" || values["creditId"] != "credit-1" {
					t.Fatalf("unexpected consume params: %v", values)
				}
				target.(*struct {
					Outcome string `json:"outcome"`
				}).Outcome = "reset"
			case "account/rateLimits/read":
				target.(*rateLimitsResponse).RateLimitResetCredits = &rateLimitResetCreditsWire{AvailableCount: 0}
			}
			return nil
		})
		if err := action(rpc); err != nil {
			return nil, mgmt.SubscriptionInspection{}, err
		}
		return bytes.Clone(raw), mgmt.SubscriptionInspection{}, nil
	}
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	result, err := adapter.ConsumeResetCredit(context.Background(), raw, nil, "request-1", "credit-1")
	if err != nil || result.Outcome != "reset" || result.Probe.ResetCredits == nil || result.Probe.ResetCredits.AvailableCount != 0 ||
		len(methods) != 2 || methods[0] != "account/rateLimitResetCredit/consume" || methods[1] != "account/rateLimits/read" {
		t.Fatalf("unexpected consume result: %+v methods=%v err=%v", result, methods, err)
	}
	clear(result.Probe.Credential)
}
