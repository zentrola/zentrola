package anthropic

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

type GatewayClient struct{ client *http.Client }

func NewGatewayClient(headerTimeout time.Duration) *GatewayClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = headerTimeout
	return &GatewayClient{client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *GatewayClient) CloseIdleConnections() { c.client.CloseIdleConnections() }
func (c *GatewayClient) Open(ctx context.Context, route gw.Route, input gw.Request, credential []byte) (*gw.Response, error) {
	baseURL, allowed := allowedBaseURL(route.BaseURL)
	if !allowed {
		return nil, gw.ErrRoute
	}
	if input.Path != "/v1/messages" && input.Path != "/v1/messages/count_tokens" {
		return nil, gw.ErrInvalid
	}
	if baseURL == "https://api.deepseek.com/anthropic" {
		adapted, removedTools, removedBeta, err := adaptDeepSeekRequest(input)
		if err != nil {
			return nil, err
		}
		input = adapted
		if input.Development && (removedTools > 0 || removedBeta) {
			slog.InfoContext(ctx, "gateway compatibility applied",
				"provider", "deepseek-official",
				"removed_tool_type", "advisor_20260301",
				"removed_tool_count", removedTools,
				"removed_beta", removedBeta,
			)
		}
	}
	if len(credential) == 0 || len(credential) > 4096 {
		return nil, gw.ErrCredential
	}
	for _, ch := range credential {
		if ch < 33 || ch > 126 {
			return nil, gw.ErrCredential
		}
	}
	url := baseURL + input.Path
	if input.BetaQuery {
		url += "?beta=true"
	}
	// 清空 GetBody，使 net/http 不会把 POST 当作可重放请求自动重试。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(input.Body))
	if err != nil {
		return nil, gw.ErrInvalid
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("x-api-key", string(credential))
	for name, values := range input.ProtocolHeaders {
		if !strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	version := input.Version
	if version == "" {
		version = "2023-06-01"
	}
	if req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", version)
	}
	if input.Beta != "" && req.Header.Get("anthropic-beta") == "" {
		req.Header.Set("anthropic-beta", input.Beta)
	}
	if input.RequestID != "" {
		req.Header.Set("X-Request-ID", input.RequestID)
	}
	client, cleanup, err := provider.ClientWithProxy(c.client, route.Proxy)
	if err != nil {
		return nil, gw.ErrProxy
	}
	resp, err := client.Do(req)
	if err != nil {
		cleanup()
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, gw.ErrCancelled
		}
		var netErr net.Error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return nil, gw.ErrTimeout
		}
		return nil, gw.ErrUpstream
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		cleanup()
		return nil, gw.ErrUpstream
	}
	return &gw.Response{Status: resp.StatusCode, Headers: resp.Header, Body: &gatewayBody{ReadCloser: resp.Body, headers: req.Header, cleanup: cleanup}}, nil
}

// 请求 Header 仅保留到 HTTP 事务结束；Body 关闭后再修改，避免与 transport 并发使用。
type gatewayBody struct {
	io.ReadCloser
	headers http.Header
	once    sync.Once
	err     error
	cleanup func()
}

func (b *gatewayBody) Close() error {
	b.once.Do(func() {
		b.err = b.ReadCloser.Close()
		b.headers.Del("x-api-key")
		if b.cleanup != nil {
			b.cleanup()
		}
	})
	return b.err
}
