// Package anthropic 提供 Anthropic 与 OpenAI 兼容上游的端到端推理检查。
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
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
			Body:           anthropicProbeBody(target.UpstreamModelCode),
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
	client           *http.Client
	privateClient    *http.Client
	standardAdapter  connectionProbeAdapter
	providerAdapters map[string]connectionProbeAdapter
}

func NewConnectionTester() *ConnectionTester {
	newClient := func(networkScope string) *http.Client {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = provider.EndpointDialContext(&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}, networkScope)
		transport.TLSHandshakeTimeout = 5 * time.Second
		transport.ResponseHeaderTimeout = 10 * time.Second
		return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	standardAdapter := standardConnectionProbeAdapter{}
	return &ConnectionTester{
		client:          newClient(catalog.NetworkScopePublic),
		privateClient:   newClient(catalog.NetworkScopePrivate),
		standardAdapter: standardAdapter,
		providerAdapters: map[string]connectionProbeAdapter{
			catalog.DeepSeekOfficialCode: deepSeekConnectionProbeAdapter{standard: standardAdapter},
			catalog.GoogleOfficialCode:   googleConnectionProbeAdapter{},
		},
	}
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
	if (target.AuthType != mgmt.AuthTypeSubscription && !validProbeCredential(credential)) ||
		(target.AuthType == mgmt.AuthTypeSubscription && (len(credential) == 0 || len(credential) > 64<<10)) {
		result.Code = "CREDENTIAL_INVALID"
		return
	}

	requestCredential := string(credential)
	accountID := ""
	oauthBearer := false
	probeAdapter := t.probeAdapter(target.ProviderCode)
	probe, ok := probeAdapter.Build(target, base)
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
			req.Header.Set("anthropic-beta", anthropicclaude.OAuthBeta)
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
		result.Code = connectionErrorCode(ctx, err, proxy)
		return
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if readErr != nil {
		result.Code = connectionErrorCode(ctx, readErr, proxy)
		return
	}
	if len(data) > 64<<10 {
		result.Code = "UPSTREAM_INVALID_RESPONSE"
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		result.Code = inferenceStatusCode(resp.StatusCode, data)
		return
	}
	if !probeAdapter.ValidResponse(data, probe) {
		result.Code = "UPSTREAM_INVALID_RESPONSE"
		return
	}
	result.OK = true
	result.Code = "OK"
	return
}

func validProbeCredential(credential []byte) bool {
	if len(credential) == 0 || len(credential) > 4096 {
		return false
	}
	for _, ch := range credential {
		if ch < 33 || ch > 126 {
			return false
		}
	}
	return true
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

func anthropicProbeBody(model string) []byte {
	body, _ := json.Marshal(struct {
		Model       string         `json:"model"`
		MaxTokens   int            `json:"max_tokens"`
		Temperature int            `json:"temperature"`
		Stream      bool           `json:"stream"`
		Messages    []probeMessage `json:"messages"`
	}{Model: model, MaxTokens: connectionProbeMaxOutputTokens, Messages: []probeMessage{{Role: "user", Content: "Reply only with OK."}}})
	return body
}

func validInferenceResponse(data []byte, format inferenceProbeFormat) bool {
	switch format {
	case anthropicInferenceProbe:
		var payload struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(data, &payload) != nil || payload.Type != "message" {
			return false
		}
		for _, content := range payload.Content {
			if strings.TrimSpace(content.Text) != "" {
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

func connectionErrorCode(ctx context.Context, err error, proxy *catalog.OutboundProxy) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "REQUEST_CANCELLED"
	}
	if errors.Is(err, provider.ErrProxyAuthentication) {
		return "PROXY_AUTH_REJECTED"
	}
	if proxy != nil {
		return "PROXY_SERVER_UNAVAILABLE"
	}
	var netErr net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return "UPSTREAM_TIMEOUT"
	}
	return "UPSTREAM_UNAVAILABLE"
}
