package postgres

import (
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func TestGatewayCandidatesOrderProvidersBeforeProtocol(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	withFixture(t, ctx, pool, func(tx pgx.Tx) {
		mustExec(t, ctx, tx, `INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
			VALUES (41,'backup','Backup','CUSTOM','ACTIVE','system','system',now(),now())`)
		mustExec(t, ctx, tx, `INSERT INTO provider_endpoint (provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at)
			VALUES (41,'OPENAI','https://example.com','system','system',now(),now())`)
		mustExec(t, ctx, tx, `INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
			VALUES (52,41,30,'sonnet-upstream',100,'system','system',now(),now())`)
		mustExec(t, ctx, tx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,created_by,updated_by,created_at,updated_at)
			VALUES (60,40,'Primary',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'system','system',now(),now()),
			       (61,41,'Backup',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'system','system',now(),now())`)
		query := dbgen.New(tx)
		mappings, err := query.ManageModelProviderMappings(ctx, 30)
		if err != nil || len(mappings) != 2 || mappings[0].ProviderID != 40 || mappings[1].ProviderID != 41 || !mappings[0].EndpointConfigured || !mappings[0].ResourceAvailable {
			t.Fatalf("model mappings unavailable or incorrectly sorted: rows=%+v err=%v", mappings, err)
		}
		checkFirst := func(want int64) {
			t.Helper()
			rows, err := query.GatewayCandidates(ctx, dbgen.GatewayCandidatesParams{ModelID: 30, PreferredProtocol: "OPENAI"})
			if err != nil || len(rows) != 2 || rows[0].ProviderID != want {
				t.Fatalf("first provider mismatch: want=%d, rows=%+v, err=%v", want, rows, err)
			}
		}
		checkFirst(40)
		updated, err := query.ManageUpdateModelMappingPriority(ctx, dbgen.ManageUpdateModelMappingPriorityParams{
			ID: 52, Priority: 10, UpdatedBy: "system", UpdatedAt: pgTime(time.Now().UTC()),
		})
		if err != nil || updated != 1 {
			t.Fatalf("priority update failed: rows=%d err=%v", updated, err)
		}
		mappings, err = query.ManageModelProviderMappings(ctx, 30)
		if err != nil || len(mappings) != 2 || mappings[0].ProviderID != 41 {
			t.Fatalf("updated model mappings not sorted by priority: rows=%+v err=%v", mappings, err)
		}
		checkFirst(41)
		mustExec(t, ctx, tx, `UPDATE provider SET status='DISABLED' WHERE id=41`)
		rows, err := query.GatewayCandidates(ctx, dbgen.GatewayCandidatesParams{ModelID: 30, PreferredProtocol: "OPENAI"})
		if err != nil || len(rows) != 1 || rows[0].ProviderID != 40 {
			t.Fatalf("disabled provider was not skipped: rows=%+v err=%v", rows, err)
		}
	})
}

func TestModelProviderPriorityMigrationClampsLegacyValues(t *testing.T) {
	ctx, pool, _ := isolatedDatabase(t)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	source, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 53); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at) VALUES (30,'legacy-model','Legacy','["TEXT"]','["TEXT"]','ACTIVE','system','system',now(),now())`,
		`INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at) VALUES (40,'legacy-provider','Legacy','CUSTOM','ACTIVE','system','system',now(),now())`,
		`INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at) VALUES (50,40,30,'legacy-model',250,'system','system',now(),now())`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migration failed with legacy priority: %v", err)
	}
	var priority int32
	if err := pool.QueryRow(ctx, `SELECT priority FROM provider_model WHERE id=50`).Scan(&priority); err != nil || priority != 100 {
		t.Fatalf("legacy priority was not clamped: value=%d err=%v", priority, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE provider_model SET priority=101 WHERE id=50`); err == nil {
		t.Fatal("priority above 100 was accepted after migration")
	}
}
