package postgres

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	domain "github.com/zentrola/zentrola/internal/domain/quota"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type QuotaStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewQuotaStore(pool *pgxpool.Pool, loggers ...*slog.Logger) *QuotaStore {
	return &QuotaStore{pool: pool, logger: optionalLogger(loggers)}
}

func (s *QuotaStore) Configured(ctx context.Context, scopeType domain.ScopeType, ids []int64) ([]domain.Scope, error) {
	if len(ids) == 0 {
		return []domain.Scope{}, nil
	}
	q := dbgen.New(s.pool)
	result := make([]domain.Scope, 0, len(ids))
	switch scopeType {
	case domain.Principal:
		rows, err := q.QuotaConfiguredPrincipals(ctx, ids)
		if err != nil {
			return nil, diagnosePostgresError(ctx, s.logger, "quota", "configured_principals", err, appsec.ErrUnavailable)
		}
		for _, row := range rows {
			if row.MonthlyTokenLimit != nil {
				result = append(result, domain.Scope{Type: domain.Principal, ID: row.ID, Limit: *row.MonthlyTokenLimit})
			}
		}
	case domain.Group:
		rows, err := q.QuotaConfiguredGroups(ctx, ids)
		if err != nil {
			return nil, diagnosePostgresError(ctx, s.logger, "quota", "configured_groups", err, appsec.ErrUnavailable)
		}
		for _, row := range rows {
			if row.MonthlyTokenLimit != nil {
				result = append(result, domain.Scope{Type: domain.Group, ID: row.ID, Limit: *row.MonthlyTokenLimit})
			}
		}
	default:
		return nil, errors.New("invalid quota scope type")
	}
	return result, nil
}

func (s *QuotaStore) Used(ctx context.Context, scopes []domain.Scope, start, _ time.Time) (map[domain.ScopeType]map[int64]int64, error) {
	result := make(map[domain.ScopeType]map[int64]int64)
	if len(scopes) == 0 {
		return result, nil
	}
	principalIDs := make([]int64, 0, len(scopes))
	groupIDs := make([]int64, 0, len(scopes))
	for _, scope := range scopes {
		switch scope.Type {
		case domain.Principal:
			principalIDs = append(principalIDs, scope.ID)
		case domain.Group:
			groupIDs = append(groupIDs, scope.ID)
		}
	}
	periodStart := pgtype.Timestamptz{Time: start.UTC(), Valid: true}
	q := dbgen.New(s.pool)
	if len(principalIDs) > 0 {
		rows, err := q.QuotaPrincipalUsedTokens(ctx, dbgen.QuotaPrincipalUsedTokensParams{ScopeIds: principalIDs, PeriodStart: periodStart})
		if err != nil {
			return nil, diagnosePostgresError(ctx, s.logger, "quota", "principal_used_tokens", err, appsec.ErrUnavailable)
		}
		result[domain.Principal] = make(map[int64]int64, len(rows))
		for _, row := range rows {
			result[domain.Principal][row.ScopeID] = row.UsedTokens
		}
	}
	if len(groupIDs) > 0 {
		rows, err := q.QuotaGroupUsedTokens(ctx, dbgen.QuotaGroupUsedTokensParams{ScopeIds: groupIDs, PeriodStart: periodStart})
		if err != nil {
			return nil, diagnosePostgresError(ctx, s.logger, "quota", "group_used_tokens", err, appsec.ErrUnavailable)
		}
		result[domain.Group] = make(map[int64]int64, len(rows))
		for _, row := range rows {
			result[domain.Group][row.ScopeID] = row.UsedTokens
		}
	}
	return result, nil
}
