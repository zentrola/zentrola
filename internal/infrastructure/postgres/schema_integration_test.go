package postgres

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

// 显式开启后仅在随机隔离 schema 中执行；不会清理或修改 public 中的业务表。
func integrationDatabase(t *testing.T) (context.Context, *pgxpool.Pool, string) {
	t.Helper()
	if os.Getenv("ZENTROLA_INTEGRATION") != "1" {
		t.Skip("set ZENTROLA_INTEGRATION=1 to run PostgreSQL integration tests")
	}
	cfg, err := config.Load("../../../.env")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	base, err := Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Close)
	schema := "zentrola_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error("cannot clean isolated schema:", err)
		}
	})
	poolConfig := base.Config().Copy()
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal("migration is not idempotent:", err)
	}
	return ctx, pool, schema
}

func TestStage1Integration(t *testing.T) {
	ctx, pool, schema := integrationDatabase(t)
	t.Run("schema conventions", func(t *testing.T) { checkSchema(t, ctx, pool, schema) })
	t.Run("partial indexes and cardinalities", func(t *testing.T) {
		withFixture(t, ctx, pool, func(tx pgx.Tx) {
			mustExec(t, ctx, tx, `INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
VALUES (70,'admin','test-hash','Admin','ACTIVE','system','system',now(),now())`)
			mustReject(t, ctx, tx, "23505", `INSERT INTO admin_user (id,username,password_hash,display_name,status,failed_login_count,locked_until,last_login_at,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 71,username,password_hash,display_name,status,failed_login_count,locked_until,last_login_at,is_deleted,created_by,updated_by,created_at,updated_at FROM admin_user WHERE id=70`)
			mustExec(t, ctx, tx, `UPDATE admin_user SET is_deleted=true WHERE id=70`)
			mustExec(t, ctx, tx, `INSERT INTO admin_user (id,username,password_hash,display_name,status,failed_login_count,locked_until,last_login_at,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 71,username,password_hash,display_name,status,failed_login_count,locked_until,last_login_at,false,created_by,updated_by,created_at,updated_at FROM admin_user WHERE id=70`)
			mustExec(t, ctx, tx, `INSERT INTO principal_group_membership (id,principal_id,group_id,created_by,updated_by,created_at,updated_at) VALUES (80,10,20,'system','system',now(),now()), (81,10,21,'system','system',now(),now())`)
			mustReject(t, ctx, tx, "23505", `INSERT INTO principal_group_membership (id,principal_id,group_id,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 82,principal_id,group_id,is_deleted,created_by,updated_by,created_at,updated_at FROM principal_group_membership WHERE id=80`)
			mustExec(t, ctx, tx, `UPDATE principal_group_membership SET is_deleted=true WHERE id=80`)
			mustExec(t, ctx, tx, `INSERT INTO principal_group_membership (id,principal_id,group_id,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 82,principal_id,group_id,false,created_by,updated_by,created_at,updated_at FROM principal_group_membership WHERE id=80`)
			mustExec(t, ctx, tx, `INSERT INTO provider (id,provider_code,provider_name,provider_type,status,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 41,'second-provider',provider_name,provider_type,status,false,created_by,updated_by,created_at,updated_at FROM provider WHERE id=40`)
			mustExec(t, ctx, tx, `INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at) VALUES(41,'ANTHROPIC','https://second.example.com','system','system',now(),now())`)
			mustExec(t, ctx, tx, `INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 52,41,model_id,upstream_model_code,100,false,created_by,updated_by,created_at,updated_at FROM provider_model WHERE id=50`)
			mustReject(t, ctx, tx, "23505", `INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 53,41,model_id,upstream_model_code,100,false,created_by,updated_by,created_at,updated_at FROM provider_model WHERE id=50`)
			mustExec(t, ctx, tx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at)
VALUES (60,40,'Resource',decode(repeat('11',32),'hex'),decode(repeat('22',12),'hex'),1,'ACTIVE','system','system',now(),now())`)
			mustReject(t, ctx, tx, "23505", `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 61,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,false,created_by,updated_by,created_at,updated_at FROM provider_credential WHERE id=60`)
			mustExec(t, ctx, tx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,is_deleted,created_by,updated_by,created_at,updated_at) SELECT 61,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,'DISABLED',false,created_by,updated_by,created_at,updated_at FROM provider_credential WHERE id=60`)
		})
	})
	t.Run("permission union and default deny", func(t *testing.T) {
		withFixture(t, ctx, pool, func(tx pgx.Tx) {
			q := dbgen.New(tx)
			args := dbgen.HasGroupModelPermissionParams{PrincipalID: 10, ModelID: 30}
			assertPermission := func(want bool) {
				t.Helper()
				got, err := q.HasGroupModelPermission(ctx, args)
				if err != nil || got != want {
					t.Fatalf("permission=%v want=%v err=%v", got, want, err)
				}
			}
			assertPermission(false)
			mustExec(t, ctx, tx, `INSERT INTO principal_group_membership (id,principal_id,group_id,created_by,updated_by,created_at,updated_at) VALUES (80,10,20,'system','system',now(),now()),(81,10,21,'system','system',now(),now())`)
			assertPermission(false)
			mustExec(t, ctx, tx, `INSERT INTO principal_group_model_permission (id,group_id,model_id,created_by,updated_by,created_at,updated_at) VALUES (90,21,30,'system','system',now(),now())`)
			assertPermission(true)
			mustExec(t, ctx, tx, `UPDATE principal_group SET status='DISABLED' WHERE id=21`)
			assertPermission(false)
			mustExec(t, ctx, tx, `UPDATE principal_group SET status='ACTIVE' WHERE id=21; UPDATE principal_group_membership SET is_deleted=true WHERE group_id=21`)
			assertPermission(false)
			mustExec(t, ctx, tx, `UPDATE principal_group_membership SET is_deleted=false WHERE group_id=21; UPDATE principal_group_model_permission SET is_deleted=true`)
			assertPermission(false)
		})
	})
	t.Run("credential binary round trip", func(t *testing.T) {
		withFixture(t, ctx, pool, func(tx pgx.Tx) {
			key, nonce := make([]byte, 32), make([]byte, 12)
			_, _ = rand.Read(key)
			_, _ = rand.Read(nonce)
			block, _ := aes.NewCipher(key)
			aead, _ := cipher.NewGCM(block)
			plain := []byte("test-only-provider-credential")
			sealed := aead.Seal(nil, nonce, plain, nil)
			mustExec(t, ctx, tx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at) VALUES (60,40,'Resource',$1,$2,7,'ACTIVE','system','system',now(),now())`, sealed, nonce)
			row, err := dbgen.New(tx).GetResource(ctx, 60)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := aead.Open(nil, row.CredentialNonce, row.CredentialCiphertext, nil)
			if err != nil || !bytes.Equal(opened, plain) || row.KeyVersion != 7 {
				t.Fatal("credential bytes changed")
			}
			mustReject(t, ctx, tx, "23514", `UPDATE provider_credential SET credential_nonce=decode('00','hex') WHERE id=60`)
		})
	})
	t.Run("model publisher round trip", func(t *testing.T) {
		withFixture(t, ctx, pool, func(tx pgx.Tx) {
			mustExec(t, ctx, tx, `UPDATE model SET publisher_provider_id=40 WHERE id=30`)
			row, err := dbgen.New(tx).ManageModel(ctx, 30)
			if err != nil || row.PublisherProviderID == nil || *row.PublisherProviderID != 40 || row.PublisherProviderName == nil || *row.PublisherProviderName != "Anthropic" {
				t.Fatalf("publisher id=%v name=%v err=%v", row.PublisherProviderID, row.PublisherProviderName, err)
			}
			mustReject(t, ctx, tx, "23514", `UPDATE model SET publisher_provider_id=0 WHERE id=30`)
		})
	})
	t.Run("credential runtime block", func(t *testing.T) {
		withFixture(t, ctx, pool, func(tx pgx.Tx) {
			mustExec(t, ctx, tx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at) VALUES (60,40,'Resource',decode(repeat('00',32),'hex'),decode(repeat('00',12),'hex'),1,'ACTIVE','system','system',now(),now())`)
			now := time.Now().UTC().Truncate(time.Microsecond)
			reason, code, status := "BILLING", "UPSTREAM_BILLING_BLOCKED", int32(402)
			changed, err := dbgen.New(tx).BlockGatewayResource(ctx, dbgen.BlockGatewayResourceParams{
				ResourceID: 60, BlockedReason: &reason,
				BlockedAt: pgtype.Timestamptz{Time: now, Valid: true}, HttpStatus: &status, ErrorCode: &code,
			})
			if err != nil || changed != 1 {
				t.Fatalf("block changed=%d err=%v", changed, err)
			}
			var runtime, gotReason, gotCode string
			var gotStatus int32
			if err := tx.QueryRow(ctx, `SELECT runtime_status,blocked_reason,last_http_status,last_error_code FROM provider_credential WHERE id=60`).Scan(&runtime, &gotReason, &gotStatus, &gotCode); err != nil || runtime != "BLOCKED" || gotReason != reason || gotStatus != status || gotCode != code {
				t.Fatalf("runtime=%s reason=%s status=%d code=%s err=%v", runtime, gotReason, gotStatus, gotCode, err)
			}
			mustReject(t, ctx, tx, "23514", `UPDATE provider_credential SET runtime_status='HEALTHY' WHERE id=60`)
		})
	})
	t.Run("usage facts and append only audit", func(t *testing.T) {
		withFixture(t, ctx, pool, func(tx pgx.Tx) {
			mustExec(t, ctx, tx, `INSERT INTO usage_record (id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,client_protocol,billing_unit,started_at,completed_at,latency_ms,status,created_at) VALUES (101,'req_test',1,10,40,50,60,30,'MODEL_GATEWAY','ANTHROPIC_MESSAGES','TOKEN',now(),now(),0,'SUCCESS',now())`)
			var unknown bool
			if err := tx.QueryRow(ctx, `SELECT input_tokens IS NULL AND cost_amount IS NULL AND cost_currency IS NULL FROM usage_record WHERE id=101`).Scan(&unknown); err != nil || !unknown {
				t.Fatal("unknown usage/cost was falsified")
			}
			mustReject(t, ctx, tx, "23505", `INSERT INTO usage_record (id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,input_tokens,output_tokens,cached_input_tokens,billing_unit,billing_quantity,cost_amount,cost_currency,started_at,completed_at,latency_ms,status,error_type,created_at,client_protocol) SELECT 102,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,input_tokens,output_tokens,cached_input_tokens,billing_unit,billing_quantity,cost_amount,cost_currency,started_at,completed_at,latency_ms,status,error_type,created_at,client_protocol FROM usage_record WHERE id=101`)
			mustExec(t, ctx, tx, `UPDATE usage_record SET attempt_no=2 WHERE id=101`)
			mustExec(t, ctx, tx, `INSERT INTO usage_record (id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,input_tokens,output_tokens,cached_input_tokens,billing_unit,billing_quantity,cost_amount,cost_currency,started_at,completed_at,latency_ms,status,error_type,created_at,client_protocol) SELECT 102,request_id,1,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,input_tokens,output_tokens,cached_input_tokens,billing_unit,billing_quantity,cost_amount,cost_currency,started_at,completed_at,latency_ms,status,error_type,created_at,client_protocol FROM usage_record WHERE id=101`)
			mustReject(t, ctx, tx, "23514", `UPDATE usage_record SET attempt_no=0 WHERE id=102`)
			mustExec(t, ctx, tx, `INSERT INTO operation_log (id,operator_type,operator_id,operator_name,module,operation_type,target_type,target_id,result,created_at) VALUES (200,'ADMIN',70,'Admin','MEMBER','MEMBER_CREATE','PRINCIPAL',10,'SUCCESS',now())`)
			for _, sql := range []string{`UPDATE operation_log SET operator_name='changed'`, `DELETE FROM operation_log`, `TRUNCATE operation_log`} {
				mustReject(t, ctx, tx, "42501", sql)
			}
		})
	})
}

func checkSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema string) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT c.relname,a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,coalesce(col_description(c.oid,a.attnum),''),coalesce(obj_description(c.oid),'')
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
WHERE n.nspname=$1 AND c.relkind='r' AND c.relname<>'goose_db_version' AND a.attnum>0 AND NOT a.attisdropped
ORDER BY c.relname,a.attnum`, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	nullable := map[string]string{
		"provider":   "official_website,proxy_url_display,proxy_url_ciphertext,proxy_url_nonce,proxy_url_key_version,proxy_headers_ciphertext,proxy_headers_nonce,proxy_headers_key_version",
		"model":      "publisher_provider_id",
		"admin_user": "locked_until,last_login_at", "principal": "remark", "principal_access_key": "expires_at,last_used_at,revoked_at",
		"principal_group": "remark", "provider_credential": "blocked_reason,blocked_at,last_error_at,last_http_status,last_error_code",
		"usage_record":  "input_tokens,output_tokens,cached_input_tokens,billing_quantity,cost_amount,cost_currency,error_type",
		"operation_log": "operator_id,target_id,target_name,request_id,request_method,request_path,ip_address,user_agent,error_code,before_data,after_data,remark",
	}
	tables := map[string]bool{}
	columns := map[string][]string{}
	for rows.Next() {
		var table, col, typ, comment, tableComment string
		var notNull bool
		if err := rows.Scan(&table, &col, &typ, &notNull, &comment, &tableComment); err != nil {
			t.Fatal(err)
		}
		tables[table] = true
		columns[table] = append(columns[table], col)
		containsHan := func(s string) bool {
			return strings.ContainsFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) })
		}
		if !containsHan(comment) || !containsHan(tableComment) {
			t.Errorf("missing Chinese comment: %s.%s", table, col)
		}
		allowNull := strings.Contains(","+nullable[table]+",", ","+col+",")
		if notNull == allowNull {
			t.Errorf("unexpected nullability: %s.%s", table, col)
		}
		if col == "id" && typ != "bigint" {
			t.Errorf("ID is not BIGINT: %s", table)
		}
		if (col == "cost_amount" || col == "billing_quantity") && typ != "numeric(20,8)" {
			t.Errorf("inexact numeric type: %s.%s", table, col)
		}
		if strings.Contains(typ, "timestamp") && typ != "timestamp with time zone" {
			t.Errorf("non-UTC timestamp type: %s.%s", table, col)
		}
		if table == "operation_log" && (col == "is_deleted" || col == "created_by" || col == "updated_by" || col == "updated_at") {
			t.Errorf("audit inherited mutable field: %s", col)
		}
		if table == "usage_record" && col == "is_deleted" {
			t.Errorf("fact table is soft deletable: %s", table)
		}
		if table == "provider_credential" && col == "provider_model_id" {
			t.Error("resource incorrectly belongs to provider model")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	expectedTables := map[string]bool{
		"admin_user": true, "principal": true,
		"principal_access_key": true, "principal_group": true,
		"principal_group_membership": true, "principal_group_model_permission": true,
		"provider": true, "provider_endpoint": true, "provider_model": true,
		"provider_credential": true, "model": true, "usage_record": true,
		"operation_log": true,
	}
	for table := range expectedTables {
		if !tables[table] {
			t.Errorf("missing business table: %s", table)
		}
	}
	for table := range tables {
		if !expectedTables[table] {
			t.Errorf("unexpected business table: %s", table)
		}
	}
	var organizationColumnCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND column_name='organization_id'`, schema).Scan(&organizationColumnCount); err != nil || organizationColumnCount != 0 {
		t.Fatal("organization_id column remains", err)
	}
	credentialColumns := make(map[string]bool, len(columns["provider_credential"]))
	for _, column := range columns["provider_credential"] {
		credentialColumns[column] = true
	}
	if credentialColumns["last_active_at"] {
		t.Error("provider_credential.last_active_at was not removed")
	}
	for _, column := range []string{
		"runtime_status", "blocked_reason", "blocked_at", "last_error_at", "last_http_status", "last_error_code",
		"credential_refreshed_at", "credential_expires_at",
	} {
		if !credentialColumns[column] {
			t.Errorf("provider_credential.%s is missing", column)
		}
	}
	preferredRecordColumns := []string{"id", "is_deleted", "status"}
	for table, actualColumns := range columns {
		present := make(map[string]bool, len(actualColumns))
		for _, column := range actualColumns {
			present[column] = true
		}
		var wantPrefix []string
		for _, column := range preferredRecordColumns {
			if present[column] {
				wantPrefix = append(wantPrefix, column)
			}
		}
		if len(wantPrefix) == 0 {
			continue
		}
		gotPrefix := actualColumns[:len(wantPrefix)]
		if strings.Join(gotPrefix, ",") != strings.Join(wantPrefix, ",") {
			t.Errorf("record columns are not leading fields: %s got=%v want=%v", table, gotPrefix, wantPrefix)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint c JOIN pg_namespace n ON n.oid=c.connamespace WHERE n.nspname=$1 AND c.contype='f'`, schema).Scan(&count); err != nil || count != 0 {
		t.Fatal("foreign key found", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name<>'goose_db_version' AND column_name='id' AND (column_default IS NOT NULL OR is_identity='YES')`, schema).Scan(&count); err != nil || count != 0 {
		t.Fatal("IDs must be generated by application", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname NOT IN ('goose_db_version','usage_record','operation_log') AND i.indisunique AND NOT i.indisprimary AND i.indpred IS NULL`, schema).Scan(&count); err != nil || count != 0 {
		t.Fatal("mutable business uniqueness must be partial", err)
	}
}

func withFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, run func(pgx.Tx)) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	mustExec(t, ctx, tx, `INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at) VALUES (10,'MEMBER','Member','ACTIVE','system','system',now(),now());
INSERT INTO principal_group (id,group_code,group_name,status,created_by,updated_by,created_at,updated_at) VALUES (20,'first','First','ACTIVE','system','system',now(),now()),(21,'second','Second','ACTIVE','system','system',now(),now());
INSERT INTO model (id,model_code,display_name,input_modalities,output_modalities,status,created_by,updated_by,created_at,updated_at) VALUES (30,'sonnet','Sonnet','["TEXT","IMAGE"]','["TEXT"]','ACTIVE','system','system',now(),now()),(31,'opus','Opus','["TEXT","IMAGE"]','["TEXT"]','ACTIVE','system','system',now(),now());
INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at) VALUES (40,'anthropic','Anthropic','OFFICIAL','ACTIVE','system','system',now(),now());
INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at) VALUES(40,'ANTHROPIC','https://api.anthropic.com','system','system',now(),now());
INSERT INTO provider_model (id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at) VALUES (50,40,30,'sonnet-upstream',100,'system','system',now(),now()),(51,40,31,'opus-upstream',100,'system','system',now(),now());`)
	run(tx)
}

func mustExec(t *testing.T, ctx context.Context, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func mustReject(t *testing.T, ctx context.Context, tx pgx.Tx, code, sql string, args ...any) {
	t.Helper()
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = savepoint.Exec(ctx, sql, args...)
	_ = savepoint.Rollback(ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("want PostgreSQL error %s, got %v", code, err)
	}
}
