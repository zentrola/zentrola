// Package quota 编排月度 Token 配额的准入和状态读取。
package quota

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/quota"
)

type UsageStore interface {
	Used(context.Context, []domain.Scope, time.Time, time.Time) (map[domain.ScopeType]map[int64]int64, error)
	Configured(context.Context, domain.ScopeType, []int64) ([]domain.Scope, error)
}

type Service struct {
	store  UsageStore
	logger *slog.Logger
}

func New(store UsageStore, logger *slog.Logger) *Service {
	return &Service{store: store, logger: logger}
}

func validScopes(scopes []domain.Scope) bool {
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if (scope.Type != domain.Principal && scope.Type != domain.Group) || scope.ID <= 0 || scope.Limit <= 0 {
			return false
		}
		key := string(scope.Type) + ":" + strconv.FormatInt(scope.ID, 10)
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
	return s.store.Used(ctx, scopes, start, end)
}
