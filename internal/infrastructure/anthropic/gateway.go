package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
	client              *http.Client
	privateClient       *http.Client
	logger              *slog.Logger
	observer            SubscriptionResponseObserver
	claudeCodeUserAgent string
}

// SubscriptionResponseObserver 旁路观测 Claude 订阅的原始上游响应头（如统一限额），
// 不得阻塞或影响转发。兼容协议转换会移除这些头，因此必须在上游客户端处观测。
type SubscriptionResponseObserver interface {
	ObserveResponse(ctx context.Context, route gw.Route, status int, header http.Header)
}

// ObserveSubscriptionResponses 在启动阶段设置订阅响应观测器，必须在处理请求前调用。
func (c *GatewayClient) ObserveSubscriptionResponses(observer SubscriptionResponseObserver) {
	c.observer = observer
}

// UseClaudeCodeVersion 设置 Claude 订阅请求声明的客户端版本，必须在处理请求前调用；
// 空值或无效版本使用内置默认版本。所有订阅请求统一使用该版本，不透传客户端自带的 User-Agent。
func (c *GatewayClient) UseClaudeCodeVersion(version string) {
	c.claudeCodeUserAgent = anthropicclaude.ClaudeCodeUserAgent(version)
}

func NewGatewayClient(headerTimeout time.Duration, loggers ...*slog.Logger) *GatewayClient {
	clients := provider.NewUpstreamClients(provider.UpstreamClientOptions{
		DialTimeout: 10 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: headerTimeout,
		DisableCompression: true,
	})
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &GatewayClient{
		client: clients.Public, privateClient: clients.Private, logger: logger,
		claudeCodeUserAgent: anthropicclaude.ClaudeCodeUserAgent(""),
	}
}

func (c *GatewayClient) CloseIdleConnections() {
	provider.UpstreamClients{Public: c.client, Private: c.privateClient}.CloseIdleConnections()
}
func (c *GatewayClient) Open(ctx context.Context, route gw.Route, input gw.Request, credential []byte) (*gw.Response, error) {
	baseURL, allowed := provider.BaseURLForScope(route.BaseURL, route.NetworkScope)
	if !allowed {
		return nil, gw.ErrRoute
	}
	if input.Path != "/v1/messages" && input.Path != "/v1/messages/count_tokens" {
		return nil, gw.ErrInvalid
	}
	networkScope := route.NetworkScope
	claudeSubscription := route.AuthType == "SUBSCRIPTION"
	if claudeSubscription {
		baseURL, networkScope = anthropicclaude.APIBaseURL, catalog.NetworkScopePublic
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
	if claudeSubscription {
		if route.AuthAdapter != anthropicclaude.AdapterCode {
			return nil, gw.ErrCredential
		}
		var credentialErr error
		requestCredential, credentialErr = anthropicclaude.RequestCredential(credential)
		if credentialErr != nil {
			return nil, gw.ErrCredential
		}
		// 订阅 Token 只接受 Claude Code 身份的请求，缺少系统提示时上游返回 429；
		// count_tokens 同样补上，使计数与实际推理请求一致。
		body, err := withClaudeCodeSystem(input.Body)
		if err != nil {
			return nil, err
		}
		input.Body = body
	} else if !provider.ValidCredential(credential) {
		return nil, gw.ErrCredential
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
		addAnthropicBeta(req.Header, anthropicclaude.ClaudeCodeBeta)
		addAnthropicBeta(req.Header, anthropicclaude.OAuthBeta)
		req.Header.Set("User-Agent", c.claudeCodeUserAgent)
		req.Header.Set("X-App", "cli")
	}
	if input.RequestID != "" {
		req.Header.Set("X-Request-ID", input.RequestID)
	}
	baseClient := c.client
	if networkScope == catalog.NetworkScopePrivate {
		baseClient = c.privateClient
	}
	client, cleanup, err := provider.ClientWithProxyForScope(ctx, baseClient, route.Proxy, networkScope, provider.ProxyRequestLog{
		Logger: c.logger, Operation: "gateway_inference", ProviderID: route.ProviderID, ResourceID: route.ResourceID, Protocol: gw.AnthropicEndpoint,
	})
	if err != nil {
		return nil, gw.ErrProxy
	}
	logFields := provider.UpstreamRequestLog{RequestID: input.RequestID, ProviderID: route.ProviderID, ResourceID: route.ResourceID, Protocol: input.Protocol}
	instrumented, sentHeaders := provider.TracedClient(client)
	resp, err := instrumented.Do(req)
	if err != nil {
		requestHeaders := provider.HeadersForLog(req.Header)
		sentHeaders.Delete(gatewaySensitiveHeaders...)
		cleanup()
		failure := gatewayTransportFailure(provider.TransportErrorCode(ctx, err, route.Proxy != nil))
		if failure == gw.ErrCancelled {
			return nil, failure
		}
		provider.LogTransportFailure(ctx, c.logger, failure.Code, err, requestHeaders, logFields)
		return nil, failure
	}
	if provider.IsRedirect(resp.StatusCode) {
		requestHeaders := provider.HeadersForLog(req.Header)
		sentHeaders.Delete(gatewaySensitiveHeaders...)
		provider.LogRedirectRejected(ctx, c.logger, resp.StatusCode, resp.Header.Get("Location"), requestHeaders, logFields)
		resp.Body.Close()
		cleanup()
		return nil, gw.ErrUpstream
	}
	if claudeSubscription && c.observer != nil {
		c.observer.ObserveResponse(ctx, route, resp.StatusCode, resp.Header)
	}
	body := resp.Body
	if claudeSubscription && resp.StatusCode >= http.StatusBadRequest {
		body = &rejectedSubscriptionBody{ReadCloser: resp.Body, log: func(prefix []byte) {
			logClaudeSubscriptionRejection(context.WithoutCancel(ctx), c.logger, "gateway_inference", resp.StatusCode, prefix, c.claudeCodeUserAgent,
				slog.String("request_id", input.RequestID), slog.Int64("provider_id", route.ProviderID), slog.Int64("resource_id", route.ResourceID))
		}}
	}
	return &gw.Response{Status: resp.StatusCode, Headers: resp.Header, Body: &gatewayBody{ReadCloser: body, sentHeaders: sentHeaders, cleanup: cleanup}}, nil
}

// gatewaySensitiveHeaders 是响应结束后需从已发送请求中清除的凭据 Header。
var gatewaySensitiveHeaders = []string{"x-api-key", "Authorization"}

// gatewayTransportFailure 把传输错误分类码映射为网关失败类型。
func gatewayTransportFailure(code string) *gw.Failure {
	switch code {
	case provider.TransportCancelled:
		return gw.ErrCancelled
	case provider.TransportProxyAuth:
		return gw.ErrProxyAuth
	case provider.TransportProxyServer:
		return gw.ErrProxyServer
	case provider.TransportTimeout:
		return gw.ErrTimeout
	default:
		return gw.ErrUpstream
	}
}

// rejectedSubscriptionBody 在不额外读取上游的前提下，保存调用方读过的错误响应前缀，关闭时交给 log 记录。
// 故障转移会先读取错误正文前缀做分类，并可能在超时后从另一协程关闭正文，因此读写共享缓冲需要加锁。
type rejectedSubscriptionBody struct {
	io.ReadCloser
	log    func([]byte)
	mu     sync.Mutex
	prefix []byte
	once   sync.Once
}

const rejectedSubscriptionBodyLogLimit = 8 << 10

func (b *rejectedSubscriptionBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.mu.Lock()
		if remaining := rejectedSubscriptionBodyLogLimit - len(b.prefix); remaining > 0 {
			b.prefix = append(b.prefix, p[:min(n, remaining)]...)
		}
		b.mu.Unlock()
	}
	return n, err
}

func (b *rejectedSubscriptionBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		b.mu.Lock()
		prefix := append([]byte(nil), b.prefix...)
		b.mu.Unlock()
		b.log(prefix)
	})
	return err
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
		b.sentHeaders.Delete(gatewaySensitiveHeaders...)
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

// withClaudeCodeSystem 确保 system 首段为 Claude Code 系统提示。已声明该身份的请求原样返回；
// 否则在最前面插入文本块，并保留客户端原有系统提示及请求中其他 JSON 原文（如 cache_control）。
func withClaudeCodeSystem(body []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(body))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, gw.ErrInvalid
	}
	objectStart := int(d.InputOffset())
	systemStart, systemEnd, fields := 0, 0, 0
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, gw.ErrInvalid
		}
		key, _ := token.(string)
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return nil, gw.ErrInvalid
		}
		fields++
		if key == "system" {
			if systemEnd != 0 {
				return nil, gw.ErrInvalid
			}
			systemEnd = int(d.InputOffset())
			systemStart = systemEnd - len(raw)
		}
	}
	block, _ := json.Marshal(claudeCodeTextBlock{Type: "text", Text: anthropicclaude.ClaudeCodeSystemPrompt})
	if systemEnd == 0 {
		insert := append([]byte(`"system":[`), block...)
		insert = append(insert, ']')
		if fields > 0 {
			insert = append(insert, ',')
		}
		return splice(body, objectStart, objectStart, insert), nil
	}
	raw := body[systemStart:systemEnd]
	switch raw[0] {
	case '"':
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, gw.ErrInvalid
		}
		if strings.HasPrefix(text, anthropicclaude.ClaudeCodeSystemPrompt) {
			return body, nil
		}
		blocks := []claudeCodeTextBlock{{Type: "text", Text: anthropicclaude.ClaudeCodeSystemPrompt}}
		if text != "" {
			blocks = append(blocks, claudeCodeTextBlock{Type: "text", Text: text})
		}
		encoded, _ := json.Marshal(blocks)
		return splice(body, systemStart, systemEnd, encoded), nil
	case '[':
		var blocks []json.RawMessage
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return nil, gw.ErrInvalid
		}
		if len(blocks) > 0 {
			var first claudeCodeTextBlock
			if json.Unmarshal(blocks[0], &first) == nil && first.Type == "text" && strings.HasPrefix(first.Text, anthropicclaude.ClaudeCodeSystemPrompt) {
				return body, nil
			}
			block = append(block, ',')
		}
		// 在 '[' 之后插入，客户端原有文本块按原文保留。
		return splice(body, systemStart+1, systemStart+1, block), nil
	case 'n':
		encoded := append([]byte{'['}, block...)
		return splice(body, systemStart, systemEnd, append(encoded, ']')), nil
	default:
		// 非法的 system 类型交由上游返回协议错误。
		return body, nil
	}
}

type claudeCodeTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func splice(body []byte, start, end int, insert []byte) []byte {
	out := make([]byte, 0, len(body)-(end-start)+len(insert))
	out = append(out, body[:start]...)
	out = append(out, insert...)
	return append(out, body[end:]...)
}
