package postgres

import (
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
)

func TestKeyPermissionIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids := idgen.New(pool)
	store := NewSecurityStore(pool, ids)
	passwords, err := cryptosec.NewPasswords(4)
	if err != nil {
		t.Fatal(err)
	}
	if err := appsec.NewAdmin(store, passwords, nil, admin.LoginPolicy{MaxFailures: 5, LockDuration: time.Minute}).Bootstrap(ctx, "permission-admin", "test-only-password-123"); err != nil {
		t.Fatal(err)
	}
	actor := admin.Identity{Username: "permission-admin", DisplayName: "permission-admin"}
	if err := pool.QueryRow(ctx, `SELECT id FROM admin_user WHERE username='permission-admin'`).Scan(&actor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO principal(id,principal_type,name,status,created_by,updated_by,created_at,updated_at)
		VALUES(300,'MEMBER','Permission Member','ACTIVE','system','system',now(),now())`); err != nil {
		t.Fatal(err)
	}
	set := func(a admin.Identity, id int64, allowed bool) (bool, error) {
		return store.SetManagementPermission(ctx, a, id, allowed, appsec.RequestMeta{})
	}
	const keyID = 301
	if _, err := pool.Exec(ctx, `INSERT INTO principal_access_key(id,principal_id,key_hash,masked_key,name,status,created_by,updated_by,created_at,updated_at)
		VALUES($1::bigint,300,decode(repeat(to_hex($1::bigint % 256),32),'hex'),'vk-perm********0001','Permission Key','ACTIVE','system','system',now(),now())`, keyID); err != nil {
		t.Fatal(err)
	}
	// 管理员会话失效必须返回未认证，而不是服务不可用。
	if _, err := set(admin.Identity{ID: actor.ID + 999}, keyID, true); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatalf("invalid actor: %v", err)
	}
	if _, err := set(actor, keyID+1000, true); !errors.Is(err, appsec.ErrNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	// 首次设置返回已变化，重复设置返回未变化，供缓存层决定是否失效。
	for _, wantChanged := range []bool{true, false} {
		if changed, err := set(actor, keyID, true); err != nil || changed != wantChanged {
			t.Fatalf("changed=%v want %v err=%v", changed, wantChanged, err)
		}
	}
	var allowed bool
	if err := pool.QueryRow(ctx, `SELECT can_manage FROM principal_access_key WHERE id=$1`, keyID).Scan(&allowed); err != nil || !allowed {
		t.Fatalf("permission not saved: %v %v", allowed, err)
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log WHERE operation_type='ACCESS_KEY_MEMBER_MANAGEMENT_UPDATE' AND target_id=$1`, keyID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("unchanged update must not be audited again: count=%d err=%v", audits, err)
	}
}
