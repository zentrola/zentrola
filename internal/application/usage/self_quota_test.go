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
	upstream      []UpstreamQuota
	upstreamCalls int
}

func (s *selfQuotaStoreStub) QuotaScopes(context.Context, int64, string) ([]quota.Scope, error) {
	return s.scopes, nil
}
func (s *selfQuotaStoreStub) QuotaUsed(_ context.Context, scopes []quota.Scope, _ time.Time) (map[quota.ScopeType]map[int64]int64, error) {
	s.usedScopes = scopes
	return s.used, nil
}
func (s *selfQuotaStoreStub) SubscriptionQuotas(context.Context, int64) ([]UpstreamQuota, error) {
	s.upstreamCalls++
	return s.upstream, nil
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
	if _, err := service.Upstream(context.Background(), identity, time.Now().UTC()); !errors.Is(err, ErrUpstreamQuotaForbidden) || store.upstreamCalls != 0 {
		t.Fatalf("err=%v calls=%d", err, store.upstreamCalls)
	}
	identity.CanManage = true
	upstream, err := service.Upstream(context.Background(), identity, time.Now().UTC())
	if err != nil || store.upstreamCalls != 1 || upstream == nil || len(upstream) != 0 {
		t.Fatalf("upstream=%+v err=%v calls=%d", upstream, err, store.upstreamCalls)
	}
}

func TestSelfQuotaUpstreamHidesStaleSnapshotPerAccount(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	identity := appsec.PrincipalIdentity{ID: 5, AccessKeyID: 7, CanManage: true}
	known := func(name string, checkedAt *time.Time) UpstreamQuota {
		return UpstreamQuota{Status: "KNOWN", ProviderName: "OpenAI", SubscriptionName: name, CheckedAt: checkedAt, Quotas: []UpstreamQuotaItem{{Code: "weekly"}}}
	}
	store := &selfQuotaStoreStub{upstream: []UpstreamQuota{
		known("fresh", ptr(now.Add(-UpstreamQuotaMaxAge))),
		known("stale", ptr(now.Add(-UpstreamQuotaMaxAge-time.Second))),
		known("never checked", nil),
	}}
	result, err := NewSelfQuota(store).Upstream(context.Background(), identity, now)
	if err != nil || len(result) != 3 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for i, want := range []struct {
		status string
		items  int
	}{{"KNOWN", 1}, {"UNKNOWN", 0}, {"UNKNOWN", 0}} {
		if result[i].Status != want.status || len(result[i].Quotas) != want.items || result[i].Quotas == nil {
			t.Fatalf("account %s: %+v", result[i].SubscriptionName, result[i])
		}
	}
}

func ptr[T any](value T) *T { return &value }
