// Package idgen 使用 PostgreSQL 全局 sequence 生成正数 int64 ID。
package idgen

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	nextIDSQL    = "SELECT nextval('zentrola_global_id_seq')"
	queryTimeout = 5 * time.Second
)

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type scopedQuerierKey struct{}

type Generator struct{ database Querier }

func New(pool *pgxpool.Pool) *Generator { return &Generator{database: pool} }

// WithQuerier 让事务内的 ID 查询复用当前连接，避免连接池耗尽时互相等待。
func WithQuerier(ctx context.Context, database Querier) context.Context {
	return context.WithValue(ctx, scopedQuerierKey{}, database)
}

func (g *Generator) NextID(parent context.Context) (int64, error) {
	database := g.database
	if scoped, ok := parent.Value(scopedQuerierKey{}).(Querier); ok {
		database = scoped
	}
	ctx, cancel := context.WithTimeout(parent, queryTimeout)
	defer cancel()
	var id int64
	if err := database.QueryRow(ctx, nextIDSQL).Scan(&id); err != nil || id <= 0 {
		return 0, errors.New("cannot generate ID from PostgreSQL sequence")
	}
	return id, nil
}
