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

// LatestSubscriptionQuota 返回该 Key 指定模型最近一次成功请求所用订阅号的额度快照；
// 快照时效由 application 层按调用时间判断。
func (s *SelfQuotaStore) LatestSubscriptionQuota(ctx context.Context, principalID, accessKeyID int64, model string) (app.UpstreamQuota, error) {
	result := app.UpstreamQuota{Status: "UNKNOWN", Model: model, Quotas: []app.UpstreamQuotaItem{}}
	q := dbgen.New(s.pool)
	// 先解析 model_id，使最近用量查询直接命中 ix_usage_record_key_model_recent。
	m, err := q.GatewayModel(ctx, model)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, diagnosePostgresError(ctx, s.logger, "self_quota", "model", err, appsec.ErrUnavailable)
	}
	var resourceID int64
	var authType string
	err = s.pool.QueryRow(ctx, `SELECT r.id,r.resource_name,p.provider_name,r.auth_type,u.completed_at,r.quota_checked_at
		FROM usage_record u
		JOIN provider_credential r ON r.id=u.provider_credential_id
		JOIN provider p ON p.id=r.provider_id
		WHERE u.access_key_id=$1 AND u.model_id=$2 AND u.principal_id=$3 AND u.status='SUCCESS'
		ORDER BY u.completed_at DESC,u.id DESC LIMIT 1`, accessKeyID, m.ID, principalID).Scan(
		&resourceID, &result.SubscriptionName, &result.ProviderName, &authType, &result.LastUsedAt, &result.CheckedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, diagnosePostgresError(ctx, s.logger, "self_quota", "latest_resource", err, appsec.ErrUnavailable)
	}
	if authType != "SUBSCRIPTION" {
		result.SubscriptionName = nil
		result.ProviderName = nil
		result.CheckedAt = nil
		return result, nil
	}
	if result.CheckedAt == nil {
		return result, nil
	}
	rows, err := q.ManageResourceQuotas(ctx, resourceID)
	if err != nil {
		return result, diagnosePostgresError(ctx, s.logger, "self_quota", "resource_quotas", err, appsec.ErrUnavailable)
	}
	if len(rows) == 0 {
		return result, nil
	}
	for _, row := range rows {
		result.Quotas = append(result.Quotas, app.UpstreamQuotaItem{
			Code: row.QuotaCode, Name: row.QuotaName, Status: row.QuotaStatus, Unit: row.QuotaUnit,
			LimitValue: numericString(row.LimitValue), UsedValue: numericString(row.UsedValue), RemainingValue: numericString(row.RemainingValue),
			ResetsAt: timePointer(row.ResetsAt), ObservedAt: row.ObservedAt.Time.UTC(),
		})
	}
	result.Status = "KNOWN"
	return result, nil
}
