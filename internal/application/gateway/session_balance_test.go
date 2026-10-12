package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type memorySessionRoutes struct {
	mu           sync.Mutex
	routes       map[string]int64
	moveErr      error
	moveConflict bool
}

type closeTrackingBody struct {
	io.ReadCloser
	closed bool
}

func (body *closeTrackingBody) Close() error {
	body.closed = true
	return body.ReadCloser.Close()
}

func (s *memorySessionRoutes) Get(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routes[key], nil
}

func (s *memorySessionRoutes) Assign(_ context.Context, key string, preferred int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.routes[key]; existing != 0 {
		return existing, nil
	}
	s.routes[key] = preferred
	return preferred, nil
}

func (s *memorySessionRoutes) Move(_ context.Context, key string, from, to int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.moveErr != nil {
		return false, s.moveErr
	}
	if s.moveConflict {
		return false, nil
	}
	if s.routes[key] == from {
		s.routes[key] = to
		return true, nil
	}
	return false, nil
}

func balanceTestRoutes() []Route {
	base := Route{
		ModelID: 1, ProviderID: 10, ProviderModelID: 100,
		ModelCode: "gpt-test", UpstreamModel: "gpt-test", EndpointProtocol: OpenAIEndpoint,
	}
	makeRoute := func(resourceID int64, authType string) Route {
		route := base
		route.ResourceID = resourceID
		route.AuthType = authType
		if authType == "SUBSCRIPTION" {
			route.AuthAdapter = "OPENAI_CODEX"
		}
		return route
	}
	return []Route{
		makeRoute(11, "SUBSCRIPTION"), makeRoute(12, "SUBSCRIPTION"),
		makeRoute(21, "API_KEY"), makeRoute(22, "API_KEY"),
	}
}

func TestSessionRouteKeyScopesClientAndCredential(t *testing.T) {
	identity := appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}
	request := Request{Protocol: OpenAIResponsesProtocol, ClientSessionID: "same-session"}
	base := sessionRouteKey(identity, request, "gpt-test")
	if base == sessionRouteKey(appsec.PrincipalIdentity{ID: 1, AccessKeyID: 4}, request, "gpt-test") {
		t.Fatal("different virtual keys shared a binding")
	}
	request.Protocol = AnthropicProtocol
	if base == sessionRouteKey(identity, request, "gpt-test") {
		t.Fatal("different client protocols shared a binding")
	}
	request.ClientAgentID = "subagent-1"
	if sessionRouteKey(identity, Request{Protocol: AnthropicProtocol, ClientSessionID: "same-session"}, "gpt-test") ==
		sessionRouteKey(identity, request, "gpt-test") {
		t.Fatal("Claude subagent shared the main session binding")
	}
}

func TestSessionBalancePreservesProviderAndCredentialClassOrder(t *testing.T) {
	base := balanceTestRoutes()
	otherSubscription := base[0]
	otherSubscription.ProviderModelID, otherSubscription.ProviderID, otherSubscription.ResourceID = 200, 20, 31
	otherAPIKey := base[2]
	otherAPIKey.ProviderModelID, otherAPIKey.ProviderID, otherAPIKey.ResourceID = 200, 20, 32
	routes := []Route{base[0], otherSubscription, base[1], base[2], otherAPIKey, base[3]}
	for i := range 40 {
		ordered := append([]Route(nil), routes...)
		orderBalanceGroups(ordered, fmt.Sprintf("session-%d", i))
		if ordered[0].ProviderModelID != 100 || ordered[1].ResourceID != 31 ||
			ordered[2].ProviderModelID != 100 || ordered[3].ProviderModelID != 100 ||
			ordered[4].ResourceID != 32 || ordered[5].ProviderModelID != 100 ||
			ordered[0].AuthType != "SUBSCRIPTION" || ordered[3].AuthType != "API_KEY" {
			t.Fatalf("priority order changed: %+v", ordered)
		}
	}
}

func TestSessionBalanceKeepsSubscriptionAheadOfAPIKey(t *testing.T) {
	bindings := &memorySessionRoutes{routes: map[string]int64{}}
	var selected int64
	service := New(&failoverStore{routes: balanceTestRoutes()}, failoverCipher{},
		upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
			selected = route.ResourceID
			return response(200, `{}`), nil
		}),
		WithSessionRouteBinding(bindings),
		WithSubscriptionRefresher(subscriptionRefreshFunc(func(_ context.Context, credential []byte, _ *catalog.OutboundProxy) ([]byte, bool, error) {
			return credential, false, nil
		})),
	)
	identity := appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}
	seen := map[int64]bool{}
	for i := range 40 {
		sessionID := fmt.Sprintf("session-%d", i)
		var firstSelected int64
		request := Request{
			Path: "/v1/responses", Protocol: OpenAIResponsesProtocol,
			Body: []byte(`{"model":"gpt-test","input":"hi"}`), ClientSessionID: sessionID,
		}
		for repeat := range 2 {
			got, err := service.Forward(context.Background(), identity, request)
			if err != nil || got.Status != 200 {
				t.Fatalf("session=%s repeat=%d response=%v err=%v", sessionID, repeat, got, err)
			}
			_ = got.Body.Close()
			if repeat == 0 {
				firstSelected = selected
			} else if selected != firstSelected {
				t.Fatalf("session %s changed resource from %d to %d", sessionID, firstSelected, selected)
			}
			seen[selected] = true
			if selected != 11 && selected != 12 {
				t.Fatalf("API key %d selected before healthy subscription", selected)
			}
		}
	}
	if !seen[11] || !seen[12] {
		t.Fatalf("subscriptions did not both receive new sessions: %v", seen)
	}
	got, err := service.Forward(context.Background(), identity, Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol,
		Body: []byte(`{"model":"gpt-test","input":"hi"}`),
	})
	if err != nil || selected != 11 {
		t.Fatalf("request without session should keep original priority: route=%d err=%v", selected, err)
	}
	_ = got.Body.Close()
}

func TestResponseRouteSelectionExposesSafeRoutingMetadata(t *testing.T) {
	response := &Response{route: &Route{
		ProviderID: 10, ProviderModelID: 20, ResourceID: 30,
		AuthType: "SUBSCRIPTION", Credential: catalog.SealedCredential{Ciphertext: []byte("secret")},
	}}
	selected, ok := response.RouteSelection()
	if !ok || selected.ProviderID != 10 || selected.ProviderModelID != 20 ||
		selected.ResourceID != 30 || selected.AuthType != "SUBSCRIPTION" {
		t.Fatalf("selection=%+v ok=%t", selected, ok)
	}
}

func TestSessionBalanceFailsOverToAPIKeyAndKeepsBinding(t *testing.T) {
	bindings := &memorySessionRoutes{routes: map[string]int64{}}
	state := &failoverState{blocked: map[int64]bool{11: true, 12: true}}
	var selected int64
	service := New(&failoverStore{routes: balanceTestRoutes()}, failoverCipher{},
		upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
			selected = route.ResourceID
			return response(200, `{}`), nil
		}), WithRouteState(state), WithSessionRouteBinding(bindings),
	)
	request := Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol,
		Body: []byte(`{"model":"gpt-test","input":"hi"}`), ClientSessionID: "same-session",
	}
	identity := appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}
	first, err := service.Forward(context.Background(), identity, request)
	if err != nil || selected < 21 {
		t.Fatalf("expected API key fallback: route=%d err=%v", selected, err)
	}
	_ = first.Body.Close()
	selectedAPIKey := selected
	seenAPIKeys := map[int64]bool{selectedAPIKey: true}
	for i := range 40 {
		request.ClientSessionID = fmt.Sprintf("api-session-%d", i)
		result, forwardErr := service.Forward(context.Background(), identity, request)
		if forwardErr != nil || selected < 21 {
			t.Fatalf("new session did not use an API key: route=%d err=%v", selected, forwardErr)
		}
		seenAPIKeys[selected] = true
		_ = result.Body.Close()
	}
	if !seenAPIKeys[21] || !seenAPIKeys[22] {
		t.Fatalf("API keys did not both receive new sessions: %v", seenAPIKeys)
	}
	request.ClientSessionID = "same-session"
	state.blocked[11], state.blocked[12] = false, false
	second, err := service.Forward(context.Background(), identity, request)
	if err != nil || selected != selectedAPIKey {
		t.Fatalf("session moved after subscription recovered: route=%d want=%d err=%v", selected, selectedAPIKey, err)
	}
	_ = second.Body.Close()
}

func TestSessionBalanceMovesBindingWhenResourceIsNoLongerCandidate(t *testing.T) {
	identity := appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}
	request := Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, ClientSessionID: "removed-resource-session",
		Body: []byte(`{"model":"gpt-test","input":"hi"}`),
	}
	key := sessionRouteKey(identity, request, "gpt-test")
	bindings := &memorySessionRoutes{routes: map[string]int64{key: 11}}
	routes := balanceTestRoutes()[1:2]
	service := New(&failoverStore{routes: routes}, failoverCipher{},
		upstreamFunc(func(_ context.Context, route Route, _ Request, _ []byte) (*Response, error) {
			if route.ResourceID != 12 {
				t.Fatalf("route=%d want=12", route.ResourceID)
			}
			return response(200, `{}`), nil
		}), WithSessionRouteBinding(bindings),
		WithSubscriptionRefresher(subscriptionRefreshFunc(func(_ context.Context, credential []byte, _ *catalog.OutboundProxy) ([]byte, bool, error) {
			return credential, false, nil
		})),
	)
	got, err := service.Forward(context.Background(), identity, request)
	if err != nil || got.Status != 200 {
		t.Fatalf("response=%v err=%v", got, err)
	}
	_ = got.Body.Close()
	if assigned, _ := bindings.Get(context.Background(), key); assigned != 12 {
		t.Fatalf("binding=%d want=12", assigned)
	}
	second, err := service.Forward(context.Background(), identity, request)
	if err != nil || second == nil || second.Status != 200 {
		t.Fatalf("next request did not use replacement: response=%v err=%v", second, err)
	}
	_ = second.Body.Close()
}

func TestSessionRouteMoveMustSucceedBeforeDeliveringResponse(t *testing.T) {
	tests := []struct {
		name     string
		moveErr  error
		conflict bool
		wantErr  error
	}{
		{name: "storage failure", moveErr: errors.New("redis unavailable"), wantErr: ErrUnavailable},
		{name: "concurrent binding change", conflict: true, wantErr: ErrSessionRoute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}
			request := Request{
				Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, ClientSessionID: "moved-session",
				Body: []byte(`{"model":"gpt-test","input":"hi"}`),
			}
			key := sessionRouteKey(identity, request, "gpt-test")
			bindings := &memorySessionRoutes{
				routes: map[string]int64{key: 11}, moveErr: test.moveErr, moveConflict: test.conflict,
			}
			body := &closeTrackingBody{ReadCloser: io.NopCloser(strings.NewReader("{}"))}
			service := New(&failoverStore{routes: balanceTestRoutes()[1:2]}, failoverCipher{},
				upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
					return &Response{Status: 200, Headers: map[string][]string{}, Body: body}, nil
				}), WithSessionRouteBinding(bindings),
				WithSubscriptionRefresher(subscriptionRefreshFunc(func(_ context.Context, credential []byte, _ *catalog.OutboundProxy) ([]byte, bool, error) {
					return credential, false, nil
				})),
			)
			got, err := service.Forward(context.Background(), identity, request)
			if got != nil || err != test.wantErr || !body.closed {
				t.Fatalf("response=%v err=%v body closed=%t; want %v", got, err, body.closed, test.wantErr)
			}
			if assigned, _ := bindings.Get(context.Background(), key); assigned != 11 {
				t.Fatalf("binding changed to %d after failed Move", assigned)
			}
		})
	}
}

func TestPreviousResponseIDDoesNotFailOverBetweenAccounts(t *testing.T) {
	called := 0
	identity := appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}
	request := Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, ClientSessionID: "codex-session",
		Body: []byte(`{"model":"gpt-test","previous_response_id":"resp_123","input":"hi"}`),
	}
	bindings := &memorySessionRoutes{routes: map[string]int64{
		sessionRouteKey(identity, request, "gpt-test"): 21,
	}}
	service := New(&failoverStore{routes: balanceTestRoutes()[2:]}, failoverCipher{},
		upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
			called++
			return response(429, `{"error":"rate limit"}`), nil
		}), WithSessionRouteBinding(bindings),
	)
	got, err := service.Forward(context.Background(), identity, request)
	if err != nil || got.Status != 429 || called != 1 {
		t.Fatalf("previous response switched resource: response=%v err=%v calls=%d", got, err, called)
	}
	_ = got.Body.Close()
}

func TestPreviousResponseIDRequiresExistingBinding(t *testing.T) {
	called := false
	service := New(&failoverStore{routes: balanceTestRoutes()[2:]}, failoverCipher{},
		upstreamFunc(func(_ context.Context, _ Route, _ Request, _ []byte) (*Response, error) {
			called = true
			return response(200, `{}`), nil
		}), WithSessionRouteBinding(&memorySessionRoutes{routes: map[string]int64{}}),
	)
	_, err := service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, Request{
		Path: "/v1/responses", Protocol: OpenAIResponsesProtocol, ClientSessionID: "new-session",
		Body: []byte(`{"model":"gpt-test","previous_response_id":"resp_123","input":"hi"}`),
	})
	if err != ErrSessionRoute || called {
		t.Fatalf("unbound continuation was sent upstream: err=%v called=%v", err, called)
	}
}
