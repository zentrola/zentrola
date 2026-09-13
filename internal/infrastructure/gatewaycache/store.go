package gatewaycache

import (
	"context"
	"log/slog"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"golang.org/x/sync/singleflight"
)

type StoreCache interface {
	IdentityKey([]byte) string
	RouteKey(appsec.PrincipalIdentity, string, string) string
	ModelsKey(appsec.PrincipalIdentity) string
	GetIdentity(context.Context, string) (appsec.PrincipalIdentity, string, bool)
	SetIdentity(context.Context, string, string, appsec.PrincipalIdentity) bool
	GetRoutes(context.Context, string) ([]gw.Route, string, bool)
	SetRoutes(context.Context, string, string, []gw.Route) bool
	GetModels(context.Context, string) ([]gw.Model, string, bool)
	SetModels(context.Context, string, string, []gw.Model) bool
	Clear(context.Context, string) bool
}

// KeyStore 为 Access Key 查询增加缓存，并在创建或撤销成功后失效 Gateway 缓存。
type KeyStore struct {
	next   appsec.KeyStore
	cache  StoreCache
	logger *slog.Logger
	loads  singleflight.Group
}

func NewKeyStore(next appsec.KeyStore, cache StoreCache, loggers ...*slog.Logger) *KeyStore {
	return &KeyStore{next: next, cache: cache, logger: selectLogger(loggers)}
}

func (s *KeyStore) Create(ctx context.Context, actor admin.Identity, key appsec.KeyRecord, meta appsec.RequestMeta) error {
	if err := s.next.Create(ctx, actor, key, meta); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "gateway data updated; invalidating cache",
		"change_type", "access_key_created", "access_key_id", key.ID, "principal_id", key.PrincipalID)
	s.cache.Clear(ctx, "access_key_created")
	return nil
}

func (s *KeyStore) Revoke(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if err := s.next.Revoke(ctx, actor, id, meta); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "gateway data updated; invalidating cache",
		"change_type", "access_key_revoked", "access_key_id", id)
	s.cache.Clear(ctx, "access_key_revoked")
	return nil
}

func (s *KeyStore) Authenticate(ctx context.Context, hash []byte, now time.Time) (appsec.PrincipalIdentity, error) {
	key := s.cache.IdentityKey(hash)
	identity, generation, ok := s.cache.GetIdentity(ctx, key)
	if ok {
		s.logger.InfoContext(ctx, "gateway identity obtained from cache",
			"cache_type", "identity", "cache_result", "hit",
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
		return identity, nil
	}
	s.logger.InfoContext(ctx, "gateway identity cache missed; loading from database",
		"cache_type", "identity", "cache_result", "miss")
	load := func() (any, error) {
		// 等待同 Key 的并发加载期间，首个请求可能已经填充缓存。
		cached, generation, hit := s.cache.GetIdentity(ctx, key)
		if hit {
			s.logger.InfoContext(ctx, "gateway identity obtained from cache after concurrent load",
				"cache_type", "identity", "cache_result", "hit",
				"principal_id", cached.ID, "access_key_id", cached.AccessKeyID)
			return cached, nil
		}
		value, loadErr := s.next.Authenticate(ctx, hash, now)
		if loadErr != nil {
			return appsec.PrincipalIdentity{}, loadErr
		}
		if s.cache.SetIdentity(ctx, key, generation, value) {
			s.logger.InfoContext(ctx, "gateway identity cached after database load",
				"cache_type", "identity", "cache_result", "populated",
				"principal_id", value.ID, "access_key_id", value.AccessKeyID)
		} else {
			s.logger.InfoContext(ctx, "gateway identity cache population skipped",
				"cache_type", "identity", "cache_result", "skipped",
				"principal_id", value.ID, "access_key_id", value.AccessKeyID)
		}
		return value, nil
	}
	if generation == "" {
		loaded, err := load()
		if err != nil {
			return appsec.PrincipalIdentity{}, err
		}
		return loaded.(appsec.PrincipalIdentity), nil
	}
	loaded, err, _ := s.loads.Do(key+":"+generation, load)
	if err != nil {
		return appsec.PrincipalIdentity{}, err
	}
	return loaded.(appsec.PrincipalIdentity), nil
}

// ManagementStore 在管理写事务成功提交后统一清空 Gateway 缓存。
type ManagementStore struct {
	next   management.Store
	cache  StoreCache
	logger *slog.Logger
}

func NewManagementStore(next management.Store, cache StoreCache, loggers ...*slog.Logger) *ManagementStore {
	return &ManagementStore{next: next, cache: cache, logger: selectLogger(loggers)}
}

func (s *ManagementStore) Read(ctx context.Context, actor admin.Identity, fn func(management.Reader) error) error {
	return s.next.Read(ctx, actor, fn)
}

func (s *ManagementStore) Write(ctx context.Context, actor admin.Identity, fn func(management.Writer) error) error {
	if err := s.next.Write(ctx, actor, fn); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "gateway configuration updated; invalidating cache",
		"change_type", "management_write", "operator_id", actor.ID)
	s.cache.Clear(ctx, "management_write")
	return nil
}

// GatewayStore 缓存鉴权后的候选路由与模型列表；运行时状态变化后立即失效。
type GatewayStore struct {
	next       gw.Store
	cache      StoreCache
	logger     *slog.Logger
	routeLoads singleflight.Group
	modelLoads singleflight.Group
}

func NewGatewayStore(next gw.Store, cache StoreCache, loggers ...*slog.Logger) *GatewayStore {
	return &GatewayStore{next: next, cache: cache, logger: selectLogger(loggers)}
}

func (s *GatewayStore) Resolve(ctx context.Context, identity appsec.PrincipalIdentity, model string, protocols ...string) (gw.Route, error) {
	routes, err := s.ResolveCandidates(ctx, identity, model, protocols...)
	if err != nil {
		return gw.Route{}, err
	}
	if len(routes) == 0 {
		return gw.Route{}, gw.ErrRoute
	}
	return routes[0], nil
}

func (s *GatewayStore) ResolveCandidates(ctx context.Context, identity appsec.PrincipalIdentity, model string, protocols ...string) ([]gw.Route, error) {
	protocol := gw.AnthropicProtocol
	if len(protocols) > 0 {
		protocol = protocols[0]
	}
	key := s.cache.RouteKey(identity, model, protocol)
	routes, generation, ok := s.cache.GetRoutes(ctx, key)
	if ok {
		s.logger.InfoContext(ctx, "gateway routes obtained from cache",
			"cache_type", "routes", "cache_result", "hit",
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
			"model", model, "protocol", protocol, "candidate_count", len(routes))
		return routes, nil
	}
	s.logger.InfoContext(ctx, "gateway route cache missed; loading from database",
		"cache_type", "routes", "cache_result", "miss",
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		"model", model, "protocol", protocol)
	load := func() (any, error) {
		cached, generation, hit := s.cache.GetRoutes(ctx, key)
		if hit {
			s.logger.InfoContext(ctx, "gateway routes obtained from cache after concurrent load",
				"cache_type", "routes", "cache_result", "hit",
				"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
				"model", model, "protocol", protocol, "candidate_count", len(cached))
			return cached, nil
		}
		next, candidateStore := s.next.(gw.CandidateStore)
		if !candidateStore {
			route, loadErr := s.next.Resolve(ctx, identity, model, protocol)
			if loadErr != nil {
				return nil, loadErr
			}
			value := []gw.Route{route}
			s.logRoutePopulation(ctx, key, generation, value, identity, model, protocol)
			return value, nil
		}
		value, loadErr := next.ResolveCandidates(ctx, identity, model, protocol)
		if loadErr != nil {
			return nil, loadErr
		}
		s.logRoutePopulation(ctx, key, generation, value, identity, model, protocol)
		return value, nil
	}
	if generation == "" {
		loaded, err := load()
		if err != nil {
			return nil, err
		}
		return loaded.([]gw.Route), nil
	}
	loaded, err, _ := s.routeLoads.Do(key+":"+generation, load)
	if err != nil {
		return nil, err
	}
	return loaded.([]gw.Route), nil
}

func (s *GatewayStore) Models(ctx context.Context, identity appsec.PrincipalIdentity) ([]gw.Model, error) {
	key := s.cache.ModelsKey(identity)
	models, generation, ok := s.cache.GetModels(ctx, key)
	if ok {
		s.logger.InfoContext(ctx, "gateway model list obtained from cache",
			"cache_type", "models", "cache_result", "hit",
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
			"model_count", len(models))
		return models, nil
	}
	s.logger.InfoContext(ctx, "gateway model list cache missed; loading from database",
		"cache_type", "models", "cache_result", "miss",
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
	load := func() (any, error) {
		cached, generation, hit := s.cache.GetModels(ctx, key)
		if hit {
			s.logger.InfoContext(ctx, "gateway model list obtained from cache after concurrent load",
				"cache_type", "models", "cache_result", "hit",
				"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
				"model_count", len(cached))
			return cached, nil
		}
		next, modelStore := s.next.(gw.ModelStore)
		if !modelStore {
			return nil, gw.ErrUnavailable
		}
		value, loadErr := next.Models(ctx, identity)
		if loadErr != nil {
			return nil, loadErr
		}
		if s.cache.SetModels(ctx, key, generation, value) {
			s.logger.InfoContext(ctx, "gateway model list cached after database load",
				"cache_type", "models", "cache_result", "populated",
				"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
				"model_count", len(value))
		} else {
			s.logger.InfoContext(ctx, "gateway model list cache population skipped",
				"cache_type", "models", "cache_result", "skipped",
				"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
		}
		return value, nil
	}
	if generation == "" {
		loaded, err := load()
		if err != nil {
			return nil, err
		}
		return loaded.([]gw.Model), nil
	}
	loaded, err, _ := s.modelLoads.Do(key+":"+generation, load)
	if err != nil {
		return nil, err
	}
	return loaded.([]gw.Model), nil
}

func (s *GatewayStore) BlockResource(ctx context.Context, identity appsec.PrincipalIdentity, resourceID int64, block gw.ResourceBlock) error {
	next, ok := s.next.(gw.ResourceBlocker)
	if !ok {
		return gw.ErrUnavailable
	}
	if err := next.BlockResource(ctx, identity, resourceID, block); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "gateway resource state updated; invalidating cache",
		"change_type", "resource_blocked", "resource_id", resourceID,
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
	s.cache.Clear(ctx, "resource_blocked")
	return nil
}

func (s *GatewayStore) UpdateResourceCredential(ctx context.Context, route gw.Route, sealed catalog.SealedCredential) error {
	next, ok := s.next.(gw.CredentialUpdater)
	if !ok {
		return gw.ErrUnavailable
	}
	if err := next.UpdateResourceCredential(ctx, route, sealed); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "gateway resource credential updated; invalidating cache",
		"change_type", "resource_credential_updated", "resource_id", route.ResourceID,
		"provider_id", route.ProviderID)
	s.cache.Clear(ctx, "resource_credential_updated")
	return nil
}

func (s *GatewayStore) UpdateResourceCredentialRefreshMetadata(ctx context.Context, route gw.Route) error {
	next, ok := s.next.(gw.CredentialRefreshMetadataUpdater)
	if !ok {
		return gw.ErrUnavailable
	}
	return next.UpdateResourceCredentialRefreshMetadata(ctx, route)
}

func (s *GatewayStore) LoadResourceCredential(ctx context.Context, route gw.Route) (catalog.SealedCredential, error) {
	next, ok := s.next.(gw.CredentialLoader)
	if !ok {
		return catalog.SealedCredential{}, gw.ErrUnavailable
	}
	return next.LoadResourceCredential(ctx, route)
}

func (s *GatewayStore) LockSubscriptionRefresh(ctx context.Context, resourceID int64) (context.Context, func(), error) {
	next, ok := s.next.(gw.SubscriptionRefreshLocker)
	if !ok {
		return ctx, nil, gw.ErrUnavailable
	}
	return next.LockSubscriptionRefresh(ctx, resourceID)
}

func (s *GatewayStore) ListSubscriptionCredentials(ctx context.Context, after int64, limit int32, refreshBefore time.Time) ([]gw.SubscriptionCredential, error) {
	next, ok := s.next.(gw.SubscriptionCredentialLister)
	if !ok {
		return nil, gw.ErrUnavailable
	}
	return next.ListSubscriptionCredentials(ctx, after, limit, refreshBefore)
}

func (s *GatewayStore) logRoutePopulation(ctx context.Context, key, generation string, value []gw.Route, identity appsec.PrincipalIdentity, model, protocol string) {
	if s.cache.SetRoutes(ctx, key, generation, value) {
		s.logger.InfoContext(ctx, "gateway routes cached after database load",
			"cache_type", "routes", "cache_result", "populated",
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
			"model", model, "protocol", protocol, "candidate_count", len(value))
		return
	}
	s.logger.InfoContext(ctx, "gateway route cache population skipped",
		"cache_type", "routes", "cache_result", "skipped",
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		"model", model, "protocol", protocol)
}

func selectLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}
	return slog.Default()
}
