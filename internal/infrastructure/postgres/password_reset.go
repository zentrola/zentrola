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
		return appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	// 与登录共用行锁；登录得到的版本始终对应其验证过的密码。
	row, err := q.GetAdminForLogin(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	}
	if err != nil {
		return appsec.ErrUnavailable
	}
	if row.Status != "ACTIVE" || !row.OrganizationActive {
		return appsec.ErrNotFound
	}
	count, err := q.ResetAdminPassword(ctx, dbgen.ResetAdminPasswordParams{
		UpdatedBy:      "system",
		OrganizationID: row.OrganizationID, ID: row.ID, PasswordHash: hash, UpdatedAt: pgTime(time.Now().UTC()),
	})
	if err != nil || count != 1 {
		return appsec.ErrUnavailable
	}
	before, _ := json.Marshal(map[string]any{"failedLoginCount": row.FailedLoginCount, "lockedUntil": timePointer(row.LockedUntil)})
	after := []byte(`{"failedLoginCount":0,"lockedUntil":null}`)
	actor := admin.Identity{OrganizationID: row.OrganizationID, DisplayName: "system"}
	if err := s.appendLog(ctx, q, actor, "AUTH", operation.AdminPasswordReset, "ADMIN_USER", row.ID, row.Username,
		"SUCCESS", "", appsec.RequestMeta{}, before, after, "CLI password reset"); err != nil {
		return appsec.ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return appsec.ErrUnavailable
	}
	return nil
}
