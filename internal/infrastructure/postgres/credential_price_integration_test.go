package postgres

import (
	"bytes"
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
