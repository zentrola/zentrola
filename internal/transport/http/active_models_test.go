package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
)

type activeModelReaderStub struct {
	models []gw.ActiveModel
	err    error
}

func (s *activeModelReaderStub) ActiveModels(context.Context) ([]gw.ActiveModel, error) {
	return s.models, s.err
}

func TestActiveModelsEndpoint(t *testing.T) {
	reader := &activeModelReaderStub{models: []gw.ActiveModel{{
		ModelName: "GPT 6", ModelCode: "gpt-6", ProviderName: "OpenAI",
	}}}
	handlers := &SecurityHandlers{ActiveModels: reader}
	router := chi.NewRouter()
	handlers.mountActiveModels(router)

	request := httptest.NewRequest(http.MethodGet, "/gateway/active-models", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"modelName":"GPT 6"`) || !strings.Contains(response.Body.String(), `"modelCode":"gpt-6"`) || !strings.Contains(response.Body.String(), `"providerName":"OpenAI"`) {
		t.Fatalf("unexpected active model response: status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/gateway/active-models?unexpected=1", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected invalid query status: %d", response.Code)
	}

	reader.err = errors.New("redis unavailable")
	request = httptest.NewRequest(http.MethodGet, "/gateway/active-models", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected unavailable status: %d", response.Code)
	}
}
