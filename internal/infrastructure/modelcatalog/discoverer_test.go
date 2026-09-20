package modelcatalog

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
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

func TestDiscoverZhipuModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://open.bigmodel.cn/api/paas/v4/models" || request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"object":"list","data":[{"id":"glm-4.7","object":"model","created":1766332800,"owned_by":"z-ai"},{"id":"embedding-3","object":"model","owned_by":"organization-owner"},{"id":"glm-4.7","object":"model","created":1766332800,"owned_by":"z-ai"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.ZhipuOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 2 || models[0].Code != "embedding-3" || models[0].Name != "Embedding 3" || models[1].Code != "glm-4.7" || models[1].Name != "GLM-4.7" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"zhipu-official"`, `"catalog_adapter":"zhipu"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestDiscoverOpenAIModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://api.openai.com/v1/models" || request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"object":"list","data":[{"id":"text-embedding-3-small","object":"model","created":1705948997,"owned_by":"system"},{"id":"gpt-4o","object":"model","created":1715367049,"owned_by":"openai"},{"id":"gpt-4o","object":"model","created":1715367049,"owned_by":"openai"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.OpenAIOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 2 || models[0].Code != "gpt-4o" || models[0].Name != "GPT-4o" || models[1].Code != "text-embedding-3-small" || models[1].Name != "Text embedding 3 small" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"openai-official"`, `"catalog_adapter":"openai"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestDiscoverGoogleModels(t *testing.T) {
	var logs bytes.Buffer
	requestCount := 0
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodGet || request.URL.Scheme != "https" ||
			request.URL.Host != "generativelanguage.googleapis.com" || request.URL.Path != "/v1beta/models" ||
			request.URL.Query().Get("pageSize") != googleCatalogPageSize ||
			request.Header.Get("X-Goog-Api-Key") != "test-secret" || request.Header.Get("Authorization") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"nextPageToken":"page-token-2","models":[{"name":"models/text-embedding-004","displayName":"Text Embedding 004","supportedGenerationMethods":["embedContent"]},{"name":"models/gemini-2.5-pro","displayName":"Gemini 2.5 Pro","supportedGenerationMethods":["generateContent","countTokens"]}]}`
		if requestCount == 1 && request.URL.Query().Get("pageToken") != "" {
			t.Fatalf("first Google catalog page contains a page token: %s", request.URL.String())
		}
		if requestCount == 2 {
			if request.URL.Query().Get("pageToken") != "page-token-2" {
				t.Fatalf("second Google catalog page is missing its page token: %s", request.URL.String())
			}
			body = `{"models":[{"name":"models/gemini-2.5-flash","displayName":"","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-2.5-pro","displayName":"Duplicate","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-3-flash-preview","displayName":"Preview","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-3-pro-image","displayName":"Image","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-flash-latest","displayName":"Latest","supportedGenerationMethods":["generateContent"]},{"name":"models/antigravity-preview-09-2026","displayName":"Antigravity","supportedGenerationMethods":["generateContent"]}]}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.GoogleOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || requestCount != 2 || len(models) != 2 ||
		models[0].Code != "gemini-2.5-flash" || models[0].Name != "Gemini 2.5 flash" ||
		models[1].Code != "gemini-2.5-pro" || models[1].Name != "Gemini 2.5 Pro" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"google-gemini-official"`, `"catalog_adapter":"google"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestGoogleStableGeneralModelFilter(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{code: "gemini-2.5-flash", want: true},
		{code: "gemini-2.5-flash-001", want: true},
		{code: "gemini-2.5-pro", want: true},
		{code: "gemini-3-flash-preview", want: false},
		{code: "gemini-2.0-flash-thinking-exp", want: false},
		{code: "gemini-flash-latest", want: false},
		{code: "gemini-3-pro-image", want: false},
		{code: "gemini-2.5-flash-preview-tts", want: false},
		{code: "gemini-2.5-computer-use", want: false},
		{code: "gemini-3.5-transcribe", want: false},
		{code: "antigravity-preview-09-2026", want: false},
		{code: "gemma-3-27b-it", want: false},
	}
	for _, test := range tests {
		if got := isGoogleStableGeneralModel(test.code); got != test.want {
			t.Errorf("isGoogleStableGeneralModel(%q) = %t, want %t", test.code, got, test.want)
		}
	}
}

func TestDiscoverMoonshotModels(t *testing.T) {
	var logs bytes.Buffer
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != "https://api.moonshot.cn/v1/models" || request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"object":"list","data":[{"id":"moonshot-v1-128k","object":"model","created":1709149142,"owned_by":"moonshot"},{"id":"kimi-k2.5","object":"model","owned_by":"moonshot"},{"id":"kimi-k2.5","object":"model","owned_by":"moonshot"}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.KimiOfficialCode}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 2 || models[0].Code != "kimi-k2.5" || models[0].Name != "Kimi K2.5" || models[1].Code != "moonshot-v1-128k" || models[1].Name != "Moonshot V1 128k" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"kimi-official"`, `"catalog_adapter":"moonshot"`, `"upstream_status":200`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestDiscoverQwenModelsAcrossWorkspaceCatalogPages(t *testing.T) {
	var logs bytes.Buffer
	requestCount := 0
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestCount++
		if request.Method != http.MethodGet || request.URL.Scheme != "https" ||
			request.URL.Host != "workspace-01.cn-beijing.maas.aliyuncs.com" ||
			request.URL.Path != "/api/v1/models" || request.Header.Get("Authorization") != "Bearer test-secret" ||
			request.Header.Get("x-api-key") != "" || request.URL.Query().Get("providers") != "qwen" ||
			request.URL.Query().Get("capabilities") != "TG" || request.URL.Query().Get("features") != "model-experience" ||
			request.URL.Query().Get("supports") != "inference" ||
			request.URL.Query().Get("language") != "en-US" || request.URL.Query().Get("page_size") != "100" ||
			request.URL.Query().Get("page_no") != strconv.Itoa(requestCount) {
			t.Fatalf("invalid discovery request: %s %s", request.Method, request.URL.String())
		}
		body := `{"success":true,"output":{"total":101,"page_no":1,"page_size":100,"models":[{"model":"qwen3-max","name":"Qwen3-Max","inference_metadata":{"request_modality":["Text"],"response_modality":["Text"]}},{"model":"deepseek-v4","name":"DeepSeek V4","inference_metadata":{"request_modality":["Text"],"response_modality":["Text"]}},{"model":"qwen-image-max","name":"Qwen-Image-Max","inference_metadata":{"request_modality":["Text","Image"],"response_modality":["Image"]}}]}}`
		if requestCount == 2 {
			body = `{"success":true,"output":{"total":101,"page_no":2,"page_size":100,"models":[{"model":"qwen-audio-turbo","name":"Qwen-Audio-Turbo","inference_metadata":{"request_modality":["Audio","Text","Audio","Unknown"],"response_modality":["Text"]}},{"model":"qwen3-max","name":"Qwen3-Max","inference_metadata":{"request_modality":["Text"],"response_modality":["Text"]}}]}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{
		ProviderCode: catalog.QwenOfficialCode,
		Endpoints: []mgmt.ProviderEndpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://workspace-01.cn-beijing.maas.aliyuncs.com/apps/anthropic"},
		},
	}, []byte("test-secret"), nil)
	if !result.OK || result.Code != "OK" || requestCount != 2 || len(models) != 3 ||
		models[0].Code != "qwen-audio-turbo" || !slices.Equal(models[0].InputModalities, []string{"AUDIO", "TEXT"}) ||
		models[1].Code != "qwen-image-max" || !slices.Equal(models[1].InputModalities, []string{"TEXT", "IMAGE"}) ||
		!slices.Equal(models[1].OutputModalities, []string{"IMAGE"}) || models[2].Code != "qwen3-max" {
		t.Fatalf("unexpected discovery: %+v %+v", models, result)
	}
	output := logs.String()
	for _, expected := range []string{"official model catalog response", `"provider_code":"qwen-official"`, `"catalog_adapter":"qwen"`, `"catalog_page":2`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("discovery response log missing %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "test-secret") {
		t.Fatalf("discovery response log leaked credential: %s", output)
	}
}

func TestQwenCatalogRequestUsesConfiguredRegionalHost(t *testing.T) {
	request, err := (qwenAdapter{}).Request(mgmt.ModelDiscoverySource{
		Endpoints: []mgmt.ProviderEndpoint{{
			ProtocolType: "OPENAI",
			BaseURL:      "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		}},
	}, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(request.URL)
	if err != nil || parsed.Host != "dashscope-intl.aliyuncs.com" || parsed.Path != "/api/v1/models" ||
		parsed.Query().Get("providers") != "qwen" || parsed.Query().Get("page_no") != "3" ||
		parsed.Query().Get("page_size") != "100" || parsed.Query().Get("language") != "en-US" ||
		parsed.Query().Get("capabilities") != "TG" || parsed.Query().Get("features") != "model-experience" ||
		parsed.Query().Get("supports") != "inference" {
		t.Fatalf("unexpected Qwen catalog request: %+v %v", request, err)
	}
	if got := qwenDisplayName("qwen-image-max", ""); got != "Qwen-Image-Max" {
		t.Fatalf("fallback display name = %q", got)
	}
}

func TestDiscovererReportsRegisteredProviderCapabilities(t *testing.T) {
	discoverer := NewDiscoverer(nil)
	if !discoverer.Supports(catalog.OpenAIOfficialCode) {
		t.Fatal("OpenAI model catalog adapter should be reported as supported")
	}
	if !discoverer.Supports(catalog.GoogleOfficialCode) {
		t.Fatal("Google model catalog adapter should be reported as supported")
	}
	if !discoverer.Supports(catalog.DeepSeekOfficialCode) {
		t.Fatal("DeepSeek model catalog adapter should be reported as supported")
	}
	if !discoverer.Supports(catalog.ZhipuOfficialCode) {
		t.Fatal("Zhipu model catalog adapter should be reported as supported")
	}
	if !discoverer.Supports(catalog.KimiOfficialCode) {
		t.Fatal("Moonshot model catalog adapter should be reported as supported")
	}
	if !discoverer.Supports(catalog.QwenOfficialCode) {
		t.Fatal("Qwen model catalog adapter should be reported as supported")
	}
	if !discoverer.Supports(catalog.XAIOfficialCode) {
		t.Fatal("xAI model catalog adapter should be reported as supported")
	}
	if discoverer.Supports("minimax-official") || discoverer.Supports("doubao-official") {
		t.Fatal("removed model catalog adapters should not be reported as supported")
	}
	if discoverer.Supports("provider-custom") {
		t.Fatal("custom provider should not be reported as supported")
	}
}

func TestDiscoverQwenRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"output":{"total":0,"page_no":1,"page_size":100,"models":[]}}`,
		`{"success":true,"output":{"total":0,"page_no":1,"page_size":100}}`,
		`{"success":true,"output":{"total":1001,"page_no":1,"page_size":100,"models":[]}}`,
		`{"success":true,"output":{"total":1,"page_no":2,"page_size":100,"models":[]}}`,
		`{"success":true,"output":{"total":1,"page_no":1,"page_size":20,"models":[]}}`,
		`{"success":true,"output":{"total":1,"page_no":1,"page_size":100,"models":[{"model":"qwen-bad\n","name":"Bad"}]}}`,
		`{"success":true,"output":{"total":101,"page_no":1,"page_size":100,"models":[]}}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{
			ProviderCode: catalog.QwenOfficialCode,
			Endpoints:    []mgmt.ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}},
		}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}

func TestDiscoverQwenRejectsMissingOrUnsafeProviderEndpoint(t *testing.T) {
	for _, endpoints := range [][]mgmt.ProviderEndpoint{
		nil,
		{{ProtocolType: "OPENAI", BaseURL: "http://dashscope.aliyuncs.com/compatible-mode/v1"}},
		{{ProtocolType: "OPENAI", BaseURL: "https://127.0.0.1/compatible-mode/v1"}},
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid Qwen endpoint reached transport")
			return nil, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{
			ProviderCode: catalog.QwenOfficialCode,
			Endpoints:    endpoints,
		}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_URL_REJECTED" || result.OK || models != nil {
			t.Fatalf("invalid endpoint accepted: %+v %+v", endpoints, result)
		}
	}
}

func TestDiscoverMoonshotRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"object":"list","data":[{"id":"bad model\n","object":"model","owned_by":"moonshot"}]}`,
		`{"object":"list","data":[{"id":"kimi-k2.5","object":"unknown","owned_by":"moonshot"}]}`,
		`{"object":"list","data":[{"id":"kimi-k2.5","object":"model","owned_by":"other"}]}`,
		`{"object":"list"}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.KimiOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}

func TestDiscoverZhipuRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"object":"list","data":[{"id":"bad model\n","object":"model","owned_by":"zai"}]}`,
		`{"object":"list","data":[{"id":"glm-4.7","object":"unknown","owned_by":"zai"}]}`,
		`{"object":"list","data":[{"id":"glm-4.7","object":"unexpected","owned_by":"system"}]}`,
		`{"object":"list"}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.ZhipuOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}

func TestDiscoverRejectsUnsupportedProviderWithoutRequest(t *testing.T) {
	discoverer := NewDiscoverer(nil)
	discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported provider reached transport")
		return nil, nil
	})
	models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: "provider-custom"}, []byte("test-secret"), nil)
	if result.OK || result.Code != "MODEL_CATALOG_UNSUPPORTED" || models != nil {
		t.Fatalf("unsupported provider accepted: %+v %+v", models, result)
	}
}

func TestDiscoverOpenAIRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"object":"list","data":[{"id":"bad model\n","object":"model","owned_by":"openai"}]}`,
		`{"object":"list","data":[{"id":"gpt-4o","object":"unknown","owned_by":"openai"}]}`,
		`{"object":"list","data":[{"id":"gpt-4o","object":"model"}]}`,
		`{"object":"list"}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.OpenAIOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
	}
}

func TestDiscoverGoogleRejectsInvalidCatalog(t *testing.T) {
	for _, body := range []string{
		`{"nextPageToken":"bad\n","models":[]}`,
		`{"models":[{"name":"gemini-2.5-pro","supportedGenerationMethods":["generateContent"]}]}`,
		`{"models":[{"name":"models/gemini-2.5-pro/bad","supportedGenerationMethods":["generateContent"]}]}`,
		`{"models":[{"name":"models/bad model\n","supportedGenerationMethods":["generateContent"]}]}`,
		`{}`,
	} {
		discoverer := NewDiscoverer(nil)
		discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		models, result := discoverer.Discover(context.Background(), mgmt.ModelDiscoverySource{ProviderCode: catalog.GoogleOfficialCode}, []byte("test-secret"), nil)
		if result.Code != "UPSTREAM_INVALID_RESPONSE" || result.OK || models != nil {
			t.Fatalf("invalid catalog accepted: %q %+v %+v", body, models, result)
		}
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
