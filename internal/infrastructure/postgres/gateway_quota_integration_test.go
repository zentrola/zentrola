package postgres

import (
	"bytes"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

func TestGatewayRecordsObservedSubscriptionQuotas(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	store := NewGatewayStore(pool)
	if _, err := pool.Exec(ctx, `INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES (41,'quota-test','Quota Test','OFFICIAL','ACTIVE','system','system',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential
(id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,auth_type,auth_adapter,subscription_type,created_by,updated_by,created_at,updated_at)
VALUES (61,41,'Claude',$1,$2,1,'SUBSCRIPTION','ANTHROPIC_CLAUDE_CODE','PERSONAL','system','system',now(),now())`,
		bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 12)); err != nil {
		t.Fatal(err)
	}
	route := gw.Route{ProviderID: 41, ResourceID: 61, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterClaudeCode}
	checkedAt := time.Date(2026, 10, 18, 3, 20, 0, 0, time.UTC)
	resetsAt := checkedAt.Add(2 * time.Hour)
	used := 50.0
	quotas := []mgmt.ResourceQuota{
		{Code: "subscription", Status: mgmt.QuotaAvailable, ObservedAt: checkedAt},
		{Code: "five_hour", Status: mgmt.QuotaAvailable, UsedPercent: &used, ResetsAt: &resetsAt, ObservedAt: checkedAt},
	}
	tests := []struct {
		name       string
		route      gw.Route
		status     string
		checkedAt  time.Time
		wantStatus string
		wantRows   int
	}{
		{name: "records observation", route: route, status: mgmt.QuotaAvailable, checkedAt: checkedAt, wantStatus: mgmt.QuotaAvailable, wantRows: 2},
		{name: "ignores older observation", route: route, status: mgmt.QuotaExhausted, checkedAt: checkedAt.Add(-time.Minute), wantStatus: mgmt.QuotaAvailable, wantRows: 2},
		{
			name:   "ignores changed authentication adapter",
			route:  gw.Route{ProviderID: 41, ResourceID: 61, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex},
			status: mgmt.QuotaExhausted, checkedAt: checkedAt.Add(time.Minute), wantStatus: mgmt.QuotaAvailable, wantRows: 2,
		},
		{name: "records newer observation", route: route, status: mgmt.QuotaNearLimit, checkedAt: checkedAt.Add(time.Minute), wantStatus: mgmt.QuotaNearLimit, wantRows: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.RecordResourceQuotas(ctx, test.route, test.status, &resetsAt, test.checkedAt, quotas); err != nil {
				t.Fatal(err)
			}
			var status string
			var version int64
			var storedReset time.Time
			var rows int
			if err := pool.QueryRow(ctx, `SELECT r.quota_status,r.version,r.quota_resets_at,
(SELECT count(*) FROM provider_credential_quota q WHERE q.provider_credential_id=r.id)
FROM provider_credential r WHERE r.id=61`).Scan(&status, &version, &storedReset, &rows); err != nil {
				t.Fatal(err)
			}
			// 被动额度观测不是配置变更，不能递增版本号与管理端乐观锁冲突。
			if status != test.wantStatus || version != 1 || !storedReset.Equal(resetsAt) || rows != test.wantRows {
				t.Fatalf("status=%s version=%d reset=%v rows=%d", status, version, storedReset, rows)
			}
		})
	}
}
