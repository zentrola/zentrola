package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

type selfProviderStore struct {
	model     string
	protocols []string
	route     gw.Route
	err       error
}

func (s *selfProviderStore) Resolve(_ context.Context, _ appsec.PrincipalIdentity, model string, protocols ...string) (gw.Route, error) {
	s.model = model
	s.protocols = append([]string(nil), protocols...)
	return s.route, s.err
}

func selfProviderRouter(store *selfProviderStore) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	keys := appsec.NewKeys(&selfUsageKeyStore{identity: appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7, Type: "MEMBER"}}, nil)
	service := gw.New(store, nil, nil)
	return NewRouter(
		logger, health.New(), CORSOptions{}, time.Second, "prod",
		&SecurityHandlers{Keys: keys, Gateway: NewGatewayHandler(service, GatewayOptions{}, logger)},
	)
}

func TestSelfProviderReturnsCurrentModelProvider(t *testing.T) {
	store := &selfProviderStore{route: gw.Route{ProviderID: 8, ProviderName: "OpenAI"}}
	router := selfProviderRouter(store)
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	for _, test := range []struct {
		name   string
		header string
	}{
		{name: "bearer", header: "Authorization"},
		{name: "api key", header: "X-Api-Key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/me/provider?model=gpt-5.6-sol", nil)
			if test.header == "Authorization" {
				request.Header.Set(test.header, "Bearer "+key)
			} else {
				request.Header.Set(test.header, key)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Code string      `json:"code"`
				Data gw.Provider `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != "OK" || body.Data.Name != "OpenAI" || store.model != "gpt-5.6-sol" {
				t.Fatalf("unexpected provider response: body=%+v model=%q", body, store.model)
			}
		})
	}
}

func TestSelfProviderRejectsInvalidRequests(t *testing.T) {
	store := &selfProviderStore{route: gw.Route{ProviderID: 8, ProviderName: "OpenAI"}}
	router := selfProviderRouter(store)
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	for _, test := range []struct {
		name    string
		path    string
		headers map[string]string
		status  int
	}{
		{name: "missing key", path: "/api/v1/me/provider?model=gpt-5.6-sol", status: http.StatusUnauthorized},
		{name: "both key forms", path: "/api/v1/me/provider?model=gpt-5.6-sol", headers: map[string]string{"Authorization": "Bearer " + key, "X-Api-Key": key}, status: http.StatusUnauthorized},
		{name: "missing model", path: "/api/v1/me/provider", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
		{name: "unknown parameter", path: "/api/v1/me/provider?model=gpt-5.6-sol&other=x", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
}

func TestSelfProviderPreservesGatewayFailure(t *testing.T) {
	router := selfProviderRouter(&selfProviderStore{err: gw.ErrModelUnknown})
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/provider?model=unknown", nil)
	request.Header.Set("Authorization", "Bearer "+key)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != gw.ErrModelUnknown.Code {
		t.Fatalf("unexpected error response: %+v", body)
	}
}
