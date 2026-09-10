// Package redisstate 保存路由的短期冷却状态；Redis 不可用时由应用层按 fail-open 继续路由。
package redisstate

import (
	"context"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/zentrola/zentrola/internal/application/gateway"
)

const (
	probeLease = 5 * time.Second
	stateTTL   = 24 * time.Hour
	retryRedis = 5 * time.Second
)

var acquireScript = redis.NewScript(`
local retry_at = redis.call('GET', KEYS[1])
if not retry_at then
  return 1
end
local now = tonumber(ARGV[1])
if tonumber(retry_at) > now then
  return 0
end
redis.call('SET', KEYS[1], now + tonumber(ARGV[2]), 'PX', ARGV[3])
return 1
`)

type State struct {
	client           *redis.Client
	unavailableUntil atomic.Int64
}

func New(host string, port, database int, password string) *State {
	return &State{client: redis.NewClient(&redis.Options{
		Addr:         net.JoinHostPort(host, strconv.Itoa(port)),
		Password:     password,
		DB:           database,
		DialTimeout:  500 * time.Millisecond,
		ReadTimeout:  500 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,
		MaxRetries:   -1,
	})}
}

func (s *State) Ping(ctx context.Context) error {
	return s.record(s.client.Ping(ctx).Err())
}

func (s *State) Close() error {
	return s.client.Close()
}

func (s *State) Acquire(ctx context.Context, organizationID int64, route gateway.Route) (bool, error) {
	if s.redisUnavailable() {
		return true, nil
	}
	now := time.Now().UnixMilli()
	result, err := acquireScript.Run(ctx, s.client, []string{routeKey(organizationID, route.ResourceID)},
		now, probeLease.Milliseconds(), stateTTL.Milliseconds()).Int64()
	return result == 1, s.record(err)
}

func (s *State) Cooldown(ctx context.Context, organizationID int64, route gateway.Route, duration time.Duration) error {
	if s.redisUnavailable() {
		return nil
	}
	if duration <= 0 {
		duration = time.Minute
	}
	retryAt := time.Now().Add(duration).UnixMilli()
	return s.record(s.client.Set(ctx, routeKey(organizationID, route.ResourceID), retryAt, stateTTL).Err())
}

func (s *State) Healthy(ctx context.Context, organizationID int64, route gateway.Route) error {
	if s.redisUnavailable() {
		return nil
	}
	return s.record(s.client.Del(ctx, routeKey(organizationID, route.ResourceID)).Err())
}

func (s *State) redisUnavailable() bool {
	return time.Now().UnixMilli() < s.unavailableUntil.Load()
}

func (s *State) record(err error) error {
	if err != nil {
		s.unavailableUntil.Store(time.Now().Add(retryRedis).UnixMilli())
		return err
	}
	s.unavailableUntil.Store(0)
	return nil
}

func routeKey(organizationID, resourceID int64) string {
	return "zentrola:route:cooldown:" + strconv.FormatInt(organizationID, 10) + ":" + strconv.FormatInt(resourceID, 10)
}
