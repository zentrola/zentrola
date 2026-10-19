package redisstate

import (
	"context"
	"errors"
	"strconv"
	"time"

	redis "github.com/redis/go-redis/v9"
)

const sessionRouteTTL = 30 * 24 * time.Hour

var assignSessionRouteScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if current then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
  return tonumber(current)
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return tonumber(ARGV[1])
`)

var moveSessionRouteScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
  return 1
end
return 0
`)

func sessionRouteRedisKey(key string) string { return "zentrola:route:session:v1:" + key }

func (s *State) Get(ctx context.Context, key string) (int64, error) {
	if s == nil || s.client == nil || s.redisUnavailable() {
		return 0, errors.New("redis unavailable")
	}
	resourceID, err := s.client.GetEx(ctx, sessionRouteRedisKey(key), sessionRouteTTL).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return resourceID, s.recordFor(ctx, err)
}

// Assign atomically creates a session binding or returns the existing resource.
func (s *State) Assign(ctx context.Context, key string, preferredResourceID int64) (int64, error) {
	if s == nil || s.client == nil || s.redisUnavailable() {
		return 0, errors.New("redis unavailable")
	}
	assigned, err := assignSessionRouteScript.Run(ctx, s.client,
		[]string{sessionRouteRedisKey(key)}, strconv.FormatInt(preferredResourceID, 10),
		strconv.FormatInt(int64(sessionRouteTTL/time.Second), 10)).Int64()
	return assigned, s.recordFor(ctx, err)
}

// Move updates a binding only if it still points to the route that failed.
func (s *State) Move(ctx context.Context, key string, fromResourceID, toResourceID int64) (bool, error) {
	if s == nil || s.client == nil || s.redisUnavailable() {
		return false, errors.New("redis unavailable")
	}
	moved, err := moveSessionRouteScript.Run(ctx, s.client,
		[]string{sessionRouteRedisKey(key)}, strconv.FormatInt(fromResourceID, 10),
		strconv.FormatInt(toResourceID, 10), strconv.FormatInt(int64(sessionRouteTTL/time.Second), 10)).Int64()
	return moved == 1, s.recordFor(ctx, err)
}
