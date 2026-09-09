package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zentrola/zentrola/internal/application/bootstrap"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type BootstrapStore struct{ pool *pgxpool.Pool }

func NewBootstrapStore(pool *pgxpool.Pool) *BootstrapStore { return &BootstrapStore{pool: pool} }

func (s *BootstrapStore) Initialized(ctx context.Context) (bool, error) {
	return dbgen.New(s.pool).BootstrapInitialized(ctx)
}

func (s *BootstrapStore) InitializeOnce(ctx context.Context, seed bootstrap.Seed) error {
	if err := s.initialize(ctx, seed); err != nil {
		return errors.New("database bootstrap failed; check schema and initialization data")
	}
	return nil
}

func (s *BootstrapStore) initialize(ctx context.Context, seed bootstrap.Seed) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	// 事务锁避免两个启动进程同时判断为空并重复初始化；不跨网络调用。
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829314002)"); err != nil {
		return err
	}
	queries := dbgen.New(tx)
	hasHistory, err := queries.HasOrganizationHistory(ctx)
	if err != nil {
		return err
	}
	if hasHistory {
		// 包括软删除记录。已初始化过就不恢复或重建组织。
		return tx.Commit(ctx)
	}
	at := pgtype.Timestamptz{Time: seed.CreatedAt, Valid: true}
	if err := queries.CreateBootstrapOrganization(ctx, dbgen.CreateBootstrapOrganizationParams{
		ID: seed.OrganizationID, OrganizationCode: "default", OrganizationName: "zentrola", CreatedAt: at,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
