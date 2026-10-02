package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func (s *SecurityStore) ChangePassword(ctx context.Context, actor admin.Identity, hash string, meta appsec.RequestMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return s.securityError(ctx, "begin_change_password", err)
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	row, err := q.GetAdminForLogin(ctx, actor.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrUnauthenticated
	}
	if err != nil {
		return s.securityError(ctx, "get_admin_for_password_change", err)
	}
	// 与登录及 CLI 重置共用行锁，拒绝验证旧密码之后发生的并发凭证变更。
	if row.ID != actor.ID || row.CredentialVersion != actor.CredentialVersion || row.Status != "ACTIVE" {
		return appsec.ErrUnauthenticated
	}
	count, err := q.ResetAdminPassword(ctx, dbgen.ResetAdminPasswordParams{
		ID: actor.ID, PasswordHash: hash,
		UpdatedAt: pgTime(time.Now().UTC()), UpdatedBy: actorRef(actor.ID),
	})
	if err != nil || count != 1 {
		if err == nil {
			err = errors.New("password update affected an unexpected number of rows")
		}
		return s.securityError(ctx, "change_password", err)
	}
	if err := s.appendLog(ctx, q, actor, "AUTH", operation.AdminPasswordReset, "ADMIN_USER", actor.ID, row.Username,
		"SUCCESS", "", meta, nil, nil, "Self-service password change"); err != nil {
		return s.securityError(ctx, "audit_change_password", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return s.securityError(ctx, "commit_change_password", err)
	}
	return nil
}
