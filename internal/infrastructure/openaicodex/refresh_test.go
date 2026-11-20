package openaicodex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const refreshTestCredential = `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"custom":{"keep":true},"tokens":{"id_token":"old-id","access_token":"old-access","refresh_token":"old-refresh","account_id":"account-1","custom_token_field":"keep"}}`

func TestRefreshUsesOAuthEndpointWithoutAppServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s content-type=%s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["grant_type"] != "refresh_token" || body["client_id"] != codexOAuthClientID ||
			body["refresh_token"] != "old-refresh" || body["refresh_reason"] != "proactive_expiry" {
			t.Error("unexpected OAuth refresh parameters")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id_token":"new-id","access_token":"new-access","refresh_token":"new-refresh"}`)
	}))
	defer server.Close()
	adapter := New("missing-codex-executable")
	adapter.refreshURL = server.URL + "/oauth/token"
	adapter.refreshHTTP = server.Client()
	updated, changed, err := adapter.RefreshIfNeeded(context.Background(), []byte(refreshTestCredential), nil)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	defer clear(updated)
	var document map[string]json.RawMessage
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	var cache authCache
	if err := json.Unmarshal(updated, &cache); err != nil {
		t.Fatal(err)
	}
	if cache.Tokens.IDToken != "new-id" || cache.Tokens.AccessToken != "new-access" ||
		cache.Tokens.RefreshToken != "new-refresh" || cache.Tokens.AccountID != "account-1" {
		t.Fatal("refreshed credential fields were not saved")
	}
	if _, ok := parseRefreshTime(cache.LastRefresh); !ok || !strings.HasSuffix(cache.LastRefresh, "Z") {
		t.Fatalf("invalid UTC refresh time: %q", cache.LastRefresh)
	}
	if string(document["custom"]) != `{"keep":true}` || string(document["OPENAI_API_KEY"]) != "null" {
		t.Fatal("unrelated auth.json fields were not preserved")
	}
	var savedTokens map[string]json.RawMessage
	if err := json.Unmarshal(document["tokens"], &savedTokens); err != nil || string(savedTokens["custom_token_field"]) != `"keep"` {
		t.Fatal("unrelated token fields were not preserved")
	}
}

func TestRefreshRejectsOAuthErrorsWithoutChangingCredential(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantCode  string
		permanent bool
	}{
		{"expired", 401, `{"error":{"code":"token_expired"}}`, "CREDENTIAL_INVALID", true},
		{"invalid grant", 400, `{"error":{"code":"invalid_grant"}}`, "CREDENTIAL_INVALID", true},
		{"reused", 401, `{"error":{"code":"refresh_token_reused"}}`, "CREDENTIAL_REVOKED", true},
		{"rate limited", 429, `{"error":{"code":"rate_limit_exceeded"}}`, "UPSTREAM_RATE_LIMITED", false},
		{"unknown unauthorized", 401, `{"error":{"code":"other"}}`, "UPSTREAM_AUTH_FAILED", true},
		{"server error", 500, `{"error":{"code":"unavailable"}}`, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"incomplete success", 200, `{"access_token":"new-access","refresh_token":"new-refresh"}`, "UPSTREAM_INVALID_RESPONSE", false},
		{"malformed success", 200, `{`, "UPSTREAM_INVALID_RESPONSE", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			adapter := New("missing-codex-executable")
			adapter.refreshURL = server.URL
			adapter.refreshHTTP = server.Client()
			updated, changed, err := adapter.RefreshIfNeeded(context.Background(), []byte(refreshTestCredential), nil)
			if changed || len(updated) != 0 || err == nil {
				t.Fatalf("changed=%v updated=%d err=%v", changed, len(updated), err)
			}
			if code, permanent := adapter.ClassifyRefreshError(err); code != tt.wantCode || permanent != tt.permanent {
				t.Fatalf("code=%s permanent=%v, want %s %v", code, permanent, tt.wantCode, tt.permanent)
			}
		})
	}
}

func TestRefreshDoesNotExposeRejectedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"token_expired","message":"old-refresh"}}`)
	}))
	defer server.Close()
	adapter := New("missing-codex-executable")
	adapter.refreshURL = server.URL
	adapter.refreshHTTP = server.Client()
	_, _, err := adapter.RefreshIfNeeded(context.Background(), []byte(refreshTestCredential), nil)
	if err == nil || strings.Contains(err.Error(), "old-refresh") {
		t.Fatal("refresh error exposed credential")
	}
}

func TestRefreshUsesProviderProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "auth.example" || r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected proxy destination: %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"id_token":"new-id","access_token":"new-access","refresh_token":"new-refresh"}`)
	}))
	defer proxy.Close()
	adapter := New("missing-codex-executable")
	adapter.refreshURL = "http://auth.example/oauth/token"
	updated, changed, err := adapter.RefreshIfNeeded(context.Background(), []byte(refreshTestCredential), &catalog.OutboundProxy{URL: proxy.URL})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	clear(updated)
}

func TestRefreshHonorsHTTPTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	adapter := New("missing-codex-executable")
	adapter.refreshURL = server.URL
	adapter.refreshHTTP = server.Client()
	adapter.refreshHTTP.Timeout = 20 * time.Millisecond
	_, _, err := adapter.RefreshIfNeeded(context.Background(), []byte(refreshTestCredential), nil)
	if code, permanent := adapter.ClassifyRefreshError(err); code != "UPSTREAM_TIMEOUT" || permanent {
		t.Fatalf("code=%s permanent=%v err=%v", code, permanent, err)
	}
}

func TestRefreshRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", maxRefreshReplyBytes+1))
	}))
	defer server.Close()
	adapter := New("missing-codex-executable")
	adapter.refreshURL = server.URL
	adapter.refreshHTTP = server.Client()
	_, _, err := adapter.RefreshIfNeeded(context.Background(), []byte(refreshTestCredential), nil)
	if code, _ := adapter.ClassifyRefreshError(err); code != "UPSTREAM_INVALID_RESPONSE" {
		t.Fatalf("code=%s err=%v", code, err)
	}
}
