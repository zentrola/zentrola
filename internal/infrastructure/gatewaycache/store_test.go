package gatewaycache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type cacheStub struct {
	mu            sync.Mutex
	identity      appsec.PrincipalIdentity
	routes        []gw.Route
	models        []gw.Model
	identityReady bool
	routesReady   bool
	modelsReady   bool
	generation    string
	clears        int
}

func (s *cacheStub) generationLocked() string {
	if s.generation == "" {
		return "1"
	}
	return s.generation
}

func (*cacheStub) IdentityKey([]byte) string { return "identity" }
func (*cacheStub) RouteKey(appsec.PrincipalIdentity, string, string) string {
	return "routes"
}
func (*cacheStub) ModelsKey(appsec.PrincipalIdentity) string { return "models" }
func (s *cacheStub) GetIdentity(context.Context, string) (appsec.PrincipalIdentity, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identity, s.generationLocked(), s.identityReady
}
func (s *cacheStub) SetIdentity(_ context.Context, _, generation string, value appsec.PrincipalIdentity) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generationLocked() {
		return false
	}
	s.identity, s.identityReady = value, true
	return true
}
func (s *cacheStub) GetRoutes(context.Context, string) ([]gw.Route, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routes, s.generationLocked(), s.routesReady
}
func (s *cacheStub) SetRoutes(_ context.Context, _, generation string, value []gw.Route) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generationLocked() {
		return false
	}
	s.routes, s.routesReady = value, true
	return true
}
func (s *cacheStub) GetModels(context.Context, string) ([]gw.Model, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.models, s.generationLocked(), s.modelsReady
}
func (s *cacheStub) SetModels(_ context.Context, _, generation string, value []gw.Model) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generationLocked() {
		return false
	}
	s.models, s.modelsReady = value, true
	return true
}
func (s *cacheStub) Clear(context.Context, string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clears++
	s.generation = s.generationLocked() + "-next"
	s.identityReady, s.routesReady, s.modelsReady = false, false, false
	return true
}

type keyStoreStub struct {
	authentications   int
	identity          appsec.PrincipalIdentity
	permissionChanged bool
}

func (*keyStoreStub) Create(context.Context, admin.Identity, appsec.KeyRecord, appsec.RequestMeta) error {
	return nil
}
func (*keyStoreStub) Revoke(context.Context, admin.Identity, int64, appsec.RequestMeta) error {
	return nil
}
func (s *keyStoreStub) SetManagementPermission(context.Context, admin.Identity, int64, bool, appsec.RequestMeta) (bool, error) {
	return s.permissionChanged, nil
}
func (s *keyStoreStub) Authenticate(context.Context, []byte, time.Time) (appsec.PrincipalIdentity, error) {
	s.authentications++
	return s.identity, nil
}

type gatewayStoreStub struct {
	resolutions    int
	blocks         int
	updates        int
	refreshUpdates int
	quotaRecords   int
	routes         []gw.Route
}

func (s *gatewayStoreStub) RecordResourceQuotas(context.Context, gw.Route, string, *time.Time, time.Time, []management.ResourceQuota) error {
	s.quotaRecords++
	return nil
}

func (s *gatewayStoreStub) Resolve(context.Context, appsec.PrincipalIdentity, string, ...string) (gw.Route, error) {
	s.resolutions++
	return s.routes[0], nil
}
func (s *gatewayStoreStub) ResolveCandidates(context.Context, appsec.PrincipalIdentity, string, ...string) ([]gw.Route, error) {
	s.resolutions++
	return s.routes, nil
}
func (s *gatewayStoreStub) Models(context.Context, appsec.PrincipalIdentity) ([]gw.Model, error) {
	return []gw.Model{{ID: "model-a"}}, nil
}
func (s *gatewayStoreStub) AnthropicModels(context.Context, appsec.PrincipalIdentity) ([]gw.DiscoverableModel, error) {
	return []gw.DiscoverableModel{{Code: "claude-sonnet", DisplayName: "Claude Sonnet"}}, nil
}

func TestGatewayStoreForwardsAnthropicModelList(t *testing.T) {
	store := NewGatewayStore(&gatewayStoreStub{}, &cacheStub{})
	models, err := store.AnthropicModels(context.Background(), appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7})
	if err != nil || len(models) != 1 || models[0].Code != "claude-sonnet" {
		t.Fatalf("unexpected Anthropic models: %+v, %v", models, err)
	}
}
func (s *gatewayStoreStub) BlockResource(context.Context, appsec.PrincipalIdentity, int64, gw.ResourceBlock) error {
	s.blocks++
	return nil
}
func (s *gatewayStoreStub) UpdateResourceCredential(context.Context, gw.Route, catalog.SealedCredential) error {
	s.updates++
	return nil
}
func (s *gatewayStoreStub) UpdateResourceCredentialRefreshMetadata(context.Context, gw.Route) error {
	s.refreshUpdates++
	return nil
}

type managementStoreStub struct{ writeErr error }

func (*managementStoreStub) Read(context.Context, admin.Identity, func(management.Reader) error) error {
	return nil
}
func (s *managementStoreStub) Write(context.Context, admin.Identity, func(management.Writer) error) error {
	return s.writeErr
}

type blockingGatewayStore struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (s *blockingGatewayStore) Resolve(context.Context, appsec.PrincipalIdentity, string, ...string) (gw.Route, error) {
	return gw.Route{}, errors.New("unexpected Resolve call")
}
func (s *blockingGatewayStore) ResolveCandidates(context.Context, appsec.PrincipalIdentity, string, ...string) ([]gw.Route, error) {
	s.calls.Add(1)
	s.started <- struct{}{}
	<-s.release
	return []gw.Route{{ModelID: 1, ResourceID: 2}}, nil
}

func TestKeysUseUnifiedNamespace(t *testing.T) {
	cache := &Cache{namespace: Namespace("prod"), generationNamespace: BaseNamespace("prod")}
	identity := appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}
	if got := cache.IdentityKey([]byte{0xab}); got != "zentrola:prod:gateway:v1:identity:ab" {
		t.Fatal(got)
	}
	if got := cache.RouteKey(identity, "model-a", gw.OpenAIProtocol); got != "zentrola:prod:gateway:v1:routes:12:34:OPENAI_CHAT:bW9kZWwtYQ" {
		t.Fatal(got)
	}
	if got := cache.ModelsKey(identity); got != "zentrola:prod:gateway:v1:models:12:34" {
		t.Fatal(got)
	}
	if got := cache.generationKey(); got != "zentrola:prod:gateway:generation" {
		t.Fatal(got)
	}
}

func TestKeyStoreCachesSuccessfulAuthenticationAndClearsOnRevoke(t *testing.T) {
	cache := &cacheStub{}
	next := &keyStoreStub{identity: appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}}
	store := NewKeyStore(next, cache)
	for range 2 {
		identity, err := store.Authenticate(context.Background(), []byte("digest"), time.Now())
		if err != nil || identity != next.identity {
			t.Fatal("authentication failed", err)
		}
	}
	if next.authentications != 1 {
		t.Fatalf("database authentication count = %d", next.authentications)
	}
	if err := store.Revoke(context.Background(), admin.Identity{}, 34, appsec.RequestMeta{}); err != nil || cache.clears != 1 {
		t.Fatal("revoke did not invalidate cache", err)
	}
}

func TestKeyPermissionChangeInvalidatesIdentityCacheOnlyWhenChanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		changed bool
		set     func(*KeyStore) (bool, error)
	}{
		{name: "grant changed", changed: true, set: func(s *KeyStore) (bool, error) {
			return s.SetManagementPermission(context.Background(), admin.Identity{ID: 1}, 34, true, appsec.RequestMeta{})
		}},
		{name: "grant unchanged", set: func(s *KeyStore) (bool, error) {
			return s.SetManagementPermission(context.Background(), admin.Identity{ID: 1}, 34, true, appsec.RequestMeta{})
		}},
		{name: "revoke changed", changed: true, set: func(s *KeyStore) (bool, error) {
			return s.SetManagementPermission(context.Background(), admin.Identity{ID: 1}, 34, false, appsec.RequestMeta{})
		}},
		{name: "revoke unchanged", set: func(s *KeyStore) (bool, error) {
			return s.SetManagementPermission(context.Background(), admin.Identity{ID: 1}, 34, false, appsec.RequestMeta{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &cacheStub{}
			changed, err := tc.set(NewKeyStore(&keyStoreStub{permissionChanged: tc.changed}, cache))
			wantClears := 0
			if tc.changed {
				wantClears = 1
			}
			if err != nil || changed != tc.changed || cache.clears != wantClears {
				t.Fatalf("changed=%v clears=%d err=%v", changed, cache.clears, err)
			}
		})
	}
}

func TestGatewayStoreCachesRoutesAndInvalidatesRuntimeChanges(t *testing.T) {
	cache := &cacheStub{}
	next := &gatewayStoreStub{routes: []gw.Route{{ModelID: 1, ResourceID: 2}}}
	store := NewGatewayStore(next, cache)
	identity := appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}
	for range 2 {
		routes, err := store.ResolveCandidates(context.Background(), identity, "model-a", gw.OpenAIProtocol)
		if err != nil || len(routes) != 1 || routes[0].ResourceID != 2 {
			t.Fatal("route resolution failed", err)
		}
	}
	if next.resolutions != 1 {
		t.Fatalf("database route resolution count = %d", next.resolutions)
	}
	if err := store.BlockResource(context.Background(), identity, 2, gw.ResourceBlock{Reason: "test"}); err != nil || cache.clears != 1 {
		t.Fatal("resource block did not invalidate cache", err)
	}
	cache.routesReady = true
	if err := store.UpdateResourceCredential(context.Background(), next.routes[0], catalog.SealedCredential{KeyVersion: 1}); err != nil || cache.clears != 2 {
		t.Fatal("credential update did not invalidate cache", err)
	}
	if err := store.UpdateResourceCredentialRefreshMetadata(context.Background(), next.routes[0]); err != nil || next.refreshUpdates != 1 || cache.clears != 3 {
		t.Fatal("credential refresh metadata update did not invalidate cache", err)
	}
}

func TestGatewayStoreInvalidatesOnlyWhenQuotaRoutabilityChanges(t *testing.T) {
	tests := []struct {
		name, previous, next string
		wantClear            bool
	}{
		{name: "available to near limit", previous: management.QuotaAvailable, next: management.QuotaNearLimit},
		{name: "available unchanged", previous: management.QuotaAvailable, next: management.QuotaAvailable},
		{name: "available to exhausted", previous: management.QuotaAvailable, next: management.QuotaExhausted, wantClear: true},
		{name: "exhausted to available", previous: management.QuotaExhausted, next: management.QuotaAvailable, wantClear: true},
		{name: "unknown to available", previous: management.QuotaUnknown, next: management.QuotaAvailable, wantClear: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := &cacheStub{}
			next := &gatewayStoreStub{}
			store := NewGatewayStore(next, cache)
			route := gw.Route{ProviderID: 1, ResourceID: 2, QuotaStatus: test.previous}
			if err := store.RecordResourceQuotas(context.Background(), route, test.next, nil, time.Now().UTC(), nil); err != nil {
				t.Fatal(err)
			}
			if next.quotaRecords != 1 || (cache.clears == 1) != test.wantClear {
				t.Fatalf("records=%d clears=%d, want clear %v", next.quotaRecords, cache.clears, test.wantClear)
			}
		})
	}
}

func TestGatewayStoreCoalescesConcurrentCacheMisses(t *testing.T) {
	cache := &cacheStub{}
	next := &blockingGatewayStore{started: make(chan struct{}), release: make(chan struct{})}
	store := NewGatewayStore(next, cache)
	identity := appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}
	const requests = 16
	errorsFound := make(chan error, requests)
	var wait sync.WaitGroup
	wait.Add(requests)
	for range requests {
		go func() {
			defer wait.Done()
			routes, err := store.ResolveCandidates(context.Background(), identity, "model-a", gw.OpenAIProtocol)
			if err != nil || len(routes) != 1 {
				errorsFound <- err
			}
		}()
	}
	<-next.started
	close(next.release)
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal("concurrent route resolution failed", err)
	}
	if calls := next.calls.Load(); calls != 1 {
		t.Fatalf("database route resolution count = %d", calls)
	}
}

func TestGatewayStoreSharedLoadSurvivesFirstRequestCancellation(t *testing.T) {
	cache := &cacheStub{}
	next := &blockingGatewayStore{started: make(chan struct{}, 1), release: make(chan struct{})}
	store := NewGatewayStore(next, cache)
	identity := appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := store.ResolveCandidates(firstCtx, identity, "model-a", gw.OpenAIProtocol)
		firstDone <- err
	}()
	<-next.started

	secondDone := make(chan error, 1)
	go func() {
		routes, err := store.ResolveCandidates(context.Background(), identity, "model-a", gw.OpenAIProtocol)
		if err == nil && len(routes) != 1 {
			err = errors.New("shared load returned no route")
		}
		secondDone <- err
	}()
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first request error = %v, want context cancellation", err)
	}
	close(next.release)
	if err := <-secondDone; err != nil {
		t.Fatalf("second request did not receive shared result: %v", err)
	}
	if calls := next.calls.Load(); calls != 1 {
		t.Fatalf("database route resolution count = %d", calls)
	}
}

func TestGatewayStoreDoesNotCoalesceAcrossGenerationChange(t *testing.T) {
	cache := &cacheStub{generation: "old"}
	next := &blockingGatewayStore{started: make(chan struct{}), release: make(chan struct{})}
	store := NewGatewayStore(next, cache)
	identity := appsec.PrincipalIdentity{ID: 12, AccessKeyID: 34}
	done := make(chan error, 2)
	request := func() {
		_, err := store.ResolveCandidates(context.Background(), identity, "model-a", gw.OpenAIProtocol)
		done <- err
	}
	go request()
	<-next.started
	cache.mu.Lock()
	cache.generation = "new"
	cache.routesReady = false
	cache.mu.Unlock()
	go request()
	select {
	case <-next.started:
	case <-time.After(time.Second):
		t.Fatal("request from new generation joined stale in-flight load")
	}
	close(next.release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls := next.calls.Load(); calls != 2 {
		t.Fatalf("database route resolution count = %d", calls)
	}
}

func TestRouteTTLDoesNotOutliveCredential(t *testing.T) {
	now := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	expires := now.Add(20 * time.Second)
	cache := &Cache{routeTTL: time.Minute, now: func() time.Time { return now }}
	if ttl := cache.routesTTL([]gw.Route{{ExpiresAt: &expires}, {}}); ttl != 20*time.Second {
		t.Fatalf("route TTL = %s", ttl)
	}
	expires = now
	if ttl := cache.routesTTL([]gw.Route{{ExpiresAt: &expires}}); ttl != 0 {
		t.Fatalf("expired route TTL = %s", ttl)
	}
}

func TestManagementStoreClearsOnlyAfterSuccessfulCommit(t *testing.T) {
	cache := &cacheStub{routesReady: true}
	next := &managementStoreStub{}
	store := NewManagementStore(next, cache)
	if err := store.Write(context.Background(), admin.Identity{}, nil); err != nil || cache.clears != 1 {
		t.Fatal("successful write did not invalidate cache", err)
	}
	next.writeErr = errors.New("write failed")
	if err := store.Write(context.Background(), admin.Identity{}, nil); !errors.Is(err, next.writeErr) || cache.clears != 1 {
		t.Fatal("failed write invalidated cache", err)
	}
}
