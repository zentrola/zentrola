package provider

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const (
	// 网关会向少数上游主机并发发起大量长请求；net/http 默认每主机仅保留 2 个空闲连接，
	// 高并发时会频繁新建 TCP/TLS 连接，因此显式放宽连接池。
	upstreamMaxIdleConns        = 256
	upstreamMaxIdleConnsPerHost = 64
	upstreamIdleConnTimeout     = 90 * time.Second
	upstreamKeepAlive           = 30 * time.Second
)

// UpstreamClientOptions 描述上游 HTTP Client 的网络范围与超时。
type UpstreamClientOptions struct {
	NetworkScope          string
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	// Timeout 是包含读取响应体在内的整体上限；0 表示不限制，由调用方的 context 控制。
	Timeout time.Duration
	// DisableCompression 为 true 时不自动协商 gzip，原样转发上游响应体。
	DisableCompression bool
}

// NewUpstreamClient 创建上游 HTTP Client：不使用环境代理，连接前按网络范围解析并固定地址，
// 拒绝跟随重定向，并统一连接池参数。服务商代理由 ClientWithProxyForScope 在此基础上派生。
func NewUpstreamClient(options UpstreamClientOptions) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = options.DisableCompression
	transport.DialContext = EndpointDialContext(&net.Dialer{Timeout: options.DialTimeout, KeepAlive: upstreamKeepAlive}, options.NetworkScope)
	transport.TLSHandshakeTimeout = options.TLSHandshakeTimeout
	transport.ResponseHeaderTimeout = options.ResponseHeaderTimeout
	transport.MaxIdleConns = upstreamMaxIdleConns
	transport.MaxIdleConnsPerHost = upstreamMaxIdleConnsPerHost
	transport.IdleConnTimeout = upstreamIdleConnTimeout
	return &http.Client{Transport: transport, Timeout: options.Timeout, CheckRedirect: RejectRedirect}
}

// UpstreamClients 是按网络范围区分的一组上游 Client。
type UpstreamClients struct {
	Public, Private *http.Client
}

// NewUpstreamClients 为公网与私网 Endpoint 分别创建 Client，二者的地址校验策略不同，不能共享连接池。
func NewUpstreamClients(options UpstreamClientOptions) UpstreamClients {
	options.NetworkScope = catalog.NetworkScopePublic
	public := NewUpstreamClient(options)
	options.NetworkScope = catalog.NetworkScopePrivate
	return UpstreamClients{Public: public, Private: NewUpstreamClient(options)}
}

// ForScope 返回对应网络范围的 Client；未知范围按公网处理，由后续校验拒绝。
func (c UpstreamClients) ForScope(networkScope string) *http.Client {
	if networkScope == catalog.NetworkScopePrivate {
		return c.Private
	}
	return c.Public
}

// CloseIdleConnections 关闭两个 Client 及其派生代理 Transport 的空闲连接。
func (c UpstreamClients) CloseIdleConnections() {
	for _, client := range []*http.Client{c.Public, c.Private} {
		if client == nil {
			continue
		}
		client.CloseIdleConnections()
		CloseIdleProxyConnections(client)
	}
}

// RejectRedirect 让 Client 返回 3xx 响应本身，避免携带凭据跟随到不受信任的地址。
func RejectRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// IsRedirect 判断上游是否返回了被拒绝跟随的重定向。
func IsRedirect(status int) bool { return status >= 300 && status < 400 }

// 上游传输错误分类码，与网关和连接测试对外暴露的错误码一致。
const (
	TransportCancelled   = "REQUEST_CANCELLED"
	TransportProxyAuth   = "PROXY_AUTH_REJECTED"
	TransportProxyServer = "PROXY_SERVER_UNAVAILABLE"
	TransportTimeout     = "UPSTREAM_TIMEOUT"
	TransportUnavailable = "UPSTREAM_UNAVAILABLE"
)

// TransportErrorCode 对未拿到上游响应的传输错误分类。经过服务商代理时，传输层故障无法归因于
// 上游服务商，统一标记为代理故障（代理明确拒绝认证时为代理认证失败）；直连时区分超时与不可用。
func TransportErrorCode(ctx context.Context, err error, proxied bool) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return TransportCancelled
	}
	if errors.Is(err, ErrProxyAuthentication) {
		return TransportProxyAuth
	}
	if proxied {
		return TransportProxyServer
	}
	if IsTimeout(ctx, err) {
		return TransportTimeout
	}
	return TransportUnavailable
}

// IsTimeout 判断请求是否因 context 截止或网络超时（含响应头超时、流式空闲超时）失败。
func IsTimeout(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// UpstreamRequestLog 是上游请求诊断日志的关联字段。
type UpstreamRequestLog struct {
	RequestID  string
	ProviderID int64
	ResourceID int64
	Protocol   string
}

// LogTransportFailure 记录脱敏后的上游传输失败；只输出错误分类，不复制原始错误文本或凭据。
func LogTransportFailure(ctx context.Context, logger *slog.Logger, errorCode string, err error, requestHeaders http.Header, fields UpstreamRequestLog) {
	diagnostic := DiagnoseNetworkError(err)
	logger.WarnContext(ctx, "provider upstream request failed",
		"error_code", errorCode,
		"request_id", fields.RequestID,
		"provider_id", fields.ProviderID,
		"resource_id", fields.ResourceID,
		"protocol", fields.Protocol,
		"failure_kind", diagnostic.Kind,
		"network_op", diagnostic.Operation,
		"network", diagnostic.Network,
		"upstream_error_type", diagnostic.ErrorType,
		"upstream_error", diagnostic.Detail,
		"upstream_headers", HeadersForLog(requestHeaders),
		"redacted", diagnostic.Redacted,
	)
}

// LogRedirectRejected 记录被拒绝的上游重定向；Location 只保留 scheme 与 host。
func LogRedirectRejected(ctx context.Context, logger *slog.Logger, status int, rawLocation string, requestHeaders http.Header, fields UpstreamRequestLog) {
	location, redacted := RedirectLocationForLog(rawLocation)
	logger.WarnContext(ctx, "provider upstream redirect rejected",
		"error_code", "UPSTREAM_REDIRECT_REJECTED",
		"request_id", fields.RequestID,
		"provider_id", fields.ProviderID,
		"resource_id", fields.ResourceID,
		"protocol", fields.Protocol,
		"upstream_status", status,
		"redirect_location", location,
		"upstream_headers", HeadersForLog(requestHeaders),
		"redacted", redacted,
	)
}

// MaxCredentialBytes 是上游凭据的统一长度上限。普通 API Key 通常不超过数百字节，
// 而 ChatGPT、Claude 订阅的 OAuth access token 为 JWT，携带账号声明时可达数 KiB；
// 16 KiB 能覆盖这些格式，同时远低于常见上游的请求头大小限制，可拒绝异常输入。
const MaxCredentialBytes = 16 << 10

// ValidCredential 校验凭据非空、不超过 MaxCredentialBytes，且只含可见 ASCII 字符（33–126），
// 防止空白、控制字符或非 ASCII 字节注入 HTTP 请求头。
func ValidCredential[T ~string | ~[]byte](credential T) bool {
	if len(credential) == 0 || len(credential) > MaxCredentialBytes {
		return false
	}
	for index := 0; index < len(credential); index++ {
		if credential[index] < 33 || credential[index] > 126 {
			return false
		}
	}
	return true
}
