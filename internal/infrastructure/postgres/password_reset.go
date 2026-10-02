package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func (s *SecurityStore) ResetPassword(ctx context.Context, username, hash string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return s.securityError(ctx, "begin_reset_password", err)
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	// 与登录共用行锁；登录得到的版本始终对应其验证过的密码。
	row, err := q.GetAdminForLogin(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	}
	if err != nil {
		return s.securityError(ctx, "get_admin_for_password_reset", err)
	}
	if row.Status != "ACTIVE" {
		return appsec.ErrNotFound
	}
	count, err := q.ResetAdminPassword(ctx, dbgen.ResetAdminPasswordParams{
		UpdatedBy: "system", ID: row.ID, PasswordHash: hash, UpdatedAt: pgTime(time.Now().UTC()),
	})
	if err != nil || count != 1 {
		if err == nil {
			err = errors.New("password reset affected an unexpected number of rows")
		}
		return s.securityError(ctx, "reset_password", err)
	}
	before, _ := json.Marshal(map[string]any{"failedLoginCount": row.FailedLoginCount, "lockedUntil": timePointer(row.LockedUntil)})
	after := []byte(`{"failedLoginCount":0,"lockedUntil":null}`)
	actor := admin.Identity{DisplayName: "system"}
	if err := s.appendLog(ctx, q, actor, "AUTH", operation.AdminPasswordReset, "ADMIN_USER", row.ID, row.Username,
		"SUCCESS", "", appsec.RequestMeta{}, before, after, "CLI password reset"); err != nil {
		return s.securityError(ctx, "audit_reset_password", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return s.securityError(ctx, "commit_reset_password", err)
	}
	return nil
}
