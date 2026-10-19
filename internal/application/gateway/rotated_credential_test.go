package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// rotatedRefreshError 模拟刷新失败、但 app-server 已轮换 refresh token 的情况。
type rotatedRefreshError struct{ credential []byte }

func (*rotatedRefreshError) Error() string               { return "codex app-server stopped unexpectedly" }
func (e *rotatedRefreshError) RotatedCredential() []byte { return e.credential }

// rotatingFailureRefresher 每次刷新都失败；rotated 非空时在错误中携带已轮换的凭据。
// 以凭据前缀表示有效期：fresh 尚未过期，其余已过期。
type rotatingFailureRefresher struct{ rotated string }

func (rotatingFailureRefresher) Supports(code string) bool { return code == "OPENAI_CODEX" }

func (rotatingFailureRefresher) NeedsRefresh([]byte) (bool, error) { return true, nil }

func (rotatingFailureRefresher) CredentialRefreshMetadata(credential []byte) (*time.Time, *time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(-time.Minute)
	if strings.HasPrefix(string(credential), "fresh") {
		expiresAt = now.Add(time.Hour)
	}
	return &now, &expiresAt, nil
}

func (rotatingFailureRefresher) ClassifyRefreshError(error) (string, bool) {
	return "UPSTREAM_AUTH_FAILED", true
}

func (r rotatingFailureRefresher) RefreshIfNeeded(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error) {
	if r.rotated == "" {
		return nil, false, errors.New("codex app-server stopped unexpectedly")
	}
	return nil, false, &rotatedRefreshError{credential: []byte(r.rotated)}
}

// 后台刷新与网关刷新共用 refreshCredential：刷新失败时只要上游已轮换凭据，就必须先保存新凭据。
func TestRefreshSubscriptionsPersistsCredentialRotatedBeforeFailure(t *testing.T) {
	tests := []struct {
		name          string
		rotated       string
		wantStored    string
		wantRefreshed int
		wantBlocked   bool
	}{
		{name: "usable rotated credential counts as refreshed", rotated: "fresh-rotated", wantStored: "fresh-rotated", wantRefreshed: 1},
		{name: "expired rotated credential is saved before blocking", rotated: "expired-rotated", wantStored: "expired-rotated", wantBlocked: true},
		{name: "no rotation keeps stored credential", wantStored: "expired-token", wantBlocked: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &subscriptionRefreshStore{credentials: map[int64]SubscriptionCredential{
				1: subscriptionCredential(1, "expired-token"),
			}}
			service := New(store, subscriptionRefreshCipher{}, nil, WithSubscriptionRefresher(rotatingFailureRefresher{rotated: test.rotated}))

			summary, err := service.RefreshSubscriptions(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			stored := store.credentials[1]
			if got := string(stored.Credential.Ciphertext); got != test.wantStored {
				t.Fatalf("stored credential=%q, want %q", got, test.wantStored)
			}
			if summary.Refreshed != test.wantRefreshed || (len(store.blocked) > 0) != test.wantBlocked {
				t.Fatalf("summary=%+v blocked=%v", summary, store.blocked)
			}
			if test.rotated != "" && stored.CredentialExpiresAt == nil {
				t.Fatal("rotated credential metadata was not saved")
			}
		})
	}
}

func TestRefreshCredentialReturnsUsableRotatedCredential(t *testing.T) {
	store := &subscriptionRefreshStore{credentials: map[int64]SubscriptionCredential{
		1: subscriptionCredential(1, "expired-token"),
	}}
	service := New(store, subscriptionRefreshCipher{}, nil, WithSubscriptionRefresher(rotatingFailureRefresher{rotated: "fresh-rotated"}))
	route := Route{ProviderID: 10, ResourceID: 1, AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"}

	got, changed, err := service.refreshCredential(context.Background(), route, []byte("expired-token"))
	if err != nil || !changed || string(got) != "fresh-rotated" {
		t.Fatalf("credential=%q changed=%v err=%v", got, changed, err)
	}
}
