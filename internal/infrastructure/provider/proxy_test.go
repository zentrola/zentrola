package provider

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestClientWithProxyIsolatedTransport(t *testing.T) {
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
	output := logs.String()
	for _, expected := range []string{"provider outbound request using proxy", `"proxy_enabled":true`, `"operation":"test"`, `"provider_id":12`, `"protocol":"OPENAI"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("proxy log missing %q: %s", expected, output)
		}
	}
	for _, secret := range []string{"user", "password", "proxy.example.com", "secret", "X-Proxy-Token"} {
		if strings.Contains(output, secret) {
			t.Fatalf("proxy log leaked %q: %s", secret, output)
		}
	}
}

func TestEnvironmentWithProxyOverridesInheritedProxyVariables(t *testing.T) {
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
	if output := logs.String(); !strings.Contains(output, `"operation":"subscription_refresh"`) || strings.Contains(output, "password") || strings.Contains(output, "proxy.example.com") {
		t.Fatalf("unexpected proxy log: %s", output)
	}
}
