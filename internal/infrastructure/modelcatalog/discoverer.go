// Package modelcatalog 根据服务商能力同步官方模型目录。
package modelcatalog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
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
	NetworkScope string
}

const (
	maxCatalogResponseBytes = 1 << 20
	maxCatalogModels        = 1000
	maxCatalogPages         = 10
)

// adapter 隔离服务商专有的目录地址、认证方式和响应结构。
// 适配器仅由服务商编码选择，与推理协议无关。
type adapter interface {
	Name() string
	Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error)
	Decode([]byte, int) ([]mgmt.DiscoveredModel, string, error)
}

type Discoverer struct {
	client        *http.Client
	privateClient *http.Client
	logger        *slog.Logger
	adapters      map[string]adapter
}

func NewDiscoverer(logger *slog.Logger) *Discoverer {
	newClient := func(networkScope string) *http.Client {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = provider.EndpointDialContext(&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}, networkScope)
		transport.TLSHandshakeTimeout = 5 * time.Second
		transport.ResponseHeaderTimeout = 10 * time.Second
		return &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Discoverer{
		client:        newClient(catalog.NetworkScopePublic),
		privateClient: newClient(catalog.NetworkScopePrivate),
		logger:        logger,
		adapters: map[string]adapter{
			catalog.OpenAIOfficialCode:   openAIAdapter{},
			catalog.GoogleOfficialCode:   googleAdapter{},
			catalog.DeepSeekOfficialCode: deepSeekAdapter{},
			catalog.ZhipuOfficialCode:    zhipuAdapter{},
			catalog.KimiOfficialCode:     moonshotAdapter{},
			catalog.QwenOfficialCode:     qwenAdapter{},
			catalog.XAIOfficialCode:      xAIAdapter{},
		},
	}
}

func (d *Discoverer) Supports(providerCode string) bool {
	_, ok := d.adapters[providerCode]
	return ok
}

func (d *Discoverer) Discover(ctx context.Context, source mgmt.ModelDiscoverySource, credential []byte, proxy *catalog.OutboundProxy) (models []mgmt.DiscoveredModel, result mgmt.ConnectionResult) {
	started := time.Now()
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()

	adapter, ok := d.adapters[source.ProviderCode]
	if !ok {
		result.Code = "MODEL_CATALOG_UNSUPPORTED"
		return
	}
	if !validCredential(credential) {
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
		req.Header.Set("Accept", "application/json")

		resp, requestErr := client.Do(req)
		req.Header.Del("Authorization")
		if requestSpec.APIKeyHeader != "" {
			req.Header.Del(requestSpec.APIKeyHeader)
		}
		if requestErr != nil {
			cleanup()
			result.Code = connectionErrorCode(ctx, requestErr, proxy)
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
			result.Code = connectionErrorCode(ctx, readErr, proxy)
			return nil, result
		}
		if len(data) > maxCatalogResponseBytes {
			result.Code = "UPSTREAM_INVALID_RESPONSE"
			return nil, result
		}
		pageModels, nextCursor, decodeErr := adapter.Decode(data, page)
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

func validCredential(credential []byte) bool {
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
