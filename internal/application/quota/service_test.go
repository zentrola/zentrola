package quota

import (
	"context"
	"testing"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/quota"
)

type quotaStoreStub struct {
	used       map[domain.ScopeType]map[int64]int64
	configured []domain.Scope
	usedCalls  int
}

func (s *quotaStoreStub) Used(context.Context, []domain.Scope, time.Time, time.Time) (map[domain.ScopeType]map[int64]int64, error) {
	s.usedCalls++
	return s.used, nil
}

func (s *quotaStoreStub) Configured(context.Context, domain.ScopeType, []int64) ([]domain.Scope, error) {
	return s.configured, nil
}

func TestCheckReadsCommittedUsageOnEveryAdmission(t *testing.T) {
	scope := domain.Scope{Type: domain.Group, ID: 20, Limit: 100}
	store := &quotaStoreStub{used: map[domain.ScopeType]map[int64]int64{domain.Group: {20: 90}}}
	service := New(store, nil)
	at := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

	first, err := service.Check(context.Background(), []domain.Scope{scope}, at)
	if err != nil || !first.Allowed {
		t.Fatalf("first admission: decision=%#v err=%v", first, err)
	}
	store.used[domain.Group][20] = 110
	second, err := service.Check(context.Background(), []domain.Scope{scope}, at)
	if err != nil || second.Allowed || second.Used != 110 || store.usedCalls != 2 {
		t.Fatalf("later committed usage was ignored: decision=%#v calls=%d err=%v", second, store.usedCalls, err)
	}
}

func TestStatusesUsePersistedMonthlyTotals(t *testing.T) {
	scope := domain.Scope{Type: domain.Principal, ID: 10, Limit: 200}
	store := &quotaStoreStub{
		configured: []domain.Scope{scope},
		used:       map[domain.ScopeType]map[int64]int64{domain.Principal: {10: 150}},
	}
	statuses, err := New(store, nil).Statuses(context.Background(), domain.Principal, []int64{10},
		time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC))
	if err != nil || len(statuses) != 1 || statuses[0].UsedTokens != 150 || statuses[0].RemainingTokens != 50 {
		t.Fatalf("unexpected statuses: %#v err=%v", statuses, err)
	}
}

func TestCheckRejectsInvalidScopesBeforeStoreRead(t *testing.T) {
	store := &quotaStoreStub{}
	_, err := New(store, nil).Check(context.Background(), []domain.Scope{{Type: domain.Group, ID: 20}}, time.Now().UTC())
	if err == nil {
		t.Fatal("invalid scope should fail")
	}
	if store.usedCalls != 0 {
		t.Fatal("invalid scope reached the store")
	}
}

func TestCheckWithoutConfiguredScopesBypassesDependencies(t *testing.T) {
	decision, err := (*Service)(nil).Check(context.Background(), nil, time.Now().UTC())
	if err != nil || !decision.Allowed {
		t.Fatalf("unconfigured quota must bypass all dependencies: %#v %v", decision, err)
	}
}
