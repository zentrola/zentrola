package postgres

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"io/fs"
	"strings"
	"testing"
)

func TestModelModalitiesDatabaseConstraints(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	withFixture(t, ctx, pool, func(tx pgx.Tx) {
		for _, column := range []string{"input_modalities", "output_modalities"} {
			for _, invalid := range []string{`[]`, `null`, `{}`, `"TEXT"`, `["TEXT","TEXT"]`, `["UNKNOWN"]`, `[null]`, `["TEXT",null]`, `[1]`, `[["TEXT"]]`, `["TEXT",["IMAGE"]]`} {
				mustReject(t, ctx, tx, "23514", "UPDATE ai_model SET "+column+"=$1::jsonb WHERE id=30", invalid)
			}
			mustReject(t, ctx, tx, "23502", "UPDATE ai_model SET "+column+"=NULL WHERE id=30")
			mustExec(t, ctx, tx, "UPDATE ai_model SET "+column+"='[\"TEXT\",\"IMAGE\",\"AUDIO\",\"VIDEO\"]'::jsonb WHERE id=30")
		}
	})
}

func TestModelCatalogUpgradePreservesExistingModels(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	source, _ := fs.Sub(migrations, "migrations")
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_model(id,model_code,display_name,model_type,status,created_by,updated_by,created_at,updated_at)
VALUES(1,'claude-sonnet','自定义显示名','CHAT','DISABLED','system','system',now(),now()),
(2,'deepseek-v4-flash','DeepSeek V4 Flash','CHAT','ACTIVE','system','system',now(),now()),
(3,'unknown-legacy-model','未知旧模型','CHAT','DISABLED','system','system',now(),now());`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err == nil || !strings.Contains(err.Error(), "Unknown legacy models") {
		t.Fatal("unknown legacy capabilities were guessed", err)
	}
	var legacyColumn bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_model' AND column_name='model_type')`).Scan(&legacyColumn); err != nil || !legacyColumn {
		t.Fatal("failed migration was not atomic")
	}
	// 仅移除本测试刚插入的未知记录，再验证安装目录中的已知模型。
	if _, err := pool.Exec(ctx, "DELETE FROM ai_model WHERE id=3"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT model_code='claude-sonnet' AND display_name='自定义显示名' AND status='DISABLED' AND input_modalities='["TEXT","IMAGE"]'::jsonb AND output_modalities='["TEXT"]'::jsonb AND remark='' FROM ai_model WHERE id=1`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("legacy identity/status or backfill incorrect", err)
	}
	if err := pool.QueryRow(ctx, `SELECT input_modalities='["TEXT"]'::jsonb FROM ai_model WHERE id=2`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("DeepSeek modality backfill incorrect", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal("repeated migration failed", err)
	}
}
