// Package quota 编排月度 Token 配额的准入、状态读取和异步计量。
package quota

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/quota"
	usage "github.com/zentrola/zentrola/internal/domain/usage"
)

type UsageStore interface {
	Used(context.Context, []domain.Scope, time.Time, time.Time) (map[domain.ScopeType]map[int64]int64, error)
	Configured(context.Context, domain.ScopeType, []int64) ([]domain.Scope, error)
}

type Cache interface {
	Load(context.Context, []domain.Scope, time.Time) (map[domain.ScopeType]map[int64]*int64, error)
	Initialize(context.Context, map[domain.ScopeType]map[int64]int64, time.Time, time.Time) (map[domain.ScopeType]map[int64]int64, error)
	Increment(context.Context, map[domain.ScopeType]map[int64]int64, time.Time, time.Time) error
}

type Service struct {
	store  UsageStore
	cache  Cache
	logger *slog.Logger
	mu     sync.Mutex
	dirty  map[string]struct{}
}

func New(store UsageStore, cache Cache, logger *slog.Logger) *Service {
	return &Service{store: store, cache: cache, logger: logger, dirty: make(map[string]struct{})}
}

func scopeKey(scope domain.Scope, start time.Time) string {
	return string(scope.Type) + ":" + start.Format("200601") + ":" + stringID(scope.ID)
}

func stringID(value int64) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[position:])
}

func validScopes(scopes []domain.Scope) bool {
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if (scope.Type != domain.Principal && scope.Type != domain.Group) || scope.ID <= 0 || scope.Limit <= 0 {
			return false
		}
		key := string(scope.Type) + ":" + stringID(scope.ID)
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func (s *Service) Check(ctx context.Context, scopes []domain.Scope, at time.Time) (domain.Decision, error) {
	if len(scopes) == 0 {
		return domain.Decision{Allowed: true}, nil
	}
	if s == nil || s.store == nil || !validScopes(scopes) {
		return domain.Decision{}, errors.New("quota service unavailable")
	}
	used, err := s.used(ctx, scopes, at)
	if err != nil {
		return domain.Decision{}, err
	}
	for index := range scopes {
		scope := scopes[index]
		value := used[scope.Type][scope.ID]
		if value >= scope.Limit {
			if s.logger != nil {
				s.logger.WarnContext(ctx, "gateway request rejected by monthly token quota",
					"error_code", "BUDGET_EXCEEDED", "quota_scope_type", scope.Type,
					"quota_scope_id", scope.ID, "monthly_token_limit", scope.Limit, "used_tokens", value)
			}
			return domain.Decision{Allowed: false, Scope: &scope, Used: value}, nil
		}
	}
	return domain.Decision{Allowed: true}, nil
}

func (s *Service) Statuses(ctx context.Context, scopeType domain.ScopeType, ids []int64, at time.Time) ([]domain.Status, error) {
	if s == nil || s.store == nil || (scopeType != domain.Principal && scopeType != domain.Group) || len(ids) == 0 {
		return nil, errors.New("invalid quota status request")
	}
	scopes, err := s.store.Configured(ctx, scopeType, ids)
	if err != nil || len(scopes) == 0 {
		return []domain.Status{}, err
	}
	used, err := s.used(ctx, scopes, at)
	if err != nil {
		return nil, err
	}
	statuses := make([]domain.Status, 0, len(scopes))
	for _, scope := range scopes {
		statuses = append(statuses, domain.NewStatus(scope, used[scope.Type][scope.ID], at))
	}
	return statuses, nil
}

func (s *Service) used(ctx context.Context, scopes []domain.Scope, at time.Time) (map[domain.ScopeType]map[int64]int64, error) {
	start, end := domain.Period(at)
	values := make(map[domain.ScopeType]map[int64]int64)
	missing := make([]domain.Scope, 0, len(scopes))
	if s.cache != nil {
		cached, err := s.cache.Load(ctx, scopes, start)
		if err == nil {
			s.mu.Lock()
			for _, scope := range scopes {
				entry := cached[scope.Type][scope.ID]
				_, dirty := s.dirty[scopeKey(scope, start)]
				if entry == nil || dirty {
					missing = append(missing, scope)
					continue
				}
				if values[scope.Type] == nil {
					values[scope.Type] = make(map[int64]int64)
				}
				values[scope.Type][scope.ID] = *entry
			}
			s.mu.Unlock()
		} else {
			missing = append(missing, scopes...)
		}
	} else {
		missing = append(missing, scopes...)
	}
	if len(missing) == 0 {
		return values, nil
	}
	rebuilt, err := s.store.Used(ctx, missing, start, end)
	if err != nil {
		return nil, err
	}
	for _, scope := range missing {
		if values[scope.Type] == nil {
			values[scope.Type] = make(map[int64]int64)
		}
		values[scope.Type][scope.ID] = rebuilt[scope.Type][scope.ID]
	}
	if s.cache != nil {
		initialized, cacheErr := s.cache.Initialize(ctx, rebuilt, start, end)
		if cacheErr == nil {
			for scopeType, entries := range initialized {
				if values[scopeType] == nil {
					values[scopeType] = make(map[int64]int64)
				}
				for id, value := range entries {
					values[scopeType][id] = value
				}
			}
			s.mu.Lock()
			for _, scope := range missing {
				delete(s.dirty, scopeKey(scope, start))
			}
			s.mu.Unlock()
		}
	}
	return values, nil
}

// Record 在 usage_record 成功提交后调用；失败只会让相应缓存进入重建状态，事实数据仍以 PostgreSQL 为准。
func (s *Service) Record(ctx context.Context, events []usage.Event) {
	if s == nil || s.cache == nil || len(events) == 0 {
		return
	}
	type periodIncrements struct {
		end    time.Time
		values map[domain.ScopeType]map[int64]int64
	}
	periods := make(map[time.Time]*periodIncrements)
	for _, event := range events {
		for _, attempt := range append(append([]usage.Attempt(nil), event.Attempts...), attemptValue(event.Attempt)...) {
			tokens := tokenValue(attempt.InputTokens) + tokenValue(attempt.OutputTokens)
			if tokens <= 0 {
				continue
			}
			observedAt := attempt.StartedAt
			if observedAt.IsZero() {
				observedAt = event.RequestAt
			}
			if observedAt.IsZero() {
				observedAt = event.CompletedAt
			}
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			start, end := domain.Period(observedAt)
			period := periods[start]
			if period == nil {
				period = &periodIncrements{end: end, values: make(map[domain.ScopeType]map[int64]int64)}
				periods[start] = period
			}
			for _, scope := range event.QuotaScopes {
				if period.values[scope.Type] == nil {
					period.values[scope.Type] = make(map[int64]int64)
				}
				period.values[scope.Type][scope.ID] += tokens
			}
		}
	}
	for start, period := range periods {
		if len(period.values) == 0 || s.cache.Increment(ctx, period.values, start, period.end) == nil {
			continue
		}
		s.mu.Lock()
		for scopeType, entries := range period.values {
			for id := range entries {
				s.dirty[scopeKey(domain.Scope{Type: scopeType, ID: id}, start)] = struct{}{}
			}
		}
		s.mu.Unlock()
		if s.logger != nil {
			s.logger.WarnContext(ctx, "token quota cache update failed; counters will rebuild from usage records", "error_code", "TOKEN_QUOTA_CACHE_UPDATE_FAILED")
		}
	}
}

func attemptValue(value *usage.Attempt) []usage.Attempt {
	if value == nil {
		return nil
	}
	return []usage.Attempt{*value}
}

func tokenValue(value *int64) int64 {
	if value == nil || *value < 0 {
		return 0
	}
	return *value
}
