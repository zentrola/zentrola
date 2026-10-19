package provider

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func newTestBase() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport}
}

func TestClientWithProxyReusesTransportForSameConfiguration(t *testing.T) {
	base := newTestBase()
	otherBase := newTestBase()
	defer CloseIdleProxyConnections(base)
	defer CloseIdleProxyConnections(otherBase)
	proxy := &catalog.OutboundProxy{URL: "http://user:password@proxy.example.com:8080", Headers: map[string]string{"X-Proxy-Token": "a"}}
	transportFor := func(base *http.Client, proxy *catalog.OutboundProxy, scope string) http.RoundTripper {
		t.Helper()
		client, cleanup, err := ClientWithProxyForScope(context.Background(), base, proxy, scope, ProxyRequestLog{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
		if err != nil {
			t.Fatal(err)
		}
		cleanup()
		return client.Transport
	}
	first := transportFor(base, proxy, catalog.NetworkScopePublic)
	tests := []struct {
		name  string
		base  *http.Client
		proxy *catalog.OutboundProxy
		scope string
		same  bool
	}{
		{"same configuration", base, &catalog.OutboundProxy{URL: proxy.URL, Headers: map[string]string{"x-proxy-token": "a"}}, catalog.NetworkScopePublic, true},
		{"different proxy password", base, &catalog.OutboundProxy{URL: "http://user:other@proxy.example.com:8080", Headers: proxy.Headers}, catalog.NetworkScopePublic, false},
		{"different proxy header", base, &catalog.OutboundProxy{URL: proxy.URL, Headers: map[string]string{"X-Proxy-Token": "b"}}, catalog.NetworkScopePublic, false},
		{"different network scope", base, proxy, catalog.NetworkScopePrivate, false},
		{"different base transport", otherBase, proxy, catalog.NetworkScopePublic, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transportFor(tt.base, tt.proxy, tt.scope); (got == first) != tt.same {
				t.Fatalf("transport reused=%v, want %v", got == first, tt.same)
			}
		})
	}
}

func TestClientWithProxyLogsTransportCreationOnce(t *testing.T) {
	base := newTestBase()
	defer CloseIdleProxyConnections(base)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	proxy := &catalog.OutboundProxy{URL: "http://user:password@proxy.example.com:8080", Headers: map[string]string{"X-Proxy-Token": "secret"}}
	for range 3 {
		if _, _, err := ClientWithProxy(context.Background(), base, proxy, ProxyRequestLog{Logger: logger, Operation: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	output := logs.String()
	if created := strings.Count(output, `"msg":"provider proxy transport created"`); created != 1 {
		t.Fatalf("transport creation logged %d times: %s", created, output)
	}
	if requests := strings.Count(output, `"level":"DEBUG","msg":"provider outbound request using proxy"`); requests != 3 {
		t.Fatalf("per-request proxy log count=%d, want 3 at DEBUG: %s", requests, output)
	}
	if strings.Contains(output, "password") || strings.Contains(output, "secret") {
		t.Fatalf("proxy credentials leaked: %s", output)
	}
}

func TestProxyTransportCacheEviction(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	cache := newProxyTransportCache(2, time.Minute, func() time.Time { return now })
	created := 0
	get := func(name string) http.RoundTripper {
		key := proxyTransportKey{networkScope: catalog.NetworkScopePublic, digest: [32]byte{name[0]}}
		transport, _ := cache.get(key, func() (http.RoundTripper, *http.Transport) {
			created++
			inner := &http.Transport{}
			return inner, inner
		})
		return transport
	}
	tests := []struct {
		name        string
		advance     time.Duration
		key         string
		wantCreated int
		wantLen     int
	}{
		{"first configuration", 0, "a", 1, 1},
		{"second configuration", 0, "b", 2, 2},
		{"cached configuration", 0, "a", 2, 2},
		{"capacity evicts least recently used", 0, "c", 3, 2},
		{"evicted configuration is recreated", 0, "b", 4, 2},
		{"idle configurations are evicted", 2 * time.Minute, "d", 5, 1},
	}
	for _, tt := range tests {
		now = now.Add(tt.advance)
		get(tt.key)
		if created != tt.wantCreated || cache.len() != tt.wantLen {
			t.Fatalf("%s: created=%d len=%d, want created=%d len=%d", tt.name, created, cache.len(), tt.wantCreated, tt.wantLen)
		}
	}
}

func TestCloseIdleProxyConnectionsRemovesBaseEntries(t *testing.T) {
	base := newTestBase()
	before := proxyTransports.len()
	for _, url := range []string{"http://proxy-a.example.com:8080", "http://proxy-b.example.com:8080"} {
		if _, _, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{URL: url}, ProxyRequestLog{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}); err != nil {
			t.Fatal(err)
		}
	}
	if got := proxyTransports.len(); got != before+2 {
		t.Fatalf("cache len=%d, want %d", got, before+2)
	}
	CloseIdleProxyConnections(base)
	if got := proxyTransports.len(); got != before {
		t.Fatalf("cache len after close=%d, want %d", got, before)
	}
}

func TestClientWithProxyReusesUpstreamConnections(t *testing.T) {
	var connections atomic.Int32
	proxyServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	proxyServer.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	proxyServer.Start()
	defer proxyServer.Close()
	base := newTestBase()
	defer CloseIdleProxyConnections(base)
	for range 3 {
		client, cleanup, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{URL: proxyServer.URL}, ProxyRequestLog{
			Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		})
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Get("http://api.example.com/v1/models")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		cleanup()
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("proxy connections=%d, want 1 reused connection", got)
	}
}
