package postgres

import (
	"testing"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func TestDeepSeekCatalogIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids, err := idgen.New(6)
	if err != nil {
		t.Fatal(err)
	}
	store := NewBootstrapStore(pool)
	setup := func() error { return bootstrap.SetupDeepSeek(ctx, store, ids) }
	if err := bootstrap.New(store, ids, "sonnet-original", "opus-original").Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := setup(); err != nil {
			t.Fatal(err)
		}
	}
	q := dbgen.New(pool)
	providers, err := q.ManageProviders(ctx, dbgen.ManageProvidersParams{ID: 0, Limit: 50})
	if err != nil || len(providers) != 2 {
		t.Fatal("DeepSeek provider not visible or duplicate seed")
	}
	var providerID int64
	for _, p := range providers {
		if p.ProviderCode == "deepseek-official" {
			providerID = p.ID
		}
	}
	for _, code := range []string{"deepseek-v4-flash", "deepseek-v4-pro", "claude-sonnet", "claude-opus"} {
		m, err := q.GatewayModel(ctx, code)
		if err != nil {
			t.Fatal(err)
		}
		mappings, err := q.GatewayMappings(ctx, dbgen.GatewayMappingsParams{ModelID: m.ID, ProtocolType: "ANTHROPIC"})
		if err != nil || len(mappings) != 1 {
			t.Fatal("model missing unique route")
		}
		want := code
		if code == "claude-sonnet" {
			want = "sonnet-original"
		}
		if code == "claude-opus" {
			want = "opus-original"
		}
		if mappings[0].UpstreamModelCode != want {
			t.Fatal("original mapping overwritten")
		}
	}
	// 幂等命令不能重新启用或恢复软删除的业务记录。
	if _, err := pool.Exec(ctx, `UPDATE ai_provider SET status='DISABLED',is_deleted=true WHERE id=$1`, providerID); err != nil {
		t.Fatal(err)
	}
	if err := setup(); err != nil {
		t.Fatal(err)
	}
	var disabled bool
	if err := pool.QueryRow(ctx, `SELECT status='DISABLED' AND is_deleted FROM ai_provider WHERE id=$1`, providerID).Scan(&disabled); err != nil || !disabled {
		t.Fatal("setup restored disabled provider")
	}
	// 不把后来手工改过的映射静默改回默认值。
	if _, err := pool.Exec(ctx, `UPDATE provider_model SET upstream_model_code='custom-model' WHERE provider_id=$1`, providerID); err != nil {
		t.Fatal(err)
	}
	if err := setup(); err == nil {
		t.Fatal("conflicting history accepted")
	}
}

func TestDeepSeekCatalogConflictRollsBack(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids, err := idgen.New(7)
	if err != nil {
		t.Fatal(err)
	}
	store := NewBootstrapStore(pool)
	if err := bootstrap.New(store, ids, "sonnet", "opus").Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ai_model SET model_code='deepseek-v4-pro' WHERE model_code='claude-opus'`); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.SetupDeepSeek(ctx, store, ids); err == nil {
		t.Fatal("conflicting model accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ai_provider WHERE provider_code='deepseek-official')+(SELECT count(*) FROM ai_model WHERE model_code='deepseek-v4-flash')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial catalog committed")
	}
}
