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
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDiscoverDeepSeekModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://api.deepseek.com/models" || request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"object":"list","data":[{"id":"deepseek-v4-flash","object":"model","owned_by":"deepseek"},{"id":"deepseek-v4-pro","object":"model","owned_by":"deepseek"},{"id":"deepseek-v4-flash-vision-exp","object":"model","owned_by":"deepseek"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: "deepseek-official"}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 3 || models[1].Code != "deepseek-v4-flash-vision-exp" || models[1].Name != "Deepseek v4 flash vision exp" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"deepseek-official"`, `"catalog_adapter":"deepseek"`, `"upstream_status":200`, `"response_body":"{\"object\":\"list\"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestDiscoverRejectsUnsupportedProviderWithoutRequest(t *testing.T) {
	discoverer := NewDiscoverer(nil)
	discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported provider reached transport")
		return nil, nil
	})
	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: "openai-official"}, []byte("test-secret"), nil)
	if result.OK || result.Code != "MODEL_CATALOG_UNSUPPORTED" || models != nil {
		t.Fatalf("unsupported provider accepted: %+v %+v", models, result)
	}
}

func TestDiscoverDeepSeekRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"object":"list","data":[{"id":"bad model\n","object":"model","owned_by":"deepseek"}]}`,
		`{"object":"list","data":[{"id":"deepseek-v4","object":"unknown","owned_by":"deepseek"}]}`,
		`{"object":"list","data":[{"id":"deepseek-v4","object":"model","owned_by":"other"}]}`,
		`{"object":"list"}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: "deepseek-official"}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}
