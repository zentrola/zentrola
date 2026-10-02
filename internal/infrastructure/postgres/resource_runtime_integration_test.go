package postgres

import (
	"bytes"
	"errors"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func TestResourceRuntimeWriteRejectsStaleVersion(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	if _, err := pool.Exec(ctx, `INSERT INTO provider
(id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES (40,'runtime-test','Runtime Test','OFFICIAL','ACTIVE','system','system',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential
(id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,created_by,updated_by,created_at,updated_at)
VALUES (60,40,'Resource',$1,$2,1,'system','system',now(),now())`, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 12)); err != nil {
		t.Fatal(err)
	}
	session := &managementSession{q: dbgen.New(pool), actor: admin.Identity{ID: 1}}
	now := time.Now().UTC()
	assertState := func(status string, version int64) {
		t.Helper()
		var actualStatus string
		var actualVersion int64
		if err := pool.QueryRow(ctx, `SELECT runtime_status,version FROM provider_credential WHERE id=60`).Scan(&actualStatus, &actualVersion); err != nil {
			t.Fatal(err)
		}
		if actualStatus != status || actualVersion != version {
			t.Fatalf("runtime state = %s/%d, want %s/%d", actualStatus, actualVersion, status, version)
		}
	}
	if err := session.BlockResourceRuntime(ctx, 60, 1, "BILLING", "UPSTREAM_BILLING_BLOCKED", nil, now); err != nil {
		t.Fatal(err)
	}
	assertState("BLOCKED", 2)
	if err := session.RestoreResourceRuntime(ctx, 60, 1, now); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("stale restore error = %v, want conflict", err)
	}
	assertState("BLOCKED", 2)
	if err := session.RestoreResourceRuntime(ctx, 60, 2, now); err != nil {
		t.Fatal(err)
	}
	assertState("HEALTHY", 3)
	if err := session.BlockResourceRuntime(ctx, 60, 2, "BILLING", "UPSTREAM_BILLING_BLOCKED", nil, now); !errors.Is(err, mgmt.ErrConflict) {
		t.Fatalf("stale block error = %v, want conflict", err)
	}
	assertState("HEALTHY", 3)
	// 成功探测也可能从未阻断的资源开始，仍须验证版本且不增加版本号。
	if err := session.RestoreResourceRuntime(ctx, 60, 3, now); err != nil {
		t.Fatal(err)
	}
	assertState("HEALTHY", 3)
}
