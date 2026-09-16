// Package openai 实现 OpenAI 兼容协议的官方上游转发。
package openai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/openaicodex"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const codexSubscriptionBaseURL = "https://chatgpt.com/backend-api/codex"

type GatewayClient struct {
	client *http.Client
	logger *slog.Logger
}

func NewGatewayClient(headerTimeout time.Duration, loggers ...*slog.Logger) *GatewayClient {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DisableCompression = true
	t.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.TLSHandshakeTimeout = 10 * time.Second
	t.ResponseHeaderTimeout = headerTimeout
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &GatewayClient{client: &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, logger: logger}
}
func (c *GatewayClient) CloseIdleConnections() { c.client.CloseIdleConnections() }
func (c *GatewayClient) Open(ctx context.Context, route gw.Route, input gw.Request, credential []byte) (*gw.Response, error) {
	base := route.BaseURL
	upstreamPath := "/chat/completions"
	requestCredential := string(credential)
	accountID := ""
	if route.AuthType == "SUBSCRIPTION" {
		if route.AuthAdapter != openaicodex.AdapterCode || input.Protocol != gw.OpenAIResponsesProtocol || input.Path != "/v1/responses" {
			return nil, gw.ErrRoute
		}
		accessToken, account, expiresAt, err := openaicodex.RequestCredential(credential)
		if err != nil || expiresAt != nil && !expiresAt.After(time.Now().Add(time.Minute)) {
			return nil, gw.ErrCredential
		}
		requestCredential, accountID = accessToken, account
		base, upstreamPath = codexSubscriptionBaseURL, "/responses"
	}
	baseURL, allowed := provider.BaseURL(base)
	if !allowed {
		return nil, gw.ErrRoute
	}
	if !gw.IsOpenAIProtocol(input.Protocol) ||
		(input.Path != "/v1/chat/completions" && input.Path != "/v1/responses" && input.Path != "/v1/images/generations") || input.BetaQuery {
		return nil, gw.ErrInvalid
	}
	if requestCredential == "" || len(requestCredential) > 16<<10 {
		return nil, gw.ErrCredential
	}
	for _, b := range []byte(requestCredential) {
		if b < 33 || b > 126 {
			return nil, gw.ErrCredential
		}
	}
	if route.AuthType != "SUBSCRIPTION" && input.Protocol == gw.OpenAIResponsesProtocol {
		upstreamPath = "/responses"
	} else if route.AuthType != "SUBSCRIPTION" && input.Protocol == gw.OpenAIImagesProtocol {
		upstreamPath = "/images/generations"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+upstreamPath, bytes.NewReader(input.Body))
	if err != nil {
		return nil, gw.ErrInvalid
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Authorization", "Bearer "+requestCredential)
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
		req.Header.Set("Originator", "zentrola")
	}
	if input.RequestID != "" {
		req.Header.Set("X-Request-ID", input.RequestID)
	}
	// 客户端 Authorization / Cookie / Anthropic / SDK Header 均不复制到上游。
	client, cleanup, err := provider.ClientWithProxy(ctx, c.client, route.Proxy, provider.ProxyRequestLog{
		Logger: c.logger, Operation: "gateway_inference", ProviderID: route.ProviderID, ResourceID: route.ResourceID, Protocol: gw.OpenAIEndpoint,
	})
	if err != nil {
		return nil, gw.ErrProxy
	}
	instrumented, sentHeaders := provider.TracedClient(client)
	resp, err := instrumented.Do(req)
	if err != nil {
		requestHeaders := provider.HeadersForLog(req.Header)
		sentHeaders.Delete("Authorization")
		sentHeaders.Delete("ChatGPT-Account-Id")
		cleanup()
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, gw.ErrCancelled
		}
		failure := gw.ErrUpstream
		var ne net.Error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			failure = gw.ErrTimeout
		}
		diagnostic := provider.DiagnoseNetworkError(err)
		c.logger.WarnContext(ctx, "provider upstream request failed",
			"error_code", failure.Code,
			"request_id", input.RequestID,
			"provider_id", route.ProviderID,
			"resource_id", route.ResourceID,
			"protocol", input.Protocol,
			"failure_kind", diagnostic.Kind,
			"network_op", diagnostic.Operation,
			"network", diagnostic.Network,
			"upstream_error_type", diagnostic.ErrorType,
			"upstream_error", diagnostic.Detail,
			"upstream_headers", requestHeaders,
			"redacted", diagnostic.Redacted,
		)
		return nil, failure
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location, redacted := provider.RedirectLocationForLog(resp.Header.Get("Location"))
		requestHeaders := provider.HeadersForLog(req.Header)
		sentHeaders.Delete("Authorization")
		sentHeaders.Delete("ChatGPT-Account-Id")
		c.logger.WarnContext(ctx, "provider upstream redirect rejected",
			"error_code", "UPSTREAM_REDIRECT_REJECTED",
			"request_id", input.RequestID,
			"provider_id", route.ProviderID,
			"resource_id", route.ResourceID,
			"protocol", input.Protocol,
			"upstream_status", resp.StatusCode,
			"redirect_location", location,
			"upstream_headers", requestHeaders,
			"redacted", redacted,
		)
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
		b.sentHeaders.Delete("ChatGPT-Account-Id")
		if b.cleanup != nil {
			b.cleanup()
		}
	})
	return b.err
}
