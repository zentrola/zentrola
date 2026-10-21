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

type modelListStore struct {
	selfProviderStore
	models []gw.DiscoverableModel
	err    error
}

func TestAnthropicModelListRouteUsesVirtualKey(t *testing.T) {
	store := &modelListStore{models: []gw.DiscoverableModel{{Code: "claude-sonnet", DisplayName: "Claude Sonnet", CreatedAt: time.Unix(1792548000, 0).UTC()}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	keys := appsec.NewKeys(&selfUsageKeyStore{identity: appsec.PrincipalIdentity{
		ID: 42, AccessKeyID: 7, Type: "MEMBER",
	}}, nil)
	router := NewRouter(logger, health.New(), CORSOptions{}, time.Second, "prod", &SecurityHandlers{
		Keys: keys, Gateway: NewGatewayHandler(gw.New(store, nil, nil), GatewayOptions{}, logger),
	})
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	request := httptest.NewRequest(http.MethodGet, "/anthropic/v1/models", nil)
	request.Header.Set("x-api-key", key)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var list AnthropicModelsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != "claude-sonnet" {
		t.Fatalf("unexpected model list: %+v", list)
	}
}

func (s *modelListStore) AnthropicModels(_ context.Context, _ appsec.PrincipalIdentity) ([]gw.DiscoverableModel, error) {
	return s.models, s.err
}

func TestAnthropicModelList(t *testing.T) {
	store := &modelListStore{models: []gw.DiscoverableModel{{Code: "claude-sonnet", DisplayName: "Claude Sonnet", CreatedAt: time.Unix(1792548000, 0).UTC()}}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewGatewayHandler(gw.New(store, nil, nil), GatewayOptions{}, logger)
	identity := appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7}
	for _, test := range []struct {
		name, method, path string
		status             int
	}{
		{"list", http.MethodGet, "/anthropic/v1/models", http.StatusOK},
		{"invalid query", http.MethodGet, "/anthropic/v1/models?unexpected=1", http.StatusBadRequest},
		{"invalid method", http.MethodPost, "/anthropic/v1/models", http.StatusMethodNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, identity))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.status != http.StatusOK {
				return
			}
			var list AnthropicModelsResponse
			if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if len(list.Data) != 1 || list.Data[0].ID != "claude-sonnet" ||
				list.Data[0].Type != "model" || list.Data[0].DisplayName != "Claude Sonnet" ||
				list.Data[0].CreatedAt != time.Unix(1792548000, 0).UTC().Format(time.RFC3339) ||
				list.HasMore || list.FirstID == nil || *list.FirstID != "claude-sonnet" ||
				list.LastID == nil || *list.LastID != "claude-sonnet" {
				t.Fatalf("unexpected model list: %+v", list)
			}
		})
	}
	store.models = nil
	request := httptest.NewRequest(http.MethodGet, "/anthropic/v1/models", nil)
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, identity))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var empty AnthropicModelsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &empty) != nil ||
		empty.Data == nil || len(empty.Data) != 0 || empty.FirstID != nil || empty.LastID != nil {
		t.Fatalf("unexpected empty model list: status=%d body=%s", response.Code, response.Body.String())
	}
	store.err = gw.ErrUnavailable
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("X-Zentrola-Error-Code") != gw.ErrUnavailable.Code {
		t.Fatalf("unexpected unavailable response: status=%d body=%s", response.Code, response.Body.String())
	}
}
