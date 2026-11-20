package openaicodex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func accountTestCredential() []byte {
	expires := strconv.FormatInt(time.Now().UTC().Add(time.Hour).Unix(), 10)
	return []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + token(`{"exp":`+expires+`}`) + `","refresh_token":"refresh","account_id":"account-1"}}`)
}

const testUsage = `{"account_id":"account-1","plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":25,"limit_window_seconds":18000,"reset_at":1800000000},"secondary_window":{"used_percent":95,"limit_window_seconds":604800,"reset_at":1800010000}},"additional_rate_limits":[{"limit_name":"Review","metered_feature":"codex_review","rate_limit":{"primary_window":{"used_percent":100,"limit_window_seconds":3600,"reset_at":1800001000}}}],"rate_limit_reset_credits":{"available_count":1}}`

const testCards = `{"available_count":1,"credits":[{"id":"credit-1","reset_type":"codex_rate_limits","status":"available","granted_at":"2026-11-20T01:02:03Z","expires_at":"2026-12-20T01:02:03Z","title":"Reset","description":"One reset"}]}`

func TestProbeReadsAccountHTTPWithoutAppServer(t *testing.T) {
	var paths []string
	raw := accountTestCredential()
	cache, err := parseCredential(raw)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+cache.Tokens.AccessToken || r.Header.Get("ChatGPT-Account-Id") != "account-1" {
			t.Error("missing account authorization headers")
		}
		switch r.URL.Path {
		case "/usage":
			_, _ = io.WriteString(w, testUsage)
		case "/rate-limit-reset-credits":
			_, _ = io.WriteString(w, testCards)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	adapter := New("missing-codex-executable")
	adapter.backendURL = server.URL
	adapter.refreshHTTP = server.Client()
	probe, err := adapter.Probe(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "GET /usage,GET /rate-limit-reset-credits" {
		t.Fatalf("paths=%v", paths)
	}
	if probe.Inspection.PlanCode != "plus" || len(probe.Quotas) != 3 || probe.ResetCredits == nil || len(probe.ResetCredits.Credits) != 1 {
		t.Fatalf("unexpected probe: plan=%q quotas=%+v cards=%+v", probe.Inspection.PlanCode, probe.Quotas, probe.ResetCredits)
	}
	if probe.Quotas[0].WindowDurationSeconds == nil || *probe.Quotas[0].WindowDurationSeconds != 18000 || probe.Quotas[1].Status != mgmt.QuotaNearLimit {
		t.Fatalf("unexpected quota mapping: %+v", probe.Quotas)
	}
	card := probe.ResetCredits.Credits[0]
	if card.ID != "credit-1" || card.ResetType != "codexRateLimits" || !card.GrantedAt.Equal(time.Date(2026, 11, 20, 1, 2, 3, 0, time.UTC)) || card.ExpiresAt == nil {
		t.Fatalf("unexpected card mapping: %+v", card)
	}
}

func TestProbeFallsBackToUsageCardCount(t *testing.T) {
	for _, tt := range []struct {
		name, response string
		status         int
	}{
		{name: "detail unavailable", status: http.StatusServiceUnavailable},
		{name: "detail missing count", status: http.StatusOK, response: `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/usage" {
					_, _ = io.WriteString(w, testUsage)
					return
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()
			adapter := New("missing-codex-executable")
			adapter.backendURL = server.URL
			adapter.refreshHTTP = server.Client()
			probe, err := adapter.Probe(context.Background(), accountTestCredential(), nil)
			if err != nil || probe.ResetCredits == nil || probe.ResetCredits.AvailableCount != 1 || len(probe.ResetCredits.Credits) != 0 {
				t.Fatalf("probe=%+v err=%v", probe, err)
			}
		})
	}
}

func TestConsumeResetCreditUsesAccountHTTP(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/rate-limit-reset-credits/consume":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Error("invalid redemption request method or content type")
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["redeem_request_id"] != "request-1" || payload["credit_id"] != "credit-1" {
				t.Errorf("invalid redemption request: %+v", payload)
			}
			_, _ = io.WriteString(w, `{"code":"already_redeemed"}`)
		case "/usage":
			_, _ = io.WriteString(w, testUsage)
		case "/rate-limit-reset-credits":
			_, _ = io.WriteString(w, testCards)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	adapter := New("missing-codex-executable")
	adapter.backendURL = server.URL
	adapter.refreshHTTP = server.Client()
	result, err := adapter.ConsumeResetCredit(context.Background(), accountTestCredential(), nil, "request-1", "credit-1")
	if err != nil || result.Outcome != "alreadyRedeemed" || len(result.Probe.Quotas) != 3 || strings.Join(calls, ",") != "POST /rate-limit-reset-credits/consume,GET /usage,GET /rate-limit-reset-credits" {
		t.Fatalf("result=%+v calls=%v err=%v", result, calls, err)
	}
}

func TestAccountHTTPClassifiesRejections(t *testing.T) {
	for _, tt := range []struct {
		status int
		want   string
	}{
		{401, "UPSTREAM_AUTH_FAILED"}, {403, "SUBSCRIPTION_UNAVAILABLE"}, {429, "UPSTREAM_RATE_LIMITED"}, {503, "UPSTREAM_UNAVAILABLE"},
	} {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status) }))
			defer server.Close()
			adapter := New("missing-codex-executable")
			adapter.backendURL = server.URL
			adapter.refreshHTTP = server.Client()
			_, err := adapter.Probe(context.Background(), accountTestCredential(), nil)
			var coded mgmt.SubscriptionConnectionError
			if !errors.As(err, &coded) || coded.ConnectionCode() != tt.want {
				t.Fatalf("error=%v, want %s", err, tt.want)
			}
		})
	}
}

func TestProbeUsesProviderProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "chatgpt.example" || r.Header.Get("ChatGPT-Account-Id") != "account-1" {
			t.Errorf("proxy request=%s account=%s", r.URL, r.Header.Get("ChatGPT-Account-Id"))
		}
		if r.URL.Path == "/usage" {
			_, _ = io.WriteString(w, testUsage)
		} else {
			_, _ = io.WriteString(w, testCards)
		}
	}))
	defer proxy.Close()
	adapter := New("missing-codex-executable")
	adapter.backendURL = "http://chatgpt.example"
	probe, err := adapter.Probe(context.Background(), accountTestCredential(), &catalog.OutboundProxy{URL: proxy.URL})
	if err != nil || len(probe.Quotas) != 3 {
		t.Fatalf("probe=%+v err=%v", probe, err)
	}
}

func TestProbePreservesRotatedCredentialAfterUsageFailure(t *testing.T) {
	expires := strconv.FormatInt(time.Now().UTC().Add(time.Hour).Unix(), 10)
	newAccess := token(`{"exp":` + expires + `}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_, _ = io.WriteString(w, `{"id_token":"new-id","access_token":"`+newAccess+`","refresh_token":"new-refresh"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+newAccess {
			t.Error("usage did not receive the rotated access token")
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	adapter := New("missing-codex-executable")
	adapter.refreshURL = server.URL + "/oauth/token"
	adapter.backendURL = server.URL
	adapter.refreshHTTP = server.Client()
	_, err := adapter.Probe(context.Background(), []byte(refreshTestCredential), nil)
	var rotated interface{ RotatedCredential() []byte }
	if !errors.As(err, &rotated) {
		t.Fatalf("missing rotated credential: %v", err)
	}
	updated, parseErr := parseCredential(rotated.RotatedCredential())
	if parseErr != nil || updated.Tokens.RefreshToken != "new-refresh" || updated.Tokens.AccountID != "account-1" {
		t.Fatalf("rotated credential was not preserved: %v", parseErr)
	}
}
