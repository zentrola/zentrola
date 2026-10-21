package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
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
		{"official limit", http.MethodGet, "/anthropic/v1/models?limit=1000", http.StatusOK},
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

func TestAnthropicModelListPagination(t *testing.T) {
	createdAt := time.Unix(1792548000, 0).UTC()
	store := &modelListStore{models: []gw.DiscoverableModel{
		{Code: "model-a", DisplayName: "Model A", CreatedAt: createdAt},
		{Code: "model-b", DisplayName: "Model B", CreatedAt: createdAt},
		{Code: "model-c", DisplayName: "Model C", CreatedAt: createdAt},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewGatewayHandler(gw.New(store, nil, nil), GatewayOptions{}, logger)
	identity := appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7}
	for _, test := range []struct {
		name, query string
		status      int
		ids         []string
		hasMore     bool
	}{
		{"all without limit", "", http.StatusOK, []string{"model-a", "model-b", "model-c"}, false},
		{"limit covers all", "?limit=1000", http.StatusOK, []string{"model-a", "model-b", "model-c"}, false},
		{"first page", "?limit=2", http.StatusOK, []string{"model-a", "model-b"}, true},
		{"after cursor", "?limit=2&after_id=model-b", http.StatusOK, []string{"model-c"}, false},
		{"after last", "?after_id=model-c", http.StatusOK, []string{}, false},
		{"before cursor", "?limit=1&before_id=model-c", http.StatusOK, []string{"model-b"}, true},
		{"before without limit", "?before_id=model-c", http.StatusOK, []string{"model-a", "model-b"}, false},
		{"zero limit", "?limit=0", http.StatusBadRequest, nil, false},
		{"limit too large", "?limit=1001", http.StatusBadRequest, nil, false},
		{"non numeric limit", "?limit=all", http.StatusBadRequest, nil, false},
		{"repeated limit", "?limit=1&limit=2", http.StatusBadRequest, nil, false},
		{"empty cursor", "?after_id=", http.StatusBadRequest, nil, false},
		{"both cursors", "?before_id=model-c&after_id=model-a", http.StatusBadRequest, nil, false},
		{"unknown cursor", "?after_id=missing", http.StatusBadRequest, nil, false},
		{"unknown parameter", "?limit=1&order=desc", http.StatusBadRequest, nil, false},
		{"malformed query", "?limit=1&%zz", http.StatusBadRequest, nil, false},
		{"semicolon separator", "?limit=1;after_id=model-a", http.StatusBadRequest, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/anthropic/v1/models"+test.query, nil)
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
			ids := make([]string, 0, len(list.Data))
			for _, model := range list.Data {
				ids = append(ids, model.ID)
			}
			if !slices.Equal(ids, test.ids) || list.HasMore != test.hasMore {
				t.Fatalf("ids=%v has_more=%v", ids, list.HasMore)
			}
			if len(ids) > 0 && (list.FirstID == nil || *list.FirstID != ids[0] ||
				list.LastID == nil || *list.LastID != ids[len(ids)-1]) {
				t.Fatalf("unexpected cursors: first=%v last=%v", list.FirstID, list.LastID)
			}
		})
	}
}
