package postgres

import (
	"testing"
	"time"

	app "github.com/zentrola/zentrola/internal/application/billing"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestSubscriptionBillingSettlementCorrectionAndStatistics(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
         VALUES (1001,'MEMBER','Alpha','ACTIVE','system','system',$1,$1),
                (1002,'MEMBER','Beta','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
         VALUES (2001,'billing-model','Billing Model','["TEXT"]','["TEXT"]','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3001,'billing-provider','Billing Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
         VALUES (4001,3001,2001,'billing-upstream',100,'system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,subscription_type,effective_at,
             created_by,updated_by,created_at,updated_at
         ) VALUES (
             5001,3001,'Personal Subscription',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,
             'SUBSCRIPTION','TEST','PERSONAL',$1,'system','system',$1,$1
         )`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,created_by,updated_by,created_at,updated_at
         ) VALUES (
             5002,3001,'API Key',decode(repeat('11',32),'hex'),decode(repeat('11',12),'hex'),1,
             'API_KEY','TEST','system','system',$1,$1
         )`,
		`INSERT INTO provider_credential_subscription_price (
             id,provider_credential_id,currency,period_amount,billing_period,effective_at,created_by,created_at
         ) VALUES (6001,5001,'CNY',1000,'MONTH',$1,'system',$1)`,
	} {
		if _, err := pool.Exec(ctx, statement, periodStart); err != nil {
			t.Fatal(err)
		}
	}
	usageAt := periodStart.Add(time.Hour)
	_, err := pool.Exec(ctx, `INSERT INTO usage_record (
        id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,
        usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,
        started_at,completed_at,latency_ms,status,created_at
    ) VALUES
        (7001,'billing-alpha',1,1001,3001,4001,5001,2001,'MODEL_GATEWAY','OPENAI_RESPONSES',60,10,0,$1,$1,0,'SUCCESS',$1),
        (7002,'billing-beta',1,1002,3001,4001,5001,2001,'MODEL_GATEWAY','OPENAI_RESPONSES',20,10,0,$1,$1,0,'SUCCESS',$1)`, usageAt)
	if err != nil {
		t.Fatal(err)
	}

	service := app.New(NewBillingStore(pool), idgen.New(pool))
	summary, err := service.SettleDueSubscriptions(ctx, periodEnd)
	if err != nil || summary.Created != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	var amount string
	var documents, items int64
	if err := pool.QueryRow(ctx, `SELECT total_amount::text FROM billing_document WHERE document_type='CHARGE'`).Scan(&amount); err != nil || amount != "1000.00000000" {
		t.Fatalf("charge amount=%q err=%v", amount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE amount IN (700,300)) FROM billing_document_item`).Scan(&items, &documents); err != nil || items != 2 || documents != 2 {
		t.Fatalf("items=%d allocated=%d err=%v", items, documents, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE provider_credential_subscription_price SET period_amount=800 WHERE id=6001`); err != nil {
		t.Fatal(err)
	}
	summary, err = service.SettleDueSubscriptions(ctx, periodEnd)
	if err != nil || summary.Adjusted != 1 {
		t.Fatalf("adjustment summary=%+v err=%v", summary, err)
	}
	if err := pool.QueryRow(ctx, `SELECT total_amount::text FROM billing_document WHERE document_type='ADJUSTMENT'`).Scan(&amount); err != nil || amount != "-200.00000000" {
		t.Fatalf("adjustment amount=%q err=%v", amount, err)
	}

	statistics, err := service.Statistics(ctx, admin.Identity{ID: 1}, app.StatisticsFilter{
		BillingType: "SUBSCRIPTION", From: periodStart, To: periodEnd, Limit: 50,
	})
	if err != nil || len(statistics.Totals) != 1 || statistics.Totals[0].TotalAmount != "800" ||
		statistics.Totals[0].BillingType != "SUBSCRIPTION" || statistics.Totals[0].TotalTokens != 100 ||
		len(statistics.Items) != 2 || statistics.Items[0].BillingType != "SUBSCRIPTION" || statistics.Total != 2 {
		t.Fatalf("statistics=%+v err=%v", statistics, err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO billing_document (
        id,status,billing_type,document_type,provider_credential_id,source_type,source_id,
        period_start,period_end,total_tokens,total_amount,currency,created_at
    ) VALUES (9001,'CONFIRMED','API_KEY','CHARGE',5002,'API_KEY_USAGE',NULL,$1,$2,25,12.5,'USD',$2)`, periodStart, periodEnd); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO billing_document_item (
        id,billing_document_id,principal_id,usage_tokens,allocation_ratio,amount,created_at
    ) VALUES (9002,9001,1001,25,NULL,12.5,$1)`, periodEnd); err != nil {
		t.Fatal(err)
	}
	apiKeyStatistics, err := service.Statistics(ctx, admin.Identity{ID: 1}, app.StatisticsFilter{
		BillingType: "API_KEY", From: periodStart, To: periodEnd, Limit: 50,
	})
	if err != nil || len(apiKeyStatistics.Totals) != 1 || apiKeyStatistics.Totals[0].BillingType != "API_KEY" ||
		apiKeyStatistics.Totals[0].TotalAmount != "12.5" || apiKeyStatistics.Totals[0].TotalTokens != 25 ||
		len(apiKeyStatistics.Items) != 1 || apiKeyStatistics.Items[0].BillingType != "API_KEY" ||
		apiKeyStatistics.Items[0].Amount != "12.5" || apiKeyStatistics.Total != 1 {
		t.Fatalf("api key statistics=%+v err=%v", apiKeyStatistics, err)
	}
	allStatistics, err := service.Statistics(ctx, admin.Identity{ID: 1}, app.StatisticsFilter{
		From: periodStart, To: periodEnd, Limit: 50,
	})
	if err != nil || len(allStatistics.Totals) != 2 || len(allStatistics.Items) != 3 || allStatistics.Total != 3 {
		t.Fatalf("all statistics=%+v err=%v", allStatistics, err)
	}

	documentsPage, err := service.Documents(ctx, admin.Identity{ID: 1}, app.DocumentFilter{
		BillingType: "SUBSCRIPTION", From: periodStart, To: periodEnd, Limit: 1,
	})
	if err != nil || documentsPage.Total != 2 || len(documentsPage.Items) != 2 ||
		documentsPage.Items[0].DocumentType != "ADJUSTMENT" ||
		documentsPage.Items[0].CredentialName != "Personal Subscription" {
		t.Fatalf("documents=%+v err=%v", documentsPage, err)
	}
	adjustmentDetail, err := service.Document(ctx, admin.Identity{ID: 1}, documentsPage.Items[0].ID)
	if err != nil || adjustmentDetail.Original == nil || len(adjustmentDetail.Items) != 2 ||
		adjustmentDetail.Items[0].AllocationRatio == nil || len(adjustmentDetail.Adjustments) != 0 {
		t.Fatalf("adjustment detail=%+v err=%v", adjustmentDetail, err)
	}
	apiKeyDetail, err := service.Document(ctx, admin.Identity{ID: 1}, 9001)
	if err != nil || len(apiKeyDetail.Items) != 1 || apiKeyDetail.Items[0].AllocationRatio != nil ||
		apiKeyDetail.RatingCount != 0 {
		t.Fatalf("api key detail=%+v err=%v", apiKeyDetail, err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential_model_price (
        id,provider_credential_id,provider_model_id,currency,input_price,output_price,cached_input_price,
        effective_at,created_by,created_at
    ) VALUES (6002,5002,4001,'CNY',5,10,1,$1,'system',$1)`, usageAt.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO usage_record (
        id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,
        usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,
        started_at,completed_at,latency_ms,status,created_at
    ) VALUES
        (7003,'billing-incomplete',1,1001,3001,4001,5002,2001,'MODEL_GATEWAY','OPENAI_RESPONSES',NULL,5,NULL,$1,$1,0,'SUCCESS',$1),
        (7004,'billing-missing-price',1,1001,3001,4001,5002,2001,'MODEL_GATEWAY','OPENAI_RESPONSES',5,5,0,$1,$1,0,'SUCCESS',$1),
        (7005,'billing-pending',1,1002,3001,4001,5002,2001,'MODEL_GATEWAY','OPENAI_RESPONSES',5,5,0,$2,$2,0,'SUCCESS',$2)`,
		usageAt, usageAt.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	unrated, err := service.Unrated(ctx, admin.Identity{ID: 1}, app.UnratedUsageFilter{
		From: periodStart, To: periodEnd, Limit: 50,
	})
	if err != nil || unrated.Total != 3 || len(unrated.Items) != 3 ||
		unrated.Items[0].Reason != "INCOMPLETE_TOKENS" || unrated.Items[0].InputTokens != nil ||
		unrated.Items[1].Reason != "MISSING_PRICE" || unrated.Items[2].Reason != "PENDING_RATING" {
		t.Fatalf("unrated=%+v err=%v", unrated, err)
	}

	for _, statement := range []string{
		`UPDATE billing_document SET total_amount=1`,
		`DELETE FROM billing_document_item`,
		`UPDATE provider_credential_subscription_price SET id=6002 WHERE id=6001`,
		`UPDATE provider_credential_subscription_price SET currency='USD' WHERE id=6001`,
		`UPDATE provider_credential_subscription_price SET billing_period='YEAR' WHERE id=6001`,
		`UPDATE provider_credential_subscription_price SET effective_at=effective_at + interval '1 day' WHERE id=6001`,
		`DELETE FROM provider_credential_subscription_price WHERE id=6001`,
		`INSERT INTO billing_document (
            id,status,billing_type,document_type,provider_credential_id,source_type,source_id,
            period_start,period_end,total_tokens,total_amount,currency,created_at
         ) VALUES (
            9010,'CONFIRMED','SUBSCRIPTION','CHARGE',5001,'SUBSCRIPTION_PRICE',9999,
            '2027-01-01T00:00:00Z','2027-02-01T00:00:00Z',0,800,'CNY','2027-02-01T00:00:00Z'
         )`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("append-only billing mutation accepted: %s", statement)
		}
	}
}

func TestAPIKeyUsageRatingSettlementCorrectionAndLateUsage(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
         VALUES (1101,'MEMBER','API Alpha','ACTIVE','system','system',$1,$1),
                (1102,'APPLICATION','API App','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
         VALUES (2101,'api-billing-model','API Billing Model','["TEXT"]','["TEXT"]','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3101,'api-billing-provider','API Billing Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
         VALUES (4101,3101,2101,'api-billing-upstream',100,'system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,created_by,updated_by,created_at,updated_at
         ) VALUES (
             5101,3101,'Rated API Key',decode(repeat('22',32),'hex'),decode(repeat('22',12),'hex'),1,
             'API_KEY','TEST','system','system',$1,$1
         )`,
		`INSERT INTO provider_credential_model_price (
             id,provider_credential_id,provider_model_id,currency,input_price,output_price,cached_input_price,
             effective_at,created_by,created_at
         ) VALUES (6101,5101,4101,'CNY',5,10,1,$1,'system',$1)`,
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
        (7101,'api-rated-alpha',1,1101,3101,4101,5101,2101,'MODEL_GATEWAY','OPENAI_RESPONSES',5,0,0,$1,$1,0,'SUCCESS',$1),
        (7102,'api-rated-app',1,1102,3101,4101,5101,2101,'MODEL_GATEWAY','OPENAI_RESPONSES',100,20,40,$1,$1,0,'SUCCESS',$1)`, usageAt); err != nil {
		t.Fatal(err)
	}

	service := app.New(NewBillingStore(pool), idgen.New(pool))
	ratingSummary, err := service.RatePendingAPIKeyUsage(ctx, 100)
	if err != nil || ratingSummary.Created != 2 {
		t.Fatalf("rating summary=%+v err=%v", ratingSummary, err)
	}
	var tinyCost string
	if err := pool.QueryRow(ctx, `SELECT total_cost::text FROM usage_rating WHERE usage_record_id=7101`).Scan(&tinyCost); err != nil || tinyCost != "0.00002500000000" {
		t.Fatalf("tiny cost=%q err=%v", tinyCost, err)
	}

	settlement, err := service.SettleDueAPIKeys(ctx, periodEnd)
	if err != nil || settlement.Created != 1 {
		t.Fatalf("settlement=%+v err=%v", settlement, err)
	}
	var chargeAmount string
	var chargeTokens int64
	if err := pool.QueryRow(ctx, `SELECT total_amount::text,total_tokens FROM billing_document
        WHERE billing_type='API_KEY' AND document_type='CHARGE'`).Scan(&chargeAmount, &chargeTokens); err != nil ||
		chargeAmount != "0.00056500" || chargeTokens != 125 {
		t.Fatalf("charge amount=%q tokens=%d err=%v", chargeAmount, chargeTokens, err)
	}

	if _, err := pool.Exec(ctx, `UPDATE provider_credential_model_price
        SET input_price=4,output_price=8,cached_input_price=0.5 WHERE id=6101`); err != nil {
		t.Fatal(err)
	}
	ratingSummary, err = service.RatePendingAPIKeyUsage(ctx, 100)
	if err != nil || ratingSummary.Created != 2 {
		t.Fatalf("rerating summary=%+v err=%v", ratingSummary, err)
	}
	settlement, err = service.SettleDueAPIKeys(ctx, periodEnd)
	if err != nil || settlement.Adjusted != 1 {
		t.Fatalf("correction settlement=%+v err=%v", settlement, err)
	}
	var adjustmentAmount string
	var adjustmentTokens int64
	if err := pool.QueryRow(ctx, `SELECT total_amount::text,total_tokens FROM billing_document
        WHERE billing_type='API_KEY' AND document_type='ADJUSTMENT'`).Scan(&adjustmentAmount, &adjustmentTokens); err != nil ||
		adjustmentAmount != "-0.00012500" || adjustmentTokens != 0 {
		t.Fatalf("adjustment amount=%q tokens=%d err=%v", adjustmentAmount, adjustmentTokens, err)
	}

	lateAt := periodEnd.Add(-time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO usage_record (
        id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,
        usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,
        started_at,completed_at,latency_ms,status,created_at
    ) VALUES (7103,'api-rated-late',1,1101,3101,4101,5101,2101,'MODEL_GATEWAY','OPENAI_RESPONSES',5,0,0,$1,$1,0,'SUCCESS',$1)`, lateAt); err != nil {
		t.Fatal(err)
	}
	if ratingSummary, err = service.RatePendingAPIKeyUsage(ctx, 100); err != nil || ratingSummary.Created != 1 {
		t.Fatalf("late rating summary=%+v err=%v", ratingSummary, err)
	}
	if settlement, err = service.SettleDueAPIKeys(ctx, periodEnd); err != nil || settlement.Adjusted != 1 {
		t.Fatalf("late settlement=%+v err=%v", settlement, err)
	}
	var netTokens int64
	if err := pool.QueryRow(ctx, `SELECT SUM(total_tokens) FROM billing_document WHERE billing_type='API_KEY'`).Scan(&netTokens); err != nil || netTokens != 130 {
		t.Fatalf("net tokens=%d err=%v", netTokens, err)
	}

	for _, statement := range []string{
		`UPDATE usage_rating SET total_cost=0`,
		`DELETE FROM usage_rating`,
		`UPDATE provider_credential_model_price SET currency='USD' WHERE id=6101`,
		`UPDATE provider_credential_model_price SET effective_at=effective_at + interval '1 day' WHERE id=6101`,
		`DELETE FROM provider_credential_model_price WHERE id=6101`,
		`INSERT INTO provider_credential_model_price (
            id,provider_credential_id,provider_model_id,currency,input_price,output_price,cached_input_price,
            effective_at,created_by,created_at
         ) VALUES (6102,5101,4101,'CNY',1,1,1,'2026-09-15T00:00:00Z','system','2026-09-15T00:00:00Z')`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("api key billing audit protection accepted mutation: %s", statement)
		}
	}

	statistics, err := service.Statistics(ctx, admin.Identity{ID: 1}, app.StatisticsFilter{
		BillingType: "API_KEY", From: periodStart, To: periodEnd, Limit: 50,
	})
	if err != nil || len(statistics.Totals) != 1 || statistics.Totals[0].TotalTokens != 130 ||
		statistics.Totals[0].TotalAmount != "0.00046" || len(statistics.Items) != 2 {
		t.Fatalf("statistics=%+v err=%v", statistics, err)
	}
}
