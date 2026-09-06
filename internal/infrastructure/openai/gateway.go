// Package openai 实现 OpenAI 兼容协议的官方上游转发。
package openai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
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
	// 本轮只启用有真实兼容协议验证的 DeepSeek 官方地址。
	if strings.TrimSuffix(route.BaseURL, "/") != "https://api.deepseek.com" {
		return nil, gw.ErrRoute
	}
	if input.Protocol != gw.OpenAIProtocol || input.Path != "/v1/chat/completions" || input.BetaQuery {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.deepseek.com/chat/completions", bytes.NewReader(input.Body))
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
	resp, err := c.client.Do(req)
	if err != nil {
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
		return nil, gw.ErrUpstream
	}
	return &gw.Response{Status: resp.StatusCode, Headers: resp.Header, Body: &responseBody{ReadCloser: resp.Body, headers: req.Header}}, nil
}

type responseBody struct {
	io.ReadCloser
	headers http.Header
	once    sync.Once
	err     error
}

func (b *responseBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close(); b.headers.Del("Authorization") })
	return b.err
}
