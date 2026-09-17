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

func TestDiscoverByteDanceStableFirstPartyModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != byteDanceCatalogURL ||
			request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"object":"list","data":[{"id":"doubao-seed-2-1-pro-260915","object":"model","owned_by":"ByteDance"},{"id":"doubao-seedream-5-0-pro-260628","object":"model","owned_by":"volcengine"},{"id":"doubao-seed-2-2-preview","object":"model","owned_by":"ByteDance"},{"id":"deepseek-v4","object":"model","owned_by":"DeepSeek"},{"id":"doubao-seed-2-1-pro-260915","object":"model","owned_by":"ByteDance"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.ByteDanceOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 2 ||
		models[0].Code != "doubao-seed-2-1-pro-260915" || models[0].Name != "Doubao Seed 2 1 Pro 260915" ||
		models[1].Code != "doubao-seedream-5-0-pro-260628" || models[1].Name != "Doubao Seedream 5 0 Pro 260628" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"doubao-official"`, `"catalog_adapter":"bytedance"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestByteDanceStableFirstPartyModelFilter(t *testing.T) {
	tests := []struct {
		code  string
		owner string
		want  bool
	}{
		{code: "doubao-seed-2-1-pro-260915", owner: "ByteDance", want: true},
		{code: "doubao-seedream-5-0-pro-260628", owner: "volcengine", want: true},
		{code: "DOUBAO-seed-1-6", owner: "doubao", want: true},
		{code: "doubao-seed-2-2-preview", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2-preview202609", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2-beta1", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2-test", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2-experimental", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2-rc2", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2-latest", owner: "ByteDance", want: false},
		{code: "doubao-seed-2-2", owner: "third-party", want: false},
		{code: "deepseek-v4", owner: "ByteDance", want: false},
		{code: "", owner: "ByteDance", want: false},
	}
	for _, test := range tests {
		if got := isByteDanceStableModel(test.code, test.owner); got != test.want {
			t.Errorf("isByteDanceStableModel(%q, %q) = %t, want %t", test.code, test.owner, got, test.want)
		}
	}
}

func TestDiscoverByteDanceRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"data":[]}`,
		`{"object":"list"}`,
		`{"object":"list","data":[{"id":"bad model\n","object":"model","owned_by":"ByteDance"}]}`,
		`{"object":"list","data":[{"id":"doubao-seed-2-1-pro","object":"invalid","owned_by":"ByteDance"}]}`,
		`{"object":"list","data":[{"id":"doubao-seed-2-1-pro","object":"model","owned_by":""}]}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.ByteDanceOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}

func TestDiscovererSupportsByteDance(t *testing.T) {
	if !NewDiscoverer(nil).Supports(catalog.ByteDanceOfficialCode) {
		t.Fatal("ByteDance model catalog adapter should be reported as supported")
	}
}
