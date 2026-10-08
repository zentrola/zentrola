package quota

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/quota"
	usage "github.com/zentrola/zentrola/internal/domain/usage"
)

type quotaStoreStub struct {
	used      map[domain.ScopeType]map[int64]int64
	usedCalls int
}

func (s *quotaStoreStub) Used(context.Context, []domain.Scope, time.Time, time.Time) (map[domain.ScopeType]map[int64]int64, error) {
	s.usedCalls++
	return s.used, nil
}

func (*quotaStoreStub) Configured(context.Context, domain.ScopeType, []int64) ([]domain.Scope, error) {
	return nil, nil
}

type quotaCacheStub struct {
	loaded         map[domain.ScopeType]map[int64]*int64
	loadErr        error
	initialized    map[domain.ScopeType]map[int64]int64
	incrementCalls []quotaIncrementCall
}

type quotaIncrementCall struct {
	values map[domain.ScopeType]map[int64]int64
	start  time.Time
}

func (s *quotaCacheStub) Load(context.Context, []domain.Scope, time.Time) (map[domain.ScopeType]map[int64]*int64, error) {
	return s.loaded, s.loadErr
}

func (s *quotaCacheStub) Initialize(_ context.Context, values map[domain.ScopeType]map[int64]int64, _, _ time.Time) (map[domain.ScopeType]map[int64]int64, error) {
	s.initialized = values
	return values, nil
}

func (s *quotaCacheStub) Increment(_ context.Context, values map[domain.ScopeType]map[int64]int64, start, _ time.Time) error {
	s.incrementCalls = append(s.incrementCalls, quotaIncrementCall{values: values, start: start})
	return nil
}

func int64Pointer(value int64) *int64 { return &value }

func TestCheckUsesOneCachedSnapshotAndRejectsReachedScope(t *testing.T) {
	scopes := []domain.Scope{
		{Type: domain.Principal, ID: 10, Limit: 100},
		{Type: domain.Group, ID: 20, Limit: 200},
	}
	store := &quotaStoreStub{}
	cache := &quotaCacheStub{loaded: map[domain.ScopeType]map[int64]*int64{
		domain.Principal: {10: int64Pointer(99)},
		domain.Group:     {20: int64Pointer(200)},
	}}
	service := New(store, cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	decision, err := service.Check(context.Background(), scopes, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Scope == nil || decision.Scope.Type != domain.Group || decision.Used != 200 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if store.usedCalls != 0 {
		t.Fatal("cache hit must not query PostgreSQL")
	}
}

func TestCheckFallsBackToUsageFactsWhenRedisIsUnavailable(t *testing.T) {
	scope := domain.Scope{Type: domain.Principal, ID: 10, Limit: 100}
	store := &quotaStoreStub{used: map[domain.ScopeType]map[int64]int64{domain.Principal: {10: 75}}}
	cache := &quotaCacheStub{loadErr: errors.New("redis unavailable")}
	service := New(store, cache, nil)
	decision, err := service.Check(context.Background(), []domain.Scope{scope}, time.Now().UTC())
	if err != nil || !decision.Allowed {
		t.Fatalf("fallback check failed: decision=%#v err=%v", decision, err)
	}
	if store.usedCalls != 1 || cache.initialized[domain.Principal][10] != 75 {
		t.Fatal("PostgreSQL facts were not used to rebuild the counter")
	}
}

func TestRecordCountsInputAndOutputAndKeepsUTCMontlyPeriodsSeparate(t *testing.T) {
	cache := &quotaCacheStub{}
	service := New(&quotaStoreStub{}, cache, nil)
	principal := domain.Scope{Type: domain.Principal, ID: 10, Limit: 1000}
	group := domain.Scope{Type: domain.Group, ID: 20, Limit: 2000}
	input, output, cached := int64(10), int64(5), int64(8)
	service.Record(context.Background(), []usage.Event{
		{QuotaScopes: []domain.Scope{principal, group}, Attempt: &usage.Attempt{StartedAt: time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC), InputTokens: &input, OutputTokens: &output, CachedInputTokens: &cached}},
		{QuotaScopes: []domain.Scope{principal}, Attempt: &usage.Attempt{StartedAt: time.Date(2026, 10, 1, 0, 0, 1, 0, time.UTC), InputTokens: &input, OutputTokens: &output, CachedInputTokens: &cached}},
	})
	if len(cache.incrementCalls) != 2 {
		t.Fatalf("expected two UTC periods, got %d", len(cache.incrementCalls))
	}
	for _, call := range cache.incrementCalls {
		if call.values[domain.Principal][10] != 15 {
			t.Fatalf("cached input tokens must not be counted: %#v", call.values)
		}
	}
}

func TestCheckWithoutConfiguredScopesIsACompleteBypass(t *testing.T) {
	decision, err := (*Service)(nil).Check(context.Background(), nil, time.Now().UTC())
	if err != nil || !decision.Allowed {
		t.Fatalf("unconfigured quota must bypass all dependencies: %#v %v", decision, err)
	}
}
