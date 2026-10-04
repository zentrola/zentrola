// Package openai 实现 OpenAI 兼容协议的官方上游转发。
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/openaicodex"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const codexSubscriptionBaseURL = "https://chatgpt.com/backend-api/codex"

type GatewayClient struct {
	client        *http.Client
	privateClient *http.Client
	logger        *slog.Logger
}

func NewGatewayClient(headerTimeout time.Duration, loggers ...*slog.Logger) *GatewayClient {
	newClient := func(networkScope string) *http.Client {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil
		t.DisableCompression = true
		t.DialContext = provider.EndpointDialContext(&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}, networkScope)
		t.TLSHandshakeTimeout = 10 * time.Second
		t.ResponseHeaderTimeout = headerTimeout
		return &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &GatewayClient{client: newClient(catalog.NetworkScopePublic), privateClient: newClient(catalog.NetworkScopePrivate), logger: logger}
}
func (c *GatewayClient) CloseIdleConnections() {
	c.client.CloseIdleConnections()
	c.privateClient.CloseIdleConnections()
}
func (c *GatewayClient) Open(ctx context.Context, route gw.Route, input gw.Request, credential []byte) (*gw.Response, error) {
	base := route.BaseURL
	upstreamPath := "/chat/completions"
	requestCredential := string(credential)
	accountID := ""
	if route.AuthType == "SUBSCRIPTION" {
		if route.AuthAdapter != openaicodex.AdapterCode || input.Protocol != gw.OpenAIResponsesProtocol || input.Path != "/v1/responses" {
			return nil, gw.ErrRoute
		}
		body, err := codexSubscriptionResponsesBody(input.Body)
		if err != nil {
			return nil, err
		}
		input.Body = body
		accessToken, account, expiresAt, err := openaicodex.RequestCredential(credential)
		if err != nil || expiresAt != nil && !expiresAt.After(time.Now().Add(time.Minute)) {
			return nil, gw.ErrCredential
		}
		requestCredential, accountID = accessToken, account
		base, upstreamPath = codexSubscriptionBaseURL, "/responses"
		route.NetworkScope = catalog.NetworkScopePublic
	}
	baseURL, allowed := provider.BaseURLForScope(base, route.NetworkScope)
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
	baseClient := c.client
	if route.NetworkScope == catalog.NetworkScopePrivate {
		baseClient = c.privateClient
	}
	client, cleanup, err := provider.ClientWithProxyForScope(ctx, baseClient, route.Proxy, route.NetworkScope, provider.ProxyRequestLog{
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
		// 网络请求经过服务商代理时，传输层故障无法归因于上游响应；将其
		// 明确标记为代理服务器故障，避免用户误以为服务商本身不可用。
		if route.Proxy != nil {
			failure = gw.ErrProxyServer
			if errors.Is(err, provider.ErrProxyAuthentication) {
				failure = gw.ErrProxyAuth
			}
		}
		var ne net.Error
		if route.Proxy == nil && (errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())) {
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

// codexSubscriptionResponsesBody 收紧 ChatGPT Codex 订阅端点的请求形态。
// 该端点只提供 SSE；网关不把流式事件重新聚合成非流式 Responses JSON。
func codexSubscriptionResponsesBody(body []byte) ([]byte, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, gw.ErrInvalid
	}
	var stream bool
	if raw, ok := request["stream"]; !ok || json.Unmarshal(raw, &stream) != nil || !stream {
		return nil, gw.ErrInvalid
	}
	request["stream"] = json.RawMessage("true")
	request["store"] = json.RawMessage("false")

	if raw, ok := request["input"]; ok {
		trimmed := bytes.TrimSpace(raw)
		switch {
		case len(trimmed) > 0 && trimmed[0] == '"':
			var text string
			if err := json.Unmarshal(trimmed, &text); err != nil {
				return nil, gw.ErrInvalid
			}
			messages := []codexInputMessage{{
				Type: "message",
				Role: "user",
				Content: []codexInputContent{{
					Type: "input_text",
					Text: text,
				}},
			}}
			encoded, err := json.Marshal(messages)
			if err != nil {
				return nil, gw.ErrInvalid
			}
			request["input"] = encoded
		case len(trimmed) == 0 || trimmed[0] != '[':
			return nil, gw.ErrInvalid
		}
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, gw.ErrInvalid
	}
	return encoded, nil
}

type codexInputMessage struct {
	Type    string              `json:"type"`
	Role    string              `json:"role"`
	Content []codexInputContent `json:"content"`
}

type codexInputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
