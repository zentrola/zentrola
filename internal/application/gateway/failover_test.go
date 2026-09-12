package gateway

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

type failoverStore struct {
	routes        []Route
	blocks        []ResourceBlock
	updatedRoute  int64
	updatedSealed catalog.SealedCredential
}

func (s *failoverStore) UpdateResourceCredential(_ context.Context, route Route, sealed catalog.SealedCredential) error {
	s.updatedRoute = route.ResourceID
	s.updatedSealed = sealed
	return nil
}

func (s *failoverStore) Resolve(context.Context, appsec.PrincipalIdentity, string, ...string) (Route, error) {
	if len(s.routes) == 0 {
		return Route{}, ErrRoute
	}
	return s.routes[0], nil
}

func (s *failoverStore) ResolveCandidates(context.Context, appsec.PrincipalIdentity, string, ...string) ([]Route, error) {
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

func (failoverCipher) DecryptProviderProxy(catalog.SealedCredential, catalog.ProviderProxyOwner) ([]byte, error) {
	return nil, errors.New("unexpected proxy decryption")
}

func (failoverCipher) Encrypt(plain []byte, _ catalog.CredentialOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{Ciphertext: append([]byte(nil), plain...), Nonce: make([]byte, 12), KeyVersion: 2}, nil
}

type subscriptionRefreshFunc func(context.Context, []byte) ([]byte, bool, error)

func (subscriptionRefreshFunc) Supports(code string) bool { return code == "OPENAI_CODEX" }
func (f subscriptionRefreshFunc) RefreshIfNeeded(ctx context.Context, credential []byte) ([]byte, bool, error) {
	return f(ctx, credential)
}

type upstreamFunc func(context.Context, Route, Request, []byte) (*Response, error)

func (f upstreamFunc) Open(ctx context.Context, route Route, request Request, credential []byte) (*Response, error) {
	return f(ctx, route, request, credential)
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
		{ModelID: 1, ProviderID: 10, ProviderModelID: 100, ResourceID: 1000, UpstreamModel: "provider-a-model"},
		{ModelID: 1, ProviderID: 20, ProviderModelID: 200, ResourceID: 2000, UpstreamModel: "provider-b-model"},
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
	store := &failoverStore{routes: []Route{route}}
	service := New(store, failoverCipher{}, upstreamFunc(func(_ context.Context, got Route, _ Request, credential []byte) (*Response, error) {
		if got.ResourceID != route.ResourceID || string(credential) != "refreshed-auth-cache" {
			t.Fatalf("unexpected refreshed route or credential: %+v %q", got, credential)
		}
		return response(200, `{}`), nil
	}), WithSubscriptionRefresher(subscriptionRefreshFunc(func(context.Context, []byte) ([]byte, bool, error) {
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
	}), WithRouteState(state), WithSubscriptionRefresher(subscriptionRefreshFunc(func(context.Context, []byte) ([]byte, bool, error) {
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
