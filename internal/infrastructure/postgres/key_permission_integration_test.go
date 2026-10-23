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
	for _, tc := range []struct {
		name   string
		keyID  int64
		column string
		event  string
		set    func(admin.Identity, int64, bool) (bool, error)
	}{
		{name: "upstream quota", keyID: 301, column: "can_view_upstream_quota", event: "ACCESS_KEY_UPSTREAM_QUOTA_UPDATE", set: func(a admin.Identity, id int64, allowed bool) (bool, error) {
			return store.SetUpstreamQuotaPermission(ctx, a, id, allowed, appsec.RequestMeta{})
		}},
		{name: "member management", keyID: 302, column: "can_manage_members", event: "ACCESS_KEY_MEMBER_MANAGEMENT_UPDATE", set: func(a admin.Identity, id int64, allowed bool) (bool, error) {
			return store.SetMemberManagementPermission(ctx, a, id, allowed, appsec.RequestMeta{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `INSERT INTO principal_access_key(id,principal_id,key_hash,masked_key,name,status,created_by,updated_by,created_at,updated_at)
				VALUES($1::bigint,300,decode(repeat(to_hex($1::bigint % 256),32),'hex'),'vk-perm********0001','Permission Key','ACTIVE','system','system',now(),now())`, tc.keyID); err != nil {
				t.Fatal(err)
			}
			// 管理员会话失效必须返回未认证，而不是服务不可用。
			if _, err := tc.set(admin.Identity{ID: actor.ID + 999}, tc.keyID, true); !errors.Is(err, appsec.ErrUnauthenticated) {
				t.Fatalf("invalid actor: %v", err)
			}
			if _, err := tc.set(actor, tc.keyID+1000, true); !errors.Is(err, appsec.ErrNotFound) {
				t.Fatalf("missing key: %v", err)
			}
			// 首次设置返回已变化，重复设置返回未变化，供缓存层决定是否失效。
			for _, wantChanged := range []bool{true, false} {
				if changed, err := tc.set(actor, tc.keyID, true); err != nil || changed != wantChanged {
					t.Fatalf("changed=%v want %v err=%v", changed, wantChanged, err)
				}
			}
			var allowed bool
			if err := pool.QueryRow(ctx, `SELECT `+tc.column+` FROM principal_access_key WHERE id=$1`, tc.keyID).Scan(&allowed); err != nil || !allowed {
				t.Fatalf("permission not saved: %v %v", allowed, err)
			}
			var audits int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log WHERE operation_type=$1 AND target_id=$2`, tc.event, tc.keyID).Scan(&audits); err != nil || audits != 1 {
				t.Fatalf("unchanged update must not be audited again: count=%d err=%v", audits, err)
			}
		})
	}
}
