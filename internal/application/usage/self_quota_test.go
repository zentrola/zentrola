package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/quota"
)

type selfQuotaStoreStub struct {
	scopes        []quota.Scope
	used          map[quota.ScopeType]map[int64]int64
	usedScopes    []quota.Scope
	upstream      UpstreamQuota
	upstreamCalls int
}

func (s *selfQuotaStoreStub) QuotaScopes(context.Context, int64, string) ([]quota.Scope, error) {
	return s.scopes, nil
}
func (s *selfQuotaStoreStub) QuotaUsed(_ context.Context, scopes []quota.Scope, _ time.Time) (map[quota.ScopeType]map[int64]int64, error) {
	s.usedScopes = scopes
	return s.used, nil
}
func (s *selfQuotaStoreStub) LatestSubscriptionQuota(_ context.Context, _, _ int64, model string) (UpstreamQuota, error) {
	s.upstreamCalls++
	if s.upstream.Status != "" {
		return s.upstream, nil
	}
	return UpstreamQuota{Status: "UNKNOWN", Model: model}, nil
}

func TestSelfQuotaUsesAllModelUsageAndMinimumApplicableRemaining(t *testing.T) {
	store := &selfQuotaStoreStub{
		scopes: []quota.Scope{{Type: quota.Principal, ID: 5, Limit: 1000}, {Type: quota.Group, ID: 9, Limit: 500}},
		used:   map[quota.ScopeType]map[int64]int64{quota.Principal: {5: 300}, quota.Group: {9: 450}},
	}
	result, err := NewSelfQuota(store).Current(context.Background(), appsec.PrincipalIdentity{ID: 5}, "gpt-test", time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.UsedTokens != 300 || result.PersonalRemaining == nil || *result.PersonalRemaining != 700 || result.EffectiveRemaining == nil || *result.EffectiveRemaining != 50 {
		t.Fatalf("unexpected quota: %+v", result)
	}
	if len(store.usedScopes) != 2 {
		t.Fatalf("personal scope must be queried once: %+v", store.usedScopes)
	}
}

func TestSelfQuotaUnlimitedAndUpstreamAuthorization(t *testing.T) {
	store := &selfQuotaStoreStub{used: map[quota.ScopeType]map[int64]int64{quota.Principal: {5: 20}}}
	service := NewSelfQuota(store)
	result, err := service.Current(context.Background(), appsec.PrincipalIdentity{ID: 5}, "", time.Now().UTC())
	if err != nil || result.UsedTokens != 20 || result.PersonalLimit != nil || result.EffectiveRemaining != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	identity := appsec.PrincipalIdentity{ID: 5, AccessKeyID: 7}
	if _, err := service.Upstream(context.Background(), identity, "gpt-test", time.Now().UTC()); !errors.Is(err, ErrUpstreamQuotaForbidden) || store.upstreamCalls != 0 {
		t.Fatalf("err=%v calls=%d", err, store.upstreamCalls)
	}
	identity.CanViewUpstreamQuota = true
	if _, err := service.Upstream(context.Background(), identity, "gpt-test", time.Now().UTC()); err != nil || store.upstreamCalls != 1 {
		t.Fatalf("err=%v calls=%d", err, store.upstreamCalls)
	}
}

func TestSelfQuotaUpstreamHidesStaleSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	identity := appsec.PrincipalIdentity{ID: 5, AccessKeyID: 7, CanViewUpstreamQuota: true}
	for _, tc := range []struct {
		name      string
		checkedAt *time.Time
		want      string
		wantItems int
	}{
		{name: "fresh", checkedAt: ptr(now.Add(-UpstreamQuotaMaxAge)), want: "KNOWN", wantItems: 1},
		{name: "stale", checkedAt: ptr(now.Add(-UpstreamQuotaMaxAge - time.Second)), want: "UNKNOWN"},
		{name: "never checked", want: "UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &selfQuotaStoreStub{upstream: UpstreamQuota{Status: "KNOWN", Model: "gpt-test", CheckedAt: tc.checkedAt, Quotas: []UpstreamQuotaItem{{Code: "weekly"}}}}
			result, err := NewSelfQuota(store).Upstream(context.Background(), identity, "gpt-test", now)
			if err != nil || result.Status != tc.want || len(result.Quotas) != tc.wantItems || result.Quotas == nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func ptr[T any](value T) *T { return &value }
