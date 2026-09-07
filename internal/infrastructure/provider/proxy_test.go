package provider

import (
	"net/http"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestClientWithProxyIsolatedTransport(t *testing.T) {
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.Proxy = nil
	base := &http.Client{Transport: baseTransport}
	client, cleanup, err := ClientWithProxy(base, &catalog.OutboundProxy{
		URL:     "http://user:password@proxy.example.com:8080",
		Headers: map[string]string{"X-Proxy-Token": "secret"},
	})
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
}
