package usage

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/quota"
)

var ErrUpstreamQuotaForbidden = errors.New("upstream quota access forbidden")

// UpstreamQuotaMaxAge 是订阅号额度快照可对外展示的最长时效，超过后只返回 UNKNOWN。
const UpstreamQuotaMaxAge = 24 * time.Hour

// UpstreamQuota 是一个订阅号的额度快照；额度属于服务商账号，与模型无关。
type UpstreamQuota struct {
	Status           string              `json:"status"`
	ProviderName     string              `json:"providerName"`
	SubscriptionName string              `json:"subscriptionName"`
	CheckedAt        *time.Time          `json:"checkedAt"`
	Quotas           []UpstreamQuotaItem `json:"quotas"`
}

type UpstreamQuotaItem struct {
	Code           string  `json:"code"`
	Name           *string `json:"name"`
	Status         string  `json:"status"`
	Unit           *string `json:"unit"`
	LimitValue     *string `json:"limitValue"`
	UsedValue      *string `json:"usedValue"`
	RemainingValue *string `json:"remainingValue"`
	// UsedPercent 与 WindowDurationSeconds 描述按百分比计量的滚动窗口，如 7 天窗口已用 1%。
	UsedPercent           *float64   `json:"usedPercent"`
	WindowDurationSeconds *int64     `json:"windowDurationSeconds"`
	ResetsAt              *time.Time `json:"resetsAt"`
	ObservedAt            time.Time  `json:"observedAt"`
}

type SelfQuota struct {
	PeriodStart        time.Time `json:"periodStart"`
	PeriodEnd          time.Time `json:"periodEnd"`
	UsedTokens         int64     `json:"usedTokens,string"`
	PersonalLimit      *int64    `json:"personalLimit,string"`
	PersonalRemaining  *int64    `json:"personalRemaining,string"`
	Model              *string   `json:"model"`
	EffectiveRemaining *int64    `json:"effectiveRemaining,string"`
}

type SelfQuotaStore interface {
	QuotaScopes(context.Context, int64, string) ([]quota.Scope, error)
	QuotaUsed(context.Context, []quota.Scope, time.Time) (map[quota.ScopeType]map[int64]int64, error)
	SubscriptionQuotas(context.Context, int64) ([]UpstreamQuota, error)
}

type SelfQuotaService struct{ store SelfQuotaStore }

func NewSelfQuota(store SelfQuotaStore) *SelfQuotaService { return &SelfQuotaService{store: store} }

func (s *SelfQuotaService) Current(ctx context.Context, identity appsec.PrincipalIdentity, model string, now time.Time) (SelfQuota, error) {
	if identity.ID <= 0 {
		return SelfQuota{}, appsec.ErrUnauthenticated
	}
	model = strings.TrimSpace(model)
	if len(model) > 128 {
		return SelfQuota{}, appsec.ErrInvalidArgument
	}
	if s == nil || s.store == nil {
		return SelfQuota{}, appsec.ErrUnavailable
	}
	start, end := quota.Period(now)
	scopes, err := s.store.QuotaScopes(ctx, identity.ID, model)
	if err != nil {
		return SelfQuota{}, err
	}
	// 总用量始终按个人范围统计；个人未配置额度时补一个仅用于统计的范围，避免重复查询。
	all := scopes
	if !slices.ContainsFunc(scopes, func(scope quota.Scope) bool { return scope.Type == quota.Principal && scope.ID == identity.ID }) {
		all = append([]quota.Scope{{Type: quota.Principal, ID: identity.ID}}, scopes...)
	}
	used, err := s.store.QuotaUsed(ctx, all, start)
	if err != nil {
		return SelfQuota{}, err
	}
	result := SelfQuota{PeriodStart: start, PeriodEnd: end, UsedTokens: used[quota.Principal][identity.ID]}
	if model != "" {
		result.Model = &model
	}
	for _, scope := range scopes {
		remaining := max(0, scope.Limit-used[scope.Type][scope.ID])
		if scope.Type == quota.Principal {
			limit := scope.Limit
			result.PersonalLimit = &limit
			result.PersonalRemaining = &remaining
		}
		if model != "" && (result.EffectiveRemaining == nil || remaining < *result.EffectiveRemaining) {
			result.EffectiveRemaining = &remaining
		}
	}
	return result, nil
}

// Upstream 返回当前成员可路由到的全部订阅号额度快照，多个账号分别返回。
func (s *SelfQuotaService) Upstream(ctx context.Context, identity appsec.PrincipalIdentity, now time.Time) ([]UpstreamQuota, error) {
	if identity.ID <= 0 {
		return nil, appsec.ErrUnauthenticated
	}
	if !identity.CanManage {
		return nil, ErrUpstreamQuotaForbidden
	}
	if s == nil || s.store == nil {
		return nil, appsec.ErrUnavailable
	}
	result, err := s.store.SubscriptionQuotas(ctx, identity.ID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = []UpstreamQuota{}
	}
	for i := range result {
		if result[i].CheckedAt == nil || now.Sub(result[i].CheckedAt.UTC()) > UpstreamQuotaMaxAge {
			result[i].Status, result[i].Quotas = "UNKNOWN", []UpstreamQuotaItem{}
		}
	}
	return result, nil
}
