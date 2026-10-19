package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

// acquireSubscriptionRefreshLock 在独立连接上持有订阅凭据刷新的 session advisory lock（键为 -resourceID）。
// 仅在未注入 Redis 订阅刷新协调器时使用：此时网关刷新与管理端订阅探测共用这把锁，保证同一凭据
// 同一时刻只有一方向上游轮换 refresh token。注入协调器后两侧都改用协调器的锁。
// report 负责记录诊断并把底层错误映射为调用方的稳定错误。
func acquireSubscriptionRefreshLock(ctx context.Context, pool *pgxpool.Pool, resourceID int64,
	report func(context.Context, string, error) error,
) (*pgxpool.Conn, func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, nil, report(ctx, "acquire_subscription_refresh_connection", err)
	}
	if _, err := dbgen.New(conn).LockGatewaySubscriptionRefresh(ctx, resourceID); err != nil {
		conn.Release()
		return nil, nil, report(ctx, "lock_subscription_refresh", err)
	}
	var once sync.Once
	return conn, func() {
		once.Do(func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			unlocked, unlockErr := dbgen.New(conn).UnlockGatewaySubscriptionRefresh(unlockCtx, resourceID)
			if unlockErr == nil && unlocked {
				conn.Release()
				return
			}
			if unlockErr == nil {
				unlockErr = errors.New("subscription refresh lock was not held")
			}
			_ = report(unlockCtx, "unlock_subscription_refresh", unlockErr)
			// 不能把仍持有 session advisory lock 的连接放回连接池。
			hijacked := conn.Hijack()
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer closeCancel()
			if closeErr := hijacked.Close(closeCtx); closeErr != nil {
				_ = report(closeCtx, "close_hijacked_subscription_connection", closeErr)
			}
		})
	}, nil
}

// LockSubscriptionRefresh 让管理端在订阅探测或兑换重置卡期间持有 per-resource 刷新锁，仅在
// 管理端与网关都未注入订阅刷新协调器时与网关互斥；返回的 release 可重复调用。
func (s *ManagementStore) LockSubscriptionRefresh(ctx context.Context, resourceID int64) (func(), error) {
	if resourceID <= 0 {
		return nil, appsec.ErrInvalidArgument
	}
	_, release, err := acquireSubscriptionRefreshLock(ctx, s.pool, resourceID, s.managementError)
	if err != nil {
		return nil, err
	}
	return release, nil
}
