package anthropicclaude

import (
	"net/http"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

type expectedQuota struct {
	code, status string
	usedPercent  *float64
	resetsAt     *time.Time
}

func percent(value float64) *float64 { return &value }

func unixTime(seconds int64) *time.Time {
	value := time.Unix(seconds, 0).UTC()
	return &value
}

func TestRateLimitQuotas(t *testing.T) {
	observedAt := time.Date(2026, 10, 18, 3, 20, 0, 123456789, time.FixedZone("UTC+8", 8*3600))
	tests := []struct {
		name    string
		headers map[string]string
		want    []expectedQuota
	}{
		{name: "no unified headers", headers: map[string]string{"anthropic-ratelimit-requests-limit": "50"}},
		{
			name: "allowed windows",
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":           "allowed",
				"anthropic-ratelimit-unified-5h-utilization":   "0.5",
				"anthropic-ratelimit-unified-5h-reset":         "1792000000",
				"anthropic-ratelimit-unified-5h-status":        "allowed",
				"anthropic-ratelimit-unified-7d-utilization":   "0.56",
				"anthropic-ratelimit-unified-7d-reset":         "1792500000",
				"anthropic-ratelimit-unified-reset":            "1792000000",
				"anthropic-ratelimit-unified-fallback-percent": "0.5",
			},
			want: []expectedQuota{
				{code: "subscription", status: mgmt.QuotaAvailable},
				{code: "five_hour", status: mgmt.QuotaAvailable, usedPercent: percent(50), resetsAt: unixTime(1792000000)},
				{code: "seven_day", status: mgmt.QuotaAvailable, usedPercent: percent(56), resetsAt: unixTime(1792500000)},
			},
		},
		{
			name: "window status overrides utilization",
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":         "allowed_warning",
				"anthropic-ratelimit-unified-reset":          "1792500000",
				"anthropic-ratelimit-unified-7d-utilization": "0.8",
				"anthropic-ratelimit-unified-7d-status":      "allowed_warning",
			},
			want: []expectedQuota{
				{code: "subscription", status: mgmt.QuotaNearLimit, resetsAt: unixTime(1792500000)},
				{code: "seven_day", status: mgmt.QuotaNearLimit, usedPercent: percent(80)},
			},
		},
		{
			name: "utilization decides status without window status",
			headers: map[string]string{
				"anthropic-ratelimit-unified-5h-utilization": "0.95",
				"anthropic-ratelimit-unified-7d-utilization": "1",
			},
			want: []expectedQuota{
				{code: "subscription", status: mgmt.QuotaAvailable},
				{code: "five_hour", status: mgmt.QuotaNearLimit, usedPercent: percent(95)},
				{code: "seven_day", status: mgmt.QuotaExhausted, usedPercent: percent(100)},
			},
		},
		{
			name: "rejected representative window uses overall reset",
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":               "rejected",
				"anthropic-ratelimit-unified-reset":                "1792000000",
				"anthropic-ratelimit-unified-representative-claim": "five_hour",
			},
			want: []expectedQuota{
				{code: "subscription", status: mgmt.QuotaExhausted, resetsAt: unixTime(1792000000)},
				{code: "five_hour", status: mgmt.QuotaExhausted, resetsAt: unixTime(1792000000)},
			},
		},
		{
			name: "rejected unknown window is kept on subscription",
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":               "rejected",
				"anthropic-ratelimit-unified-reset":                "1792500000",
				"anthropic-ratelimit-unified-representative-claim": "seven_day_opus",
				"anthropic-ratelimit-unified-5h-utilization":       "0.2",
			},
			want: []expectedQuota{
				{code: "subscription", status: mgmt.QuotaExhausted, resetsAt: unixTime(1792500000)},
				{code: "five_hour", status: mgmt.QuotaAvailable, usedPercent: percent(20)},
			},
		},
		{
			name: "invalid values are ignored",
			headers: map[string]string{
				"anthropic-ratelimit-unified-status":         "unexpected",
				"anthropic-ratelimit-unified-5h-utilization": "NaN",
				"anthropic-ratelimit-unified-5h-reset":       "2026-10-18T03:20:00Z",
				"anthropic-ratelimit-unified-7d-utilization": "-0.1",
			},
			want: []expectedQuota{
				{code: "subscription", status: mgmt.QuotaAvailable},
				{code: "five_hour", status: mgmt.QuotaUnknown},
				{code: "seven_day", status: mgmt.QuotaUnknown},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			for name, value := range test.headers {
				header.Set(name, value)
			}
			quotas := RateLimitQuotas(header, observedAt)
			if test.want == nil {
				if quotas != nil {
					t.Fatalf("quotas = %+v, want nil", quotas)
				}
				return
			}
			if len(quotas) != len(test.want) {
				t.Fatalf("quotas = %+v, want %d entries", quotas, len(test.want))
			}
			for index, want := range test.want {
				got := quotas[index]
				if got.Code != want.code || got.Status != want.status || !samePercent(got.UsedPercent, want.usedPercent) || !sameTime(got.ResetsAt, want.resetsAt) {
					t.Fatalf("quota[%d] = %+v (used=%v resets=%v), want %+v", index, got, got.UsedPercent, got.ResetsAt, want)
				}
				if !got.ObservedAt.Equal(observedAt.Truncate(time.Microsecond)) || got.ObservedAt.Location() != time.UTC {
					t.Fatalf("observedAt = %v, want UTC %v", got.ObservedAt, observedAt)
				}
				if got.ResetsAt != nil && got.ResetsAt.Location() != time.UTC {
					t.Fatalf("resetsAt = %v, want UTC", got.ResetsAt)
				}
				if got.Code != "subscription" && (got.WindowDurationSeconds == nil || got.Unit == nil || *got.Unit != "PERCENT") {
					t.Fatalf("window quota missing duration or unit: %+v", got)
				}
			}
		})
	}
}

func samePercent(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
