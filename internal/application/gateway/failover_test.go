package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

type blockingReadCloser struct {
	closed chan struct{}
	once   sync.Once
}

func (b *blockingReadCloser) Read([]byte) (int, error) {
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *blockingReadCloser) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type failoverStore struct {
	routes        []Route
	blocks        []ResourceBlock
	updatedRoute  int64
	updatedSealed catalog.SealedCredential
	resolvedModel string
	protocols     []string
}

func (s *failoverStore) UpdateResourceCredential(_ context.Context, route Route, sealed catalog.SealedCredential) error {
	s.updatedRoute = route.ResourceID
	s.updatedSealed = sealed
	return nil
}

func (s *failoverStore) Resolve(_ context.Context, _ appsec.PrincipalIdentity, model string, protocols ...string) (Route, error) {
	s.resolvedModel = model
	s.protocols = append([]string(nil), protocols...)
	if len(s.routes) == 0 {
		return Route{}, ErrRoute
	}
	return s.routes[0], nil
}

func (s *failoverStore) ResolveCandidates(_ context.Context, _ appsec.PrincipalIdentity, model string, protocols ...string) ([]Route, error) {
	s.resolvedModel = model
	s.protocols = append([]string(nil), protocols...)
	return append([]Route(nil), s.routes...), nil
}

func (s *failoverStore) BlockResource(_ context.Context, _ appsec.PrincipalIdentity, _ int64, block ResourceBlock) error {
	s.blocks = append(s.blocks, block)
	return nil
}

type failoverCipher struct{ errResource int64 }

func (c failoverCipher) Decrypt(_ catalog.SealedCredential, owner catalog.CredentialOwner) ([]byte, error) {
	if owner.ResourceID == c.errResource {
		return nil, errors.New("cannot decrypt")
	}
	return []byte("secret"), nil
}

func (failoverCipher) DecryptProviderProxy(sealed catalog.SealedCredential, _ catalog.ProviderProxyOwner) ([]byte, error) {
	return append([]byte(nil), sealed.Ciphertext...), nil
}

func (failoverCipher) Encrypt(plain []byte, _ catalog.CredentialOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{Ciphertext: append([]byte(nil), plain...), Nonce: make([]byte, 12), KeyVersion: 2}, nil
}

type subscriptionRefreshFunc func(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error)

func (subscriptionRefreshFunc) Supports(code string) bool { return code == "OPENAI_CODEX" }
func (f subscriptionRefreshFunc) RefreshIfNeeded(ctx context.Context, credential []byte, proxy *catalog.OutboundProxy) ([]byte, bool, error) {
	return f(ctx, credential, proxy)
}

type classifiedSubscriptionRefresher struct {
	err       error
	code      string
	permanent bool
}

func (classifiedSubscriptionRefresher) Supports(code string) bool { return code == "OPENAI_CODEX" }
func (r classifiedSubscriptionRefresher) RefreshIfNeeded(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error) {
	return nil, false, r.err
}
func (r classifiedSubscriptionRefresher) ClassifyRefreshError(error) (string, bool) {
	return r.code, r.permanent
}

type upstreamFunc func(context.Context, Route, Request, []byte) (*Response, error)

func (f upstreamFunc) Open(ctx context.Context, route Route, request Request, credential []byte) (*Response, error) {
	return f(ctx, route, request, credential)
}

type activeRouteRecorderStub struct {
	routes []Route
	err    error
}

func (s *activeRouteRecorderStub) RecordActiveRoute(_ context.Context, route Route) error {
	s.routes = append(s.routes, route)
	return s.err
}

type failoverState struct {
	blocked   map[int64]bool
	cooldowns []int64
	healthy   []int64
}

func (s *failoverState) Acquire(_ context.Context, route Route) (bool, error) {
	return !s.blocked[route.ResourceID], nil
}

func (s *failoverState) Cooldown(_ context.Context, route Route, _ time.Duration) error {
	s.cooldowns = append(s.cooldowns, route.ResourceID)
	return nil
}

func (s *failoverState) Healthy(_ context.Context, route Route) error {
	s.healthy = append(s.healthy, route.ResourceID)
	return nil
}

func testRoutes() []Route {
	return []Route{
		{ModelID: 1, ProviderID: 10, ProviderModelID: 100, ResourceID: 1000, ModelName: "Claude Opus 5", ModelCode: "claude-opus-5", ProviderName: "Provider A", UpstreamModel: "provider-a-model"},
		{ModelID: 1, ProviderID: 20, ProviderModelID: 200, ResourceID: 2000, ModelName: "Claude Opus 5", ModelCode: "claude-opus-5", ProviderName: "Provider B", UpstreamModel: "provider-b-model"},
	}
}

func testForward(t *testing.T, service *Service, trace *usage.Event) (*Response, error) {
	t.Helper()
	return service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, Request{
		Path: "/v1/messages", Protocol: AnthropicProtocol, Body: []byte(`{"model":"claude-opus-5"}`), Trace: trace,
	})
}

func response(status int, body string) *Response {
	return &Response{Status: status, Headers: map[string][]string{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestProviderReturnsFirstCurrentRouteForModel(t *testing.T) {
	store := &failoverStore{routes: []Route{
		{ProviderID: 10, ProviderName: "OpenAI"},
		{ProviderID: 20, ProviderName: "Anthropic"},
	}}
	result, err := New(store, nil, nil).Provider(
		context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, "gpt-5.6-sol",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "OpenAI" || store.resolvedModel != "gpt-5.6-sol" ||
		len(store.protocols) != 1 || store.protocols[0] != OpenAIResponsesProtocol {
		t.Fatalf("unexpected provider resolution: result=%+v model=%q protocols=%v", result, store.resolvedModel, store.protocols)
	}
}

func TestProviderValidatesIdentityAndModel(t *testing.T) {
	service := New(&failoverStore{}, nil, nil)
	for _, test := range []struct {
		name     string
		identity appsec.PrincipalIdentity
		model    string
		want     error
	}{
		{name: "missing identity", model: "gpt-5.6-sol", want: ErrAuthentication},
		{name: "missing access key", identity: appsec.PrincipalIdentity{ID: 1}, model: "gpt-5.6-sol", want: ErrAuthentication},
		{name: "invalid model", identity: appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, model: "", want: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Provider(context.Background(), test.identity, test.model); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestForwardFailsOverAndRecordsEveryAttempt(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	state := &failoverState{blocked: map[int64]bool{}}
	var called []int64
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, request Request, credential []byte) (*Response, error) {
		called = append(called, route.ResourceID)
		if !strings.Contains(string(request.Body), route.UpstreamModel) || string(credential) != "secret" {
			t.Fatal("upstream request was not rewritten for the selected route")
		}
		if route.ResourceID == 1000 {
			return response(503, `{"error":"temporarily unavailable"}`), nil
		}
		return response(200, `{"content":[]}`), nil
	}), WithRouteState(state))
	trace := &usage.Event{}
	got, err := testForward(t, service, trace)
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	defer got.Body.Close()
	if len(called) != 2 || called[0] != 1000 || called[1] != 2000 {
		t.Fatalf("calls=%v", called)
	}
	if len(state.cooldowns) != 1 || state.cooldowns[0] != 1000 {
		t.Fatalf("cooldowns=%v", state.cooldowns)
	}
	if len(trace.Attempts) != 1 || trace.Attempts[0].AttemptNo != 1 || trace.Attempts[0].Status != usage.Failed || trace.Attempt == nil || trace.Attempt.AttemptNo != 2 || trace.Attempt.ResourceID != 2000 {
		t.Fatalf("trace=%+v attempts=%+v", trace.Attempt, trace.Attempts)
	}
}

func TestForwardDoesNotCooldownProxyServerFailure(t *testing.T) {
	state := &failoverState{blocked: map[int64]bool{}}
	service := New(&failoverStore{routes: testRoutes()[:1]}, failoverCipher{}, upstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		return nil, ErrProxyServer
	}), WithRouteState(state))

	got, err := testForward(t, service, nil)
	if got != nil || !errors.Is(err, ErrProxyServer) {
		t.Fatalf("response=%v err=%v", got, err)
	}
	if len(state.cooldowns) != 0 {
		t.Fatalf("proxy server failure unexpectedly cooled down route: %v", state.cooldowns)
	}
}

func TestForwardRecordsOnlyActuallySelectedRouteAndFailsOpen(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	recorder := &activeRouteRecorderStub{err: errors.New("redis unavailable")}
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		if route.ResourceID == 1000 {
			return response(503, `{}`), nil
		}
		return response(200, `{}`), nil
	}), WithRouteState(&failoverState{blocked: map[int64]bool{}}), WithActiveRouteRecorder(recorder))

	got, err := testForward(t, service, nil)
	if err != nil || got == nil || got.Status != 200 {
		t.Fatalf("response=%v err=%v", got, err)
	}
	got.Body.Close()
	if len(recorder.routes) != 1 || recorder.routes[0].ResourceID != 2000 || recorder.routes[0].ProviderName != "Provider B" {
		t.Fatalf("recorded routes=%+v", recorder.routes)
	}
}

func TestForwardTriesEveryCandidateUntilOneSucceeds(t *testing.T) {
	routes := append(testRoutes(), Route{
		ModelID: 1, ProviderID: 30, ProviderModelID: 300, ResourceID: 3000,
		UpstreamModel: "provider-c-model",
	})
	store := &failoverStore{routes: routes}
	state := &failoverState{blocked: map[int64]bool{}}
	trace := &usage.Event{}
	var called []int64
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		called = append(called, route.ResourceID)
		if route.ResourceID != 3000 {
			return response(503, `{"error":"temporarily unavailable"}`), nil
		}
		return response(200, `{"content":[]}`), nil
	}), WithRouteState(state))

	got, err := testForward(t, service, trace)
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	defer got.Body.Close()
	if len(called) != 3 || called[0] != 1000 || called[1] != 2000 || called[2] != 3000 {
		t.Fatalf("calls=%v", called)
	}
	if len(state.cooldowns) != 2 || state.cooldowns[0] != 1000 || state.cooldowns[1] != 2000 {
		t.Fatalf("cooldowns=%v", state.cooldowns)
	}
	if len(trace.Attempts) != 2 || trace.Attempts[0].AttemptNo != 1 || trace.Attempts[1].AttemptNo != 2 ||
		trace.Attempt == nil || trace.Attempt.AttemptNo != 3 || trace.Attempt.ResourceID != 3000 {
		t.Fatalf("trace=%+v attempts=%+v", trace.Attempt, trace.Attempts)
	}
}

func TestForwardSkipsCoolingRoute(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	state := &failoverState{blocked: map[int64]bool{1000: true}}
	var called int64
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		called = route.ResourceID
		return response(200, `{}`), nil
	}), WithRouteState(state))
	got, err := testForward(t, service, nil)
	if err != nil || got.Status != 200 || called != 2000 {
		t.Fatalf("status=%v called=%d err=%v", got, called, err)
	}
	got.Body.Close()
}

func TestForwardReportsCooldownWhenEveryConfiguredRouteIsCooling(t *testing.T) {
	routes := testRoutes()
	state := &failoverState{blocked: map[int64]bool{
		routes[0].ResourceID: true,
		routes[1].ResourceID: true,
	}}
	called := false
	service := New(&failoverStore{routes: routes}, failoverCipher{}, upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
		called = true
		return response(200, `{}`), nil
	}), WithRouteState(state))

	got, err := testForward(t, service, nil)
	if got != nil || !errors.Is(err, ErrRouteCooldown) {
		t.Fatalf("response=%v err=%v", got, err)
	}
	if errors.Is(err, ErrRoute) {
		t.Fatalf("cooldown was misreported as missing route: %v", err)
	}
	if called {
		t.Fatal("upstream was called while every route was cooling")
	}
}

func TestForwardBlocksPermanentFailureAndUsesNextProvider(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	trace := &usage.Event{}
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		if route.ResourceID == 1000 {
			return response(402, `{"error":{"message":"insufficient balance"}}`), nil
		}
		return response(200, `{}`), nil
	}))
	got, err := testForward(t, service, trace)
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	got.Body.Close()
	if len(store.blocks) != 1 || store.blocks[0].Reason != "BILLING" || store.blocks[0].HTTPStatus != 402 {
		t.Fatalf("blocks=%+v", store.blocks)
	}
	if len(trace.Attempts) != 1 || trace.Attempts[0].ErrorType != "UPSTREAM_BILLING_BLOCKED" {
		t.Fatalf("attempts=%+v", trace.Attempts)
	}
}

func TestForwardClassifiesFinalPaymentFailure(t *testing.T) {
	store := &failoverStore{routes: testRoutes()[:1]}
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
		return response(402, `{"error":{"message":"insufficient balance"}}`), nil
	}))
	got, err := testForward(t, service, &usage.Event{})
	if err != nil || got.Status != 402 || got.ErrorType != "UPSTREAM_BILLING_BLOCKED" {
		t.Fatalf("response=%+v err=%v", got, err)
	}
	got.Body.Close()
}

func TestForwardBlocksInactiveUpstreamSubscription(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	trace := &usage.Event{}
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		if route.ResourceID == 1000 {
			return response(403, `{"error":{"message":"No active subscription found for this group"}}`), nil
		}
		return response(200, `{}`), nil
	}))

	got, err := testForward(t, service, trace)
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	got.Body.Close()
	if len(store.blocks) != 1 || store.blocks[0].Reason != "BILLING" ||
		store.blocks[0].HTTPStatus != 403 ||
		store.blocks[0].ErrorCode != "UPSTREAM_BILLING_BLOCKED" {
		t.Fatalf("blocks=%+v", store.blocks)
	}
	if len(trace.Attempts) != 1 || trace.Attempts[0].ErrorType != "UPSTREAM_BILLING_BLOCKED" {
		t.Fatalf("attempts=%+v", trace.Attempts)
	}
}

func TestForwardDoesNotSwitchOnClientRequestError(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	calls := 0
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
		calls++
		return response(400, `{"error":"invalid request"}`), nil
	}))
	got, err := testForward(t, service, nil)
	if err != nil || got.Status != 400 || calls != 1 {
		t.Fatalf("status=%v calls=%d err=%v", got, calls, err)
	}
	got.Body.Close()
}

func TestReadResponsePrefixTimesOutAndClosesStalledBody(t *testing.T) {
	body := &blockingReadCloser{closed: make(chan struct{})}
	started := time.Now()
	prefix, replay, err := readResponsePrefix(body, 64<<10, 20*time.Millisecond)
	if !errors.Is(err, errResponseInspectionTimeout) || len(prefix) != 0 || replay != http.NoBody {
		t.Fatalf("prefix=%q body=%T err=%v", prefix, replay, err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("stalled response inspection did not stop promptly")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("stalled response body was not closed")
	}
}

func TestRetryAfterAcceptsDeltaSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, time.September, 20, 3, 4, 5, 0, time.UTC)
	if got := retryAfterAt(map[string][]string{"Retry-After": {"12"}}, now); got != 12*time.Second {
		t.Fatalf("delta-seconds=%v", got)
	}
	date := now.Add(45 * time.Second).Format(http.TimeFormat)
	if got := retryAfterAt(map[string][]string{"Retry-After": {date}}, now); got != 45*time.Second {
		t.Fatalf("http-date=%v", got)
	}
	if got := retryAfterAt(map[string][]string{"Retry-After": {now.Add(2 * time.Hour).Format(http.TimeFormat)}}, now); got != defaultRouteCooldown {
		t.Fatalf("unbounded retry-after=%v", got)
	}
}

func TestForwardBlocksUnrecoverableCredentialAndUsesNextProvider(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	var called int64
	service := New(store, failoverCipher{errResource: 1000}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		called = route.ResourceID
		return response(200, `{}`), nil
	}))
	got, err := testForward(t, service, nil)
	if err != nil || got.Status != 200 || called != 2000 {
		t.Fatalf("status=%v called=%d err=%v", got, called, err)
	}
	got.Body.Close()
	if len(store.blocks) != 1 || store.blocks[0].Reason != "CREDENTIAL_UNRECOVERABLE" {
		t.Fatalf("blocks=%+v", store.blocks)
	}
}

func TestForwardSkipsLocallyIncompatibleRouteWithoutUsageOrCooldown(t *testing.T) {
	store := &failoverStore{routes: testRoutes()}
	state := &failoverState{blocked: map[int64]bool{}}
	trace := &usage.Event{}
	calls := 0
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
		calls++
		if calls == 1 {
			return nil, ErrRoute
		}
		return response(200, `{}`), nil
	}), WithRouteState(state))
	got, err := testForward(t, service, trace)
	if err != nil || got.Status != 200 || calls != 2 {
		t.Fatalf("status=%v calls=%d err=%v", got, calls, err)
	}
	got.Body.Close()
	if len(trace.Attempts) != 0 || trace.Attempt == nil || trace.Attempt.AttemptNo != 1 || len(state.cooldowns) != 0 {
		t.Fatalf("trace=%+v attempts=%+v cooldowns=%v", trace.Attempt, trace.Attempts, state.cooldowns)
	}
}

func TestForwardRefreshesSubscriptionCredentialAndPersistsIt(t *testing.T) {
	route := testRoutes()[0]
	route.AuthType = "SUBSCRIPTION"
	route.AuthAdapter = "OPENAI_CODEX"
	route.ProxyEnabled = true
	route.ProxyURL = catalog.SealedCredential{Ciphertext: []byte("http://proxy.example.com:8080"), KeyVersion: 1}
	store := &failoverStore{routes: []Route{route}}
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, got Route, _ Request, credential []byte) (*Response, error) {
		if got.ResourceID != route.ResourceID || string(credential) != "refreshed-auth-cache" || got.Proxy == nil || got.Proxy.URL != "http://proxy.example.com:8080" {
			t.Fatalf("unexpected refreshed route or credential: %+v %q", got, credential)
		}
		return response(200, `{}`), nil
	}), WithSubscriptionRefresher(subscriptionRefreshFunc(func(_ context.Context, _ []byte, proxy *catalog.OutboundProxy) ([]byte, bool, error) {
		if proxy == nil || proxy.URL != "http://proxy.example.com:8080" {
			t.Fatalf("subscription refresh did not receive provider proxy: %+v", proxy)
		}
		return []byte("refreshed-auth-cache"), true, nil
	})))

	got, err := service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, Body: []byte(`{"model":"model"}`),
	})
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	got.Body.Close()
	if store.updatedRoute != route.ResourceID || string(store.updatedSealed.Ciphertext) != "refreshed-auth-cache" {
		t.Fatalf("refreshed credential was not persisted: route=%d sealed=%+v", store.updatedRoute, store.updatedSealed)
	}
}

func TestForwardFallsBackToAPIKeyWhenSubscriptionRefreshFails(t *testing.T) {
	routes := testRoutes()
	routes[0].AuthType = "SUBSCRIPTION"
	routes[0].AuthAdapter = "OPENAI_CODEX"
	routes[1].AuthType = "API_KEY"
	store := &failoverStore{routes: routes}
	state := &failoverState{blocked: map[int64]bool{}}
	var called []int64
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		called = append(called, route.ResourceID)
		return response(200, `{}`), nil
	}), WithRouteState(state), WithSubscriptionRefresher(subscriptionRefreshFunc(func(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error) {
		return nil, false, errors.New("refresh failed")
	})))

	got, err := service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, Body: []byte(`{"model":"model"}`),
	})
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	got.Body.Close()
	if len(called) != 1 || called[0] != routes[1].ResourceID || len(state.cooldowns) != 1 || len(store.blocks) != 0 {
		t.Fatalf("calls=%v cooldowns=%v blocks=%v", called, state.cooldowns, store.blocks)
	}
}

func TestForwardBlocksPermanentSubscriptionRefreshFailure(t *testing.T) {
	routes := testRoutes()
	routes[0].AuthType = "SUBSCRIPTION"
	routes[0].AuthAdapter = "OPENAI_CODEX"
	routes[1].AuthType = "API_KEY"
	store := &failoverStore{routes: routes}
	state := &failoverState{blocked: map[int64]bool{}}
	var called []int64
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
		called = append(called, route.ResourceID)
		return response(200, `{}`), nil
	}), WithRouteState(state), WithSubscriptionRefresher(classifiedSubscriptionRefresher{
		err: errors.New("refresh token revoked"), code: "CREDENTIAL_REVOKED", permanent: true,
	}))

	got, err := service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, Body: []byte(`{"model":"model"}`),
	})
	if err != nil || got.Status != 200 {
		t.Fatalf("status=%v err=%v", got, err)
	}
	got.Body.Close()
	if len(called) != 1 || called[0] != routes[1].ResourceID || len(state.cooldowns) != 0 ||
		len(store.blocks) != 1 || store.blocks[0].Reason != "CREDENTIAL_REVOKED" ||
		store.blocks[0].ErrorCode != "CREDENTIAL_REVOKED" {
		t.Fatalf("calls=%v cooldowns=%v blocks=%+v", called, state.cooldowns, store.blocks)
	}
}
