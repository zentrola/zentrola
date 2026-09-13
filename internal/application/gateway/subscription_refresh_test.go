package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type subscriptionRefreshStore struct {
	mu                   sync.Mutex
	refreshLock          sync.Mutex
	credentials          map[int64]SubscriptionCredential
	listCalls, lockCalls []int64
	updated, unlockCalls []int64
	listed               chan struct{}
	listedOnce           sync.Once
}

type subscriptionRefreshLockContextKey struct{}

func (s *subscriptionRefreshStore) Resolve(context.Context, appsec.PrincipalIdentity, string, ...string) (Route, error) {
	return Route{}, ErrRoute
}

func (s *subscriptionRefreshStore) ListSubscriptionCredentials(_ context.Context, after int64, limit int32, _ time.Time) ([]SubscriptionCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls = append(s.listCalls, after)
	if s.listed != nil {
		s.listedOnce.Do(func() { close(s.listed) })
	}
	ids := make([]int64, 0, len(s.credentials))
	for id := range s.credentials {
		if id > after {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	if len(ids) > int(limit) {
		ids = ids[:limit]
	}
	result := make([]SubscriptionCredential, 0, len(ids))
	for _, id := range ids {
		credential := s.credentials[id]
		credential.Credential = cloneSealedCredential(credential.Credential)
		result = append(result, credential)
	}
	return result, nil
}

func (s *subscriptionRefreshStore) LoadResourceCredential(ctx context.Context, route Route) (catalog.SealedCredential, error) {
	if lockedResource, _ := ctx.Value(subscriptionRefreshLockContextKey{}).(int64); lockedResource != route.ResourceID {
		return catalog.SealedCredential{}, errors.New("credential load did not use the lock context")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, ok := s.credentials[route.ResourceID]
	if !ok || credential.ProviderID != route.ProviderID {
		return catalog.SealedCredential{}, ErrUnavailable
	}
	return cloneSealedCredential(credential.Credential), nil
}

func (s *subscriptionRefreshStore) LockSubscriptionRefresh(ctx context.Context, resourceID int64) (context.Context, func(), error) {
	s.mu.Lock()
	s.lockCalls = append(s.lockCalls, resourceID)
	s.mu.Unlock()
	s.refreshLock.Lock()
	var once sync.Once
	lockedCtx := context.WithValue(ctx, subscriptionRefreshLockContextKey{}, resourceID)
	return lockedCtx, func() {
		once.Do(func() {
			s.mu.Lock()
			s.unlockCalls = append(s.unlockCalls, resourceID)
			s.mu.Unlock()
			s.refreshLock.Unlock()
		})
	}, nil
}

func (s *subscriptionRefreshStore) UpdateResourceCredential(ctx context.Context, route Route, sealed catalog.SealedCredential) error {
	if lockedResource, _ := ctx.Value(subscriptionRefreshLockContextKey{}).(int64); lockedResource != route.ResourceID {
		return errors.New("credential update did not use the lock context")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, ok := s.credentials[route.ResourceID]
	if !ok {
		return ErrUnavailable
	}
	credential.Credential = cloneSealedCredential(sealed)
	credential.CredentialRefreshedAt = route.CredentialRefreshedAt
	credential.CredentialExpiresAt = route.CredentialExpiresAt
	s.credentials[route.ResourceID] = credential
	s.updated = append(s.updated, route.ResourceID)
	return nil
}

func (s *subscriptionRefreshStore) UpdateResourceCredentialRefreshMetadata(_ context.Context, route Route) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, ok := s.credentials[route.ResourceID]
	if !ok {
		return ErrUnavailable
	}
	credential.CredentialRefreshedAt = route.CredentialRefreshedAt
	credential.CredentialExpiresAt = route.CredentialExpiresAt
	s.credentials[route.ResourceID] = credential
	return nil
}

func cloneSealedCredential(value catalog.SealedCredential) catalog.SealedCredential {
	return catalog.SealedCredential{
		Ciphertext: append([]byte(nil), value.Ciphertext...),
		Nonce:      append([]byte(nil), value.Nonce...),
		KeyVersion: value.KeyVersion,
	}
}

type subscriptionRefreshCipher struct{}

func (subscriptionRefreshCipher) Decrypt(value catalog.SealedCredential, _ catalog.CredentialOwner) ([]byte, error) {
	return append([]byte(nil), value.Ciphertext...), nil
}

func (subscriptionRefreshCipher) DecryptProviderProxy(sealed catalog.SealedCredential, _ catalog.ProviderProxyOwner) ([]byte, error) {
	return append([]byte(nil), sealed.Ciphertext...), nil
}

func (subscriptionRefreshCipher) Encrypt(plain []byte, _ catalog.CredentialOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{Ciphertext: append([]byte(nil), plain...), Nonce: make([]byte, 12), KeyVersion: 1}, nil
}

type scheduledSubscriptionRefresher struct{}

func (scheduledSubscriptionRefresher) Supports(code string) bool { return code == "OPENAI_CODEX" }

func (scheduledSubscriptionRefresher) NeedsRefresh(credential []byte) (bool, error) {
	return !strings.HasPrefix(string(credential), "fresh"), nil
}

func (scheduledSubscriptionRefresher) RefreshIfNeeded(_ context.Context, credential []byte, _ *catalog.OutboundProxy) ([]byte, bool, error) {
	if string(credential) == "bad" {
		return nil, false, errors.New("refresh failed")
	}
	if strings.HasPrefix(string(credential), "fresh") {
		return nil, false, nil
	}
	return []byte("fresh-" + string(credential)), true, nil
}

func newSubscriptionRefreshService(store *subscriptionRefreshStore) *Service {
	return New(store, subscriptionRefreshCipher{}, nil, WithSubscriptionRefresher(scheduledSubscriptionRefresher{}))
}

func subscriptionCredential(id int64, raw string) SubscriptionCredential {
	return SubscriptionCredential{
		ProviderID: 10, ResourceID: id, AuthAdapter: "OPENAI_CODEX",
		Credential: catalog.SealedCredential{Ciphertext: []byte(raw), Nonce: make([]byte, 12), KeyVersion: 1},
	}
}

func TestRefreshSubscriptionsContinuesAfterCredentialFailure(t *testing.T) {
	store := &subscriptionRefreshStore{credentials: map[int64]SubscriptionCredential{
		1: subscriptionCredential(1, "fresh-token"),
		2: subscriptionCredential(2, "expiring-token"),
		3: subscriptionCredential(3, "bad"),
		4: func() SubscriptionCredential {
			credential := subscriptionCredential(4, "other-adapter-token")
			credential.AuthAdapter = "OTHER"
			return credential
		}(),
	}}
	summary, err := newSubscriptionRefreshService(store).RefreshSubscriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Checked != 3 || summary.Refreshed != 1 || len(summary.Failures) != 1 || summary.Failures[0].ResourceID != 3 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if !slices.Equal(store.updated, []int64{2}) {
		t.Fatalf("updated=%v", store.updated)
	}
	if got := string(store.credentials[2].Credential.Ciphertext); got != "fresh-expiring-token" {
		t.Fatalf("persisted credential=%q", got)
	}
}

type proxyCapturingSubscriptionRefresher struct {
	proxy *catalog.OutboundProxy
}

func (*proxyCapturingSubscriptionRefresher) Supports(code string) bool { return code == "OPENAI_CODEX" }

func (r *proxyCapturingSubscriptionRefresher) RefreshIfNeeded(_ context.Context, _ []byte, proxy *catalog.OutboundProxy) ([]byte, bool, error) {
	r.proxy = proxy
	return nil, false, nil
}

func TestRefreshSubscriptionsUsesProviderProxy(t *testing.T) {
	credential := subscriptionCredential(1, "expiring-token")
	credential.ProxyEnabled = true
	credential.ProxyURL = catalog.SealedCredential{Ciphertext: []byte("https://proxy.example.com:8443"), KeyVersion: 1}
	credential.ProxyHeaders = catalog.SealedCredential{Ciphertext: []byte(`{"X-Proxy-Token":"secret"}`), KeyVersion: 1}
	store := &subscriptionRefreshStore{credentials: map[int64]SubscriptionCredential{1: credential}}
	refresher := &proxyCapturingSubscriptionRefresher{}
	service := New(store, subscriptionRefreshCipher{}, nil, WithSubscriptionRefresher(refresher))

	summary, err := service.RefreshSubscriptions(context.Background())
	if err != nil || summary.Checked != 1 || len(summary.Failures) != 0 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if refresher.proxy == nil || refresher.proxy.URL != "https://proxy.example.com:8443" || refresher.proxy.Headers["X-Proxy-Token"] != "secret" {
		t.Fatalf("subscription refresh did not receive provider proxy: %+v", refresher.proxy)
	}
}

func TestRefreshSubscriptionsPaginates(t *testing.T) {
	store := &subscriptionRefreshStore{credentials: make(map[int64]SubscriptionCredential)}
	for id := int64(1); id <= 101; id++ {
		store.credentials[id] = subscriptionCredential(id, "fresh-token")
	}
	summary, err := newSubscriptionRefreshService(store).RefreshSubscriptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Checked != 101 || summary.Refreshed != 0 || len(summary.Failures) != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if !slices.Equal(store.listCalls, []int64{0, 100}) {
		t.Fatalf("pagination calls=%v", store.listCalls)
	}
}

func TestRefreshCredentialSerializesAcrossServiceInstances(t *testing.T) {
	store := &subscriptionRefreshStore{credentials: map[int64]SubscriptionCredential{
		1: subscriptionCredential(1, "expiring-token"),
	}}
	services := []*Service{newSubscriptionRefreshService(store), newSubscriptionRefreshService(store)}
	route := Route{ProviderID: 10, ResourceID: 1, AuthType: "SUBSCRIPTION", AuthAdapter: "OPENAI_CODEX"}
	type result struct {
		credential string
		changed    bool
		err        error
	}
	results := make(chan result, len(services))
	start := make(chan struct{})
	for _, service := range services {
		go func(service *Service) {
			<-start
			updated, changed, err := service.refreshCredential(context.Background(), route, []byte("expiring-token"))
			results <- result{credential: string(updated), changed: changed, err: err}
			clear(updated)
		}(service)
	}
	close(start)
	changedCount := 0
	for range services {
		got := <-results
		if got.err != nil || got.credential != "fresh-expiring-token" {
			t.Fatalf("unexpected refresh result: %+v", got)
		}
		if got.changed {
			changedCount++
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if changedCount != 1 || !slices.Equal(store.updated, []int64{1}) ||
		len(store.lockCalls) != 2 || len(store.unlockCalls) != 2 {
		t.Fatalf("changed=%d updated=%v locks=%v unlocks=%v", changedCount, store.updated, store.lockCalls, store.unlockCalls)
	}
}

func TestSubscriptionRefreshWorkerRunsImmediatelyAndStops(t *testing.T) {
	store := &subscriptionRefreshStore{
		credentials: map[int64]SubscriptionCredential{},
		listed:      make(chan struct{}),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker, err := NewSubscriptionRefreshWorker(newSubscriptionRefreshService(store), logger, time.Hour, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.listed:
	case <-time.After(time.Second):
		t.Fatal("worker did not run its startup scan")
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
}
