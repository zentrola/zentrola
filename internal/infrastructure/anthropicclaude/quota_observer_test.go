package anthropicclaude

import (
	"context"
	"net/http"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

type recordedQuota struct {
	resourceID int64
	status     string
	resetsAt   *time.Time
	checkedAt  time.Time
	quotas     []mgmt.ResourceQuota
}

type quotaRecorderStub struct{ records []recordedQuota }

func (r *quotaRecorderStub) RecordResourceQuotas(_ context.Context, route gw.Route, status string, resetsAt *time.Time, checkedAt time.Time, quotas []mgmt.ResourceQuota) error {
	r.records = append(r.records, recordedQuota{resourceID: route.ResourceID, status: status, resetsAt: resetsAt, checkedAt: checkedAt, quotas: quotas})
	return nil
}

func unifiedHeader(values map[string]string) http.Header {
	header := make(http.Header)
	for name, value := range values {
		header.Set(name, value)
	}
	return header
}

func TestQuotaObserverRecordsThrottledObservations(t *testing.T) {
	claude := gw.Route{ProviderID: 7, ResourceID: 9, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: AdapterCode}
	allowed := unifiedHeader(map[string]string{
		"anthropic-ratelimit-unified-status":         "allowed",
		"anthropic-ratelimit-unified-5h-utilization": "0.5",
		"anthropic-ratelimit-unified-5h-reset":       "1792000000",
	})
	warning := unifiedHeader(map[string]string{
		"anthropic-ratelimit-unified-status":         "allowed_warning",
		"anthropic-ratelimit-unified-reset":          "1792000000",
		"anthropic-ratelimit-unified-5h-utilization": "0.92",
		"anthropic-ratelimit-unified-5h-reset":       "1792000000",
	})
	rejected := unifiedHeader(map[string]string{
		"anthropic-ratelimit-unified-status":               "rejected",
		"anthropic-ratelimit-unified-reset":                "1792000000",
		"anthropic-ratelimit-unified-representative-claim": "five_hour",
	})
	rejectedWithoutReset := unifiedHeader(map[string]string{
		"anthropic-ratelimit-unified-status":               "rejected",
		"anthropic-ratelimit-unified-representative-claim": "five_hour",
	})
	type observation struct {
		route  gw.Route
		status int
		header http.Header
		after  time.Duration
	}
	tests := []struct {
		name         string
		observations []observation
		want         []string
	}{
		{name: "first observation", observations: []observation{{route: claude, status: 200, header: allowed}}, want: []string{mgmt.QuotaAvailable}},
		{
			name: "unchanged observation within interval",
			observations: []observation{
				{route: claude, status: 200, header: allowed},
				{route: claude, status: 200, header: allowed, after: time.Minute},
			},
			want: []string{mgmt.QuotaAvailable},
		},
		{
			name: "unchanged observation after interval",
			observations: []observation{
				{route: claude, status: 200, header: allowed},
				{route: claude, status: 200, header: allowed, after: defaultQuotaRecordInterval},
			},
			want: []string{mgmt.QuotaAvailable, mgmt.QuotaAvailable},
		},
		{
			name: "status change within interval",
			observations: []observation{
				{route: claude, status: 200, header: allowed},
				{route: claude, status: 200, header: warning, after: time.Second},
				{route: claude, status: 429, header: rejected, after: time.Second},
			},
			want: []string{mgmt.QuotaAvailable, mgmt.QuotaNearLimit, mgmt.QuotaExhausted},
		},
		{
			name:         "rejected but served response stays routable",
			observations: []observation{{route: claude, status: 200, header: rejected}},
			want:         []string{mgmt.QuotaNearLimit},
		},
		{
			name:         "exhausted without reset time is skipped",
			observations: []observation{{route: claude, status: 429, header: rejectedWithoutReset}},
		},
		{name: "response without unified headers", observations: []observation{{route: claude, status: 200, header: make(http.Header)}}},
		{
			name: "non Claude subscription route",
			observations: []observation{
				{route: gw.Route{ProviderID: 7, ResourceID: 9, AuthType: mgmt.AuthTypeAPIKey}, status: 200, header: allowed},
				{route: gw.Route{ProviderID: 7, ResourceID: 9, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex}, status: 200, header: allowed},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &quotaRecorderStub{}
			observer := NewQuotaObserver(recorder, nil)
			now := time.Date(2026, 10, 18, 3, 20, 0, 0, time.UTC)
			observer.now = func() time.Time { return now }
			observer.run = func(record func()) { record() }
			for _, item := range test.observations {
				now = now.Add(item.after)
				observer.ObserveResponse(context.Background(), item.route, item.status, item.header)
			}
			if len(recorder.records) != len(test.want) {
				t.Fatalf("records = %+v, want statuses %v", recorder.records, test.want)
			}
			for index, status := range test.want {
				record := recorder.records[index]
				if record.status != status || record.resourceID != claude.ResourceID || len(record.quotas) == 0 || record.checkedAt.Location() != time.UTC {
					t.Fatalf("record[%d] = %+v, want status %s", index, record, status)
				}
			}
		})
	}
}
