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
		mappings, err := q.GatewayMappings(ctx, dbgen.GatewayMappingsParams{ModelID: m.ID, ProtocolType: "ANTHROPIC_MESSAGES"})
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
	providerID, err := ids.NextID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_provider(id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at) VALUES($1,'deepseek-official','冲突服务商','OFFICIAL','DISABLED','system','system',now(),now())`, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at) VALUES($1,'ANTHROPIC_MESSAGES','https://wrong.example.com','system','system',now(),now())`, providerID); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.SetupDeepSeek(ctx, store, ids); err == nil {
		t.Fatal("conflicting model accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM provider_model pm JOIN ai_model m ON m.id=pm.model_id WHERE m.model_code IN ('deepseek-v4-flash','deepseek-v4-pro')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial catalog committed")
	}
}
