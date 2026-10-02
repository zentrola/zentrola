// Package postgres 实现 PostgreSQL 连接及 Migration，后续承载 sqlc Repository。
package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

func Open(ctx context.Context, cfg config.Postgres, loggers ...*slog.Logger) (*pgxpool.Pool, error) {
	logger := optionalLogger(loggers)
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   "/" + cfg.Database,
	}
	q := url.Values{"sslmode": {cfg.SSLMode}}
	u.RawQuery = q.Encode()
	poolConfig, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		// ParseConfig 错误可能回显含密码的连接串，因此只记录错误类型。
		logger.ErrorContext(ctx, "PostgreSQL configuration rejected", "cause_type", fmt.Sprintf("%T", err))
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "zentrola"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.ErrorContext(ctx, "PostgreSQL pool initialization failed", "cause_type", fmt.Sprintf("%T", err))
		return nil, errors.New("cannot initialize PostgreSQL pool")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		public := errors.New("cannot connect to PostgreSQL; check local connection settings and database availability")
		return nil, diagnosePostgresError(ctx, logger, "connection", "ping", err, public)
	}
	return pool, nil
}
