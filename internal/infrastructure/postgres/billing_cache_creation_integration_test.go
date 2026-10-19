package postgres

import (
	"testing"
	"time"

	app "github.com/zentrola/zentrola/internal/application/billing"
	"github.com/zentrola/zentrola/internal/domain/admin"
	usage "github.com/zentrola/zentrola/internal/domain/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestAPIKeyCacheCreationRatingAndRepricing(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, statement := range []string{
		`INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
         VALUES (9911,'cache-creation-admin','test-hash','Cache Creation Admin','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
         VALUES (1201,'MEMBER','Cache Member','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at)
         VALUES (2201,'cache-creation-model','Cache Creation Model','["TEXT"]','["TEXT"]','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
         VALUES (3201,'cache-creation-provider','Cache Creation Provider','OFFICIAL','ACTIVE','system','system',$1,$1)`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
         VALUES (4201,3201,2201,'cache-creation-upstream',100,'system','system',$1,$1)`,
		`INSERT INTO provider_credential (
             id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,
             auth_type,auth_adapter,created_by,updated_by,created_at,updated_at
         ) VALUES (
             5201,3201,'Cache Creation Key',decode(repeat('33',32),'hex'),decode(repeat('33',12),'hex'),1,
             'API_KEY','TEST','system','system',$1,$1
         )`,
		// 未配置缓存写入单价：缓存写入按普通输入单价计费。
		`INSERT INTO provider_credential_model_price (
             id,provider_credential_id,provider_model_id,currency,input_price,output_price,cached_input_price,
             effective_at,created_by,created_at
         ) VALUES (6201,5201,4201,'USD',3,15,0.3,$1,'system',$1)`,
	} {
		if _, err := pool.Exec(ctx, statement, periodStart); err != nil {
			t.Fatal(err)
		}
	}

	usageAt := periodStart.Add(time.Hour)
	tokens := func(value int64) *int64 { return &value }
	attempt := func(requestID string, input, cached, output int64, cacheCreation *int64) usage.Event {
		return usage.Event{
			RequestID: requestID, ClientProtocol: "ANTHROPIC_MESSAGES", PrincipalID: 1201, ModelID: 2201,
			RequestAt: usageAt, CompletedAt: usageAt, Status: usage.Success,
			Attempt: &usage.Attempt{
				ProviderID: 3201, ProviderModelID: 4201, ResourceID: 5201, ModelID: 2201,
				StartedAt: usageAt, CompletedAt: usageAt, Status: usage.Success,
				InputTokens: tokens(input), CachedInputTokens: tokens(cached), OutputTokens: tokens(output),
				CacheCreationInputTokens: cacheCreation,
			},
		}
	}
	usageStore := NewUsageStore(pool)
	// 总输入 110 = 普通输入 50 + 缓存命中 20 + 缓存写入 40。
	if err := usageStore.WriteBatch(ctx, []usage.Event{
		attempt("cache-creation-written", 110, 20, 10, tokens(40)),
		attempt("cache-creation-unknown", 5, 0, 0, nil),
	}); err != nil {
		t.Fatal(err)
	}
	if err := usageStore.WriteBatch(ctx, []usage.Event{attempt("cache-creation-overflow", 10, 6, 0, tokens(5))}); err == nil {
		t.Fatal("cache creation tokens above uncached input were accepted")
	}
	var writtenID, unknownID int64
	var writtenCacheCreation *int64
	if err := pool.QueryRow(ctx, `SELECT id, cache_creation_input_tokens FROM usage_record WHERE request_id='cache-creation-written'`).
		Scan(&writtenID, &writtenCacheCreation); err != nil || writtenCacheCreation == nil || *writtenCacheCreation != 40 {
		t.Fatalf("written cache creation=%v err=%v", writtenCacheCreation, err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM usage_record WHERE request_id='cache-creation-unknown' AND cache_creation_input_tokens IS NULL`).
		Scan(&unknownID); err != nil {
		t.Fatal("unknown cache creation was not stored as NULL:", err)
	}

	billingStore := NewBillingStore(pool)
	service := app.New(billingStore, idgen.New(pool))
	latest := func(usageRecordID int64) (revision int32, cacheCreationTokens int64, cacheCreationPrice *string, cacheCreationCost, totalCost string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT revision, cache_creation_input_tokens, cache_creation_input_price::text,
                cache_creation_input_cost::text, total_cost::text
            FROM usage_rating WHERE usage_record_id=$1 ORDER BY revision DESC LIMIT 1`, usageRecordID).
			Scan(&revision, &cacheCreationTokens, &cacheCreationPrice, &cacheCreationCost, &totalCost); err != nil {
			t.Fatal(err)
		}
		return
	}
	rate := func(want int) {
		t.Helper()
		summary, err := service.RatePendingAPIKeyUsage(ctx, 100)
		if err != nil || summary.Created != want || summary.Failed != 0 {
			t.Fatalf("rating summary=%+v err=%v want created=%d", summary, err, want)
		}
	}

	rate(2)
	revision, cacheCreationTokens, cacheCreationPrice, cacheCreationCost, totalCost := latest(writtenID)
	if revision != 1 || cacheCreationTokens != 40 || cacheCreationPrice != nil ||
		cacheCreationCost != "0.00012000000000" || totalCost != "0.00042600000000" {
		t.Fatalf("fallback rating revision=%d tokens=%d price=%v cost=%s total=%s",
			revision, cacheCreationTokens, cacheCreationPrice, cacheCreationCost, totalCost)
	}
	if _, cacheCreationTokens, _, _, totalCost = latest(unknownID); cacheCreationTokens != 0 || totalCost != "0.00001500000000" {
		t.Fatalf("unknown cache creation rating tokens=%d total=%s", cacheCreationTokens, totalCost)
	}
	rate(0)

	// 补录缓存写入单价后，两条记录的价格快照都与当前价格不一致，均追加 revision。
	if _, err := pool.Exec(ctx, `UPDATE provider_credential_model_price SET cache_creation_input_price=3.75 WHERE id=6201`); err != nil {
		t.Fatal(err)
	}
	rate(2)
	revision, _, cacheCreationPrice, cacheCreationCost, totalCost = latest(writtenID)
	if revision != 2 || cacheCreationPrice == nil || *cacheCreationPrice != "3.75000000" ||
		cacheCreationCost != "0.00015000000000" || totalCost != "0.00045600000000" {
		t.Fatalf("configured rating revision=%d price=%v cost=%s total=%s", revision, cacheCreationPrice, cacheCreationCost, totalCost)
	}
	rate(0)

	costQuery := app.NewUsageCostQuery(billingStore)
	costFilter := app.UsageCostFilter{From: periodStart, To: periodEnd, Limit: 50}
	summary, err := costQuery.Summary(ctx, admin.Identity{ID: 9911}, costFilter)
	if err != nil || summary.CacheCreationInputTokens != 40 || summary.Rated != 2 || summary.Unrated != 0 ||
		len(summary.Totals) != 1 || summary.Totals[0].Amount != "0.000471" {
		t.Fatalf("cache creation cost summary=%+v err=%v", summary, err)
	}
	detail, err := costQuery.Cost(ctx, admin.Identity{ID: 9911}, writtenID)
	if err != nil || detail.RatingStatus != "RATED" || detail.CacheCreationInputTokens == nil ||
		*detail.CacheCreationInputTokens != 40 || len(detail.Ratings) != 2 ||
		detail.Ratings[0].CacheCreationInputPrice == nil || *detail.Ratings[0].CacheCreationInputPrice != "3.75" ||
		detail.Ratings[0].CacheCreationInputCost != "0.00015" || detail.Ratings[1].CacheCreationInputPrice != nil ||
		detail.Ratings[1].CacheCreationInputCost != "0.00012" {
		t.Fatalf("cache creation cost detail=%+v err=%v", detail, err)
	}

	// 撤销缓存写入单价后回退为普通输入单价，并再次追加 revision。
	if _, err := pool.Exec(ctx, `UPDATE provider_credential_model_price SET cache_creation_input_price=NULL WHERE id=6201`); err != nil {
		t.Fatal(err)
	}
	rate(2)
	if revision, _, cacheCreationPrice, _, totalCost = latest(writtenID); revision != 3 || cacheCreationPrice != nil || totalCost != "0.00042600000000" {
		t.Fatalf("reverted rating revision=%d price=%v total=%s", revision, cacheCreationPrice, totalCost)
	}
	rate(0)

	// 触发器按缓存写入单价复核成本，拒绝与公式不一致的核算快照。
	if _, err := pool.Exec(ctx, `INSERT INTO usage_rating (
            id, usage_record_id, revision, supersedes_rating_id, principal_id, provider_credential_id, provider_model_id,
            model_price_id, usage_started_at, input_tokens, cached_input_tokens, output_tokens,
            input_price, cached_input_price, output_price, input_cost, cached_input_cost, output_cost, total_cost,
            currency, created_at, cache_creation_input_tokens, cache_creation_input_price, cache_creation_input_cost
        )
        SELECT 990001, usage_record_id, revision + 1, id, principal_id, provider_credential_id, provider_model_id,
            model_price_id, usage_started_at, input_tokens, cached_input_tokens, output_tokens,
            input_price, cached_input_price, output_price, input_cost, cached_input_cost, output_cost,
            input_cost + cached_input_cost + output_cost, currency, created_at, cache_creation_input_tokens, NULL, 0
        FROM usage_rating WHERE usage_record_id=$1 ORDER BY revision DESC LIMIT 1`, writtenID); err == nil {
		t.Fatal("usage rating with inconsistent cache creation cost was accepted")
	}
}
