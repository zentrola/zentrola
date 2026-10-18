package anthropicclaude

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

// 订阅推理响应通过 anthropic-ratelimit-unified-* 响应头返回统一限额；setup-token 没有
// user:profile 权限，无法读取额度接口，因此额度只能从推理响应中被动获取。
const unifiedRateLimitHeaderPrefix = "anthropic-ratelimit-unified-"

type unifiedWindow struct {
	header, code, claim string
	seconds             int64
}

// unifiedWindows 只列出已确认上游会返回的窗口；其他窗口触发拒绝时由订阅总体状态体现。
var unifiedWindows = []unifiedWindow{
	{header: "5h", code: "five_hour", claim: "five_hour", seconds: 5 * 60 * 60},
	{header: "7d", code: "seven_day", claim: "seven_day", seconds: 7 * 24 * 60 * 60},
}

// RateLimitQuotas 从推理响应头解析 Claude 订阅的统一限额。响应未包含统一限额头时返回 nil。
// 利用率头是 0 到 1 的比例，重置时间是 Unix 秒；结果统一为百分比和 UTC 时间。
func RateLimitQuotas(header http.Header, observedAt time.Time) []mgmt.ResourceQuota {
	if !hasUnifiedRateLimit(header) {
		return nil
	}
	observedAt = observedAt.UTC().Truncate(time.Microsecond)
	overallStatus := unifiedStatus(header.Get(unifiedRateLimitHeaderPrefix + "status"))
	overallReset := unifiedReset(header.Get(unifiedRateLimitHeaderPrefix + "reset"))
	claim := strings.TrimSpace(header.Get(unifiedRateLimitHeaderPrefix + "representative-claim"))

	name := "Subscription"
	subscription := mgmt.ResourceQuota{Code: "subscription", Name: &name, Status: mgmt.QuotaAvailable, ObservedAt: observedAt}
	if overallStatus != mgmt.QuotaUnknown {
		subscription.Status = overallStatus
	}
	if subscription.Status != mgmt.QuotaAvailable {
		subscription.ResetsAt = overallReset
	}
	quotas := []mgmt.ResourceQuota{subscription}
	for _, window := range unifiedWindows {
		prefix := unifiedRateLimitHeaderPrefix + window.header + "-"
		rawUtilization := header.Get(prefix + "utilization")
		rawReset := header.Get(prefix + "reset")
		rawStatus := header.Get(prefix + "status")
		representative := overallStatus == mgmt.QuotaExhausted && claim == window.claim
		if rawUtilization == "" && rawReset == "" && rawStatus == "" && !representative {
			continue
		}
		unit, seconds := "PERCENT", window.seconds
		quota := mgmt.ResourceQuota{
			Code: window.code, Unit: &unit, WindowDurationSeconds: &seconds,
			UsedPercent: unifiedUtilization(rawUtilization), ResetsAt: unifiedReset(rawReset), ObservedAt: observedAt,
		}
		quota.Status = unifiedStatus(rawStatus)
		if quota.Status == mgmt.QuotaUnknown {
			quota.Status = quotaStatus(quota.UsedPercent)
		}
		if representative {
			quota.Status = mgmt.QuotaExhausted
			if quota.ResetsAt == nil {
				quota.ResetsAt = overallReset
			}
		}
		quotas = append(quotas, quota)
	}
	return quotas
}

// AvailableQuotas 用于推理成功但上游未返回统一限额头的情况：推理成功本身证明订阅可用。
func AvailableQuotas(observedAt time.Time) []mgmt.ResourceQuota {
	name := "Subscription"
	return []mgmt.ResourceQuota{{
		Code: "subscription", Name: &name, Status: mgmt.QuotaAvailable, ObservedAt: observedAt.UTC().Truncate(time.Microsecond),
	}}
}

func hasUnifiedRateLimit(header http.Header) bool {
	for name := range header {
		if strings.HasPrefix(strings.ToLower(name), unifiedRateLimitHeaderPrefix) {
			return true
		}
	}
	return false
}

func unifiedStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allowed":
		return mgmt.QuotaAvailable
	case "allowed_warning":
		return mgmt.QuotaNearLimit
	case "rejected":
		return mgmt.QuotaExhausted
	default:
		return mgmt.QuotaUnknown
	}
}

func unifiedUtilization(value string) *float64 {
	ratio, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
		return nil
	}
	percent := math.Round(ratio*100*100) / 100
	return &percent
}

func unifiedReset(value string) *time.Time {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || seconds <= 0 {
		return nil
	}
	resetsAt := time.Unix(seconds, 0).UTC()
	return &resetsAt
}
