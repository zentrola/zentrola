// Package redisbreaker 提供 Redis 适配器共用的短时熔断：Redis 故障后在 RetryAfter 内跳过访问，
// 调用方自身上下文结束导致的错误不计入故障。
package redisbreaker

import (
	"context"
	"sync/atomic"
	"time"
)

// RetryAfter 是 Redis 故障后暂停访问的时长。
const RetryAfter = 5 * time.Second

// Breaker 记录 Redis 恢复访问的时间点；零值可直接使用，表示 Redis 可用。
// 时间由调用方传入，便于适配器使用可注入的时钟。
type Breaker struct {
	unavailableUntil atomic.Int64
}

// Open 报告 now 时熔断是否生效，即是否应跳过 Redis 访问。
func (b *Breaker) Open(now time.Time) bool {
	return now.UnixMilli() < b.unavailableUntil.Load()
}

// Record 根据一次 Redis 调用的结果更新熔断状态并原样返回 err：失败时打开熔断，成功时关闭。
func (b *Breaker) Record(now time.Time, err error) error {
	if err != nil {
		b.unavailableUntil.Store(now.Add(RetryAfter).UnixMilli())
		return err
	}
	b.unavailableUntil.Store(0)
	return nil
}

// RecordFor 与 Record 相同，但调用方上下文已结束时不改变熔断状态：客户端断开或请求超时
// 导致的错误不代表 Redis 故障，不能让单个请求取消使所有调用跳过 Redis。
func (b *Breaker) RecordFor(ctx context.Context, now time.Time, err error) error {
	if err != nil && CallerContextDone(ctx) {
		return err
	}
	return b.Record(now, err)
}

// CallerContextDone 只依据调用方上下文判断请求是否已结束。net 拨号超时等 Redis 侧错误
// 同样满足 errors.Is(err, context.DeadlineExceeded)，不能据错误本身判定，否则熔断永远不会打开。
func CallerContextDone(ctx context.Context) bool {
	return ctx.Err() != nil
}
