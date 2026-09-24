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
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

type GatewayClient struct {
	client        *http.Client
	privateClient *http.Client
	logger        *slog.Logger
}

func NewGatewayClient(headerTimeout time.Duration, loggers ...*slog.Logger) *GatewayClient {
	newClient := func(networkScope string) *http.Client {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DisableCompression = true
		transport.DialContext = provider.EndpointDialContext(&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}, networkScope)
		transport.TLSHandshakeTimeout = 10 * time.Second
		transport.ResponseHeaderTimeout = headerTimeout
		return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
	baseURL, allowed := provider.BaseURLForScope(route.BaseURL, route.NetworkScope)
	if !allowed {
		return nil, gw.ErrRoute
	}
	if input.Path != "/v1/messages" && input.Path != "/v1/messages/count_tokens" {
		return nil, gw.ErrInvalid
	}
	// 根据服务商能力自动过滤不支持的功能
	capabilities := catalog.GetProviderCapabilities(baseURL)
	if !capabilities.SupportsAdvisor {
		adapted, removedTools, removedBeta, err := adaptDeepSeekRequest(input)
		if err != nil {
			return nil, err
		}
		input = adapted
		if input.Development && (removedTools > 0 || removedBeta) {
			c.logger.InfoContext(ctx, "gateway compatibility applied",
				"base_url", baseURL,
				"removed_tool_type", "advisor_20260301",
				"removed_tool_count", removedTools,
				"removed_beta", removedBeta,
			)
		}
	}
	requestCredential := string(credential)
	claudeSubscription := route.AuthType == "SUBSCRIPTION"
	if claudeSubscription {
		if route.AuthAdapter != anthropicclaude.AdapterCode {
			return nil, gw.ErrCredential
		}
		var credentialErr error
		requestCredential, credentialErr = anthropicclaude.RequestCredential(credential)
		if credentialErr != nil {
			return nil, gw.ErrCredential
		}
	} else {
		if len(credential) == 0 || len(credential) > 4096 {
			return nil, gw.ErrCredential
		}
		for _, ch := range credential {
			if ch < 33 || ch > 126 {
				return nil, gw.ErrCredential
			}
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
	if claudeSubscription {
		req.Header.Set("Authorization", "Bearer "+requestCredential)
	} else {
		req.Header.Set("x-api-key", requestCredential)
	}
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
	if claudeSubscription {
		addAnthropicBeta(req.Header, anthropicclaude.OAuthBeta)
	}
	if input.RequestID != "" {
		req.Header.Set("X-Request-ID", input.RequestID)
	}
	baseClient := c.client
	if route.NetworkScope == catalog.NetworkScopePrivate {
		baseClient = c.privateClient
	}
	client, cleanup, err := provider.ClientWithProxyForScope(ctx, baseClient, route.Proxy, route.NetworkScope, provider.ProxyRequestLog{
		Logger: c.logger, Operation: "gateway_inference", ProviderID: route.ProviderID, ResourceID: route.ResourceID, Protocol: gw.AnthropicEndpoint,
	})
	if err != nil {
		return nil, gw.ErrProxy
	}
	instrumented, sentHeaders := provider.TracedClient(client)
	resp, err := instrumented.Do(req)
	if err != nil {
		requestHeaders := provider.HeadersForLog(req.Header)
		sentHeaders.Delete("x-api-key")
		sentHeaders.Delete("Authorization")
		cleanup()
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, gw.ErrCancelled
		}
		failure := gw.ErrUpstream
		// 网络请求经过服务商代理时，传输层故障无法归因于上游响应；将其
		// 明确标记为代理服务器故障，避免用户误以为服务商本身不可用。
		if route.Proxy != nil {
			failure = gw.ErrProxyServer
		}
		var netErr net.Error
		if route.Proxy == nil && (errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())) {
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
		sentHeaders.Delete("x-api-key")
		sentHeaders.Delete("Authorization")
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
	return &gw.Response{Status: resp.StatusCode, Headers: resp.Header, Body: &gatewayBody{ReadCloser: resp.Body, sentHeaders: sentHeaders, cleanup: cleanup}}, nil
}

// 请求 Header 仅保留到 HTTP 事务结束；Body 关闭后再修改，避免与 transport 并发使用。
type gatewayBody struct {
	io.ReadCloser
	sentHeaders *provider.SentHeaders
	once        sync.Once
	err         error
	cleanup     func()
}

func (b *gatewayBody) Close() error {
	b.once.Do(func() {
		b.err = b.ReadCloser.Close()
		b.sentHeaders.Delete("x-api-key")
		b.sentHeaders.Delete("Authorization")
		if b.cleanup != nil {
			b.cleanup()
		}
	})
	return b.err
}

func addAnthropicBeta(header http.Header, beta string) {
	values := strings.Join(header.Values("anthropic-beta"), ",")
	for _, value := range strings.Split(values, ",") {
		if strings.TrimSpace(value) == beta {
			return
		}
	}
	if strings.TrimSpace(values) == "" {
		header.Set("anthropic-beta", beta)
		return
	}
	header.Set("anthropic-beta", values+","+beta)
}
