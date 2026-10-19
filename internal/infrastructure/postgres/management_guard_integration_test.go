package postgres

import (
	"errors"
	"slices"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestPriceGuardsRejectChangesAffectingBilledHistory(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
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

	if _, err := service.SaveSubscriptionPrice(ctx, actor, 5402, mgmt.SubscriptionPriceInput{
		Currency: "CNY", PeriodAmount: "90", BillingPeriod: "MONTH", EffectiveAt: periodStart.AddDate(0, -1, 0),
	}, meta); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("backfilled subscription price error=%v; want conflict", err)
	}
	if _, err := service.SaveSubscriptionPrice(ctx, actor, 5402, mgmt.SubscriptionPriceInput{
		Currency: "CNY", PeriodAmount: "90", BillingPeriod: "MONTH", EffectiveAt: periodEnd,
	}, meta); err != nil {
		t.Fatalf("subscription price at charged boundary: %v", err)
	}
	if _, err := service.UpdateSubscriptionPrice(ctx, actor, 5402, 6402, mgmt.SubscriptionPriceInput{
		Currency: "CNY", PeriodAmount: "80", BillingPeriod: "MONTH", EffectiveAt: periodStart,
	}, meta); err != nil {
		t.Fatalf("charged subscription amount correction: %v", err)
	}

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

func TestOperationLogFiltersAndCountsMatch(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	createdAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
        VALUES (72,'log-admin','test-hash','Log Admin','ACTIVE','system','system',$1,$1)`, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO operation_log (
        id,operator_type,operator_id,operator_name,module,operation_type,target_type,target_id,target_name,
        request_id,result,error_code,created_at
    ) VALUES
        (91001,'ADMIN',72,'Log Admin','RESOURCE','RESOURCE_CREATE','RESOURCE',501,'Key 100%_off','req-alpha','SUCCESS',NULL,$1),
        (91002,'ADMIN',72,'Log Admin','RESOURCE','RESOURCE_CREATE','RESOURCE',502,'Key 100 off','req-beta','FAILED','CONFLICT',$1),
        (91003,'SYSTEM',NULL,'system','RESOURCE','RESOURCE_STATUS_CHANGE','RESOURCE',503,'Other\path','REQ-GAMMA','SUCCESS',NULL,$1)`,
		createdAt); err != nil {
		t.Fatal(err)
	}
	store := NewManagementStore(pool, idgen.New(pool))
	service := mgmt.New(store, idgen.New(pool), nil, nil)
	actor := admin.Identity{ID: 72, DisplayName: "Log Admin"}
	for _, tt := range []struct {
		name                  string
		keyword, kind, result string
		page                  mgmt.Page
		want                  []int64
		wantTotal             int64
	}{
		{name: "all", page: mgmt.Page{Limit: 10}, want: []int64{91003, 91002, 91001}, wantTotal: 3},
		{name: "percent is literal", keyword: "100%", page: mgmt.Page{Limit: 10}, want: []int64{91001}, wantTotal: 1},
		{name: "underscore is literal", keyword: "%_", page: mgmt.Page{Limit: 10}, want: []int64{91001}, wantTotal: 1},
		{name: "backslash is literal", keyword: `r\p`, page: mgmt.Page{Limit: 10}, want: []int64{91003}, wantTotal: 1},
		{name: "case insensitive request id", keyword: "req-gamma", page: mgmt.Page{Limit: 10}, want: []int64{91003}, wantTotal: 1},
		{name: "target id text", keyword: "502", page: mgmt.Page{Limit: 10}, want: []int64{91002}, wantTotal: 1},
		{name: "result keyword", keyword: "failed", page: mgmt.Page{Limit: 10}, want: []int64{91002}, wantTotal: 1},
		{name: "type and result", kind: "RESOURCE_CREATE", result: "SUCCESS", page: mgmt.Page{Limit: 10}, want: []int64{91001}, wantTotal: 1},
		{name: "cursor keeps filter", kind: "RESOURCE_CREATE", page: mgmt.Page{After: 91002, Limit: 10}, want: []int64{91001}, wantTotal: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := mgmt.NewOperationFilter(tt.keyword, tt.kind, tt.result)
			if err != nil {
				t.Fatal(err)
			}
			page, err := service.Operations(ctx, actor, tt.page, filter)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int64, 0, len(page.Items))
			for _, item := range page.Items {
				got = append(got, item.ID)
			}
			if !slices.Equal(got, tt.want) || page.Total != tt.wantTotal {
				t.Fatalf("items=%v total=%d; want %v total=%d", got, page.Total, tt.want, tt.wantTotal)
			}
		})
	}
}

func TestManagementSubscriptionRefreshLockAndConditionalCredentialReplace(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	const resourceID = 987654321
	for _, statement := range []string{
		`INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
         VALUES (73,'lock-admin','test-hash','Lock Admin','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3501,'lock-provider','Lock Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,subscription_type,created_by,updated_by,created_at,updated_at
         ) VALUES (987654321,3501,'Lock Subscription',decode(repeat('51',32),'hex'),decode(repeat('52',12),'hex'),1,
             'SUBSCRIPTION','OPENAI_CODEX','PERSONAL','system','system',$1,$1)`,
	} {
		if _, err := pool.Exec(ctx, statement, now); err != nil {
			t.Fatal(err)
		}
	}
	store := NewManagementStore(pool, idgen.New(pool))
	release, err := store.LockSubscriptionRefresh(ctx, resourceID)
	if err != nil {
		t.Fatal(err)
	}
	var acquired bool
	if err := pool.QueryRow(ctx, `SELECT pg_try_advisory_lock(-$1::bigint)`, resourceID).Scan(&acquired); err != nil || acquired {
		t.Fatalf("gateway refresh lock was not held by management: acquired=%v err=%v", acquired, err)
	}
	release()
	release()
	gatewayCtx, gatewayRelease, err := NewGatewayStore(pool).LockSubscriptionRefresh(ctx, resourceID)
	if err != nil || gatewayCtx == nil {
		t.Fatalf("gateway lock after management release: %v", err)
	}
	gatewayRelease()

	var original catalog.SealedCredential
	if err := pool.QueryRow(ctx, `SELECT credential_ciphertext,credential_nonce,key_version FROM provider_credential WHERE id=$1`, resourceID).
		Scan(&original.Ciphertext, &original.Nonce, &original.KeyVersion); err != nil {
		t.Fatal(err)
	}
	replacement := catalog.SealedCredential{Ciphertext: []byte("rotated-credential"), Nonce: make([]byte, 12), KeyVersion: 1}
	stale := catalog.SealedCredential{Ciphertext: []byte("stale"), Nonce: original.Nonce, KeyVersion: 1}
	actor := admin.Identity{ID: 73, DisplayName: "Lock Admin"}
	for _, tt := range []struct {
		name     string
		expected catalog.SealedCredential
		want     bool
	}{
		{name: "stale expectation", expected: stale, want: false},
		{name: "current expectation", expected: original, want: true},
	} {
		var replaced bool
		err := store.Write(ctx, actor, func(writer mgmt.Writer) error {
			replacer, ok := writer.(mgmt.SubscriptionCredentialReplacer)
			if !ok {
				t.Fatal("management session does not support conditional credential replacement")
			}
			var replaceErr error
			replaced, replaceErr = replacer.ReplaceSubscriptionCredential(ctx, resourceID, tt.expected, replacement, nil, nil, now)
			return replaceErr
		})
		if err != nil || replaced != tt.want {
			t.Fatalf("%s: replaced=%v err=%v", tt.name, replaced, err)
		}
	}
	var stored []byte
	var version int64
	if err := pool.QueryRow(ctx, `SELECT credential_ciphertext,version FROM provider_credential WHERE id=$1`, resourceID).
		Scan(&stored, &version); err != nil || string(stored) != "rotated-credential" || version != 2 {
		t.Fatalf("stored=%q version=%d err=%v", stored, version, err)
	}
}
