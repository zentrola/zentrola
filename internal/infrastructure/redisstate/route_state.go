// Package redisstate 保存路由的短期冷却状态；Redis 不可用时由应用层按 fail-open 继续路由。
// 冷却 key 的 TTL 表示实际冷却时长，探测 key 在冷却结束后提供短暂的恢复租约。
package redisstate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/zentrola/zentrola/internal/application/gateway"
)

const (
	probeLease                   = 5 * time.Second
	probeMarkerRetention         = 24 * time.Hour
	retryRedis                   = 5 * time.Second
	subscriptionSchedulerLockTTL = 20 * time.Second
	subscriptionResourceLockTTL  = 60 * time.Second
)

var releaseLockScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

var acquireScript = redis.NewScript(`
-- KEYS[1] is the cooldown marker; KEYS[2] is the recovery probe marker/lease.
if redis.call('GET', KEYS[1]) then
	return 0
end
local probe = redis.call('GET', KEYS[2])
if not probe then
	return 1
end
local now = tonumber(ARGV[1])
if probe == 'ready' or tonumber(probe) + tonumber(ARGV[2]) <= now then
	redis.call('SET', KEYS[2], ARGV[1], 'PX', ARGV[2])
	return 1
end
return 0
`)

var cooldownScript = redis.NewScript(`
-- Keep the cooldown TTL equal to the requested delay. Retain the probe marker
-- after cooldown so the first later request can still acquire a short lease.
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('SET', KEYS[2], 'ready', 'PX', ARGV[3])
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

func (s *State) Acquire(ctx context.Context, route gateway.Route) (bool, error) {
	if s.redisUnavailable() {
		return true, nil
	}
	now := time.Now().UnixMilli()
	result, err := acquireScript.Run(ctx, s.client,
		[]string{cooldownKey(route.ResourceID), probeKey(route.ResourceID)},
		now, probeLease.Milliseconds()).Int64()
	return result == 1, s.record(err)
}

func (s *State) Cooldown(ctx context.Context, route gateway.Route, duration time.Duration) error {
	if s.redisUnavailable() {
		return nil
	}
	if duration <= 0 {
		duration = time.Minute
	}
	if duration < time.Millisecond {
		duration = time.Millisecond
	}
	// The probe marker needs to outlive cooldown for quiet routes. Keep the
	// addition within time.Duration's range for callers that provide a custom delay.
	if duration > time.Duration(1<<63-1)-probeMarkerRetention {
		duration = time.Duration(1<<63-1) - probeMarkerRetention
	}
	retryAt := time.Now().Add(duration).UnixMilli()
	_, err := cooldownScript.Run(ctx, s.client,
		[]string{cooldownKey(route.ResourceID), probeKey(route.ResourceID)},
		retryAt, duration.Milliseconds(), (duration + probeMarkerRetention).Milliseconds()).Int64()
	return s.record(err)
}

func (s *State) Healthy(ctx context.Context, route gateway.Route) error {
	if s.redisUnavailable() {
		return nil
	}
	return s.record(s.client.Del(ctx, cooldownKey(route.ResourceID), probeKey(route.ResourceID)).Err())
}

// AcquireSubscriptionRefreshScheduler takes a short-lived lease used only
// while one instance scans the subscription table. It deliberately fails
// closed when Redis is unavailable so multiple instances do not refresh the
// same batch concurrently.
func (s *State) AcquireSubscriptionRefreshScheduler(ctx context.Context) (func(), bool, error) {
	return s.acquireSubscriptionLock(ctx, "zentrola:subscription:refresh:scheduler:v1", subscriptionSchedulerLockTTL)
}

// AcquireSubscriptionRefreshResource serializes refreshes for one resource
// across all application instances.
func (s *State) AcquireSubscriptionRefreshResource(ctx context.Context, resourceID int64) (func(), bool, error) {
	key := "zentrola:subscription:refresh:resource:v1:" + strconv.FormatInt(resourceID, 10)
	return s.acquireSubscriptionLock(ctx, key, subscriptionResourceLockTTL)
}

func (s *State) acquireSubscriptionLock(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	if s == nil || s.client == nil || s.redisUnavailable() {
		return nil, false, errors.New("redis unavailable")
	}
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return nil, false, err
	}
	owner := hex.EncodeToString(ownerBytes)
	result, err := s.client.SetArgs(ctx, key, owner, redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		s.record(err)
		return nil, false, err
	}
	if result != "OK" {
		return nil, false, nil
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = releaseLockScript.Run(releaseCtx, s.client, []string{key}, owner).Err()
	}, true, nil
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

func cooldownKey(resourceID int64) string {
	return "zentrola:route:cooldown:v3:" + strconv.FormatInt(resourceID, 10)
}

func probeKey(resourceID int64) string {
	return "zentrola:route:probe:v1:" + strconv.FormatInt(resourceID, 10)
}
