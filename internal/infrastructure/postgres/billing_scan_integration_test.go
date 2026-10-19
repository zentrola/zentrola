package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	app "github.com/zentrola/zentrola/internal/application/billing"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestBillingScansReturnOnlyPendingWork(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
         VALUES (1201,'MEMBER','Scan Alpha','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
         VALUES (2201,'scan-model','Scan Model','["TEXT"]','["TEXT"]','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3201,'scan-provider','Scan Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
         VALUES (4201,3201,2201,'scan-upstream',100,'system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,created_by,updated_by,created_at,updated_at
         ) VALUES
             (5201,3201,'Scan Priced Key',decode(repeat('33',32),'hex'),decode(repeat('33',12),'hex'),1,'API_KEY','TEST','system','system',$1,$1),
             (5202,3201,'Scan Unpriced Key',decode(repeat('34',32),'hex'),decode(repeat('34',12),'hex'),1,'API_KEY','TEST','system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,subscription_type,effective_at,created_by,updated_by,created_at,updated_at
         ) VALUES (5203,3201,'Scan Subscription',decode(repeat('35',32),'hex'),decode(repeat('35',12),'hex'),1,
             'SUBSCRIPTION','TEST','PERSONAL',$1,'system','system',$1,$1)`,
		`INSERT INTO provider_credential_model_price (
             id,provider_credential_id,provider_model_id,currency,input_price,output_price,cached_input_price,
             effective_at,created_by,created_at
         ) VALUES (6201,5201,4201,'CNY',5,10,1,$1,'system',$1)`,
	} {
		if _, err := pool.Exec(ctx, statement, periodStart); err != nil {
			t.Fatal(err)
		}
	}
	usageAt := periodStart.Add(time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO usage_record (
        id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,
        usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,
        started_at,completed_at,latency_ms,status,created_at
    ) VALUES
        (7201,'scan-priced-1',1,1201,3201,4201,5201,2201,'MODEL_GATEWAY','OPENAI_RESPONSES',10,5,0,$1,$1,0,'SUCCESS',$1),
        (7202,'scan-priced-2',1,1201,3201,4201,5201,2201,'MODEL_GATEWAY','OPENAI_RESPONSES',20,5,10,$1,$1,0,'SUCCESS',$1),
        (7203,'scan-incomplete',1,1201,3201,4201,5201,2201,'MODEL_GATEWAY','OPENAI_RESPONSES',20,NULL,0,$1,$1,0,'SUCCESS',$1),
        (7204,'scan-unpriced',1,1201,3201,4201,5202,2201,'MODEL_GATEWAY','OPENAI_RESPONSES',20,5,0,$1,$1,0,'SUCCESS',$1),
        (7205,'scan-subscription',1,1201,3201,4201,5203,2201,'MODEL_GATEWAY','OPENAI_RESPONSES',20,5,0,$1,$1,0,'SUCCESS',$1)`,
		usageAt); err != nil {
		t.Fatal(err)
	}

	store := NewBillingStore(pool)
	service := app.New(store, idgen.New(pool))
	candidateIDs := func(scan app.RatingScan) []int64 {
		t.Helper()
		candidates, err := store.UsageRatingCandidates(ctx, scan, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.UsageRecordID)
		}
		return ids
	}
	periodCount := func() int {
		t.Helper()
		periods, err := store.APIKeyPeriods(ctx, periodEnd)
		if err != nil {
			t.Fatal(err)
		}
		return len(periods)
	}

	if ids := candidateIDs(app.RatingScanUnrated); len(ids) != 2 || ids[0] != 7201 || ids[1] != 7202 {
		t.Fatalf("unrated candidates=%v; want priced API key usage only", ids)
	}
	if ids := candidateIDs(app.RatingScanRepriced); len(ids) != 0 {
		t.Fatalf("repriced candidates before rating=%v", ids)
	}
	if summary, err := service.RatePendingAPIKeyUsage(ctx, 1); err != nil || summary.Created != 2 || summary.Failed != 0 {
		t.Fatalf("rating summary=%+v err=%v", summary, err)
	}
	if ids := candidateIDs(app.RatingScanUnrated); len(ids) != 0 {
		t.Fatalf("unrated candidates after rating=%v", ids)
	}
	if count := periodCount(); count != 1 {
		t.Fatalf("unsettled periods before settlement=%d", count)
	}
	if summary, err := service.SettleDueAPIKeys(ctx, periodEnd); err != nil || summary.Created != 1 {
		t.Fatalf("settlement=%+v err=%v", summary, err)
	}
	if count := periodCount(); count != 0 {
		t.Fatalf("settled period is still scanned: %d", count)
	}

	if _, err := pool.Exec(ctx, `UPDATE provider_credential_model_price SET output_price=12 WHERE id=6201`); err != nil {
		t.Fatal(err)
	}
	if ids := candidateIDs(app.RatingScanRepriced); len(ids) != 2 || ids[0] != 7201 || ids[1] != 7202 {
		t.Fatalf("repriced candidates=%v", ids)
	}
	if summary, err := service.RatePendingAPIKeyUsage(ctx, 100); err != nil || summary.Created != 2 {
		t.Fatalf("rerating summary=%+v err=%v", summary, err)
	}
	// 被新 revision 替代的旧核算价格快照仍与当前价格不同，但不能再次成为候选。
	if ids := candidateIDs(app.RatingScanRepriced); len(ids) != 0 {
		t.Fatalf("superseded ratings were scanned again: %v", ids)
	}
	if _, err := pool.Exec(ctx, `UPDATE provider_credential_model_price SET input_price=3 WHERE id=6201`); err != nil {
		t.Fatal(err)
	}
	if ids := candidateIDs(app.RatingScanRepriced); len(ids) != 2 {
		t.Fatalf("lower price correction candidates=%v", ids)
	}
	if summary, err := service.RatePendingAPIKeyUsage(ctx, 100); err != nil || summary.Created != 2 {
		t.Fatalf("second rerating summary=%+v err=%v", summary, err)
	}
	if count := periodCount(); count != 1 {
		t.Fatalf("corrected period was not rescanned: %d", count)
	}
	if summary, err := service.SettleDueAPIKeys(ctx, periodEnd); err != nil || summary.Adjusted != 1 {
		t.Fatalf("correction settlement=%+v err=%v", summary, err)
	}
	if count := periodCount(); count != 0 {
		t.Fatalf("adjusted period is still scanned: %d", count)
	}
	var revisions int
	if err := pool.QueryRow(ctx, `SELECT max(revision) FROM usage_rating WHERE usage_record_id=7201`).Scan(&revisions); err != nil || revisions != 3 {
		t.Fatalf("latest revision=%d err=%v", revisions, err)
	}
}

func TestSubscriptionChargeOverlapIsRejected(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	original := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	backfilled := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3301,'overlap-provider','Overlap Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`, []any{original}},
		{`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,subscription_type,effective_at,created_by,updated_by,created_at,updated_at
         ) VALUES (5301,3301,'Overlap Subscription',decode(repeat('36',32),'hex'),decode(repeat('36',12),'hex'),1,
             'SUBSCRIPTION','TEST','PERSONAL',$2,'system','system',$1,$1)`, []any{original, backfilled}},
		{`INSERT INTO provider_credential_subscription_price (
             id,provider_credential_id,currency,period_amount,billing_period,effective_at,created_by,created_at
         ) VALUES (6301,5301,'CNY',100,'MONTH',$1,'system',$1)`, []any{original}},
	} {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	store := NewBillingStore(pool)
	service := app.New(store, idgen.New(pool))
	if summary, err := service.SettleDueSubscriptions(ctx, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)); err != nil || summary.Created != 1 {
		t.Fatalf("initial settlement=%+v err=%v", summary, err)
	}
	// 绕过管理端校验直接补录更早价格，模拟历史数据：锚点漂移后结算必须跳过重叠账期。
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential_subscription_price (
        id,provider_credential_id,currency,period_amount,billing_period,effective_at,created_by,created_at
    ) VALUES (6302,5301,'CNY',90,'MONTH',$1,'system',$1)`, backfilled); err != nil {
		t.Fatal(err)
	}
	summary, err := service.SettleDueSubscriptions(ctx, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if err != nil || summary.Failed == 0 || !strings.Contains(strings.Join(summary.FailureSamples, ";"), "overlaps") {
		t.Fatalf("overlap settlement=%+v err=%v", summary, err)
	}
	var overlaps int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_document a JOIN billing_document b
        ON a.provider_credential_id=b.provider_credential_id AND a.id<b.id
       AND a.period_start<b.period_end AND a.period_end>b.period_start
        WHERE a.billing_type='SUBSCRIPTION' AND b.billing_type='SUBSCRIPTION'
          AND a.document_type='CHARGE' AND b.document_type='CHARGE'`).Scan(&overlaps); err != nil || overlaps != 0 {
		t.Fatalf("overlapping charges=%d err=%v", overlaps, err)
	}

	// 数据库触发器作为最后防线拒绝部分重叠的新账期。
	_, err = store.CreateCharge(ctx, app.Document{
		ID: 9301, CredentialID: 5301, SourceID: 6301, BillingType: "SUBSCRIPTION", DocumentType: "CHARGE",
		Status: "CONFIRMED", SourceType: "SUBSCRIPTION_PRICE",
		PeriodStart: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		TotalAmount: "100", Currency: "CNY", CreatedAt: time.Now().UTC(),
	})
	var storeErr *app.StoreError
	var pgErr *pgconn.PgError
	if !errors.As(err, &storeErr) || storeErr.SQLState != "23P01" || !errors.As(err, &pgErr) ||
		!strings.Contains(app.SafeErrorText(err), "overlaps an existing charge") {
		t.Fatalf("overlapping charge error=%v", err)
	}
}
