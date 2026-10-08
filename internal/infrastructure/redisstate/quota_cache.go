package redisstate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	redis "github.com/redis/go-redis/v9"
	domain "github.com/zentrola/zentrola/internal/domain/quota"
)

const tokenQuotaCounterTTL = 5 * time.Minute

var initializeQuotaScript = redis.NewScript(`
local result = {}
local expires_at = tonumber(ARGV[1])
for index, key in ipairs(KEYS) do
  local desired = tonumber(ARGV[index + 1])
  local current = tonumber(redis.call('GET', key))
  if not current or current < desired then
    current = desired
    redis.call('SET', key, tostring(desired))
  end
  redis.call('EXPIREAT', key, expires_at)
  result[index] = current
end
return result
`)

var incrementQuotaScript = redis.NewScript(`
for _, key in ipairs(KEYS) do
  if redis.call('EXISTS', key) == 0 then
    return redis.error_reply('token quota counter is not initialized')
  end
end
for index, key in ipairs(KEYS) do
  redis.call('INCRBY', key, ARGV[index + 1])
end
return #KEYS
`)

func (s *State) Load(ctx context.Context, scopes []domain.Scope, periodStart time.Time) (map[domain.ScopeType]map[int64]*int64, error) {
	result := make(map[domain.ScopeType]map[int64]*int64)
	if len(scopes) == 0 {
		return result, nil
	}
	if s == nil || s.client == nil || s.redisUnavailable() {
		return nil, errors.New("redis unavailable")
	}
	keys := make([]string, len(scopes))
	for index, scope := range scopes {
		keys[index] = tokenQuotaKey(scope.Type, scope.ID, periodStart)
		if result[scope.Type] == nil {
			result[scope.Type] = make(map[int64]*int64)
		}
	}
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, s.record(err)
	}
	for index, value := range values {
		if value == nil {
			continue
		}
		parsed, parseErr := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if parseErr != nil || parsed < 0 {
			return nil, errors.New("invalid token quota cache value")
		}
		copy := parsed
		result[scopes[index].Type][scopes[index].ID] = &copy
	}
	s.record(nil)
	return result, nil
}

func (s *State) Initialize(ctx context.Context, values map[domain.ScopeType]map[int64]int64, start, end time.Time) (map[domain.ScopeType]map[int64]int64, error) {
	result := make(map[domain.ScopeType]map[int64]int64)
	keys, arguments, refs := quotaArguments(values, start, end)
	if len(keys) == 0 {
		return result, nil
	}
	if s == nil || s.client == nil || s.redisUnavailable() {
		return nil, errors.New("redis unavailable")
	}
	entries, err := initializeQuotaScript.Run(ctx, s.client, keys, arguments...).Int64Slice()
	if err != nil {
		return nil, s.record(err)
	}
	for index, scope := range refs {
		if result[scope.Type] == nil {
			result[scope.Type] = make(map[int64]int64)
		}
		result[scope.Type][scope.ID] = entries[index]
	}
	s.record(nil)
	return result, nil
}

func (s *State) Increment(ctx context.Context, values map[domain.ScopeType]map[int64]int64, start, end time.Time) error {
	keys, arguments, _ := quotaArguments(values, start, end)
	if len(keys) == 0 {
		return nil
	}
	if s == nil || s.client == nil || s.redisUnavailable() {
		return errors.New("redis unavailable")
	}
	return s.record(incrementQuotaScript.Run(ctx, s.client, keys, arguments...).Err())
}

func quotaArguments(values map[domain.ScopeType]map[int64]int64, start, end time.Time) ([]string, []any, []domain.Scope) {
	keys := make([]string, 0)
	expiresAt := time.Now().UTC().Add(tokenQuotaCounterTTL)
	retentionEnd := end.Add(7 * 24 * time.Hour).UTC()
	if retentionEnd.Before(expiresAt) {
		expiresAt = retentionEnd
	}
	arguments := []any{expiresAt.Unix()}
	refs := make([]domain.Scope, 0)
	for _, scopeType := range []domain.ScopeType{domain.Principal, domain.Group} {
		for id, value := range values[scopeType] {
			if id <= 0 || value < 0 {
				continue
			}
			keys = append(keys, tokenQuotaKey(scopeType, id, start))
			arguments = append(arguments, value)
			refs = append(refs, domain.Scope{Type: scopeType, ID: id})
		}
	}
	return keys, arguments, refs
}

func tokenQuotaKey(scopeType domain.ScopeType, id int64, start time.Time) string {
	return "zentrola:token-quota:v1:" + start.UTC().Format("200601") + ":" + string(scopeType) + ":" + strconv.FormatInt(id, 10)
}
