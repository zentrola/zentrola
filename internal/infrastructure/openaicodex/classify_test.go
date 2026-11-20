package openaicodex

import (
	"context"
	"errors"
	"fmt"
	"testing"
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
