package modelcatalog

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
)

func TestDiscoverAnthropicModelsAcrossPagesWithAPIKey(t *testing.T) {
	var logs bytes.Buffer
	requestCount := 0
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		query := request.URL.Query()
		if request.Method != http.MethodGet || request.URL.Scheme != "https" || request.URL.Host != "api.anthropic.com" ||
			request.URL.Path != "/v1/models" || query.Get("limit") != anthropicCatalogPageSize ||
			request.Header.Get("x-api-key") != "test-secret" || request.Header.Get("Authorization") != "" ||
			request.Header.Get("anthropic-version") != anthropicAPIVersion || request.Header.Get("anthropic-beta") != "" {
			t.Fatalf("invalid discovery request: %s %s %+v", request.Method, request.URL.String(), request.Header)
		}
		body := `{"data":[` +
			`{"type":"model","id":"claude-sonnet-5-5","display_name":"Claude Sonnet 5.5","lifecycle":"active","deprecated_at":null,"capabilities":{"image_input":{"supported":true}}},` +
			`{"type":"model","id":"claude-opus-4-5-20251101","display_name":"Claude Opus 4.5","lifecycle":"deprecated","deprecated_at":"2026-09-01T00:00:00Z"}` +
			`],"has_more":true,"first_id":"claude-sonnet-5-5","last_id":"claude-opus-4-5-20251101"}`
		switch requestCount {
		case 1:
			if query.Has("after_id") {
				t.Fatalf("first Anthropic catalog page contains a cursor: %s", request.URL.String())
			}
		case 2:
			if query.Get("after_id") != "claude-opus-4-5-20251101" {
				t.Fatalf("second Anthropic catalog page is missing its cursor: %s", request.URL.String())
			}
			body = `{"data":[` +
				`{"type":"model","id":"claude-haiku-5-5","display_name":"","capabilities":{"image_input":{"supported":false}}},` +
				`{"type":"model","id":"claude-sonnet-5-5","display_name":"Duplicate"}` +
				`],"has_more":false,"first_id":"claude-haiku-5-5","last_id":"claude-sonnet-5-5"}`
		default:
			t.Fatalf("unexpected Anthropic catalog page %d", requestCount)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{
		ProviderCode: catalog.AnthropicOfficialCode, AuthType: mgmt.AuthTypeAPIKey,
	}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || requestCount != 2 || len(models) != 2 ||
		models[0].Code != "claude-haiku-5-5" || models[0].Name != "Claude haiku 5 5" ||
		!slices.Equal(models[0].InputModalities, []string{"TEXT"}) ||
		models[1].Code != "claude-sonnet-5-5" || models[1].Name != "Claude Sonnet 5.5" ||
		!slices.Equal(models[1].InputModalities, []string{"TEXT", "IMAGE"}) ||
		!slices.Equal(models[1].OutputModalities, []string{"TEXT"}) {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	if !strings.Contains(output, `"catalog_adapter":"anthropic"`) || strings.Contains(output, "test-secret") {
		t.Fatalf("unexpected discovery log: %s", output)
	}
}

func TestDiscoverAnthropicModelsWithClaudeSubscriptionToken(t *testing.T) {
	discoverer := NewDiscoverer(nil)
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer sk-ant-oat01-test" || request.Header.Get("x-api-key") != "" ||
			request.Header.Get("anthropic-beta") != anthropicclaude.OAuthBeta ||
			request.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Fatalf("unexpected Claude subscription catalog headers: %+v", request.Header)
		}
		body := `{"data":[{"type":"model","id":"claude-opus-5-5","display_name":"Claude Opus 5.5"}],"has_more":false}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{
		ProviderCode: catalog.AnthropicOfficialCode,
		AuthType:     mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode,
	}, []byte(`{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`), nil)
	if !result.OK || len(models) != 1 || models[0].Code != "claude-opus-5-5" {
		t.Fatalf("unexpected subscription discovery: %+v %+v", models, result)
	}
}

func TestDiscovererReportsCredentialCapabilities(t *testing.T) {
	discoverer := NewDiscoverer(nil)
	for _, tt := range []struct {
		providerCode, authType, authAdapter string
		want                                bool
	}{
		{catalog.AnthropicOfficialCode, mgmt.AuthTypeAPIKey, "API_KEY", true},
		{catalog.AnthropicOfficialCode, "", "", true},
		{catalog.AnthropicOfficialCode, mgmt.AuthTypeSubscription, mgmt.AuthAdapterClaudeCode, true},
		{catalog.AnthropicOfficialCode, mgmt.AuthTypeSubscription, mgmt.AuthAdapterOpenAICodex, false},
		{catalog.OpenAIOfficialCode, mgmt.AuthTypeAPIKey, "API_KEY", true},
		{catalog.OpenAIOfficialCode, mgmt.AuthTypeSubscription, mgmt.AuthAdapterOpenAICodex, true},
		{catalog.OpenAIOfficialCode, mgmt.AuthTypeSubscription, mgmt.AuthAdapterClaudeCode, false},
		{catalog.GoogleOfficialCode, mgmt.AuthTypeSubscription, mgmt.AuthAdapterOpenAICodex, false},
		{catalog.AnthropicOfficialCode, "UNKNOWN", "", false},
		{"provider-custom", mgmt.AuthTypeAPIKey, "API_KEY", false},
	} {
		if got := discoverer.SupportsCredential(tt.providerCode, tt.authType, tt.authAdapter); got != tt.want {
			t.Errorf("SupportsCredential(%s, %s, %s) = %v, want %v", tt.providerCode, tt.authType, tt.authAdapter, got, tt.want)
		}
	}
}

func TestDiscoverRejectsUnsupportedOrInvalidSubscriptionWithoutRequest(t *testing.T) {
	for _, tt := range []struct {
		name       string
		source     mgmt.ModelDiscoverySource
		credential string
		code       string
	}{
		{"Claude subscription on OpenAI", mgmt.ModelDiscoverySource{
			ProviderCode: catalog.OpenAIOfficialCode, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterClaudeCode,
		}, `{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`, "MODEL_CATALOG_UNSUPPORTED"},
		{"Codex subscription on Google", mgmt.ModelDiscoverySource{
			ProviderCode: catalog.GoogleOfficialCode, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex,
		}, `{"access_token":"token"}`, "MODEL_CATALOG_UNSUPPORTED"},
		{"invalid Codex auth file", mgmt.ModelDiscoverySource{
			ProviderCode: catalog.OpenAIOfficialCode, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex,
		}, `{"access_token":"token"}`, "CREDENTIAL_INVALID"},
		{"invalid Claude token", mgmt.ModelDiscoverySource{
			ProviderCode: catalog.AnthropicOfficialCode, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode,
		}, `{"kind":"unknown","access_token":"token"}`, "CREDENTIAL_INVALID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			discoverer := NewDiscoverer(nil)
			discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("rejected subscription reached transport")
				return nil, nil
			})
			models, result := discoverer.Discover(context.Background(), tt.source, []byte(tt.credential), nil)
			if result.OK || result.Code != tt.code || models != nil {
				t.Fatalf("subscription accepted: %+v %+v", models, result)
			}
		})
	}
}

func TestDiscoverAnthropicRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"type":"model","id":"claude-test"}]}`,
		`{"has_more":false}`,
		`{"data":[{"type":"unknown","id":"claude-test"}],"has_more":false}`,
		`{"data":[{"type":"model","id":"bad model\n"}],"has_more":false}`,
		`{"data":[{"type":"model","id":"claude-test"}],"has_more":true}`,
		`{`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.AnthropicOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}
