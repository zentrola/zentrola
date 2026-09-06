package postgres

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
)

type unavailableResetIDs struct{}

func (unavailableResetIDs) NextID() (int64, error) { return 0, errors.New("audit ID unavailable") }

type resetBeforeIssue struct {
	appsec.Tokens
	reset func()
}

func (s resetBeforeIssue) Issue(identity admin.Identity) (string, time.Time, error) {
	s.reset()
	return s.Tokens.Issue(identity)
}

func TestAdminPasswordResetIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids, err := idgen.New(15)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.New(NewBootstrapStore(pool), ids, "sonnet", "opus").Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	passwords, err := cryptosec.NewPasswords(4)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := NewSecurityStore(pool, ids)
	policy := admin.LoginPolicy{MaxFailures: 2, LockDuration: time.Hour}
	service := appsec.NewAdmin(store, passwords, tokens, policy)
	reset := appsec.NewPasswordReset(store, passwords)
	const original = "original-test-password"
	if err := service.Bootstrap(ctx, "admin", original); err != nil {
		t.Fatal(err)
	}
	login, err := service.Login(ctx, "admin", original, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	// 另一位管理员的凭证不受重置影响。
	_, err = pool.Exec(ctx, `INSERT INTO admin_user (id,organization_id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
SELECT 12345,organization_id,'other',password_hash,'Other','ACTIVE','system','system',now(),now() FROM admin_user WHERE username='admin'`)
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Login(ctx, "other", original, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_, _ = service.Login(ctx, "admin", "wrong", appsec.RequestMeta{})
	}
	if _, err := service.Login(ctx, "admin", original, appsec.RequestMeta{}); err == nil {
		t.Fatal("account should be locked")
	}
	newPassword, err := reset.Reset(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(newPassword) < 26 || len(newPassword) > 72 {
		t.Fatal("invalid generated password length")
	}
	var hash string
	var failed, version int64
	var unlocked bool
	if err := pool.QueryRow(ctx, `SELECT password_hash,failed_login_count,locked_until IS NULL,credential_version FROM admin_user WHERE id=$1`, identity.ID).Scan(&hash, &failed, &unlocked, &version); err != nil {
		t.Fatal(err)
	}
	if !passwords.Verify(hash, newPassword) || passwords.Verify(hash, original) || failed != 0 || !unlocked || version != 1 {
		t.Fatal("password reset state mismatch")
	}
	if _, err := service.Authenticate(ctx, login.Token); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatal("old token survived reset")
	}
	if _, err := service.Authenticate(ctx, other.Token); err != nil {
		t.Fatal("other account token invalidated")
	}
	if _, err := service.Login(ctx, "admin", original, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatal("old password survived reset")
	}
	fresh, err := service.Login(ctx, "admin", newPassword, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, fresh.Token); err != nil {
		t.Fatal("new token rejected")
	}
	var audit string
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*),string_agg(row_to_json(l)::text,' ') FROM operation_log l WHERE operation_type='ADMIN_PASSWORD_RESET'`).Scan(&count, &audit); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !strings.Contains(audit, `"operator_type":"SYSTEM"`) || strings.Contains(audit, hash) || strings.Contains(audit, newPassword) || strings.Contains(audit, original) {
		t.Fatal("unsafe or missing reset audit")
	}

	t.Run("audit failure rolls back all changes", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE admin_user SET failed_login_count=2,locked_until=now()+interval '1 hour' WHERE id=$1`, identity.ID)
		if err != nil {
			t.Fatal(err)
		}
		broken := appsec.NewPasswordReset(NewSecurityStore(pool, unavailableResetIDs{}), passwords)
		if password, err := broken.Reset(ctx, "admin"); err == nil || password != "" {
			t.Fatal("failed transaction exposed password")
		}
		var unchanged bool
		if err := pool.QueryRow(ctx, `SELECT password_hash=$2 AND credential_version=1 AND failed_login_count=2 AND locked_until IS NOT NULL FROM admin_user WHERE id=$1`, identity.ID, hash).Scan(&unchanged); err != nil || !unchanged {
			t.Fatal("partial reset persisted", err)
		}
		if _, err := service.Authenticate(ctx, fresh.Token); err != nil {
			t.Fatal("rollback invalidated token")
		}
	})

	t.Run("reject missing inactive deleted and inactive organization", func(t *testing.T) {
		for _, username := range []string{"missing", "", " admin"} {
			if password, err := reset.Reset(ctx, username); err == nil || password != "" {
				t.Fatal("invalid account accepted")
			}
		}
		for _, sql := range []string{`UPDATE admin_user SET status='DISABLED' WHERE username='admin'`, `UPDATE admin_user SET status='ACTIVE',is_deleted=true WHERE username='admin'`, `UPDATE admin_user SET is_deleted=false WHERE username='admin'; UPDATE organization SET status='DISABLED'`} {
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
			if password, err := reset.Reset(ctx, "admin"); !errors.Is(err, appsec.ErrNotFound) || password != "" {
				t.Fatal("inactive account reset")
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE organization SET status='ACTIVE'`); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reset between login commit and token issue", func(t *testing.T) {
		current, err := reset.Reset(ctx, "admin")
		if err != nil {
			t.Fatal(err)
		}
		var latest string
		racingTokens := resetBeforeIssue{Tokens: tokens, reset: func() {
			latest, err = reset.Reset(ctx, "admin")
			if err != nil {
				t.Fatal(err)
			}
		}}
		racingService := appsec.NewAdmin(store, passwords, racingTokens, policy)
		stale, err := racingService.Login(ctx, "admin", current, appsec.RequestMeta{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Authenticate(ctx, stale.Token); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("racing login obtained valid stale token")
		}
		if latest == current {
			t.Fatal("password reused")
		}
		latestLogin, err := service.Login(ctx, "admin", latest, appsec.RequestMeta{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Authenticate(ctx, latestLogin.Token); err != nil {
			t.Fatal(err)
		}
	})
}
