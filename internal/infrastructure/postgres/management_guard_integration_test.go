package postgres

import (
	"errors"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestPriceGuardsRejectChangesAffectingBilledHistory(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
         VALUES (71,'guard-admin','test-hash','Guard Admin','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
         VALUES (1401,'MEMBER','Guard Member','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
         VALUES (2401,'guard-model','Guard Model','["TEXT"]','["TEXT"]','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3401,'guard-provider','Guard Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
         VALUES (4401,3401,2401,'guard-upstream',100,'system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,created_by,updated_by,created_at,updated_at
         ) VALUES (5401,3401,'Guard Key',decode(repeat('41',32),'hex'),decode(repeat('41',12),'hex'),1,'API_KEY','TEST','system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,subscription_type,effective_at,created_by,updated_by,created_at,updated_at
         ) VALUES (5402,3401,'Guard Subscription',decode(repeat('42',32),'hex'),decode(repeat('42',12),'hex'),1,
             'SUBSCRIPTION','OPENAI_CODEX','PERSONAL',$1,'system','system',$1,$1)`,
		`INSERT INTO provider_credential_model_price (
             id,provider_credential_id,provider_model_id,currency,input_price,output_price,cached_input_price,
             effective_at,created_by,created_at
         ) VALUES (6401,5401,4401,'CNY',5,10,1,$1,'system',$1)`,
		`INSERT INTO provider_credential_subscription_price (
             id,provider_credential_id,currency,period_amount,billing_period,effective_at,created_by,created_at
         ) VALUES (6402,5402,'CNY',100,'MONTH',$1,'system',$1)`,
		`INSERT INTO usage_record (
             id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,
             usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,
             started_at,completed_at,latency_ms,status,created_at
         ) VALUES (7401,'guard-usage',1,1401,3401,4401,5401,2401,'MODEL_GATEWAY','OPENAI_RESPONSES',10,5,0,
             $1::timestamptz + interval '14 days',$1::timestamptz + interval '14 days',0,'SUCCESS',$1::timestamptz + interval '14 days')`,
		`INSERT INTO usage_rating (
             id,usage_record_id,revision,supersedes_rating_id,principal_id,provider_credential_id,provider_model_id,
             model_price_id,usage_started_at,input_tokens,cached_input_tokens,output_tokens,input_price,cached_input_price,
             output_price,input_cost,cached_input_cost,output_cost,total_cost,currency,created_at
         ) VALUES (8401,7401,1,NULL,1401,5401,4401,6401,$1::timestamptz + interval '14 days',10,0,5,5,1,10,
             0.00005,0,0.00005,0.0001,'CNY',$1::timestamptz + interval '15 days')`,
		`INSERT INTO billing_document (
             id,status,billing_type,document_type,provider_credential_id,source_type,source_id,
             period_start,period_end,total_tokens,total_amount,currency,created_at
         ) VALUES (9401,'CONFIRMED','SUBSCRIPTION','CHARGE',5402,'SUBSCRIPTION_PRICE',6402,
             $1,$1::timestamptz + interval '1 month',0,100,'CNY',$1::timestamptz + interval '1 month')`,
	} {
		if _, err := pool.Exec(ctx, statement, periodStart); err != nil {
			t.Fatal(err)
		}
	}
	ids := idgen.New(pool)
	store := NewManagementStore(pool, ids)
	service := mgmt.New(store, ids, nil, nil, mgmt.WithPriceStore(store))
	actor := admin.Identity{ID: 71, DisplayName: "Guard Admin"}
	meta := appsec.RequestMeta{}

	if err := service.DeleteModelPrice(ctx, actor, 5401, 4401, 6401, meta); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("referenced model price delete error=%v; want conflict", err)
	}
	if _, err := service.UpdateModelPrice(ctx, actor, 5401, 6401, mgmt.ModelPriceInput{
		ProviderModelID: 4401, Currency: "USD", InputPrice: "5", OutputPrice: "10", CachedInputPrice: "1", EffectiveAt: periodStart,
	}, meta); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("referenced model price currency change error=%v; want conflict", err)
	}
	modelPrice := func(effectiveAt time.Time) mgmt.ModelPriceInput {
		return mgmt.ModelPriceInput{ProviderModelID: 4401, Currency: "CNY", InputPrice: "1", OutputPrice: "1",
			CachedInputPrice: "1", EffectiveAt: effectiveAt}
	}
	// 已核算调用发生在 9/15：9/10 生效的新版本会覆盖其价格，必须拒绝；之前或之后的版本不影响历史。
	if _, err := service.SaveModelPrice(ctx, actor, 5401, modelPrice(periodStart.AddDate(0, 0, 9)), meta); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("model price overriding rated usage error=%v; want conflict", err)
	}
	if _, err := service.SaveModelPrice(ctx, actor, 5401, modelPrice(periodStart.AddDate(0, 0, -1)), meta); err != nil {
		t.Fatalf("model price before current version: %v", err)
	}
	if _, err := service.SaveModelPrice(ctx, actor, 5401, modelPrice(periodStart.AddDate(0, 0, 19)), meta); err != nil {
		t.Fatalf("model price after rated usage: %v", err)
	}

	// 应用层预检查之外，数据库触发器的同类拒绝也映射为业务冲突而不是 503。
	err := store.WritePrice(ctx, actor, func(mgmt.PriceWriteSession) error {
		_, execErr := pool.Exec(ctx, `DELETE FROM provider_credential_model_price WHERE id=6401`)
		return execErr
	})
	if !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("trigger rejection error=%v; want conflict", err)
	}
}
