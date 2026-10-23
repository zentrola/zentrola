package postgres

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/principal"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
)

func TestMemberProvisionIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	_, err := pool.Exec(ctx, `INSERT INTO principal(id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
		VALUES(100,'MEMBER','Manager','ACTIVE','system','system',now(),now())`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO principal_group(id,group_code,group_name,status,created_by,updated_by,created_at,updated_at)
		VALUES(200,'test-group','Test Group','ACTIVE','system','system',now(),now())`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO principal_access_key(id,principal_id,key_hash,masked_key,name,status,can_manage_members,created_by,updated_by,created_at,updated_at)
		VALUES(101,100,decode(repeat('ab',32),'hex'),'vk-manager********0001','Manager Key','ACTIVE',true,'system','system',now(),now())`)
	if err != nil {
		t.Fatal(err)
	}
	ids := idgen.New(pool)
	store := NewMemberProvisionStore(pool, ids, nil)
	memberID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	keyID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	actor := appsec.PrincipalIdentity{ID: 100, AccessKeyID: 101, Type: principal.Member, CanManageMembers: true}
	key := appsec.KeyRecord{ID: keyID, PrincipalID: memberID, PrincipalType: principal.Member, Hash: make([]byte, 32), MaskedKey: "vk-child********0001", Name: "First", CreatedAt: time.Now().UTC()}
	if err := store.CreateMemberAndKey(ctx, actor, memberID, "Ordinary", "", []int64{200}, key, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	var kind, status string
	var inherited bool
	if err := pool.QueryRow(ctx, `SELECT principal_type,status FROM principal WHERE id=$1`, memberID).Scan(&kind, &status); err != nil {
		t.Fatal(err)
	}
	if kind != "MEMBER" || status != "ACTIVE" {
		t.Fatalf("created kind=%s status=%s", kind, status)
	}
	if err := pool.QueryRow(ctx, `SELECT can_manage_members FROM principal_access_key WHERE id=$1`, keyID).Scan(&inherited); err != nil {
		t.Fatal(err)
	}
	if inherited {
		t.Fatal("new Key inherited member management")
	}
	var assigned int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM principal_group_membership WHERE principal_id=$1 AND group_id=200 AND NOT is_deleted`, memberID).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned != 1 {
		t.Fatal("member was not assigned to the requested group")
	}
	var audited int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log WHERE operator_type='MEMBER_KEY' AND operator_id=101`).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 3 {
		t.Fatalf("audit count=%d, want 3", audited)
	}
	duplicateMemberID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicateKeyID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicateKey := appsec.KeyRecord{ID: duplicateKeyID, PrincipalID: duplicateMemberID, PrincipalType: principal.Member, Hash: []byte("abcdefghijklmnopqrstuvwxyz123456"), MaskedKey: "vk-child********0003", Name: "Duplicate", CreatedAt: time.Now().UTC()}
	if err := store.CreateMemberAndKey(ctx, actor, duplicateMemberID, "Ordinary", "", nil, duplicateKey, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrMemberNameExists) {
		t.Fatalf("duplicate member name: %v", err)
	}
	var duplicateRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM principal WHERE id=$1`, duplicateMemberID).Scan(&duplicateRows); err != nil || duplicateRows != 0 {
		t.Fatalf("duplicate member inserted: count=%d err=%v", duplicateRows, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM principal_access_key WHERE id=$1`, duplicateKeyID).Scan(&duplicateRows); err != nil || duplicateRows != 0 {
		t.Fatalf("duplicate member Key inserted: count=%d err=%v", duplicateRows, err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO principal(id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
		VALUES(102,'MEMBER','Disabled','DISABLED','system','system',now(),now())`)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateMemberAndKey(ctx, actor, duplicateMemberID, "Disabled", "", nil, duplicateKey, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrMemberNameExists) {
		t.Fatalf("disabled member name should remain reserved: %v", err)
	}
	otherKeyID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other := appsec.KeyRecord{ID: otherKeyID, PrincipalID: 102, PrincipalType: principal.Member, Hash: []byte("12345678901234567890123456789012"), MaskedKey: "vk-child********0002", Name: "Second", CreatedAt: time.Now().UTC()}
	if err := store.IssueMemberKey(ctx, actor, other, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM principal WHERE id=102`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ACTIVE" {
		t.Fatal("existing member was not activated")
	}
	// 管理员路径同样受唯一索引约束。
	_, err = pool.Exec(ctx, `INSERT INTO principal(id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
		VALUES(103,'MEMBER',' Ordinary ','DISABLED','system','system',now(),now())`)
	if !isMemberNameConflict(err) || !errors.Is(managementError(err), appsec.ErrMemberNameExists) {
		t.Fatalf("admin duplicate member name: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE principal_access_key SET can_manage_members=false WHERE id=101`); err != nil {
		t.Fatal(err)
	}
	key.ID, key.Name = keyID+1000, "Denied"
	if err := store.IssueMemberKey(ctx, actor, key, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrForbidden) {
		t.Fatalf("revoked permission: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM principal_access_key WHERE id=$1`, key.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("unauthorized Key was inserted")
	}
}

func TestMemberNameUniqueMigrationRejectsDuplicatesIntegration(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 62); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO principal(id,principal_type,name,status,is_deleted,created_by,updated_by,created_at,updated_at) VALUES
		(1,'MEMBER','Same','ACTIVE',false,'system','system',now(),now()),
		(2,'MEMBER','Same ','DISABLED',false,'system','system',now(),now()),
		(3,'MEMBER','Deleted','ACTIVE',true,'system','system',now(),now()),
		(4,'MEMBER','Deleted','ACTIVE',false,'system','system',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 63); err == nil || !strings.Contains(err.Error(), "Duplicate member names") {
		t.Fatalf("duplicate names must block migration: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE principal SET name='Same 2' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 63); err != nil {
		t.Fatal(err)
	}
}
