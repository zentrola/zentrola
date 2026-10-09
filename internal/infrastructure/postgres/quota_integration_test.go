package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func TestMonthlyQuotaFactsIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	withFixture(t, ctx, pool, func(tx pgx.Tx) {
		mustExec(t, ctx, tx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,created_by,updated_by,created_at,updated_at)
VALUES (60,40,'Resource',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'system','system',now(),now())`)
		mustExec(t, ctx, tx, `INSERT INTO principal_group_membership (id,principal_id,group_id,created_by,updated_by,created_at,updated_at)
VALUES (80,10,20,'system','system',now(),now())`)
		mustExec(t, ctx, tx, `INSERT INTO principal_group_model_permission (id,group_id,model_id,created_by,updated_by,created_at,updated_at)
VALUES (90,20,30,'system','system',now(),now())`)
		q := dbgen.New(tx)
		configuration, err := q.GatewayTokenQuotaConfiguration(ctx, dbgen.GatewayTokenQuotaConfigurationParams{
			PrincipalID: 10, ModelID: 30,
		})
		if err != nil {
			t.Fatal(err)
		}
		var groups []struct {
			ID    int64  `json:"id"`
			Limit *int64 `json:"limit"`
		}
		if err := json.Unmarshal([]byte(configuration.GroupQuotas), &groups); err != nil || len(groups) != 1 || groups[0].ID != 20 || groups[0].Limit != nil {
			t.Fatalf("unlimited authorized group missing from call-time attribution: groups=%v err=%v", groups, err)
		}
		first := []byte(`[{"request_id":"quota-1","attempt_no":1,"client_protocol":"ANTHROPIC_MESSAGES","principal_id":10,"provider_id":40,"provider_model_id":50,"provider_credential_id":60,"model_id":30,"quota_group_ids":[20],"input_tokens":70,"output_tokens":30,"started_at":"2026-10-08T00:00:00Z","completed_at":"2026-10-08T00:00:01Z","latency_ms":1000,"status":"SUCCESS"}]`)
		if err := q.InsertUsageAttempts(ctx, first); err != nil {
			t.Fatal(err)
		}
		assertMonthlyQuotaTotals(t, ctx, q, 100, 100)
		if err := q.InsertUsageAttempts(ctx, first); err != nil {
			t.Fatal("duplicate attempt should be idempotent:", err)
		}
		assertMonthlyQuotaTotals(t, ctx, q, 100, 100)

		mustExec(t, ctx, tx, `UPDATE principal_group_membership SET is_deleted=true WHERE id=80`)
		mustExec(t, ctx, tx, `UPDATE principal_group_model_permission SET is_deleted=true WHERE id=90`)
		assertMonthlyQuotaTotals(t, ctx, q, 100, 100)

		second := []byte(`[{"request_id":"quota-2","attempt_no":1,"client_protocol":"ANTHROPIC_MESSAGES","principal_id":10,"provider_id":40,"provider_model_id":50,"provider_credential_id":60,"model_id":30,"quota_group_ids":[],"input_tokens":20,"output_tokens":10,"started_at":"2026-10-08T00:01:00Z","completed_at":"2026-10-08T00:01:01Z","latency_ms":1000,"status":"SUCCESS"}]`)
		if err := q.InsertUsageAttempts(ctx, second); err != nil {
			t.Fatal(err)
		}
		assertMonthlyQuotaTotals(t, ctx, q, 130, 100)
	})
}

func assertMonthlyQuotaTotals(t *testing.T, ctx context.Context, q *dbgen.Queries, principal, group int64) {
	t.Helper()
	period := pgtype.Timestamptz{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	principals, err := q.QuotaPrincipalUsedTokens(ctx, dbgen.QuotaPrincipalUsedTokensParams{
		ScopeIds: []int64{10}, PeriodStart: period,
	})
	if err != nil || len(principals) != 1 || principals[0].UsedTokens != principal {
		t.Fatalf("principal total=%v want=%d err=%v", principals, principal, err)
	}
	groups, err := q.QuotaGroupUsedTokens(ctx, dbgen.QuotaGroupUsedTokensParams{
		ScopeIds: []int64{20}, PeriodStart: period,
	})
	if err != nil || len(groups) != 1 || groups[0].UsedTokens != group {
		t.Fatalf("group total=%v want=%d err=%v", groups, group, err)
	}
}
