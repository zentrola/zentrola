package provider

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestNewUpstreamClientConfiguresPoolAndRedirects(t *testing.T) {
	clients := NewUpstreamClients(UpstreamClientOptions{
		DialTimeout: time.Second, TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
		Timeout: 4 * time.Second, DisableCompression: true,
	})
	defer clients.CloseIdleConnections()
	for _, client := range []*http.Client{clients.Public, clients.Private} {
		transport := client.Transport.(*http.Transport)
		if transport.Proxy != nil || !transport.DisableCompression || transport.TLSHandshakeTimeout != 2*time.Second ||
			transport.ResponseHeaderTimeout != 3*time.Second || client.Timeout != 4*time.Second ||
			transport.MaxIdleConns != upstreamMaxIdleConns || transport.MaxIdleConnsPerHost != upstreamMaxIdleConnsPerHost ||
			transport.IdleConnTimeout != upstreamIdleConnTimeout {
			t.Fatalf("unexpected upstream transport: %+v client timeout=%s", transport, client.Timeout)
		}
		if err := client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
			t.Fatalf("redirects are not rejected: %v", err)
		}
	}
	if clients.ForScope(catalog.NetworkScopePrivate) != clients.Private || clients.ForScope(catalog.NetworkScopePublic) != clients.Public {
		t.Fatal("clients are not selected by network scope")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestTransportErrorCode(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, expire := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expire()
	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		proxied bool
		want    string
	}{
		{"cancelled request", cancelled, errors.New("transport failed"), true, TransportCancelled},
		{"proxy authentication", context.Background(), ErrProxyAuthentication, true, TransportProxyAuth},
		{"proxy transport failure", context.Background(), timeoutError{}, true, TransportProxyServer},
		{"direct deadline", expired, errors.New("transport failed"), false, TransportTimeout},
		{"direct network timeout", context.Background(), &net.OpError{Op: "read", Err: timeoutError{}}, false, TransportTimeout},
		{"direct failure", context.Background(), errors.New("connection refused"), false, TransportUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TransportErrorCode(tt.ctx, tt.err, tt.proxied); got != tt.want {
				t.Fatalf("code=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidCredential(t *testing.T) {
	tests := []struct {
		name       string
		credential string
		want       bool
	}{
		{"visible ASCII", "sk-test_ABC.123~!", true},
		{"maximum length", strings.Repeat("a", MaxCredentialBytes), true},
		{"empty", "", false},
		{"too long", strings.Repeat("a", MaxCredentialBytes+1), false},
		{"space", "sk test", false},
		{"header injection", "sk-test\r\nX-Injected: 1", false},
		{"delete control", "sk-test\x7f", false},
		{"non ASCII", "sk-测试", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidCredential(tt.credential); got != tt.want {
				t.Fatalf("string credential valid=%v, want %v", got, tt.want)
			}
			if got := ValidCredential([]byte(tt.credential)); got != tt.want {
				t.Fatalf("byte credential valid=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportFailureLogsAreRedacted(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	headers := http.Header{"Authorization": {"Bearer secret-token"}, "Content-Type": {"application/json"}}
	fields := UpstreamRequestLog{RequestID: "request-1", ProviderID: 1, ResourceID: 2, Protocol: "OPENAI_CHAT"}
	LogTransportFailure(context.Background(), logger, TransportUnavailable, errors.New("dial tcp: credential=network-secret"), headers, fields)
	LogRedirectRejected(context.Background(), logger, http.StatusTemporaryRedirect, "https://user:pass@example.com/path?token=redirect-secret", headers, fields)
	output := logs.String()
	for _, expected := range []string{`"msg":"provider upstream request failed"`, `"error_code":"UPSTREAM_UNAVAILABLE"`,
		`"msg":"provider upstream redirect rejected"`, `"redirect_location":"https://example.com"`, `"request_id":"request-1"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("log missing %s: %s", expected, output)
		}
	}
	for _, secret := range []string{"secret-token", "network-secret", "redirect-secret", "pass@"} {
		if strings.Contains(output, secret) {
			t.Fatalf("log leaked %q: %s", secret, output)
		}
	}
}
