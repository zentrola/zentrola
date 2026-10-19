package openaicodex

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestClassifyRefreshErrorUsesOnlyTypedFailures(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantCode      string
		wantPermanent bool
	}{
		{"JSON-RPC error mentioning authentication", &rpcError{Code: -32603, Message: "authentication failed"}, "SUBSCRIPTION_REFRESH_FAILED", false},
		{"free text authentication failure", errors.New("codex ChatGPT authentication failed"), "SUBSCRIPTION_REFRESH_FAILED", false},
		{"invalid credential", fmt.Errorf("stage: %w", errInvalidCredential), "CREDENTIAL_INVALID", true},
		{"permanent connection code", &connectionFailure{code: "UPSTREAM_AUTH_FAILED", cause: errors.New("rejected")}, "UPSTREAM_AUTH_FAILED", true},
		{"retryable connection code", &connectionFailure{code: "UPSTREAM_INVALID_RESPONSE", cause: errors.New("unexpected")}, "UPSTREAM_INVALID_RESPONSE", false},
		{"interrupted request", interruptedRequest("account/read", context.DeadlineExceeded), "UPSTREAM_TIMEOUT", false},
		{"app-server unavailable", fmt.Errorf("%w: cannot start process", errAppServerUnavailable), "CODEX_APP_SERVER_UNAVAILABLE", false},
		{"app-server stopped", errAppServerStopped, "SUBSCRIPTION_REFRESH_FAILED", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, permanent := New("").ClassifyRefreshError(tt.err); code != tt.wantCode || permanent != tt.wantPermanent {
				t.Fatalf("code=%q permanent=%v, want %q %v", code, permanent, tt.wantCode, tt.wantPermanent)
			}
		})
	}
}

func TestRefreshTreatsUnexpectedAccountResponseAsRetryable(t *testing.T) {
	tests := []struct {
		name    string
		account string
	}{
		{name: "missing account", account: ""},
		{name: "API key account", account: "apiKey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := New("missing-codex-executable")
			adapter.runSession = func(_ context.Context, raw []byte, _ *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
				rpc := accountRPCFunc(func(_ context.Context, _ int64, method string, _ any, target any) error {
					if method != "account/read" {
						t.Fatalf("unexpected method %s", method)
					}
					if tt.account != "" {
						response := target.(*accountResponse)
						response.Account = &struct {
							Type     string `json:"type"`
							PlanType string `json:"planType"`
						}{Type: tt.account}
					}
					return nil
				})
				if err := action(rpc); err != nil {
					return nil, mgmt.SubscriptionInspection{}, err
				}
				return append([]byte(nil), raw...), mgmt.SubscriptionInspection{}, nil
			}
			raw := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"account-1"}}`)
			_, changed, err := adapter.RefreshIfNeeded(context.Background(), raw, nil)
			if changed || err == nil {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			if code, permanent := adapter.ClassifyRefreshError(err); code != "UPSTREAM_INVALID_RESPONSE" || permanent {
				t.Fatalf("code=%q permanent=%v", code, permanent)
			}
		})
	}
}
