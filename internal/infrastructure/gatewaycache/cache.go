// Package gatewaycache 为 Gateway 身份、权限与路由查询提供 Redis Cache-Aside 缓存。
package gatewaycache

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	redis "github.com/redis/go-redis/v9"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

const (
	Prefix             = "zentrola"
	CacheSchemaVersion = "v1"
)

const (
	defaultIdentityTTL = 10 * time.Minute
	defaultRouteTTL    = time.Minute
	retryRedis         = 5 * time.Second
	retryInvalidation  = time.Second
)

var getScript = redis.NewScript(`
local generation = redis.call('GET', KEYS[1])
if not generation then
  redis.call('SET', KEYS[1], ARGV[2], 'NX')
  generation = redis.call('GET', KEYS[1])
elseif generation == '' then
  redis.call('SET', KEYS[1], ARGV[2])
  generation = ARGV[2]
end
return {generation, redis.call('GET', ARGV[1] .. ':' .. generation)}
`)

var setScript = redis.NewScript(`
local generation = redis.call('GET', KEYS[1])
if generation ~= ARGV[1] then
  return 0
end
redis.call('SET', ARGV[2] .. ':' .. generation, ARGV[3], 'PX', ARGV[4])
return 1
`)

var recordActiveRouteScript = redis.NewScript(`
local generation = redis.call('GET', KEYS[1])
if not generation then
  redis.call('SET', KEYS[1], ARGV[1], 'NX')
  generation = redis.call('GET', KEYS[1])
elseif generation == '' then
  redis.call('SET', KEYS[1], ARGV[1])
  generation = ARGV[1]
end
redis.call('SET', ARGV[2] .. ':' .. generation, ARGV[3], 'PX', ARGV[4])
return generation
`)

type Config struct {
	Host, Password                 string
	Port, Database                 int
	Namespace, GenerationNamespace string
	IdentityTTL, RouteTTL          time.Duration
	Logger                         *slog.Logger
}

// Cache 使用共享 generation 实现跨实例整体失效；旧 generation 的键由 TTL 自动回收。
type Cache struct {
	client                *redis.Client
	namespace             string
	generationNamespace   string
	logger                *slog.Logger
	identityTTL, routeTTL time.Duration
	unavailableUntil      atomic.Int64
	invalidationPending   atomic.Bool
	stop                  context.CancelFunc
	workers               sync.WaitGroup
	now                   func() time.Time
}

func BaseNamespace(environment string) string {
	environment = strings.Trim(strings.TrimSpace(environment), ":")
	if environment == "" {
		environment = "default"
	}
	return Prefix + ":" + environment + ":gateway"
}

func Namespace(environment string) string {
	return BaseNamespace(environment) + ":" + CacheSchemaVersion
}

func New(config Config) *Cache {
	if strings.TrimSpace(config.Namespace) == "" {
		config.Namespace = Namespace("")
	}
	if strings.TrimSpace(config.GenerationNamespace) == "" {
		config.GenerationNamespace = BaseNamespace("")
	}
	if config.IdentityTTL <= 0 {
		config.IdentityTTL = defaultIdentityTTL
	}
	if config.RouteTTL <= 0 {
		config.RouteTTL = defaultRouteTTL
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	workerContext, stop := context.WithCancel(context.Background())
	cache := &Cache{
		client: redis.NewClient(&redis.Options{
			Addr:         net.JoinHostPort(config.Host, strconv.Itoa(config.Port)),
			Password:     config.Password,
			DB:           config.Database,
			DialTimeout:  500 * time.Millisecond,
			ReadTimeout:  500 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
			MaxRetries:   -1,
		}),
		namespace:           strings.TrimRight(config.Namespace, ":"),
		generationNamespace: strings.TrimRight(config.GenerationNamespace, ":"),
		logger:              config.Logger,
		identityTTL:         config.IdentityTTL,
		routeTTL:            config.RouteTTL,
		stop:                stop,
		now:                 time.Now,
	}
	cache.workers.Add(1)
	go cache.retryInvalidations(workerContext)
	return cache
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.record(c.client.Ping(ctx).Err())
}

func (c *Cache) Close() error {
	if c.stop != nil {
		c.stop()
		c.workers.Wait()
	}
	return c.client.Close()
}

func (c *Cache) IdentityKey(hash []byte) string {
	return c.namespace + ":identity:" + hex.EncodeToString(hash)
}

func (c *Cache) RouteKey(identity appsec.PrincipalIdentity, model, protocol string) string {
	encodedModel := base64.RawURLEncoding.EncodeToString([]byte(model))
	return c.namespace + ":routes:" + strconv.FormatInt(identity.ID, 10) + ":" +
		strconv.FormatInt(identity.AccessKeyID, 10) + ":" + protocol + ":" + encodedModel
}

func (c *Cache) ModelsKey(identity appsec.PrincipalIdentity) string {
	return c.namespace + ":models:" + strconv.FormatInt(identity.ID, 10) + ":" +
		strconv.FormatInt(identity.AccessKeyID, 10)
}

func (c *Cache) GetIdentity(ctx context.Context, key string) (appsec.PrincipalIdentity, string, bool) {
	var identity appsec.PrincipalIdentity
	generation, ok := c.get(ctx, key, &identity, "identity")
	if !ok {
		return appsec.PrincipalIdentity{}, generation, false
	}
	if identity.ExpiresAt != nil && !c.now().Before(*identity.ExpiresAt) {
		_ = c.client.Del(ctx, key+":"+generation).Err()
		return appsec.PrincipalIdentity{}, generation, false
	}
	return identity, generation, true
}

func (c *Cache) SetIdentity(ctx context.Context, key, generation string, value appsec.PrincipalIdentity) bool {
	ttl := c.identityTTL
	if value.ExpiresAt != nil {
		remaining := value.ExpiresAt.Sub(c.now())
		if remaining <= 0 {
			return false
		}
		if remaining < ttl {
			ttl = remaining
		}
	}
	return c.set(ctx, key, generation, value, ttl, "identity")
}

func (c *Cache) GetRoutes(ctx context.Context, key string) ([]gw.Route, string, bool) {
	var routes []gw.Route
	generation, ok := c.get(ctx, key, &routes, "routes")
	if !ok {
		return nil, generation, false
	}
	for _, route := range routes {
		if route.ExpiresAt != nil && !c.now().Before(*route.ExpiresAt) {
			_ = c.client.Del(ctx, key+":"+generation).Err()
			return nil, generation, false
		}
	}
	return routes, generation, ok
}

func (c *Cache) SetRoutes(ctx context.Context, key, generation string, value []gw.Route) bool {
	return c.set(ctx, key, generation, value, c.routesTTL(value), "routes")
}

func (c *Cache) GetModels(ctx context.Context, key string) ([]gw.Model, string, bool) {
	var models []gw.Model
	generation, ok := c.get(ctx, key, &models, "models")
	return models, generation, ok
}

func (c *Cache) SetModels(ctx context.Context, key, generation string, value []gw.Model) bool {
	return c.set(ctx, key, generation, value, c.routeTTL, "models")
}

// RecordActiveRoute 记录当前模型最近一次真正转发到的服务商。
func (c *Cache) RecordActiveRoute(ctx context.Context, route gw.Route) error {
	active := gw.ActiveModel{
		ModelName:    strings.TrimSpace(route.ModelName),
		ModelCode:    strings.TrimSpace(route.ModelCode),
		ProviderName: strings.TrimSpace(route.ProviderName),
	}
	if active.ModelName == "" || active.ModelCode == "" || active.ProviderName == "" {
		return gw.ErrInvalid
	}
	if !c.available(ctx) {
		return gw.ErrUnavailable
	}
	ttl := c.routesTTL([]gw.Route{route})
	if ttl <= 0 {
		return gw.ErrUnavailable
	}
	seed, err := newGeneration()
	if err != nil {
		return gw.ErrUnavailable
	}
	encoded, err := json.Marshal(active)
	if err != nil {
		return gw.ErrUnavailable
	}
	ttlMillis := ttl.Milliseconds()
	if ttlMillis <= 0 {
		return gw.ErrUnavailable
	}
	_, err = recordActiveRouteScript.Run(ctx, c.client, []string{c.generationKey()},
		seed, c.activeModelKey(active.ModelCode), encoded, ttlMillis).Result()
	_ = c.record(err)
	if err != nil {
		c.logger.WarnContext(ctx, "active gateway route cache write failed",
			"cache_type", "active_model", "error_code", "REDIS_UNAVAILABLE")
		return gw.ErrUnavailable
	}
	return nil
}

// ActiveModels 返回当前 generation 中仍处于 TTL 内的实际路由结果。
// 只有真正发起过上游请求的模型才会被记录，每个模型仅保留最近一次服务商。
func (c *Cache) ActiveModels(ctx context.Context) ([]gw.ActiveModel, error) {
	if !c.available(ctx) {
		return nil, gw.ErrUnavailable
	}
	for attempt := 0; attempt < 2; attempt++ {
		generation, err := c.client.Get(ctx, c.generationKey()).Result()
		if err == redis.Nil {
			return []gw.ActiveModel{}, nil
		}
		if err != nil {
			_ = c.record(err)
			return nil, gw.ErrUnavailable
		}
		models, err := c.activeModelsForGeneration(ctx, generation)
		if err != nil {
			_ = c.record(err)
			return nil, gw.ErrUnavailable
		}
		current, err := c.client.Get(ctx, c.generationKey()).Result()
		if err == nil && current == generation {
			_ = c.record(nil)
			return models, nil
		}
		if err != nil && err != redis.Nil {
			_ = c.record(err)
			return nil, gw.ErrUnavailable
		}
	}
	return []gw.ActiveModel{}, nil
}

func (c *Cache) activeModelsForGeneration(ctx context.Context, generation string) ([]gw.ActiveModel, error) {
	modelsByCode := make(map[string]gw.ActiveModel)
	pattern := c.namespace + ":active-models:*:" + generation
	var cursor uint64
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			values, err := c.client.MGet(ctx, keys...).Result()
			if err != nil {
				return nil, err
			}
			for _, value := range values {
				encoded, ok := value.(string)
				if !ok {
					continue
				}
				var model gw.ActiveModel
				if json.Unmarshal([]byte(encoded), &model) != nil {
					continue
				}
				model.ModelName = strings.TrimSpace(model.ModelName)
				model.ModelCode = strings.TrimSpace(model.ModelCode)
				model.ProviderName = strings.TrimSpace(model.ProviderName)
				if model.ModelName == "" || model.ModelCode == "" || model.ProviderName == "" {
					continue
				}
				modelsByCode[model.ModelCode] = model
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	models := make([]gw.ActiveModel, 0, len(modelsByCode))
	for _, model := range modelsByCode {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ModelCode < models[j].ModelCode })
	return models, nil
}

func (c *Cache) activeModelKey(modelCode string) string {
	encodedModel := base64.RawURLEncoding.EncodeToString([]byte(modelCode))
	return c.namespace + ":active-models:" + encodedModel
}

// Clear 在数据库写事务提交后替换 generation，使所有实例的旧缓存立即不可见。
func (c *Cache) Clear(ctx context.Context, reason string) bool {
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	generation, err := c.replaceGeneration(cacheCtx)
	if err != nil {
		c.invalidationPending.Store(true)
		_ = c.record(err)
		c.logger.WarnContext(ctx, "gateway cache invalidation failed; retry scheduled",
			"cache_reason", reason, "error_code", "REDIS_UNAVAILABLE")
		return false
	}
	c.invalidationPending.Store(false)
	_ = c.record(nil)
	c.logger.InfoContext(ctx, "gateway cache invalidated",
		"cache_reason", reason, "cache_generation", generation)
	return true
}

func (c *Cache) get(ctx context.Context, key string, target any, cacheType string) (string, bool) {
	if !c.available(ctx) {
		c.logger.WarnContext(ctx, "gateway cache unavailable; loading from database",
			"cache_type", cacheType, "cache_result", "bypass", "error_code", "REDIS_UNAVAILABLE")
		return "", false
	}
	seed, err := newGeneration()
	if err != nil {
		c.logger.WarnContext(ctx, "gateway cache generation failed; loading from database",
			"cache_type", cacheType, "cache_result", "bypass", "error_code", "CACHE_GENERATION_FAILED")
		return "", false
	}
	result, err := getScript.Run(ctx, c.client, []string{c.generationKey()}, key, seed).Slice()
	if err != nil || len(result) != 2 {
		c.invalidationPending.Store(true)
		_ = c.record(err)
		c.logger.WarnContext(ctx, "gateway cache read failed; loading from database",
			"cache_type", cacheType, "cache_result", "error", "error_code", "REDIS_UNAVAILABLE")
		return "", false
	}
	_ = c.record(nil)
	generation, _ := result[0].(string)
	if result[1] == nil {
		return generation, false
	}
	encoded, ok := result[1].(string)
	if !ok || json.Unmarshal([]byte(encoded), target) != nil {
		_ = c.client.Del(ctx, key+":"+generation).Err()
		c.logger.WarnContext(ctx, "gateway cache entry invalid; entry removed",
			"cache_type", cacheType, "cache_result", "invalid", "error_code", "CACHE_ENTRY_INVALID")
		return generation, false
	}
	return generation, true
}

func (c *Cache) set(ctx context.Context, key, generation string, value any, ttl time.Duration, cacheType string) bool {
	if generation == "" || ttl <= 0 || !c.available(ctx) {
		return false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		c.logger.WarnContext(ctx, "gateway cache serialization failed",
			"cache_type", cacheType, "error_code", "CACHE_SERIALIZATION_FAILED")
		return false
	}
	ttlMillis := ttl.Milliseconds()
	if ttlMillis <= 0 {
		return false
	}
	result, err := setScript.Run(ctx, c.client, []string{c.generationKey()},
		generation, key, encoded, ttlMillis).Int()
	_ = c.record(err)
	if err != nil {
		c.invalidationPending.Store(true)
		c.logger.WarnContext(ctx, "gateway cache write failed; database result remains usable",
			"cache_type", cacheType, "error_code", "REDIS_UNAVAILABLE")
		return false
	}
	return result == 1
}

func (c *Cache) available(ctx context.Context) bool {
	if c.now().UnixMilli() < c.unavailableUntil.Load() {
		return false
	}
	if !c.invalidationPending.Load() {
		return true
	}
	// 上一次失效失败时，必须先补做 generation 替换，禁止重新读取旧缓存。
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if _, err := c.replaceGeneration(cacheCtx); err != nil {
		_ = c.record(err)
		return false
	}
	c.invalidationPending.Store(false)
	_ = c.record(nil)
	c.logger.InfoContext(ctx, "gateway cache pending invalidation completed",
		"cache_reason", "pending_retry")
	return true
}

func (c *Cache) generationKey() string {
	return c.generationNamespace + ":generation"
}

func (c *Cache) retryInvalidations(ctx context.Context) {
	defer c.workers.Done()
	ticker := time.NewTicker(retryInvalidation)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.invalidationPending.Load() || c.now().UnixMilli() < c.unavailableUntil.Load() {
				continue
			}
			cacheCtx, cancel := context.WithTimeout(ctx, time.Second)
			_, err := c.replaceGeneration(cacheCtx)
			cancel()
			if err != nil {
				_ = c.record(err)
				continue
			}
			c.invalidationPending.Store(false)
			_ = c.record(nil)
			c.logger.InfoContext(ctx, "gateway cache pending invalidation completed",
				"cache_reason", "background_retry")
		}
	}
}

func (c *Cache) routesTTL(routes []gw.Route) time.Duration {
	ttl := c.routeTTL
	now := c.now()
	for _, route := range routes {
		if route.ExpiresAt == nil {
			continue
		}
		remaining := route.ExpiresAt.Sub(now)
		if remaining <= 0 {
			return 0
		}
		if remaining < ttl {
			ttl = remaining
		}
	}
	return ttl
}

func (c *Cache) replaceGeneration(ctx context.Context) (string, error) {
	generation, err := newGeneration()
	if err != nil {
		return "", err
	}
	if err := c.client.Set(ctx, c.generationKey(), generation, 0).Err(); err != nil {
		return "", err
	}
	return generation, nil
}

func newGeneration() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func (c *Cache) record(err error) error {
	if err != nil {
		c.unavailableUntil.Store(c.now().Add(retryRedis).UnixMilli())
		return err
	}
	c.unavailableUntil.Store(0)
	return nil
}
