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
		{"standard invalid grant", 400, `{"error":"invalid_grant","error_description":"rejected"}`, "CREDENTIAL_INVALID", true},
		{"top level expired", 400, `{"code":"refresh_token_expired"}`, "CREDENTIAL_INVALID", true},
		{"invalidated", 400, `{"error":{"code":"refresh_token_invalidated"}}`, "CREDENTIAL_REVOKED", true},
		{"normalized invalidated", 400, `{"error":" REFRESH_TOKEN_INVALIDATED "}`, "CREDENTIAL_REVOKED", true},
		{"reused", 401, `{"error":{"code":"refresh_token_reused"}}`, "CREDENTIAL_REVOKED", true},
		{"rate limited", 429, `{"error":{"code":"rate_limit_exceeded"}}`, "UPSTREAM_RATE_LIMITED", false},
		{"unknown unauthorized", 401, `{"error":{"code":"other"}}`, "UPSTREAM_AUTH_FAILED", true},
		{"edge forbidden", 403, `<html>Access denied</html>`, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"unknown forbidden", 403, `{"error":{"code":"other"}}`, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"explicit revoked forbidden", 403, `{"error":"refresh_token_invalidated"}`, "CREDENTIAL_REVOKED", true},
		{"invalid grant server error", 503, `{"error":"invalid_grant"}`, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"malformed error", 400, `{`, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"server error", 500, `{"error":{"code":"unavailable"}}`, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"empty success", 200, `{}`, "UPSTREAM_INVALID_RESPONSE", false},
		{"null success", 200, `null`, "UPSTREAM_INVALID_RESPONSE", false},
		{"null tokens", 200, `{"id_token":null,"access_token":null,"refresh_token":null}`, "UPSTREAM_INVALID_RESPONSE", false},
		{"empty token", 200, `{"access_token":""}`, "UPSTREAM_INVALID_RESPONSE", false},
		{"wrong token type", 200, `{"access_token":123}`, "UPSTREAM_INVALID_RESPONSE", false},
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

func TestRefreshMergesOptionalTokenFields(t *testing.T) {
	for _, tt := range []struct {
		name, body, id, access, refresh string
	}{
		{"omitted ID token", `{"access_token":"new-access","refresh_token":"new-refresh"}`, "old-id", "new-access", "new-refresh"},
		{"omitted refresh token", `{"id_token":"new-id","access_token":"new-access"}`, "new-id", "new-access", "old-refresh"},
		{"access token only", `{"access_token":"new-access"}`, "old-id", "new-access", "old-refresh"},
		{"refresh token only", `{"refresh_token":"new-refresh"}`, "old-id", "old-access", "new-refresh"},
		{"ID token only", `{"id_token":"new-id"}`, "new-id", "old-access", "old-refresh"},
		{"null fields preserve originals", `{"id_token":null,"access_token":"new-access","refresh_token":null}`, "old-id", "new-access", "old-refresh"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			adapter := New("")
			adapter.refreshURL, adapter.refreshHTTP = server.URL, server.Client()
			raw := []byte(refreshTestCredential)
			updated, changed, err := adapter.RefreshIfNeeded(context.Background(), raw, nil)
			if err != nil || !changed {
				t.Fatalf("changed=%v error=%v", changed, err)
			}
			defer clear(updated)
			cache, err := parseCredential(updated)
			if err != nil || cache.Tokens.IDToken != tt.id || cache.Tokens.AccessToken != tt.access || cache.Tokens.RefreshToken != tt.refresh {
				t.Fatal("optional token fields were not merged correctly")
			}
			if cache.Tokens.AccountID != "account-1" || string(raw) != refreshTestCredential {
				t.Fatal("refresh changed the account or the caller's original credential")
			}
			if _, ok := parseRefreshTime(cache.LastRefresh); !ok {
				t.Fatal("refresh time was not saved")
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
