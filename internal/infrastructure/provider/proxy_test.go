package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	transport := client.Transport.(*publicTargetTransport).transport
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
		entry["proxy_url"] != "http://******:******@proxy.example.com:8080" || entry["proxy_auth"] != true ||
		entry["proxy_redacted"] != true || entry["operation"] != "test" ||
		entry["provider_id"] != float64(12) || entry["protocol"] != "OPENAI" {
		t.Fatalf("unexpected proxy log: %v", entry)
	}
	headers, ok := entry["proxy_headers"].(map[string]any)
	if !ok || len(headers) != 1 || headers["X-Proxy-Token"] != "******" {
		t.Fatalf("unexpected proxy headers in log: %v", entry["proxy_headers"])
	}
	if strings.Contains(logs.String(), "password") || strings.Contains(logs.String(), "secret") {
		t.Fatalf("proxy credentials leaked: %s", logs.String())
	}
}

func TestClientWithSOCKS5Proxy(t *testing.T) {
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.Proxy = nil
	base := &http.Client{Transport: baseTransport}
	client, cleanup, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{
		URL: "socks5h://user:password@proxy.example.com:1080",
	}, ProxyRequestLog{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	transport := client.Transport.(*publicTargetTransport).transport
	request, _ := http.NewRequest(http.MethodGet, "https://api.example.com", nil)
	proxyURL, err := transport.Proxy(request)
	if err != nil || proxyURL.String() != "socks5h://user:password@proxy.example.com:1080" {
		t.Fatalf("unexpected SOCKS5 proxy URL: %v, %v", proxyURL, err)
	}
}

func TestProxyAuthenticationRejectionIsClassified(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxyServer.Close()
	proxyURL := strings.Replace(proxyServer.URL, "://", "://user:wrong-password@", 1)
	base := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	var logs bytes.Buffer
	client, cleanup, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{URL: proxyURL}, ProxyRequestLog{
		Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	response, err := client.Get("https://api.example.com/v1")
	if response != nil {
		response.Body.Close()
	}
	if !errors.Is(err, ErrProxyAuthentication) {
		t.Fatalf("proxy 407 was not classified: response=%v error=%v", response, err)
	}
	if strings.Contains(logs.String(), "wrong-password") {
		t.Fatalf("proxy password leaked: %s", logs.String())
	}
}

func TestHTTPProxy407RemainsAnAmbiguousResponse(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxyServer.Close()
	base := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	client, cleanup, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{
		URL: proxyServer.URL,
	}, ProxyRequestLog{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	response, err := client.Get("http://api.example.com/v1")
	if err != nil {
		t.Fatalf("ambiguous HTTP 407 became a proxy error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("HTTP status = %d, want 407", response.StatusCode)
	}
}

func TestUpstreamHTTPS407IsNotProxyAuthentication(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer upstream.Close()
	transport := upstream.Client().Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := proxyRoundTrip(transport, request, "http")
	if err != nil {
		t.Fatalf("upstream 407 became a transport error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("upstream status = %d, want 407", response.StatusCode)
	}
}

func TestSOCKS5AuthenticationRejectionIsClassified(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	served := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			served <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		var greeting [2]byte
		if _, err = io.ReadFull(conn, greeting[:]); err == nil {
			_, err = io.CopyN(io.Discard, conn, int64(greeting[1]))
		}
		if err == nil {
			_, err = conn.Write([]byte{5, 2})
		}
		var auth [2]byte
		if err == nil {
			_, err = io.ReadFull(conn, auth[:])
		}
		if err == nil {
			_, err = io.CopyN(io.Discard, conn, int64(auth[1]))
		}
		var passwordLength [1]byte
		if err == nil {
			_, err = io.ReadFull(conn, passwordLength[:])
		}
		if err == nil {
			_, err = io.CopyN(io.Discard, conn, int64(passwordLength[0]))
		}
		if err == nil {
			_, err = conn.Write([]byte{1, 1})
		}
		served <- err
	}()
	base := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 3 * time.Second}
	client, cleanup, err := ClientWithProxy(context.Background(), base, &catalog.OutboundProxy{
		URL: "socks5://user:wrong-password@" + listener.Addr().String(),
	}, ProxyRequestLog{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	response, err := client.Get("https://api.example.com/v1")
	if response != nil {
		response.Body.Close()
	}
	if !errors.Is(err, ErrProxyAuthentication) {
		t.Fatalf("SOCKS5 rejection was not classified: response=%v error=%v", response, err)
	}
	if err := <-served; err != nil {
		t.Fatalf("SOCKS5 test proxy failed: %v", err)
	}
}

func TestProxyTransportRejectsPrivateProviderTarget(t *testing.T) {
	transport := &publicTargetTransport{transport: http.DefaultTransport.(*http.Transport).Clone()}
	request, _ := http.NewRequest(http.MethodGet, "https://127.0.0.1/private", nil)
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("private provider target was accepted through proxy transport")
	}
}

func TestPrivateProxyTransportAllowsPrivateButRejectsMetadataTarget(t *testing.T) {
	if err := endpointLiteralError("192.168.1.20", catalog.NetworkScopePrivate); err != nil {
		t.Fatalf("private provider target rejected: %v", err)
	}
	if err := endpointLiteralError("169.254.169.254", catalog.NetworkScopePrivate); err == nil || !strings.Contains(err.Error(), "disallowed address") {
		t.Fatalf("metadata target was not rejected: %v", err)
	}
}

func TestClientWithProxyRejectsInvalidNetworkScopeWithoutProxy(t *testing.T) {
	base := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	if _, _, err := ClientWithProxyForScope(context.Background(), base, nil, "INVALID", ProxyRequestLog{}); err == nil {
		t.Fatal("invalid network scope accepted without proxy")
	}
}

func TestNonPublicLiteralError(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.5", "169.254.169.254", "::1"} {
		if err := nonPublicLiteralError(host); err == nil {
			t.Fatalf("non-public literal accepted: %s", host)
		}
	}
	for _, host := range []string{"8.8.8.8", "1.1.1.1", "chatgpt.com", "api.deepseek.com"} {
		if err := nonPublicLiteralError(host); err != nil {
			t.Fatalf("public literal or domain rejected: %s, err=%v", host, err)
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
	if output := logs.String(); !strings.Contains(output, `"operation":"subscription_refresh"`) ||
		!strings.Contains(output, `"proxy":true`) || !strings.Contains(output, `"proxy_url":"https://******:******@proxy.example.com:8443"`) ||
		!strings.Contains(output, `"proxy_auth":true`) || !strings.Contains(output, `"proxy_headers":{}`) || !strings.Contains(output, `"proxy_redacted":true`) ||
		strings.Contains(output, "password") || strings.Contains(output, "user") {
		t.Fatalf("unexpected proxy log: %s", output)
	}
}

func TestEnvironmentWithSOCKS5ProxySetsAllProxyVariables(t *testing.T) {
	got, err := EnvironmentWithProxy(context.Background(), []string{
		"PATH=test", "HTTP_PROXY=http://old.example", "HTTPS_PROXY=http://old.example", "ALL_PROXY=socks5://old.example", "NO_PROXY=localhost",
	}, &catalog.OutboundProxy{URL: "socks5://user:password@proxy.example.com:1080"}, ProxyRequestLog{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "PATH=test") || strings.Count(joined, "socks5://user:password@proxy.example.com:1080") != 3 ||
		strings.Contains(joined, "old.example") || strings.Contains(strings.ToUpper(joined), "NO_PROXY=") {
		t.Fatalf("unexpected SOCKS5 proxy environment: %q", got)
	}
}

func TestProductionProxyLogMasksHeaderValues(t *testing.T) {
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
