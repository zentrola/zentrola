package usage

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/quota"
	"golang.org/x/sync/singleflight"
)

var ErrUpstreamQuotaForbidden = errors.New("upstream quota access forbidden")

const (
	// UpstreamQuotaFreshness 是订阅号额度快照的缓存时效：时效内直接返回快照，过期后按需向上游刷新。
	UpstreamQuotaFreshness = 5 * time.Minute
	// upstreamQuotaRetryAfter 是刷新失败后的冷却时间，避免上游故障期间每次查询都重试。
	upstreamQuotaRetryAfter = time.Minute
	// upstreamQuotaRefreshWait 是单次查询等待刷新的上限，超时的账号返回已有快照并标为 STALE。
	upstreamQuotaRefreshWait = 15 * time.Second
	// upstreamQuotaRefreshTimeout 约束一次共享刷新本身，不随首个调用方取消而中断。
	upstreamQuotaRefreshTimeout = 45 * time.Second
)

// 订阅号额度状态：KNOWN 为时效内快照；STALE 为刷新失败或冷却中返回的过期快照；UNKNOWN 为从未取得额度。
const (
	UpstreamQuotaKnown   = "KNOWN"
	UpstreamQuotaStale   = "STALE"
	UpstreamQuotaUnknown = "UNKNOWN"
)

// UpstreamQuota 是一个订阅号的额度快照；额度属于服务商账号，与模型无关。
type UpstreamQuota struct {
	ID               int64      `json:"-"`
	Status           string     `json:"status"`
	ProviderName     string     `json:"providerName"`
	SubscriptionName string     `json:"subscriptionName"`
	CheckedAt        *time.Time `json:"checkedAt"`
	// RefreshError 是本次未能刷新该账号额度时的错误码；为空表示快照在时效内或刚刷新成功。
	RefreshError *string             `json:"refreshError"`
	Quotas       []UpstreamQuotaItem `json:"quotas"`
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

// UpstreamQuotaRefresher 主动向上游读取一个订阅号的额度并保存快照，返回结果码（成功为 OK）。
type UpstreamQuotaRefresher interface {
	RefreshSubscriptionQuota(context.Context, appsec.PrincipalIdentity, int64, appsec.RequestMeta) (string, error)
}

type SelfQuotaOption func(*SelfQuotaService)

// WithUpstreamQuotaRefresher 启用订阅号额度按需刷新；未配置时只返回已保存的快照。
func WithUpstreamQuotaRefresher(refresher UpstreamQuotaRefresher) SelfQuotaOption {
	return func(s *SelfQuotaService) { s.refresher = refresher }
}

type SelfQuotaService struct {
	store     SelfQuotaStore
	refresher UpstreamQuotaRefresher
	// refreshes 按订阅号合并并发刷新：同一账号同时只有一个上游请求，其余调用方等待并共享结果。
	refreshes   singleflight.Group
	refreshWait time.Duration
	mu          sync.Mutex
	attempts    map[int64]refreshAttempt
}

// refreshAttempt 记录进程内最近一次刷新；code 为空表示成功。
type refreshAttempt struct {
	at   time.Time
	code string
}

func NewSelfQuota(store SelfQuotaStore, options ...SelfQuotaOption) *SelfQuotaService {
	s := &SelfQuotaService{store: store, refreshWait: upstreamQuotaRefreshWait, attempts: map[int64]refreshAttempt{}}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	return s
}

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

// Upstream 返回当前成员可路由到的全部订阅号额度，多个账号分别返回。快照在 UpstreamQuotaFreshness 内直接返回；
// 过期的账号并行按需刷新，刷新失败、冷却中或等待超时时返回原快照并标为 STALE。
func (s *SelfQuotaService) Upstream(ctx context.Context, identity appsec.PrincipalIdentity, meta appsec.RequestMeta, now time.Time) ([]UpstreamQuota, error) {
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
	failures := map[int64]string{}
	if s.refresher != nil {
		var due []int64
		for _, item := range result {
			if !freshUpstreamQuota(item, now) {
				due = append(due, item.ID)
			}
		}
		var reload bool
		if failures, reload = s.refreshDue(ctx, identity, meta, due, now); reload {
			if result, err = s.store.SubscriptionQuotas(ctx, identity.ID); err != nil {
				return nil, err
			}
		}
	}
	if result == nil {
		result = []UpstreamQuota{}
	}
	for i := range result {
		item := &result[i]
		if item.Quotas == nil {
			item.Quotas = []UpstreamQuotaItem{}
		}
		switch {
		case item.CheckedAt == nil || len(item.Quotas) == 0:
			item.Status = UpstreamQuotaUnknown
		case freshUpstreamQuota(*item, now):
			item.Status = UpstreamQuotaKnown
		default:
			item.Status = UpstreamQuotaStale
		}
		if code, failed := failures[item.ID]; failed && item.Status != UpstreamQuotaKnown {
			item.RefreshError = &code
		}
	}
	return result, nil
}

func freshUpstreamQuota(item UpstreamQuota, now time.Time) bool {
	return item.CheckedAt != nil && now.Sub(item.CheckedAt.UTC()) < UpstreamQuotaFreshness
}

// refreshDue 并行刷新过期账号，返回未能刷新的账号错误码，以及是否需要重新读取快照。
// 进程内记录最近一次刷新：成功后 UpstreamQuotaFreshness 内、失败后 upstreamQuotaRetryAfter 内不再访问上游，
// 这也覆盖了读取快照后、其他请求刚完成刷新的情况。
func (s *SelfQuotaService) refreshDue(ctx context.Context, identity appsec.PrincipalIdentity, meta appsec.RequestMeta, due []int64, now time.Time) (map[int64]string, bool) {
	failures := map[int64]string{}
	reload := false
	pending := map[int64]<-chan singleflight.Result{}
	for _, id := range due {
		s.mu.Lock()
		attempt, attempted := s.attempts[id]
		s.mu.Unlock()
		if attempted && attempt.code == "" && now.Sub(attempt.at) < UpstreamQuotaFreshness {
			reload = true
			continue
		}
		if attempted && attempt.code != "" && now.Sub(attempt.at) < upstreamQuotaRetryAfter {
			failures[id] = attempt.code
			continue
		}
		pending[id] = s.refreshes.DoChan(strconv.FormatInt(id, 10), func() (any, error) {
			return s.refresh(ctx, identity, meta, id, now), nil
		})
	}
	if len(pending) == 0 {
		return failures, reload
	}
	wait := time.NewTimer(s.refreshWait)
	defer wait.Stop()
	for id, done := range pending {
		select {
		case outcome := <-done:
			delete(pending, id)
			if code, _ := outcome.Val.(string); code != "" {
				failures[id] = code
			} else {
				reload = true
			}
		case <-wait.C:
			// 计时器只触发一次：尚未完成的账号都按超时处理，刷新本身继续在后台完成并落盘。
			for rest := range pending {
				failures[rest] = "QUOTA_REFRESH_TIMEOUT"
			}
			return failures, true
		case <-ctx.Done():
			return failures, false
		}
	}
	return failures, reload
}

// refresh 执行一次共享刷新并记录结果。刷新不随首个调用方取消而中断，以免等待中的其他调用方一起失败。
func (s *SelfQuotaService) refresh(ctx context.Context, identity appsec.PrincipalIdentity, meta appsec.RequestMeta, id int64, now time.Time) string {
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), upstreamQuotaRefreshTimeout)
	defer cancel()
	code, err := s.refresher.RefreshSubscriptionQuota(refreshCtx, identity, id, meta)
	switch {
	case err != nil:
		code = "QUOTA_REFRESH_FAILED"
	case code == "OK":
		code = ""
	case code == "":
		code = "QUOTA_REFRESH_FAILED"
	}
	s.mu.Lock()
	s.attempts[id] = refreshAttempt{at: now, code: code}
	s.mu.Unlock()
	return code
}
