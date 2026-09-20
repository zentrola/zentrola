package modelcatalog

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestDiscoverXAILanguageModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != xAILanguageModelsURL ||
			request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"models":[{"id":"latest","aliases":["grok-4.3-latest"],"object":"model","owned_by":"xai","input_modalities":["text","image"],"output_modalities":["text"]},{"id":"grok-420-reasoning","aliases":[],"object":"model","owned_by":"xai","input_modalities":["text"],"output_modalities":["text"]},{"id":"internal","aliases":[],"object":"model","owned_by":"xai","input_modalities":["text"],"output_modalities":["text"]},{"id":"grok-420-reasoning","aliases":[],"object":"model","owned_by":"xai","input_modalities":["text"],"output_modalities":["text"]}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.XAIOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 2 ||
		models[0].Code != "grok-4.3-latest" || models[0].Name != "Grok 4.3 latest" ||
		len(models[0].InputModalities) != 2 || models[0].InputModalities[1] != "IMAGE" ||
		models[1].Code != "grok-420-reasoning" || models[1].Name != "Grok 420 reasoning" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"xai-official"`, `"catalog_adapter":"xai"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestDiscoverXAIRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"models":[{"id":"grok-4","aliases":[],"object":"invalid","owned_by":"xai","input_modalities":["text"],"output_modalities":["text"]}]}`,
		`{"models":[{"id":"grok-4","aliases":[],"object":"model","owned_by":"other","input_modalities":["text"],"output_modalities":["text"]}]}`,
		`{"models":[{"id":"latest","aliases":["bad\nmodel"],"object":"model","owned_by":"xai","input_modalities":["text"],"output_modalities":["text"]}]}`,
		`{"models":[{"id":"grok-4","aliases":[],"object":"model","owned_by":"xai","input_modalities":["text","text"],"output_modalities":["text"]}]}`,
		`{"models":[{"id":"grok-4","aliases":[],"object":"model","owned_by":"xai","input_modalities":["text"],"output_modalities":["unknown"]}]}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.XAIOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}
