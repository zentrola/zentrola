package anthropicclaude

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

const (
	defaultQuotaRecordInterval = 5 * time.Minute
	quotaRecordTimeout         = 2 * time.Second
)

// QuotaRecorder 持久化网关观测到的订阅额度。实现只更新额度字段，不应视为资源配置变更。
type QuotaRecorder interface {
	RecordResourceQuotas(ctx context.Context, route gw.Route, status string, resetsAt *time.Time, checkedAt time.Time, quotas []mgmt.ResourceQuota) error
}

type quotaObservation struct {
	status     string
	resetsAt   *time.Time
	recordedAt time.Time
}

// QuotaObserver 从网关转发的 Claude 订阅响应头中被动更新额度。每个资源在额度状态或
// 重置时间不变时最多每个间隔写库一次，写库在后台执行，不阻塞响应转发。
type QuotaObserver struct {
	recorder QuotaRecorder
	logger   *slog.Logger
	interval time.Duration
	now      func() time.Time
	run      func(func())

	mu   sync.Mutex
	last map[int64]quotaObservation
}

func NewQuotaObserver(recorder QuotaRecorder, logger *slog.Logger) *QuotaObserver {
	if logger == nil {
		logger = slog.Default()
	}
	return &QuotaObserver{
		recorder: recorder, logger: logger, interval: defaultQuotaRecordInterval,
		now: time.Now, run: func(record func()) { go record() },
		last: make(map[int64]quotaObservation),
	}
}

func (o *QuotaObserver) ObserveResponse(ctx context.Context, route gw.Route, status int, header http.Header) {
	if o == nil || o.recorder == nil || route.ResourceID <= 0 ||
		route.AuthType != mgmt.AuthTypeSubscription || route.AuthAdapter != AdapterCode {
		return
	}
	now := o.now().UTC().Truncate(time.Microsecond)
	quotas := RateLimitQuotas(header, now)
	if quotas == nil {
		return
	}
	quotaStatus, resetsAt := mgmt.ObservedQuotaStatus(quotas, status >= http.StatusOK && status < http.StatusMultipleChoices)
	// 缺少重置时间的耗尽状态会让资源无限期退出路由，此时交给路由冷却处理，不写入额度。
	if quotaStatus == mgmt.QuotaExhausted && resetsAt == nil {
		return
	}
	if !o.claim(route.ResourceID, quotaStatus, resetsAt, now) {
		return
	}
	o.run(func() {
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), quotaRecordTimeout)
		defer cancel()
		if err := o.recorder.RecordResourceQuotas(recordCtx, route, quotaStatus, resetsAt, now, quotas); err != nil {
			o.logger.WarnContext(recordCtx, "subscription quota observation not recorded",
				"error_code", "QUOTA_OBSERVATION_FAILED", "provider_id", route.ProviderID, "resource_id", route.ResourceID)
		}
	})
}

// claim 判断本次观测是否需要写库，并在写库前登记，避免并发请求重复写入同一资源。
func (o *QuotaObserver) claim(resourceID int64, status string, resetsAt *time.Time, now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	previous, exists := o.last[resourceID]
	if exists && previous.status == status && sameTime(previous.resetsAt, resetsAt) && now.Sub(previous.recordedAt) < o.interval {
		return false
	}
	o.last[resourceID] = quotaObservation{status: status, resetsAt: resetsAt, recordedAt: now}
	return true
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
