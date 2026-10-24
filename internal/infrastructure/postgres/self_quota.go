package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	app "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/quota"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type SelfQuotaStore struct {
	pool   *pgxpool.Pool
	quotas *QuotaStore
	logger *slog.Logger
}

func NewSelfQuotaStore(pool *pgxpool.Pool, logger *slog.Logger) *SelfQuotaStore {
	return &SelfQuotaStore{pool: pool, quotas: NewQuotaStore(pool, logger), logger: logger}
}

func (s *SelfQuotaStore) QuotaScopes(ctx context.Context, principalID int64, model string) ([]quota.Scope, error) {
	q := dbgen.New(s.pool)
	result := make([]quota.Scope, 0)
	if model == "" {
		var limit *int64
		err := s.pool.QueryRow(ctx, `SELECT monthly_token_limit FROM principal WHERE id=$1 AND principal_type='MEMBER' AND NOT is_deleted`, principalID).Scan(&limit)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, appsec.ErrNotFound
		}
		if err != nil {
			return nil, diagnosePostgresError(ctx, s.logger, "self_quota", "principal_limit", err, appsec.ErrUnavailable)
		}
		if limit != nil {
			result = append(result, quota.Scope{Type: quota.Principal, ID: principalID, Limit: *limit})
		}
		return result, nil
	}
	m, err := q.GatewayModel(ctx, model)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appsec.ErrNotFound
	}
	if err != nil {
		return nil, diagnosePostgresError(ctx, s.logger, "self_quota", "model", err, appsec.ErrUnavailable)
	}
	if m.Status != "ACTIVE" {
		return nil, appsec.ErrNotFound
	}
	allowed, err := q.HasGroupModelPermission(ctx, dbgen.HasGroupModelPermissionParams{PrincipalID: principalID, ModelID: m.ID})
	if err != nil {
		return nil, diagnosePostgresError(ctx, s.logger, "self_quota", "permission", err, appsec.ErrUnavailable)
	}
	if !allowed {
		return nil, app.ErrUpstreamQuotaForbidden
	}
	config, err := q.GatewayTokenQuotaConfiguration(ctx, dbgen.GatewayTokenQuotaConfigurationParams{PrincipalID: principalID, ModelID: m.ID})
	if err != nil {
		return nil, diagnosePostgresError(ctx, s.logger, "self_quota", "configuration", err, appsec.ErrUnavailable)
	}
	if config.MonthlyTokenLimit != nil {
		result = append(result, quota.Scope{Type: quota.Principal, ID: principalID, Limit: *config.MonthlyTokenLimit})
	}
	var groups []struct {
		ID    int64  `json:"id"`
		Limit *int64 `json:"limit"`
	}
	if err := json.Unmarshal([]byte(config.GroupQuotas), &groups); err != nil {
		return nil, appsec.ErrUnavailable
	}
	for _, group := range groups {
		if group.Limit != nil {
			result = append(result, quota.Scope{Type: quota.Group, ID: group.ID, Limit: *group.Limit})
		}
	}
	return result, nil
}

func (s *SelfQuotaStore) QuotaUsed(ctx context.Context, scopes []quota.Scope, start time.Time) (map[quota.ScopeType]map[int64]int64, error) {
	return s.quotas.Used(ctx, scopes, start, start.AddDate(0, 1, 0))
}

// SubscriptionQuotas 返回成员有权模型可路由到的全部订阅号额度快照，每个账号一条；
// 快照时效由 application 层按调用时间判断。
func (s *SelfQuotaStore) SubscriptionQuotas(ctx context.Context, principalID int64) ([]app.UpstreamQuota, error) {
	return subscriptionQuotas(ctx, s.pool, s.logger, principalID)
}

func subscriptionQuotas(ctx context.Context, db dbgen.DBTX, logger *slog.Logger, principalID int64) ([]app.UpstreamQuota, error) {
	// 账号池与 GatewayCandidates 一致：有权模型 → 服务商模型 → 启用服务商 → 订阅号。
	rows, err := db.Query(ctx, `SELECT r.id,p.provider_name,r.resource_name,r.quota_checked_at
		FROM provider_credential r
		JOIN provider p ON p.id=r.provider_id
		WHERE r.auth_type='SUBSCRIPTION' AND NOT r.is_deleted AND NOT p.is_deleted AND p.status='ACTIVE'
		  AND EXISTS (
		    SELECT 1 FROM principal pr
		    JOIN principal_group_membership pg ON pg.principal_id=pr.id AND NOT pg.is_deleted
		    JOIN principal_group g ON g.id=pg.group_id AND g.status='ACTIVE' AND NOT g.is_deleted
		    JOIN principal_group_model_permission permission ON permission.group_id=g.id AND NOT permission.is_deleted
		    JOIN model m ON m.id=permission.model_id AND m.status='ACTIVE' AND NOT m.is_deleted
		    JOIN provider_model pm ON pm.model_id=m.id AND pm.provider_id=p.id AND NOT pm.is_deleted
		    WHERE pr.id=$1 AND pr.principal_type IN ('MEMBER','APPLICATION') AND pr.status='ACTIVE' AND NOT pr.is_deleted)
		ORDER BY p.provider_name,r.priority,r.id`, principalID)
	if err != nil {
		return nil, diagnosePostgresError(ctx, logger, "self_quota", "subscriptions", err, appsec.ErrUnavailable)
	}
	var ids []int64
	result := make([]app.UpstreamQuota, 0)
	for rows.Next() {
		var id int64
		item := app.UpstreamQuota{Status: "UNKNOWN", Quotas: []app.UpstreamQuotaItem{}}
		if err := rows.Scan(&id, &item.ProviderName, &item.SubscriptionName, &item.CheckedAt); err != nil {
			rows.Close()
			return nil, diagnosePostgresError(ctx, logger, "self_quota", "subscriptions", err, appsec.ErrUnavailable)
		}
		if item.CheckedAt != nil {
			checkedAt := item.CheckedAt.UTC()
			item.CheckedAt = &checkedAt
		}
		item.ID = id
		ids, result = append(ids, id), append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, diagnosePostgresError(ctx, logger, "self_quota", "subscriptions", err, appsec.ErrUnavailable)
	}
	q := dbgen.New(db)
	for i, id := range ids {
		if result[i].CheckedAt == nil {
			continue
		}
		quotas, err := q.ManageResourceQuotas(ctx, id)
		if err != nil {
			return nil, diagnosePostgresError(ctx, logger, "self_quota", "resource_quotas", err, appsec.ErrUnavailable)
		}
		for _, row := range quotas {
			result[i].Quotas = append(result[i].Quotas, app.UpstreamQuotaItem{
				Code: row.QuotaCode, Name: row.QuotaName, Status: row.QuotaStatus, Unit: row.QuotaUnit,
				LimitValue: numericString(row.LimitValue), UsedValue: numericString(row.UsedValue), RemainingValue: numericString(row.RemainingValue),
				UsedPercent: numericFloat(row.UsedPercent), WindowDurationSeconds: row.WindowDurationSeconds,
				ResetsAt: timePointer(row.ResetsAt), ObservedAt: row.ObservedAt.Time.UTC(),
			})
		}
		if len(quotas) > 0 {
			result[i].Status = "KNOWN"
		}
	}
	return result, nil
}
