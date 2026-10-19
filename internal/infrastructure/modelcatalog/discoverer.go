// Package modelcatalog 根据服务商能力同步官方模型目录。
package modelcatalog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

type catalogRequest struct {
	URL          string
	Bearer       bool
	APIKeyHeader string
	// AccountIDHeader 非空时注入订阅凭据解析出的账号标识，缺少账号标识的凭据会被拒绝。
	AccountIDHeader string
	NetworkScope    string
	// Headers 只承载协议版本等非敏感请求头，凭据始终由 Bearer 或 APIKeyHeader 注入。
	Headers map[string]string
}

const (
	// Codex 订阅目录随模型附带完整的客户端指令，单页体积远大于 API Key 目录。
	maxCatalogResponseBytes = 4 << 20
	maxCatalogModels        = 1000
	maxCatalogPages         = 10
)

var errSubscriptionExpired = errors.New("subscription access token expired")

// adapter 隔离服务商专有的目录地址、认证方式和响应结构。
// 适配器仅由服务商编码选择，与推理协议无关；同一服务商的不同凭据类型可在适配器内部使用不同目录。
type adapter interface {
	Name() string
	Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error)
	Decode(mgmt.ModelDiscoverySource, []byte, int) ([]mgmt.DiscoveredModel, string, error)
}

// subscriptionToken 是订阅凭据转换出的单次目录请求凭据，Token 使用后必须清零。
type subscriptionToken struct {
	Token     []byte
	AccountID string
}

// subscriptionAdapter 由支持订阅凭据读取模型目录的适配器实现，负责把存储凭据转换为请求 Token。
type subscriptionAdapter interface {
	SupportsSubscription(authAdapter string) bool
	SubscriptionCredential([]byte) (subscriptionToken, error)
}

type Discoverer struct {
	client        *http.Client
	privateClient *http.Client
	logger        *slog.Logger
	adapters      map[string]adapter
}

func NewDiscoverer(logger *slog.Logger) *Discoverer {
	clients := provider.NewUpstreamClients(provider.UpstreamClientOptions{
		DialTimeout: 5 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, Timeout: 15 * time.Second,
	})
	if logger == nil {
		logger = slog.Default()
	}
	return &Discoverer{
		client:        clients.Public,
		privateClient: clients.Private,
		logger:        logger,
		adapters: map[string]adapter{
			catalog.AnthropicOfficialCode: anthropicAdapter{},
			catalog.OpenAIOfficialCode:    openAIAdapter{},
			catalog.GoogleOfficialCode:    googleAdapter{},
			catalog.DeepSeekOfficialCode:  deepSeekAdapter{},
			catalog.ZhipuOfficialCode:     zhipuAdapter{},
			catalog.KimiOfficialCode:      moonshotAdapter{},
			catalog.QwenOfficialCode:      qwenAdapter{},
			catalog.XAIOfficialCode:       xAIAdapter{},
		},
	}
}

func (d *Discoverer) Supports(providerCode string) bool {
	_, ok := d.adapters[providerCode]
	return ok
}

func (d *Discoverer) SupportsCredential(providerCode, authType, authAdapter string) bool {
	adapter, ok := d.adapters[providerCode]
	if !ok {
		return false
	}
	switch authType {
	case "", mgmt.AuthTypeAPIKey:
		return true
	case mgmt.AuthTypeSubscription:
		subscription, ok := adapter.(subscriptionAdapter)
		return ok && subscription.SupportsSubscription(authAdapter)
	default:
		return false
	}
}

func (d *Discoverer) Discover(ctx context.Context, source mgmt.ModelDiscoverySource, credential []byte, proxy *catalog.OutboundProxy) (models []mgmt.DiscoveredModel, result mgmt.ConnectionResult) {
	started := time.Now()
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()

	adapter, ok := d.adapters[source.ProviderCode]
	if !ok {
		result.Code = "MODEL_CATALOG_UNSUPPORTED"
		return
	}
	var accountID string
	if source.AuthType == mgmt.AuthTypeSubscription {
		if !d.SupportsCredential(source.ProviderCode, source.AuthType, source.AuthAdapter) {
			result.Code = "MODEL_CATALOG_UNSUPPORTED"
			return
		}
		token, err := adapter.(subscriptionAdapter).SubscriptionCredential(credential)
		if err != nil {
			result.Code = "CREDENTIAL_INVALID"
			if errors.Is(err, errSubscriptionExpired) {
				result.Code = "SUBSCRIPTION_CREDENTIAL_EXPIRED"
			}
			return
		}
		defer clear(token.Token)
		credential, accountID = token.Token, token.AccountID
	}
	if !provider.ValidCredential(credential) {
		result.Code = "CREDENTIAL_INVALID"
		return
	}

	seen := make(map[string]struct{})
	seenCursors := make(map[string]struct{})
	var cursor string
	for page := 1; page <= maxCatalogPages; page++ {
		requestSpec, requestErr := adapter.Request(source, page, cursor)
		if requestErr != nil {
			result.Code = "UPSTREAM_URL_REJECTED"
			return nil, result
		}
		if requestSpec.AccountIDHeader != "" && !provider.ValidCredential([]byte(accountID)) {
			result.Code = "CREDENTIAL_INVALID"
			return nil, result
		}
		baseClient := d.client
		if requestSpec.NetworkScope == catalog.NetworkScopePrivate {
			baseClient = d.privateClient
		}
		client, cleanup, proxyErr := provider.ClientWithProxyForScope(ctx, baseClient, proxy, requestSpec.NetworkScope, provider.ProxyRequestLog{
			Logger: d.logger, Operation: "model_catalog_sync", ProviderCode: source.ProviderCode,
		})
		if proxyErr != nil {
			result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
			return nil, result
		}
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, requestSpec.URL, nil)
		if requestErr != nil {
			cleanup()
			result.Code = "UPSTREAM_URL_REJECTED"
			return nil, result
		}
		if requestSpec.Bearer {
			req.Header.Set("Authorization", "Bearer "+string(credential))
		}
		if requestSpec.APIKeyHeader != "" {
			req.Header.Set(requestSpec.APIKeyHeader, string(credential))
		}
		if requestSpec.AccountIDHeader != "" {
			req.Header.Set(requestSpec.AccountIDHeader, accountID)
		}
		for name, value := range requestSpec.Headers {
			req.Header.Set(name, value)
		}
		req.Header.Set("Accept", "application/json")

		resp, requestErr := client.Do(req)
		req.Header.Del("Authorization")
		if requestSpec.APIKeyHeader != "" {
			req.Header.Del(requestSpec.APIKeyHeader)
		}
		if requestSpec.AccountIDHeader != "" {
			req.Header.Del(requestSpec.AccountIDHeader)
		}
		if requestErr != nil {
			cleanup()
			result.Code = provider.TransportErrorCode(ctx, requestErr, proxy != nil)
			return nil, result
		}
		result.HTTPStatus = resp.StatusCode
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxCatalogResponseBytes+1))
		_ = resp.Body.Close()
		cleanup()
		d.logger.InfoContext(ctx, "official model catalog response",
			"provider_code", source.ProviderCode,
			"catalog_adapter", adapter.Name(),
			"catalog_page", page,
			"upstream_url", requestSpec.URL,
			"upstream_status", resp.StatusCode,
			"response_bytes", len(data),
		)
		if resp.StatusCode != http.StatusOK {
			result.Code = connectionStatusCode(resp.StatusCode)
			return nil, result
		}
		if readErr != nil {
			result.Code = provider.TransportErrorCode(ctx, readErr, proxy != nil)
			return nil, result
		}
		if len(data) > maxCatalogResponseBytes {
			result.Code = "UPSTREAM_INVALID_RESPONSE"
			return nil, result
		}
		pageModels, nextCursor, decodeErr := adapter.Decode(source, data, page)
		if decodeErr != nil {
			result.Code = "UPSTREAM_INVALID_RESPONSE"
			return nil, result
		}
		for _, model := range pageModels {
			if _, duplicate := seen[model.Code]; duplicate {
				continue
			}
			seen[model.Code] = struct{}{}
			models = append(models, model)
			if len(models) > maxCatalogModels {
				result.Code = "UPSTREAM_INVALID_RESPONSE"
				return nil, result
			}
		}
		if nextCursor == "" {
			sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
			result.OK = true
			result.Code = "OK"
			return models, result
		}
		if _, duplicate := seenCursors[nextCursor]; duplicate {
			result.Code = "UPSTREAM_INVALID_RESPONSE"
			return nil, result
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	result.Code = "UPSTREAM_INVALID_RESPONSE"
	return nil, result
}

func connectionStatusCode(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "UPSTREAM_AUTH_FAILED"
	case http.StatusTooManyRequests:
		return "UPSTREAM_RATE_LIMITED"
	default:
		return "UPSTREAM_UNAVAILABLE"
	}
}
