package gateway

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// busySubscriptionCoordinator 在前 busyCalls 次请求资源锁时报告锁已被占用。
type busySubscriptionCoordinator struct {
	mu        sync.Mutex
	busyCalls int
	calls     int
	err       error
}

func (*busySubscriptionCoordinator) AcquireSubscriptionRefreshScheduler(context.Context) (func(), bool, error) {
	return nil, false, nil
}

func (c *busySubscriptionCoordinator) AcquireSubscriptionRefreshResource(context.Context, int64) (func(), bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.err != nil {
		return nil, false, c.err
	}
	if c.busyCalls < 0 || c.calls <= c.busyCalls {
		return nil, false, nil
	}
	return func() {}, true, nil
}

// expiringSubscriptionRefresher 以凭据前缀表示有效期：valid / fresh 尚未过期，其余已过期。
type expiringSubscriptionRefresher struct {
	mu        sync.Mutex
	refreshes int
}

func (*expiringSubscriptionRefresher) Supports(code string) bool { return code == "OPENAI_CODEX" }

func (*expiringSubscriptionRefresher) NeedsRefresh(credential []byte) (bool, error) {
	return !strings.HasPrefix(string(credential), "fresh"), nil
}

func (*expiringSubscriptionRefresher) CredentialRefreshMetadata(credential []byte) (*time.Time, *time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(-time.Minute)
	if strings.HasPrefix(string(credential), "valid") || strings.HasPrefix(string(credential), "fresh") {
		expiresAt = now.Add(time.Hour)
	}
	return &now, &expiresAt, nil
}

func (r *expiringSubscriptionRefresher) RefreshIfNeeded(_ context.Context, credential []byte, _ *catalog.OutboundProxy) ([]byte, bool, error) {
	if strings.HasPrefix(string(credential), "fresh") {
		return nil, false, nil
	}
	r.mu.Lock()
	r.refreshes++
	r.mu.Unlock()
	return []byte("fresh-" + string(credential)), true, nil
}

func TestRefreshCredentialWhenRefreshLockIsBusy(t *testing.T) {
	tests := []struct {
		name           string
		credential     string
		stored         string
		coordinator    *busySubscriptionCoordinator
		want           string
		wantBusy       bool
		wantRefreshes  int
		minCoordinator int
	}{
		{
			name: "valid credential keeps working", credential: "valid-token", stored: "valid-token",
			coordinator: &busySubscriptionCoordinator{busyCalls: -1}, want: "valid-token", minCoordinator: 1,
		},
		{
			name: "redis error with valid credential", credential: "valid-token", stored: "valid-token",
			coordinator: &busySubscriptionCoordinator{err: errors.New("redis unavailable")}, want: "valid-token", minCoordinator: 1,
		},
		{
			name: "expired credential waits and reloads", credential: "expired-token", stored: "fresh-by-other-instance",
			coordinator: &busySubscriptionCoordinator{busyCalls: 2}, want: "fresh-by-other-instance", minCoordinator: 3,
		},
		{
			name: "expired credential with redis error", credential: "expired-token", stored: "expired-token",
			coordinator: &busySubscriptionCoordinator{err: errors.New("redis unavailable")}, wantBusy: true, minCoordinator: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &subscriptionRefreshStore{credentials: map[int64]SubscriptionCredential{
				1: subscriptionCredential(1, test.stored),
			}}
			refresher := &expiringSubscriptionRefresher{}
			service := New(store, subscriptionRefreshCipher{}, nil,
				WithSubscriptionRefresher(refresher), WithSubscriptionRefreshCoordinator(test.coordinator))
			route := Route{ProviderID: 10, ResourceID: 1, AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"}
			got, changed, err := service.refreshCredential(context.Background(), route, []byte(test.credential))
			if test.wantBusy {
				if !errors.Is(err, errSubscriptionRefreshBusy) {
					t.Fatalf("err=%v, want refresh busy", err)
				}
			} else if err != nil || changed || string(got) != test.want {
				t.Fatalf("credential=%q changed=%v err=%v want %q", got, changed, err, test.want)
			}
			if refresher.refreshes != test.wantRefreshes || test.coordinator.calls < test.minCoordinator {
				t.Fatalf("refreshes=%d coordinator calls=%d", refresher.refreshes, test.coordinator.calls)
			}
		})
	}
}

func TestForwardDoesNotCooldownSubscriptionOnRefreshLockContention(t *testing.T) {
	routes := []Route{
		{ModelID: 1, ProviderID: 10, ResourceID: 1, UpstreamModel: "gpt-test", AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"},
		{ModelID: 1, ProviderID: 10, ResourceID: 2, UpstreamModel: "gpt-test", AuthType: "API_KEY"},
	}
	state := &failoverState{blocked: map[int64]bool{}}
	var selected []int64
	service := New(&failoverStore{routes: routes}, failoverCipher{},
		upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
			selected = append(selected, route.ResourceID)
			return response(200, `{}`), nil
		}),
		WithRouteState(state),
		WithSubscriptionRefresher(&expiringSubscriptionRefresher{}),
		WithSubscriptionRefreshCoordinator(&busySubscriptionCoordinator{err: errors.New("redis unavailable")}),
	)
	got, err := service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}, Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, Body: []byte(`{"model":"gpt-test","input":"hi"}`),
	})
	if err != nil || got == nil {
		t.Fatalf("response=%v err=%v", got, err)
	}
	_ = got.Body.Close()
	// failoverCipher 解密出的 "secret" 被视为已过期，锁不可用时应换到下一条路由且不冷却。
	if len(selected) != 1 || selected[0] != 2 || len(state.cooldowns) != 0 {
		t.Fatalf("selected=%v cooldowns=%v", selected, state.cooldowns)
	}
}
