package http

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zentrola/zentrola/internal/application/health"
	"github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

func TestSwaggerEnvironmentAndAssets(t *testing.T) {
	for _, environment := range []string{"dev", "test", "prod", "", "unknown"} {
		t.Run(environment, func(t *testing.T) {
			router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(),
				config.CORS{Enabled: true, Origins: []string{"http://localhost:5173"}}, time.Second, environment, &SecurityHandlers{})
			enabled := environment == "dev" || environment == "test"
			for path, content := range map[string]string{
				"/swagger/index.html":                      "SwaggerUIBundle",
				"/swagger/doc.json":                        `"swagger": "2.0"`,
				"/swagger/swagger-ui.css":                  ".swagger-ui",
				"/swagger/swagger-ui-bundle.js":            "SwaggerUIBundle",
				"/swagger/swagger-ui-standalone-preset.js": "SwaggerUIStandalonePreset",
			} {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				want := http.StatusNotFound
				if enabled {
					want = http.StatusOK
				}
				if rec.Code != want {
					t.Fatalf("%s returned %d, want %d", path, rec.Code, want)
				}
				if enabled && !strings.Contains(rec.Body.String(), content) {
					t.Fatalf("%s missing expected content", path)
				}
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger", nil))
			if enabled && (rec.Code != http.StatusFound || rec.Header().Get("Location") != "/swagger/index.html") {
				t.Fatal("Swagger shortcut must redirect to the UI")
			}
			if !enabled && rec.Code != http.StatusNotFound {
				t.Fatal("disabled Swagger shortcut must be absent")
			}
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatal("Swagger must not bypass API authentication")
			}
		})
	}
}

func TestSwaggerCoversRoutesAndResolvesSchemas(t *testing.T) {
	var doc struct {
		Swagger     string                                `json:"swagger"`
		Host        string                                `json:"host"`
		BasePath    string                                `json:"basePath"`
		Paths       map[string]map[string]json.RawMessage `json:"paths"`
		Definitions map[string]json.RawMessage            `json:"definitions"`
		Security    map[string]json.RawMessage            `json:"securityDefinitions"`
	}
	if err := json.Unmarshal(swaggerJSON, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Swagger != "2.0" || doc.Host != "" || doc.BasePath != "/" {
		t.Fatal("Swagger must use the current origin and root base path")
	}
	for _, name := range []string{"AdminBearer", "GatewayBearer", "GatewayKey"} {
		if _, ok := doc.Security[name]; !ok {
			t.Fatalf("missing security definition %s", name)
		}
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(), config.CORS{}, time.Second, "prod",
		&SecurityHandlers{Management: &management.Service{}, Usage: &usage.QueryService{}, UsageWriter: &usage.Writer{}})
	expected := map[string]bool{}
	err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// 页面和构建资产不是 API，不加入 Swagger 接口覆盖清单。
		if strings.Contains(route, "*") || route == "/" || route == "/favicon.svg" {
			return nil
		}
		expected[strings.ToLower(method)+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Gateway 在通配路由内部按协议分发，显式列出支持的接口。
	for _, route := range []string{"post /anthropic/v1/messages", "post /anthropic/v1/messages/count_tokens", "post /v1/chat/completions", "get /v1/models"} {
		expected[route] = true
	}
	for path, operations := range doc.Paths {
		for method := range operations {
			key := method + " " + path
			if !expected[key] {
				t.Errorf("documented route does not exist: %s", key)
			}
			delete(expected, key)
		}
	}
	for key := range expected {
		t.Errorf("route has no Swagger documentation: %s", key)
	}

	var tree any
	if err := json.Unmarshal(swaggerJSON, &tree); err != nil {
		t.Fatal(err)
	}
	var checkRefs func(any)
	checkRefs = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				if _, ok := doc.Definitions[strings.TrimPrefix(ref, "#/definitions/")]; !ok {
					t.Errorf("unresolved schema reference: %s", ref)
				}
			}
			for _, child := range value {
				checkRefs(child)
			}
		case []any:
			for _, child := range value {
				checkRefs(child)
			}
		}
	}
	checkRefs(tree)
	for name, raw := range doc.Definitions {
		if strings.HasSuffix(name, ".Member") || strings.HasSuffix(name, ".CreateResourceRequest") || strings.HasSuffix(name, ".Operation") {
			var schema struct {
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"id", "providerId"} {
				if property, ok := schema.Properties[field]; ok && property.Type != "string" {
					t.Errorf("%s.%s must be a string", name, field)
				}
			}
			if strings.HasSuffix(name, ".Operation") && schema.Properties["before"].Type != "object" {
				t.Fatal("audit JSON must not be documented as bytes")
			}
		}
	}
}
