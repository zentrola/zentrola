// Package anthropic 提供 Anthropic 与 OpenAI 兼容上游的端到端推理检查。
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
	"github.com/zentrola/zentrola/internal/infrastructure/openaicodex"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const connectionProbeMaxOutputTokens = 5

// quotaProbeMaxOutputTokens 是刷新额度时的最小输出 Token 数；额度只从响应头读取，回复内容会被丢弃。
const quotaProbeMaxOutputTokens = 1

type connectionProbe struct {
	URL            string
	Body           []byte
	ResponseFormat inferenceProbeFormat
	APIKeyHeader   string
}

// connectionProbeAdapter 为连接探测定义统一扩展点。标准实现按协议构造和校验探测，
// 有兼容差异的服务商只覆盖自己的探测行为，网络和安全策略仍由 ConnectionTester 统一处理。
type connectionProbeAdapter interface {
	Build(mgmt.ConnectionTarget, string) (connectionProbe, bool)
	ValidResponse([]byte, connectionProbe) bool
}

type standardConnectionProbeAdapter struct{}

func (standardConnectionProbeAdapter) Build(target mgmt.ConnectionTarget, base string) (connectionProbe, bool) {
	switch target.Protocol {
	case "OPENAI":
		return connectionProbe{
			URL:            strings.TrimSuffix(base, "/") + "/chat/completions",
			Body:           openAIChatProbeBody(target.UpstreamModelCode),
			ResponseFormat: openAIChatInferenceProbe,
		}, true
	case "ANTHROPIC":
		return connectionProbe{
			URL:            strings.TrimSuffix(base, "/") + "/v1/messages",
			Body:           anthropicProbeBody(target.UpstreamModelCode, "", connectionProbeMaxOutputTokens),
			ResponseFormat: anthropicInferenceProbe,
		}, true
	default:
		return connectionProbe{}, false
	}
}

func (standardConnectionProbeAdapter) ValidResponse(data []byte, probe connectionProbe) bool {
	return validInferenceResponse(data, probe.ResponseFormat)
}

type ConnectionTester struct {
	client              *http.Client
	privateClient       *http.Client
	standardAdapter     connectionProbeAdapter
	providerAdapters    map[string]connectionProbeAdapter
	logger              *slog.Logger
	claudeCodeUserAgent string
}

func NewConnectionTester(loggers ...*slog.Logger) *ConnectionTester {
	clients := provider.NewUpstreamClients(provider.UpstreamClientOptions{
		DialTimeout: 5 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, Timeout: 15 * time.Second,
	})
	standardAdapter := standardConnectionProbeAdapter{}
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &ConnectionTester{
		client:              clients.Public,
		privateClient:       clients.Private,
		standardAdapter:     standardAdapter,
		logger:              logger,
		claudeCodeUserAgent: anthropicclaude.ClaudeCodeUserAgent(""),
		providerAdapters: map[string]connectionProbeAdapter{
			catalog.DeepSeekOfficialCode: deepSeekConnectionProbeAdapter{standard: standardAdapter},
			catalog.GoogleOfficialCode:   googleConnectionProbeAdapter{},
		},
	}
}

// UseClaudeCodeVersion 设置 Claude 订阅探测声明的客户端版本，须在开始处理请求前调用；
// 空值或无效版本使用内置默认版本。
func (t *ConnectionTester) UseClaudeCodeVersion(version string) {
	t.claudeCodeUserAgent = anthropicclaude.ClaudeCodeUserAgent(version)
}

func (t *ConnectionTester) probeAdapter(providerCode string) connectionProbeAdapter {
	if adapter := t.providerAdapters[strings.TrimSpace(providerCode)]; adapter != nil {
		return adapter
	}
	return t.standardAdapter
}

func (t *ConnectionTester) Test(ctx context.Context, target mgmt.ConnectionTarget, credential []byte, proxy *catalog.OutboundProxy) (result mgmt.ConnectionResult) {
	started := time.Now()
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()
	if strings.TrimSpace(target.UpstreamModelCode) == "" {
		result.Code = "PROVIDER_MODEL_MAPPING_REQUIRED"
		return
	}
	base, allowed := provider.BaseURLForScope(target.BaseURL, target.NetworkScope)
	if !allowed {
		result.Code = "UPSTREAM_URL_REJECTED"
		return
	}
	if (target.AuthType != mgmt.AuthTypeSubscription && !provider.ValidCredential(credential)) ||
		(target.AuthType == mgmt.AuthTypeSubscription && (len(credential) == 0 || len(credential) > 64<<10)) {
		result.Code = "CREDENTIAL_INVALID"
		return
	}

	requestCredential := string(credential)
	accountID := ""
	oauthBearer := false
	probeAdapter := t.probeAdapter(target.ProviderCode)
	probe, ok := probeAdapter.Build(target, base)
	if target.QuotaProbe && (target.AuthType != mgmt.AuthTypeSubscription || target.AuthAdapter != anthropicclaude.AdapterCode) {
		result.Code = "SUBSCRIPTION_ADAPTER_UNAVAILABLE"
		return
	}
	if target.AuthType == mgmt.AuthTypeSubscription {
		switch target.AuthAdapter {
		case openaicodex.AdapterCode:
			if target.Protocol != "OPENAI" {
				result.Code = "SUBSCRIPTION_ADAPTER_UNAVAILABLE"
				return
			}
			accessToken, account, expiresAt, err := openaicodex.RequestCredential(credential)
			if err != nil || expiresAt != nil && !expiresAt.After(time.Now().Add(time.Minute)) {
				result.Code = "UPSTREAM_AUTH_FAILED"
				return
			}
			requestCredential, accountID = accessToken, account
			target.NetworkScope = catalog.NetworkScopePublic
			probeAdapter = t.standardAdapter
			probe = connectionProbe{
				URL:            "https://chatgpt.com/backend-api/codex/responses",
				Body:           openAIResponsesProbeBody(target.UpstreamModelCode),
				ResponseFormat: openAIResponsesInferenceProbe,
			}
			ok = true
		case anthropicclaude.AdapterCode:
			if target.Protocol != "ANTHROPIC" {
				result.Code = "SUBSCRIPTION_ADAPTER_UNAVAILABLE"
				return
			}
			accessToken, err := anthropicclaude.RequestCredential(credential)
			if err != nil {
				result.Code = "UPSTREAM_AUTH_FAILED"
				return
			}
			requestCredential, oauthBearer = accessToken, true
			target.NetworkScope = catalog.NetworkScopePublic
			probeAdapter = t.standardAdapter
			maxTokens := connectionProbeMaxOutputTokens
			if target.QuotaProbe {
				maxTokens = quotaProbeMaxOutputTokens
			}
			probe = connectionProbe{
				URL:            anthropicclaude.APIBaseURL + "/v1/messages",
				Body:           anthropicProbeBody(target.UpstreamModelCode, anthropicclaude.ClaudeCodeSystemPrompt, maxTokens),
				ResponseFormat: anthropicInferenceProbe,
			}
			ok = true
		default:
			result.Code = "SUBSCRIPTION_ADAPTER_UNAVAILABLE"
			return
		}
	}
	if !ok {
		result.Code = "UPSTREAM_URL_REJECTED"
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, probe.URL, bytes.NewReader(probe.Body))
	if err != nil {
		result.Code = "UPSTREAM_UNAVAILABLE"
		return
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	if probe.APIKeyHeader != "" {
		req.Header.Set(probe.APIKeyHeader, requestCredential)
	} else if target.Protocol == "OPENAI" || oauthBearer {
		req.Header.Set("Authorization", "Bearer "+requestCredential)
		if accountID != "" {
			req.Header.Set("ChatGPT-Account-Id", accountID)
			req.Header.Set("Originator", "zentrola")
		}
		if oauthBearer {
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("anthropic-beta", anthropicclaude.ClaudeCodeBeta+","+anthropicclaude.OAuthBeta)
			req.Header.Set("User-Agent", t.claudeCodeUserAgent)
			req.Header.Set("X-App", "cli")
		}
	} else {
		req.Header.Set("x-api-key", requestCredential)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	defer func() {
		req.Header.Del("Authorization")
		req.Header.Del("x-api-key")
		req.Header.Del("ChatGPT-Account-Id")
		if probe.APIKeyHeader != "" {
			req.Header.Del(probe.APIKeyHeader)
		}
	}()

	baseClient := t.client
	if target.NetworkScope == catalog.NetworkScopePrivate {
		baseClient = t.privateClient
	}
	client, cleanup, err := provider.ClientWithProxyForScope(ctx, baseClient, proxy, target.NetworkScope, provider.ProxyRequestLog{
		Operation: "provider_connection_test", Protocol: target.Protocol,
	})
	if err != nil {
		result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
		return
	}
	defer cleanup()
	resp, err := client.Do(req)
	if err != nil {
		result.Code = provider.TransportErrorCode(ctx, err, proxy != nil)
		return
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if oauthBearer {
		// 限流拒绝同样携带统一限额头，失败响应也要记录，以便额度耗尽时停止路由。
		result.ObservedQuotas = anthropicclaude.RateLimitQuotas(resp.Header, time.Now())
	}
	if target.QuotaProbe && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		// 额度探测只关心响应头，不读取或校验模型回复；丢弃有界正文以便复用连接。
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		result.OK = true
		result.Code = "OK"
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if readErr != nil {
		result.Code = provider.TransportErrorCode(ctx, readErr, proxy != nil)
		return
	}
	if len(data) > 64<<10 {
		result.Code = "UPSTREAM_INVALID_RESPONSE"
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		result.Code = inferenceStatusCode(resp.StatusCode, data)
		if oauthBearer {
			logClaudeSubscriptionRejection(ctx, t.logger, "provider_connection_test", resp.StatusCode, data, t.claudeCodeUserAgent, slog.String("result_code", result.Code))
		}
		return
	}
	if !probeAdapter.ValidResponse(data, probe) {
		result.Code = "UPSTREAM_INVALID_RESPONSE"
		return
	}
	result.OK = true
	result.Code = "OK"
	if oauthBearer && result.ObservedQuotas == nil {
		result.ObservedQuotas = anthropicclaude.AvailableQuotas(time.Now())
	}
	return
}

type inferenceProbeFormat uint8

const (
	openAIChatInferenceProbe inferenceProbeFormat = iota
	anthropicInferenceProbe
	openAIResponsesInferenceProbe
)

type probeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func openAIChatProbeBody(model string) []byte {
	body, _ := json.Marshal(struct {
		Model       string         `json:"model"`
		Messages    []probeMessage `json:"messages"`
		MaxTokens   int            `json:"max_tokens"`
		Temperature float64        `json:"temperature"`
		Stream      bool           `json:"stream"`
	}{Model: model, Messages: []probeMessage{{Role: "user", Content: "Reply only with OK."}}, MaxTokens: connectionProbeMaxOutputTokens})
	return body
}

func openAIResponsesProbeBody(model string) []byte {
	body, _ := json.Marshal(struct {
		Model           string `json:"model"`
		Input           string `json:"input"`
		MaxOutputTokens int    `json:"max_output_tokens"`
		Stream          bool   `json:"stream"`
	}{Model: model, Input: "Reply only with OK.", MaxOutputTokens: connectionProbeMaxOutputTokens})
	return body
}

type probeTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicProbeBody 在 system 非空时附加系统提示，供需要声明客户端身份的订阅凭据使用。
func anthropicProbeBody(model, system string, maxTokens int) []byte {
	var systemBlocks []probeTextBlock
	if system != "" {
		systemBlocks = []probeTextBlock{{Type: "text", Text: system}}
	}
	body, _ := json.Marshal(struct {
		Model     string           `json:"model"`
		MaxTokens int              `json:"max_tokens"`
		Stream    bool             `json:"stream"`
		System    []probeTextBlock `json:"system,omitempty"`
		Messages  []probeMessage   `json:"messages"`
	}{Model: model, MaxTokens: maxTokens, System: systemBlocks, Messages: []probeMessage{{Role: "user", Content: "Hi, how are you?"}}})
	return body
}

func validInferenceResponse(data []byte, format inferenceProbeFormat) bool {
	switch format {
	case anthropicInferenceProbe:
		var payload struct {
			Type    string `json:"type"`
			Content []struct {
				Type      string `json:"type"`
				Text      string `json:"text"`
				Signature string `json:"signature"`
				Data      string `json:"data"`
			} `json:"content"`
		}
		if json.Unmarshal(data, &payload) != nil || payload.Type != "message" {
			return false
		}
		// 默认开启思考的模型可能把探测的少量输出 Token 全部用于思考，带签名的思考块同样证明推理成功。
		for _, content := range payload.Content {
			if strings.TrimSpace(content.Text) != "" ||
				content.Type == "thinking" && content.Signature != "" ||
				content.Type == "redacted_thinking" && content.Data != "" {
				return true
			}
		}
		return false
	case openAIResponsesInferenceProbe:
		var payload struct {
			ID     string `json:"id"`
			Object string `json:"object"`
			Output []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if json.Unmarshal(data, &payload) != nil || payload.ID == "" || payload.Object != "response" {
			return false
		}
		for _, output := range payload.Output {
			for _, content := range output.Content {
				if strings.TrimSpace(content.Text) != "" {
					return true
				}
			}
		}
		return false
	default:
		var payload struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &payload) != nil {
			return false
		}
		for _, choice := range payload.Choices {
			if strings.TrimSpace(choice.Message.Content) != "" {
				return true
			}
		}
		return false
	}
}

func inferenceStatusCode(status int, body []byte) string {
	lower := strings.ToLower(string(body))
	if status == http.StatusPaymentRequired || containsProbeText(lower,
		"insufficient_balance", "insufficient balance", "credit balance", "payment required",
		"billing_error", "billing error", "subscription expired", "subscription has expired",
		"plan expired", "subscription inactive", "no active subscription") {
		return "UPSTREAM_BILLING_BLOCKED"
	}
	if containsProbeText(lower, "account suspended", "account_suspended", "account disabled", "account_disabled") {
		return "UPSTREAM_ACCOUNT_SUSPENDED"
	}
	switch status {
	case http.StatusUnauthorized:
		return "UPSTREAM_AUTH_FAILED"
	case http.StatusForbidden:
		return "UPSTREAM_MODEL_UNAVAILABLE"
	case http.StatusTooManyRequests:
		return "UPSTREAM_RATE_LIMITED"
	case http.StatusNotFound:
		return "UPSTREAM_MODEL_UNAVAILABLE"
	case http.StatusBadRequest:
		return "UPSTREAM_REQUEST_REJECTED"
	default:
		return "UPSTREAM_UNAVAILABLE"
	}
}

func containsProbeText(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

// logClaudeSubscriptionRejection 记录 Claude 订阅请求被上游拒绝时的错误类型与消息，便于区分凭据问题、
// 客户端身份或版本被拒等原因。只记录解析出的 error.type 与截断后的 error.message，不记录原始正文或凭据。
func logClaudeSubscriptionRejection(ctx context.Context, logger *slog.Logger, operation string, status int, body []byte, userAgent string, attrs ...slog.Attr) {
	errorType, message := anthropicclaude.UpstreamError(body)
	args := []any{
		slog.String("operation", operation),
		slog.Int("upstream_status", status),
		slog.String("upstream_error_type", errorType),
		slog.String("upstream_error_message", message),
		slog.String("claude_code_user_agent", userAgent),
	}
	for _, attr := range attrs {
		args = append(args, attr)
	}
	logger.WarnContext(ctx, "claude subscription request rejected by upstream", args...)
}
