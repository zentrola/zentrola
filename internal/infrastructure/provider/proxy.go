package provider

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// ProxyRequestLog 描述一次使用服务商代理的出站请求。不得在日志字段中加入
// 代理 URL、认证信息、Header 或上游凭据。
type ProxyRequestLog struct {
	Logger       *slog.Logger
	Operation    string
	ProviderID   int64
	ResourceID   int64
	ProviderCode string
	Protocol     string
}

func parseProxyURL(proxy *catalog.OutboundProxy) (*url.URL, error) {
	if proxy == nil {
		return nil, nil
	}
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil || proxyURL.Hostname() == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") ||
		proxyURL.RawQuery != "" || proxyURL.Fragment != "" || (proxyURL.EscapedPath() != "" && proxyURL.EscapedPath() != "/") {
		return nil, errors.New("invalid proxy URL")
	}
	return proxyURL, nil
}

func logProxyRequest(ctx context.Context, proxyURL *url.URL, details ProxyRequestLog) {
	logger := details.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attributes := []any{"proxy_enabled", true, "proxy_scheme", proxyURL.Scheme}
	if details.Operation != "" {
		attributes = append(attributes, "operation", details.Operation)
	}
	if details.ProviderID > 0 {
		attributes = append(attributes, "provider_id", details.ProviderID)
	}
	if details.ResourceID > 0 {
		attributes = append(attributes, "resource_id", details.ResourceID)
	}
	if details.ProviderCode != "" {
		attributes = append(attributes, "provider_code", details.ProviderCode)
	}
	if details.Protocol != "" {
		attributes = append(attributes, "protocol", details.Protocol)
	}
	logger.InfoContext(ctx, "provider outbound request using proxy", attributes...)
}

// ClientWithProxy 为单次代理调用克隆 Transport，避免不同服务商之间串用代理配置。
// 返回的 cleanup 必须在响应体关闭时调用。
func ClientWithProxy(ctx context.Context, base *http.Client, proxy *catalog.OutboundProxy, details ProxyRequestLog) (*http.Client, func(), error) {
	if proxy == nil {
		return base, func() {}, nil
	}
	proxyURL, err := parseProxyURL(proxy)
	if err != nil {
		return nil, nil, err
	}
	baseTransport, ok := base.Transport.(*http.Transport)
	if !ok {
		return nil, nil, errors.New("unsupported base transport")
	}
	transport := baseTransport.Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	transport.ProxyConnectHeader = make(http.Header, len(proxy.Headers))
	for key, value := range proxy.Headers {
		transport.ProxyConnectHeader.Set(key, value)
	}
	client := *base
	client.Transport = transport
	logProxyRequest(ctx, proxyURL, details)
	return &client, transport.CloseIdleConnections, nil
}

// EnvironmentWithProxy 为不能注入 http.Client 的子进程设置服务商专属代理。
// 配置代理时会覆盖继承的代理环境，避免 NO_PROXY 或其他全局代理绕过该服务商配置。
func EnvironmentWithProxy(ctx context.Context, environment []string, proxy *catalog.OutboundProxy, details ProxyRequestLog) ([]string, error) {
	if proxy == nil {
		return append([]string(nil), environment...), nil
	}
	proxyURL, err := parseProxyURL(proxy)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(environment)+2)
	for _, item := range environment {
		key, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		default:
			result = append(result, item)
		}
	}
	result = append(result, "HTTP_PROXY="+proxyURL.String(), "HTTPS_PROXY="+proxyURL.String())
	logProxyRequest(ctx, proxyURL, details)
	return result, nil
}
