package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestClientWithProxyIsolatedTransport(t *testing.T) {
	ConfigureLogEnvironment("dev")
	t.Cleanup(func() { ConfigureLogEnvironment("prod") })
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.Proxy = nil
	base := &http.Client{Transport: baseTransport}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	client, cleanup, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{
		URL:     "http://user:password@proxy.example.com:8080",
		Headers: map[string]string{"X-Proxy-Token": "secret"},
	}, ProxyRequestLog{Logger: logger, Operation: "test", ProviderID: 12, Protocol: "OPENAI"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	transport := client.Transport.(*http.Transport)
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com", nil)
	proxyURL, err := transport.Proxy(request)
	if err != nil || proxyURL.String() != "http://user:password@proxy.example.com:8080" {
		t.Fatalf("unexpected proxy URL: %v, %v", proxyURL, err)
	}
	if transport.ProxyConnectHeader.Get("X-Proxy-Token") != "secret" {
		t.Fatal("proxy header not configured")
	}
	if baseTransport.Proxy != nil || baseTransport.ProxyConnectHeader != nil {
		t.Fatal("base transport was mutated")
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode proxy log: %v", err)
	}
	if entry["msg"] != "provider outbound request using proxy" || entry["proxy"] != true ||
		entry["proxy_url"] != "http://user:password@proxy.example.com:8080" || entry["proxy_auth"] != true ||
		entry["proxy_redacted"] != false || entry["operation"] != "test" ||
		entry["provider_id"] != float64(12) || entry["protocol"] != "OPENAI" {
		t.Fatalf("unexpected proxy log: %v", entry)
	}
	headers, ok := entry["proxy_headers"].(map[string]any)
	if !ok || len(headers) != 1 || headers["X-Proxy-Token"] != "secret" {
		t.Fatalf("unexpected proxy headers in log: %v", entry["proxy_headers"])
	}
}

func TestEnvironmentWithProxyOverridesInheritedProxyVariables(t *testing.T) {
	ConfigureLogEnvironment("prod")
	t.Cleanup(func() { ConfigureLogEnvironment("prod") })
	var logs bytes.Buffer
	got, err := EnvironmentWithProxy(context.Background(), []string{
		"PATH=test", "http_proxy=http://old.example", "HTTPS_PROXY=http://old.example", "NO_PROXY=chatgpt.com", "ALL_PROXY=socks5://old.example",
	}, &catalog.OutboundProxy{URL: "https://user:password@proxy.example.com:8443"}, ProxyRequestLog{
		Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Operation: "subscription_refresh", ProviderCode: "openai-official",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "PATH=test") || strings.Count(joined, "https://user:password@proxy.example.com:8443") != 2 ||
		strings.Contains(joined, "old.example") || strings.Contains(strings.ToUpper(joined), "NO_PROXY=") || strings.Contains(strings.ToUpper(joined), "ALL_PROXY=") {
		t.Fatalf("unexpected proxy environment: %q", got)
	}
	if output := logs.String(); !strings.Contains(output, `"operation":"subscription_refresh"`) ||
		!strings.Contains(output, `"proxy":true`) || !strings.Contains(output, `"proxy_url":"https://******:******@proxy.example.com:8443"`) ||
		!strings.Contains(output, `"proxy_auth":true`) || !strings.Contains(output, `"proxy_headers":{}`) || !strings.Contains(output, `"proxy_redacted":true`) ||
		strings.Contains(output, "password") || strings.Contains(output, "user") {
		t.Fatalf("unexpected proxy log: %s", output)
	}
}

func TestProductionProxyLogMasksHeaderValues(t *testing.T) {
	ConfigureLogEnvironment("prod")
	t.Cleanup(func() { ConfigureLogEnvironment("prod") })
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	proxyURL, err := parseProxyURL(&catalog.OutboundProxy{URL: "http://proxy-user:proxy-password@proxy.example.com:8080"})
	if err != nil {
		t.Fatal(err)
	}
	logProxyRequest(context.Background(), proxyURL, map[string]string{"X-Proxy-Token": "proxy-secret"}, ProxyRequestLog{Logger: logger})

	output := logs.String()
	for _, expected := range []string{
		`"proxy_url":"http://******:******@proxy.example.com:8080"`,
		`"proxy_headers":{"X-Proxy-Token":"******"}`,
		`"proxy_redacted":true`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("proxy log missing %q: %s", expected, output)
		}
	}
	for _, secret := range []string{"proxy-user", "proxy-password", "proxy-secret"} {
		if strings.Contains(output, secret) {
			t.Fatalf("production proxy log leaked %q: %s", secret, output)
		}
	}
}
