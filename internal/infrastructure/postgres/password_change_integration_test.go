package postgres

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
)

func TestAdminPasswordChangeIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids := idgen.New(pool)
	passwords, err := cryptosec.NewPasswords(4)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := NewSecurityStore(pool, ids)
	policy := admin.LoginPolicy{MaxFailures: 2, LockDuration: time.Hour}
	service := appsec.NewAdmin(store, passwords, tokens, policy)
	const original, next = "original-password", "123456"
	meta := appsec.RequestMeta{Path: "/api/v1/auth/password", Method: "POST", RequestID: "password-change-test"}
	if err := service.Bootstrap(ctx, "admin", original); err != nil {
		t.Fatal(err)
	}
	login, err := service.Login(ctx, "admin", original, meta)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := service.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"short", strings.Repeat("密", 25), "contains-null\x00-password", original} {
		if err := service.ChangePassword(ctx, actor, original, value, meta); !errors.Is(err, appsec.ErrInvalidArgument) {
			t.Fatalf("invalid password accepted: %v", err)
		}
	}
	if err := service.ChangePassword(ctx, actor, "wrong", next, meta); !errors.Is(err, appsec.ErrCurrentPassword) {
		t.Fatalf("wrong current password: %v", err)
	}
	var failures int
	if err := pool.QueryRow(ctx, `SELECT failed_login_count FROM admin_user WHERE id=$1`, actor.ID).Scan(&failures); err != nil || failures != 1 {
		t.Fatal("failure count not recorded", err)
	}
	if _, err := service.Authenticate(ctx, login.Token); err != nil {
		t.Fatal("wrong password invalidated session", err)
	}
	broken := appsec.NewAdmin(NewSecurityStore(pool, unavailableResetIDs{}), passwords, tokens, policy)
	// 直接验证存储事务：日志写入失败时密码与凭证版本均回滚。
	hash, _ := passwords.Hash(next)
	if err := broken.ChangePassword(ctx, actor, original, next, meta); err == nil {
		t.Fatal("audit failure accepted")
	}
	if err := NewSecurityStore(pool, unavailableResetIDs{}).ChangePassword(ctx, actor, hash, meta); err == nil {
		t.Fatal("password change without audit accepted")
	}
	if _, err := service.Authenticate(ctx, login.Token); err != nil {
		t.Fatal("rollback invalidated session", err)
	}
	if err := service.ChangePassword(ctx, actor, original, next, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatal("old token survived password change")
	}
	if err := store.ChangePassword(ctx, actor, hash, meta); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatal("stale concurrent change accepted")
	}
	if _, err := service.Login(ctx, "admin", original, meta); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatal("old password survived")
	}
	fresh, err := service.Login(ctx, "admin", next, meta)
	if err != nil {
		t.Fatal(err)
	}
	freshActor, err := service.Authenticate(ctx, fresh.Token)
	if err != nil {
		t.Fatal(err)
	}
	var audit, updatedBy string
	if err := pool.QueryRow(ctx, `SELECT row_to_json(l)::text FROM operation_log l WHERE operation_type='ADMIN_PASSWORD_RESET'`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(audit, `"operator_type":"ADMIN"`) || !strings.Contains(audit, meta.RequestID) || strings.Contains(audit, original) || strings.Contains(audit, next) || strings.Contains(audit, hash) {
		t.Fatal("unsafe or missing change audit")
	}
	if err := pool.QueryRow(ctx, `SELECT updated_by FROM admin_user WHERE id=$1`, actor.ID).Scan(&updatedBy); err != nil || updatedBy != actorRef(actor.ID) {
		t.Fatal("wrong update actor", err)
	}
	_ = service.ChangePassword(ctx, freshActor, "wrong", original, meta)
	var locked *appsec.AccountLockedError
	if err := service.ChangePassword(ctx, freshActor, "wrong", original, meta); !errors.As(err, &locked) {
		t.Fatalf("change attempts bypassed lock: %v", err)
	}
	if err := service.ChangePassword(ctx, freshActor, next, original, meta); !errors.As(err, &locked) {
		t.Fatal("locked account changed password")
	}
}
