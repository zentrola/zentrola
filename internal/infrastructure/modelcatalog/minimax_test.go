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

func TestDiscoverMiniMaxStableModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != miniMaxCatalogURL ||
			request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"object":"list","data":[{"id":"MiniMax-M2.7","object":"model","owned_by":"MiniMax"},{"id":"MiniMax-M2.7-highspeed","object":"model","owned_by":"MiniMax"},{"id":"image-01-live","object":"model","owned_by":"MiniMax"},{"id":"MiniMax-M3-preview","object":"model","owned_by":"MiniMax"},{"id":"MiniMax-M3-test-202609","object":"model","owned_by":"MiniMax"},{"id":"MiniMax-M3-beta1","object":"model","owned_by":"MiniMax"},{"id":"MiniMax-M2.7","object":"model","owned_by":"MiniMax"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.MiniMaxOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 3 ||
		models[0].Code != "MiniMax-M2.7" || models[0].Name != "MiniMax M2.7" ||
		models[1].Code != "MiniMax-M2.7-highspeed" || models[1].Name != "MiniMax M2.7 highspeed" ||
		models[2].Code != "image-01-live" || models[2].Name != "Image 01 live" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"minimax-official"`, `"catalog_adapter":"minimax"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestMiniMaxStableModelFilter(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{code: "MiniMax-M3", want: true},
		{code: "MiniMax-M2.7-highspeed", want: true},
		{code: "image-01-live", want: true},
		{code: "MiniMax-M3-preview", want: false},
		{code: "MiniMax-M3-preview202609", want: false},
		{code: "MiniMax-M3-test", want: false},
		{code: "MiniMax-M3-testing", want: false},
		{code: "MiniMax-M3-beta1", want: false},
		{code: "MiniMax-M3-alpha", want: false},
		{code: "MiniMax-M3-experimental", want: false},
		{code: "MiniMax-M3-exp", want: false},
		{code: "MiniMax-M3-dev", want: false},
		{code: "MiniMax-M3-canary", want: false},
		{code: "MiniMax-M3-rc2", want: false},
		{code: "MiniMax-M3-latest", want: false},
		{code: "MiniMax-M3-trial1", want: false},
		{code: "MiniMax-contest", want: true},
		{code: "MiniMax-expert", want: true},
		{code: "", want: false},
	}
	for _, test := range tests {
		if got := isMiniMaxStableModel(test.code); got != test.want {
			t.Errorf("isMiniMaxStableModel(%q) = %t, want %t", test.code, got, test.want)
		}
	}
}

func TestDiscoverMiniMaxRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"data":[]}`,
		`{"object":"list"}`,
		`{"object":"list","data":[{"id":"bad model\n","object":"model","owned_by":"MiniMax"}]}`,
		`{"object":"list","data":[{"id":"MiniMax-M3","object":"invalid","owned_by":"MiniMax"}]}`,
		`{"object":"list","data":[{"id":"MiniMax-M3","object":"model","owned_by":""}]}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.MiniMaxOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}

func TestDiscovererSupportsMiniMax(t *testing.T) {
	if !NewDiscoverer(nil).Supports(catalog.MiniMaxOfficialCode) {
		t.Fatal("MiniMax model catalog adapter should be reported as supported")
	}
}
