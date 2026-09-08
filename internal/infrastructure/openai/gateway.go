// Package openai 实现 OpenAI 兼容协议的官方上游转发。
package openai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

type GatewayClient struct{ client *http.Client }

func NewGatewayClient(headerTimeout time.Duration) *GatewayClient {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DisableCompression = true
	t.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = 10 * time.Second
	t.ResponseHeaderTimeout = headerTimeout
	return &GatewayClient{&http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *GatewayClient) CloseIdleConnections() { c.client.CloseIdleConnections() }
func (c *GatewayClient) Open(ctx context.Context, route gw.Route, input gw.Request, credential []byte) (*gw.Response, error) {
	baseURL, allowed := provider.BaseURL(route.BaseURL)
	if !allowed {
		return nil, gw.ErrRoute
	}
	if (input.Protocol != gw.OpenAIProtocol && input.Protocol != gw.OpenAIResponsesProtocol) ||
		(input.Path != "/v1/chat/completions" && input.Path != "/v1/responses") || input.BetaQuery {
		return nil, gw.ErrInvalid
	}
	if len(credential) == 0 || len(credential) > 4096 {
		return nil, gw.ErrCredential
	}
	for _, b := range credential {
		if b < 33 || b > 126 {
			return nil, gw.ErrCredential
		}
	}
	upstreamPath := "/chat/completions"
	if input.Protocol == gw.OpenAIResponsesProtocol {
		upstreamPath = "/responses"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+upstreamPath, bytes.NewReader(input.Body))
	if err != nil {
		return nil, gw.ErrInvalid
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Authorization", "Bearer "+string(credential))
	if input.RequestID != "" {
		req.Header.Set("X-Request-ID", input.RequestID)
	}
	// 客户端 Authorization / Cookie / Anthropic / SDK Header 均不复制到上游。
	client, cleanup, err := provider.ClientWithProxy(c.client, route.Proxy)
	if err != nil {
		return nil, gw.ErrProxy
	}
	instrumented, sentHeaders := provider.TracedClient(client)
	resp, err := instrumented.Do(req)
	if err != nil {
		sentHeaders.Delete("Authorization")
		cleanup()
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, gw.ErrCancelled
		}
		var ne net.Error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			return nil, gw.ErrTimeout
		}
		return nil, gw.ErrUpstream
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		cleanup()
		return nil, gw.ErrUpstream
	}
	return &gw.Response{Status: resp.StatusCode, Headers: resp.Header, Body: &responseBody{ReadCloser: resp.Body, sentHeaders: sentHeaders, cleanup: cleanup}}, nil
}

type responseBody struct {
	io.ReadCloser
	sentHeaders *provider.SentHeaders
	once        sync.Once
	err         error
	cleanup     func()
}

func (b *responseBody) Close() error {
	b.once.Do(func() {
		b.err = b.ReadCloser.Close()
		b.sentHeaders.Delete("Authorization")
		if b.cleanup != nil {
			b.cleanup()
		}
	})
	return b.err
}
