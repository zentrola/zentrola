package usage

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/quota"
)

type selfQuotaStoreStub struct {
	mu            sync.Mutex
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upstreamCalls++
	if s.upstream == nil {
		return nil, nil
	}
	result := make([]UpstreamQuota, len(s.upstream))
	for i, item := range s.upstream {
		item.Quotas = slices.Clone(item.Quotas)
		result[i] = item
	}
	return result, nil
}

func (s *selfQuotaStoreStub) markChecked(id int64, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.upstream {
		if s.upstream[i].ID == id {
			s.upstream[i].CheckedAt = &at
			s.upstream[i].Quotas = []UpstreamQuotaItem{{Code: "refreshed"}}
		}
	}
}

// refresherStub 模拟按需刷新：返回 OK 时把快照标记为刷新时间。
type refresherStub struct {
	store   *selfQuotaStoreStub
	at      time.Time
	code    string
	err     error
	release chan struct{}
	mu      sync.Mutex
	calls   map[int64]int
	keys    []int64
}

func (r *refresherStub) RefreshSubscriptionQuota(_ context.Context, identity appsec.PrincipalIdentity, id int64, _ appsec.RequestMeta) (string, error) {
	r.mu.Lock()
	if r.calls == nil {
		r.calls = map[int64]int{}
	}
	r.calls[id]++
	r.keys = append(r.keys, identity.AccessKeyID)
	code, err := r.code, r.err
	r.mu.Unlock()
	if r.release != nil {
		<-r.release
	}
	if err == nil && code == "OK" {
		r.store.markChecked(id, r.at)
	}
	return code, err
}

func (r *refresherStub) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, calls := range r.calls {
		total += calls
	}
	return total
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
	if _, err := service.Upstream(context.Background(), identity, appsec.RequestMeta{}, time.Now().UTC()); !errors.Is(err, ErrUpstreamQuotaForbidden) || store.upstreamCalls != 0 {
		t.Fatalf("err=%v calls=%d", err, store.upstreamCalls)
	}
	identity.CanManage = true
	upstream, err := service.Upstream(context.Background(), identity, appsec.RequestMeta{}, time.Now().UTC())
	if err != nil || store.upstreamCalls != 1 || upstream == nil || len(upstream) != 0 {
		t.Fatalf("upstream=%+v err=%v calls=%d", upstream, err, store.upstreamCalls)
	}
}

var upstreamIdentity = appsec.PrincipalIdentity{ID: 5, AccessKeyID: 7, CanManage: true}

func upstreamAccount(id int64, checkedAt *time.Time) UpstreamQuota {
	item := UpstreamQuota{ID: id, ProviderName: "Anthropic", SubscriptionName: "account", CheckedAt: checkedAt, Quotas: []UpstreamQuotaItem{}}
	if checkedAt != nil {
		item.Quotas = []UpstreamQuotaItem{{Code: "seven_day"}}
	}
	return item
}

func TestSelfQuotaUpstreamRefreshesExpiredAccounts(t *testing.T) {
	now := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	expired := ptr(now.Add(-UpstreamQuotaFreshness))
	tests := []struct {
		name      string
		checkedAt *time.Time
		code      string
		err       error
		wantCalls int
		want      string
		wantError string
		wantItem  string
	}{
		{name: "fresh snapshot is served from cache", checkedAt: ptr(now.Add(-UpstreamQuotaFreshness + time.Second)), want: UpstreamQuotaKnown, wantItem: "seven_day"},
		{name: "expired snapshot refreshes", checkedAt: expired, code: "OK", wantCalls: 1, want: UpstreamQuotaKnown, wantItem: "refreshed"},
		{name: "never checked refreshes", code: "OK", wantCalls: 1, want: UpstreamQuotaKnown, wantItem: "refreshed"},
		{name: "failed refresh keeps stale snapshot", checkedAt: expired, code: "UPSTREAM_AUTH_FAILED", wantCalls: 1, want: UpstreamQuotaStale, wantError: "UPSTREAM_AUTH_FAILED", wantItem: "seven_day"},
		{name: "refresh error keeps stale snapshot", checkedAt: expired, err: errors.New("lock busy"), wantCalls: 1, want: UpstreamQuotaStale, wantError: "QUOTA_REFRESH_FAILED", wantItem: "seven_day"},
		{name: "failed first refresh stays unknown", code: "PROVIDER_MODEL_MAPPING_REQUIRED", wantCalls: 1, want: UpstreamQuotaUnknown, wantError: "PROVIDER_MODEL_MAPPING_REQUIRED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &selfQuotaStoreStub{upstream: []UpstreamQuota{upstreamAccount(60, test.checkedAt)}}
			refresher := &refresherStub{store: store, at: now, code: test.code, err: test.err}
			result, err := NewSelfQuota(store, WithUpstreamQuotaRefresher(refresher)).Upstream(context.Background(), upstreamIdentity, appsec.RequestMeta{}, now)
			if err != nil || len(result) != 1 || refresher.total() != test.wantCalls {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, refresher.total())
			}
			item := result[0]
			gotError, gotItem := "", ""
			if item.RefreshError != nil {
				gotError = *item.RefreshError
			}
			if len(item.Quotas) > 0 {
				gotItem = item.Quotas[0].Code
			}
			if item.Status != test.want || gotError != test.wantError || gotItem != test.wantItem || item.Quotas == nil {
				t.Fatalf("account=%+v", item)
			}
			if test.wantCalls > 0 && refresher.keys[0] != upstreamIdentity.AccessKeyID {
				t.Fatalf("refresh must be attributed to the calling key: %v", refresher.keys)
			}
		})
	}
}

func TestSelfQuotaUpstreamRefreshCooldown(t *testing.T) {
	now := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	store := &selfQuotaStoreStub{upstream: []UpstreamQuota{upstreamAccount(60, ptr(now.Add(-time.Hour)))}}
	refresher := &refresherStub{store: store, at: now, code: "UPSTREAM_RATE_LIMITED"}
	service := NewSelfQuota(store, WithUpstreamQuotaRefresher(refresher))
	query := func(at time.Time) UpstreamQuota {
		t.Helper()
		result, err := service.Upstream(context.Background(), upstreamIdentity, appsec.RequestMeta{}, at)
		if err != nil || len(result) != 1 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		return result[0]
	}
	query(now)
	// 失败冷却期内不再访问上游，但仍返回上次的错误码。
	if item := query(now.Add(upstreamQuotaRetryAfter - time.Second)); refresher.total() != 1 || item.Status != UpstreamQuotaStale ||
		item.RefreshError == nil || *item.RefreshError != "UPSTREAM_RATE_LIMITED" {
		t.Fatalf("calls=%d account=%+v", refresher.total(), item)
	}
	refresher.code = "OK"
	refresher.at = now.Add(upstreamQuotaRetryAfter)
	if item := query(now.Add(upstreamQuotaRetryAfter)); refresher.total() != 2 || item.Status != UpstreamQuotaKnown {
		t.Fatalf("calls=%d account=%+v", refresher.total(), item)
	}
	// 成功后时效内即使读到旧快照（如其他请求刚刷新完），也只重新读取快照而不访问上游。
	store.upstream[0].CheckedAt = ptr(now.Add(-time.Hour))
	before := store.upstreamCalls
	if query(now.Add(upstreamQuotaRetryAfter + time.Minute)); refresher.total() != 2 || store.upstreamCalls != before+2 {
		t.Fatalf("calls=%d reads=%d", refresher.total(), store.upstreamCalls-before)
	}
}

func TestSelfQuotaUpstreamMergesConcurrentRefreshes(t *testing.T) {
	now := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	store := &selfQuotaStoreStub{upstream: []UpstreamQuota{upstreamAccount(60, nil), upstreamAccount(61, ptr(now.Add(-time.Hour)))}}
	refresher := &refresherStub{store: store, at: now, code: "OK", release: make(chan struct{})}
	service := NewSelfQuota(store, WithUpstreamQuotaRefresher(refresher))
	var wg sync.WaitGroup
	results := make([][]UpstreamQuota, 5)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = service.Upstream(context.Background(), upstreamIdentity, appsec.RequestMeta{}, now)
		}()
	}
	for refresher.total() < 2 {
		time.Sleep(time.Millisecond)
	}
	close(refresher.release)
	wg.Wait()
	refresher.mu.Lock()
	defer refresher.mu.Unlock()
	if refresher.calls[60] != 1 || refresher.calls[61] != 1 {
		t.Fatalf("each account must be refreshed once: %v", refresher.calls)
	}
	for _, result := range results {
		if len(result) != 2 || result[0].Status != UpstreamQuotaKnown || result[1].Status != UpstreamQuotaKnown {
			t.Fatalf("result=%+v", result)
		}
	}
}

func TestSelfQuotaUpstreamReturnsStaleSnapshotOnTimeout(t *testing.T) {
	now := time.Date(2026, 10, 24, 2, 0, 0, 0, time.UTC)
	store := &selfQuotaStoreStub{upstream: []UpstreamQuota{upstreamAccount(60, ptr(now.Add(-time.Hour)))}}
	refresher := &refresherStub{store: store, at: now, code: "UPSTREAM_AUTH_FAILED", release: make(chan struct{})}
	defer close(refresher.release)
	service := NewSelfQuota(store, WithUpstreamQuotaRefresher(refresher))
	service.refreshWait = 10 * time.Millisecond
	result, err := service.Upstream(context.Background(), upstreamIdentity, appsec.RequestMeta{}, now)
	if err != nil || len(result) != 1 || result[0].Status != UpstreamQuotaStale || result[0].RefreshError == nil ||
		*result[0].RefreshError != "QUOTA_REFRESH_TIMEOUT" || len(result[0].Quotas) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func ptr[T any](value T) *T { return &value }
