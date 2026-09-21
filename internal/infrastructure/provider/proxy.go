package provider

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// ProxyRequestLog 描述一次使用服务商代理的出站请求。
type ProxyRequestLog struct {
	Logger       *slog.Logger
	Operation    string
	ProviderID   int64
	ResourceID   int64
	ProviderCode string
	Protocol     string
}

var redactProviderLogSecrets atomic.Bool

type publicTargetTransport struct {
	transport *http.Transport
}

func (t *publicTargetTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL == nil || request.URL.Hostname() == "" {
		return nil, errors.New("invalid provider endpoint")
	}
	// 走代理时本地只连接代理，目标域名的解析发生在代理端；本机 DNS 在
	// fake-ip 代理环境下不可信，因此仅拦截字面量内网地址，不做域名解析校验。
	if err := nonPublicLiteralError(request.URL.Hostname()); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(request)
}

func (t *publicTargetTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

// nonPublicLiteralError 仅拒绝字面量内网地址。域名不做本地解析校验，
// 因为代理场景下目标由代理端解析，本机 DNS（如 fake-ip）不可信。
func nonPublicLiteralError(host string) error {
	if ip := net.ParseIP(host); ip != nil && !publicIP(ip) {
		return errors.New("provider endpoint is a non-public address")
	}
	return nil
}

type scopedTargetTransport struct {
	transport    *http.Transport
	networkScope string
}

func (t *scopedTargetTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL == nil || request.URL.Hostname() == "" {
		return nil, errors.New("invalid provider endpoint")
	}
	if err := endpointLiteralError(request.URL.Hostname(), t.networkScope); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(request)
}

func endpointLiteralError(host, networkScope string) error {
	if ip := net.ParseIP(host); ip != nil && !endpointIPAllowed(ip, networkScope) {
		return errors.New("provider endpoint is a disallowed address")
	}
	return nil
}

func (t *scopedTargetTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

func init() {
	// 未显式配置运行环境时采用生产环境策略，避免测试工具或独立调用意外输出凭据。
	redactProviderLogSecrets.Store(true)
}

// ConfigureLogEnvironment 配置进程级 Provider 日志策略。只有明确的 dev/test 环境
// 输出完整诊断信息，prod 或未知环境均脱敏。
func ConfigureLogEnvironment(environment string) {
	environment = strings.ToLower(strings.TrimSpace(environment))
	redactProviderLogSecrets.Store(environment != "dev" && environment != "test")
}

func parseProxyURL(proxy *catalog.OutboundProxy) (*url.URL, error) {
	if proxy == nil {
		return nil, nil
	}
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil || proxyURL.Hostname() == "" || !catalog.ValidProxyScheme(proxyURL.Scheme) ||
		proxyURL.RawQuery != "" || proxyURL.Fragment != "" || (proxyURL.EscapedPath() != "" && proxyURL.EscapedPath() != "/") {
		return nil, errors.New("invalid proxy URL")
	}
	return proxyURL, nil
}

func proxyLogURL(proxyURL *url.URL, redact bool) string {
	if !redact || proxyURL.User == nil {
		return proxyURL.String()
	}
	safe := *proxyURL
	if _, hasPassword := safe.User.Password(); hasPassword {
		safe.User = url.UserPassword("******", "******")
	} else {
		safe.User = url.User("******")
	}
	// net/url 会把 Userinfo 中的星号转义为 %2A；日志展示需要保留直观掩码。
	return strings.ReplaceAll(safe.String(), "%2A", "*")
}

func proxyLogHeaders(headers map[string]string, redact bool) map[string]string {
	result := make(map[string]string, len(headers))
	for name, value := range headers {
		if redact {
			value = "******"
		}
		result[http.CanonicalHeaderKey(name)] = value
	}
	return result
}

func logProxyRequest(ctx context.Context, proxyURL *url.URL, headers map[string]string, details ProxyRequestLog) {
	logger := details.Logger
	if logger == nil {
		logger = slog.Default()
	}
	redact := redactProviderLogSecrets.Load()
	attributes := []any{
		"proxy", true,
		"proxy_url", proxyLogURL(proxyURL, redact),
		"proxy_auth", proxyURL.User != nil,
		"proxy_headers", proxyLogHeaders(headers, redact),
		"proxy_redacted", redact,
	}
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
	return ClientWithProxyForScope(ctx, base, proxy, catalog.NetworkScopePublic, details)
}

// ClientWithProxyForScope 为使用代理的请求保留 Endpoint 网络范围约束。代理负责
// 解析域名，因此这里只校验 URL 中的字面量 IP；域名目标由代理端网络策略约束。
func ClientWithProxyForScope(ctx context.Context, base *http.Client, proxy *catalog.OutboundProxy, networkScope string, details ProxyRequestLog) (*http.Client, func(), error) {
	networkScope = normalizedNetworkScope(networkScope)
	if !catalog.ValidNetworkScope(networkScope) {
		return nil, nil, errors.New("invalid endpoint network scope")
	}
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
	// 代理地址是管理员显式配置的网络出口，允许使用内网代理；字面量目标
	// 仍按 Endpoint 网络范围校验，域名目标交由代理端解析。
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.ProxyConnectHeader = make(http.Header, len(proxy.Headers))
	for key, value := range proxy.Headers {
		transport.ProxyConnectHeader.Set(key, value)
	}
	client := *base
	if networkScope == catalog.NetworkScopePublic {
		client.Transport = &publicTargetTransport{transport: transport}
	} else {
		client.Transport = &scopedTargetTransport{transport: transport, networkScope: networkScope}
	}
	logProxyRequest(ctx, proxyURL, proxy.Headers, details)
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
	result := make([]string, 0, len(environment)+3)
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
	if catalog.SOCKSProxyScheme(proxyURL.Scheme) {
		result = append(result, "ALL_PROXY="+proxyURL.String())
	}
	logProxyRequest(ctx, proxyURL, proxy.Headers, details)
	return result, nil
}
