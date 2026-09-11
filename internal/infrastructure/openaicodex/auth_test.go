package openaicodex

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func token(payload string) string {
	return "x." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".x"
}

func TestRefreshKeepsFreshAccessToken(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour).Unix()
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + token(`{"exp":`+strconv.FormatInt(expires, 10)+`}`) + `","refresh_token":"refresh","account_id":"account-1"}}`)
	updated, changed, err := New("").RefreshIfNeeded(context.Background(), raw)
	if err != nil || changed || len(updated) != 0 {
		t.Fatalf("updated=%q changed=%v err=%v", updated, changed, err)
	}
}

func TestInspectAuthCache(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"` + token(`{"exp":`+strconv.FormatInt(expires.Unix(), 10)+`,"https://api.openai.com/auth":{"chatgpt_plan_type":"plus"}}`) + `","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	inspection, err := New("").Inspect(raw)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.AccountRef != "account-1" || inspection.PlanCode != "plus" || inspection.ExpiresAt != nil {
		t.Fatalf("unexpected inspection: %+v", inspection)
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
	raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
	probe, err := adapter.Probe(context.Background(), raw)
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
	if _, err := adapter.Probe(context.Background(), raw); err == nil {
		t.Fatal("expected account mismatch")
	}
}
