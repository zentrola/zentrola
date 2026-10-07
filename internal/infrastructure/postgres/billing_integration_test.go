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
    ) VALUES (9001,'CONFIRMED','API_KEY','CHARGE',5002,'API_KEY_USAGE',8001,$1,$2,25,12.5,'USD',$2)`, periodStart, periodEnd); err != nil {
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
