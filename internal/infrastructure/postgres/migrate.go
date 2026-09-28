package postgres

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

//go:embed baseline/*.sql
var baselineMigrations embed.FS

// Migrate 使用独立 Provider 避免全局注册状态，迁移随二进制交付。
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	// 首发版本的新数据库直接使用当前结构基线，并把 Goose 版本记为 41；
	// 已经存在迁移记录的数据库继续使用历史迁移链，保证升级兼容。
	var hasMigrationTable bool
	if err := pool.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = current_schema() AND table_name = 'goose_db_version'
)`).Scan(&hasMigrationTable); err != nil {
		return errors.New("cannot inspect migration state")
	}
	if !hasMigrationTable {
		source, err := fs.Sub(baselineMigrations, "baseline")
		if err != nil {
			return err
		}
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return err
		}
		provider, err := goose.NewProvider(goose.DialectPostgres, db, source,
			goose.WithSessionLocker(locker), goose.WithDisableGlobalRegistry(true))
		if err != nil {
			return errors.New("cannot initialize migrations")
		}
		if _, err := provider.Up(ctx); err != nil {
			return errors.New("PostgreSQL migration failed")
		}
		return nil
	}
	source, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	providerOptions := []goose.ProviderOption{
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	}
	// Baseline 数据库只记录版本 41；达到该版本后，旧迁移已由 baseline 代表，
	// 后续执行必须排除 00001–00041，避免 Goose 将它们识别为缺失的乱序迁移。
	var currentVersion int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&currentVersion); err != nil {
		return errors.New("cannot inspect migration version")
	}
	if currentVersion >= 41 {
		excluded := make([]int64, 41)
		for index := range excluded {
			excluded[index] = int64(index + 1)
		}
		providerOptions = append(providerOptions, goose.WithExcludeVersions(excluded))
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source, providerOptions...)
	if err != nil {
		return errors.New("cannot initialize migrations")
	}
	if _, err := provider.Up(ctx); err != nil {
		return errors.New("PostgreSQL migration failed")
	}
	return nil
}
