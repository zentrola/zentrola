// Package postgres 实现 PostgreSQL 连接及 Migration，后续承载 sqlc Repository。
package postgres

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

func Open(ctx context.Context, cfg config.Postgres) (*pgxpool.Pool, error) {
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
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "zentrola"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, errors.New("cannot initialize PostgreSQL pool")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("cannot connect to PostgreSQL; check local connection settings and database availability")
	}
	return pool, nil
}
