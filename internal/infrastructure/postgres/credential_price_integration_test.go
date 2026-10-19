package postgres

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestCredentialPriceIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	for _, statement := range []string{
		`INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
VALUES (70,'price-admin','test-hash','Price Admin','ACTIVE','system','system',now(),now())`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES (40,'price-provider','Price Provider','OFFICIAL','ACTIVE','system','system',now(),now())`,
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
VALUES (30,'price-model','Price Model','["TEXT"]','["TEXT"]','ACTIVE','system','system',now(),now())`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
VALUES (50,40,30,'price-model',100,'system','system',now(),now())`,
		`INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,created_by,updated_by,created_at,updated_at)
VALUES (60,40,'Price API Key',decode(repeat('11',32),'hex'),decode(repeat('22',12),'hex'),1,'system','system',now(),now())`,
		`INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,auth_type,auth_adapter,subscription_type,key_version,created_by,updated_by,created_at,updated_at)
VALUES (61,40,'Price Subscription',decode(repeat('33',32),'hex'),decode(repeat('44',12),'hex'),'SUBSCRIPTION','OPENAI_CODEX','PERSONAL',1,'system','system',now(),now())`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	ids := idgen.New(pool)
	var diagnostics bytes.Buffer
	store := NewManagementStore(pool, ids, slog.New(slog.NewTextHandler(&diagnostics, nil)))
	service := mgmt.New(store, ids, nil, nil, mgmt.WithPriceStore(store))
	actor := admin.Identity{ID: 70, DisplayName: "Price Admin"}
	modelInput := mgmt.ModelPriceInput{ProviderModelID: 50, Currency: "USD", InputPrice: "1.25", OutputPrice: "2.5", CachedInputPrice: "0.5",
		EffectiveAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}
	first, err := service.SaveModelPrice(ctx, actor, 60, modelInput, appsec.RequestMeta{})
	if err != nil {
		t.Fatalf("save model price: %v; diagnostics: %s", err, diagnostics.String())
	}
	modelInput.OutputPrice = "3"
	modelInput.EffectiveAt = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	latest, err := service.SaveModelPrice(ctx, actor, 60, modelInput, appsec.RequestMeta{})
	if err != nil || latest.ID == first.ID {
		t.Fatalf("model price version = %+v, err = %v", latest, err)
	}
	subscription, err := service.SaveSubscriptionPrice(ctx, actor, 61, mgmt.SubscriptionPriceInput{
		Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH",
		EffectiveAt: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
	}, appsec.RequestMeta{})
	if err != nil || subscription.PeriodAmount != "100" {
		t.Fatalf("subscription price = %+v, err = %v", subscription, err)
	}
	modelPrices, err := service.CredentialPrices(ctx, actor, 60)
	if err != nil || len(modelPrices.ModelPrices) != 2 || modelPrices.ModelPrices[0].ID != latest.ID || modelPrices.ModelPrices[1].ID != first.ID {
		t.Fatalf("model prices = %+v, err = %v", modelPrices, err)
	}
	subscriptionPrices, err := service.CredentialPrices(ctx, actor, 61)
	if err != nil || subscriptionPrices.SubscriptionPrice == nil || subscriptionPrices.SubscriptionPrice.ID != subscription.ID {
		t.Fatalf("subscription prices = %+v, err = %v", subscriptionPrices, err)
	}
	resources, err := service.Resources(ctx, actor, mgmt.Page{Limit: 10})
	if err != nil {
		t.Fatalf("list resources with subscription price: %v", err)
	}
	var listedSubscription *mgmt.Resource
	for index := range resources.Items {
		if resources.Items[index].ID == 61 {
			listedSubscription = &resources.Items[index]
			break
		}
	}
	if listedSubscription == nil || listedSubscription.SubscriptionPrice == nil ||
		listedSubscription.SubscriptionPrice.PeriodAmount != "100" ||
		!listedSubscription.SubscriptionPrice.EffectiveAt.Equal(subscription.EffectiveAt) {
		t.Fatalf("listed subscription resource = %+v", listedSubscription)
	}
	var versions, audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM provider_credential_model_price WHERE provider_credential_id=60`).Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("model versions = %d, err = %v", versions, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log WHERE operation_type='RESOURCE_PRICE_UPDATE'`).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("price audit count = %d, err = %v", audits, err)
	}
}

// 订阅价格的已出账检查必须与结算出账串行化：结算事务已写入但尚未提交账期单据时，
// 价格写入需等待其提交并据此拒绝，不能基于未见到的账期放行。
func TestSubscriptionPriceWriteWaitsForInFlightCharge(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
VALUES (71,'race-price-admin','test-hash','Race Price Admin','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES (41,'race-price-provider','Race Price Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,auth_type,auth_adapter,subscription_type,effective_at,key_version,created_by,updated_by,created_at,updated_at)
VALUES (62,41,'Race Save Subscription',decode(repeat('55',32),'hex'),decode(repeat('55',12),'hex'),'SUBSCRIPTION','OPENAI_CODEX','PERSONAL',$1,1,'system','system',$1,$1),
       (63,41,'Race Move Subscription',decode(repeat('66',32),'hex'),decode(repeat('66',12),'hex'),'SUBSCRIPTION','OPENAI_CODEX','PERSONAL',$1,1,'system','system',$1,$1)`,
		`INSERT INTO provider_credential_subscription_price (id,provider_credential_id,currency,period_amount,billing_period,effective_at,created_by,created_at)
VALUES (6201,62,'CNY',100,'MONTH',$1,'system',$1),
       (6301,63,'CNY',100,'MONTH',$1,'system',$1),
       (6302,63,'CNY',120,'MONTH',$1::timestamptz + interval '2 months','system',$1)`,
	} {
		if _, err := pool.Exec(ctx, statement, periodStart); err != nil {
			t.Fatal(err)
		}
	}
	ids := idgen.New(pool)
	store := NewManagementStore(pool, ids, slog.New(slog.NewTextHandler(io.Discard, nil)))
	service := mgmt.New(store, ids, nil, nil, mgmt.WithPriceStore(store))
	actor := admin.Identity{ID: 71, DisplayName: "Race Price Admin"}
	backfilled := periodStart.Add(14 * 24 * time.Hour)
	tests := []struct {
		name         string
		credentialID int64
		sourceID     int64
		write        func() error
	}{
		{
			name: "save price inside charged period", credentialID: 62, sourceID: 6201,
			write: func() error {
				_, err := service.SaveSubscriptionPrice(ctx, actor, 62, mgmt.SubscriptionPriceInput{
					Currency: "CNY", PeriodAmount: "90", BillingPeriod: "MONTH", EffectiveAt: backfilled,
				}, appsec.RequestMeta{})
				return err
			},
		},
		{
			name: "move later price into charged period", credentialID: 63, sourceID: 6301,
			write: func() error {
				_, err := service.UpdateSubscriptionPrice(ctx, actor, 63, 6302, mgmt.SubscriptionPriceInput{
					Currency: "CNY", PeriodAmount: "120", BillingPeriod: "MONTH", EffectiveAt: backfilled,
				}, appsec.RequestMeta{})
				return err
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			charge, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer charge.Rollback(context.Background())
			if _, err := charge.Exec(ctx, `INSERT INTO billing_document (
                    id,status,billing_type,document_type,provider_credential_id,source_type,source_id,
                    period_start,period_end,total_tokens,total_amount,currency,created_at
                ) VALUES ($1,'CONFIRMED','SUBSCRIPTION','CHARGE',$2,'SUBSCRIPTION_PRICE',$3,$4,$5,0,100,'CNY',$5)`,
				int64(9201+index), test.credentialID, test.sourceID, periodStart, periodEnd); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- test.write() }()
			var writeErr error
			finished := false
			select {
			case writeErr = <-result:
				finished = true
			case <-time.After(500 * time.Millisecond):
			}
			if err := charge.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if !finished {
				writeErr = <-result
			}
			if !errors.Is(writeErr, mgmt.ErrConflict) {
				t.Fatalf("price write during in-flight charge: finished before commit=%v err=%v; want conflict", finished, writeErr)
			}
			var inside int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM provider_credential_subscription_price
                WHERE provider_credential_id=$1 AND effective_at > $2 AND effective_at < $3`,
				test.credentialID, periodStart, periodEnd).Scan(&inside); err != nil || inside != 0 {
				t.Fatalf("prices inside charged period=%d err=%v", inside, err)
			}
		})
	}
}
