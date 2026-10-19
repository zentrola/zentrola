package provider

import (
	"bytes"
	"context"
	"errors"
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
		{"stream idle timeout", context.Background(), ErrStreamIdleTimeout, false, TransportTimeout},
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

func TestTrackedBodyIdleTimeoutCancelsStalledStream(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for range 3 {
			_, _ = io.WriteString(w, "data: {}\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var cleaned atomic.Int32
	body := NewTrackedBody(response.Body, TrackedBodyOptions{
		IdleTimeout: 150 * time.Millisecond, Cancel: cancel, Cleanup: func() { cleaned.Add(1) },
	})
	started := time.Now()
	data, err := io.ReadAll(body)
	if !errors.Is(err, ErrStreamIdleTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read error=%v, want stream idle timeout", err)
	}
	if string(data) != strings.Repeat("data: {}\n\n", 3) {
		t.Fatalf("data before timeout was lost: %q", data)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("idle timeout took %s", elapsed)
	}
	if ctx.Err() == nil {
		t.Fatal("request context was not cancelled")
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, ErrStreamIdleTimeout) {
		t.Fatalf("read after timeout error=%v", err)
	}
	_ = body.Close()
	_ = body.Close()
	if cleaned.Load() != 1 {
		t.Fatalf("cleanup calls=%d, want 1", cleaned.Load())
	}
}

type slowReader struct {
	chunks []string
	delay  time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

func TestTrackedBody(t *testing.T) {
	sentinel := errors.New("gateway timeout")
	tests := []struct {
		name      string
		reader    io.Reader
		idle      time.Duration
		idleError error
		wantData  string
		wantErr   error
	}{
		{"steady stream within timeout", &slowReader{chunks: []string{"a", "b", "c"}, delay: 30 * time.Millisecond}, 200 * time.Millisecond, nil, "abc", nil},
		{"idle timeout disabled", &slowReader{chunks: []string{"a", "b"}, delay: 60 * time.Millisecond}, 0, nil, "ab", nil},
		{"custom idle error", &slowReader{chunks: []string{"a", "b"}, delay: 300 * time.Millisecond}, 100 * time.Millisecond, sentinel, "", sentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cancelled atomic.Bool
			body := NewTrackedBody(io.NopCloser(tt.reader), TrackedBodyOptions{
				IdleTimeout: tt.idle, IdleError: tt.idleError, Cancel: func() { cancelled.Store(true) },
			})
			defer body.Close()
			data, err := io.ReadAll(body)
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err=%v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (string(data) != tt.wantData || cancelled.Load()) {
				t.Fatalf("data=%q cancelled=%v", data, cancelled.Load())
			}
			if tt.wantErr != nil && !cancelled.Load() {
				t.Fatal("idle timeout did not cancel the request")
			}
		})
	}
}

func TestTrackedBodyClearsSentCredentialsOnClose(t *testing.T) {
	headers := http.Header{"Authorization": {"Bearer secret"}, "X-Api-Key": {"secret"}, "Content-Type": {"application/json"}}
	sent := &SentHeaders{headers: headers}
	body := NewTrackedBody(io.NopCloser(strings.NewReader("ok")), TrackedBodyOptions{SentHeaders: sent, SensitiveHeaders: []string{"Authorization", "x-api-key"}})
	if headers.Get("Authorization") == "" {
		t.Fatal("credentials cleared before close")
	}
	_ = body.Close()
	if headers.Get("Authorization") != "" || headers.Get("X-Api-Key") != "" || headers.Get("Content-Type") == "" {
		t.Fatalf("unexpected headers after close: %v", headers)
	}
}
