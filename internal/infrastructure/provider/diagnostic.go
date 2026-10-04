package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"syscall"
)

// NetworkDiagnostic 只包含可安全记录的上游传输错误分类，不复制原始错误。
type NetworkDiagnostic struct {
	Kind, Operation, Network, ErrorType, Detail string
	Redacted                                    bool
}

func DiagnoseNetworkError(err error) NetworkDiagnostic {
	diagnostic := NetworkDiagnostic{Kind: networkFailureKind(err), Redacted: true}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		diagnostic.Operation = urlErr.Op
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Op != "" {
			diagnostic.Operation = netErr.Op
		}
		diagnostic.Network = netErr.Net
	}
	root := err
	for {
		next := errors.Unwrap(root)
		if next == nil {
			break
		}
		root = next
	}
	if root != nil {
		diagnostic.ErrorType = reflect.TypeOf(root).String()
	}
	diagnostic.Detail = redactedFailureDetail(diagnostic.Kind)
	return diagnostic
}

func networkFailureKind(err error) string {
	switch {
	case errors.Is(err, ErrProxyAuthentication):
		return "proxy_auth"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "unreachable"
	case errors.Is(err, syscall.EPIPE):
		return "broken_pipe"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "eof"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	var certificateErr x509.UnknownAuthorityError
	if errors.As(err, &certificateErr) {
		return "tls"
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return "tls"
	}
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return "tls"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "transport"
}

func redactedFailureDetail(kind string) string {
	switch kind {
	case "proxy_auth":
		return "proxy authentication rejected"
	case "cancelled":
		return "upstream request cancelled"
	case "timeout":
		return "upstream request timed out"
	case "connection_reset":
		return "upstream connection reset"
	case "connection_refused":
		return "upstream connection refused"
	case "unreachable":
		return "upstream network unreachable"
	case "broken_pipe":
		return "upstream connection broken"
	case "eof":
		return "upstream connection closed unexpectedly"
	case "dns":
		return "upstream DNS lookup failed"
	case "tls":
		return "upstream TLS negotiation failed"
	default:
		return "upstream transport failed"
	}
}

// RedirectLocationForLog 只保留 scheme 和 host，不暴露凭据、路径或查询参数。
func RedirectLocationForLog(raw string) (location string, redacted bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "******", true
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

var productionHeaderAllowlist = map[string]struct{}{
	"accept":            {},
	"accept-encoding":   {},
	"anthropic-beta":    {},
	"anthropic-version": {},
	"content-type":      {},
	"openai-beta":       {},
	"originator":        {},
	"traceparent":       {},
	"tracestate":        {},
	"user-agent":        {},
	"x-request-id":      {},
}

// HeadersForLog 仅保留明确安全的 Header Value，未知 Header 一律脱敏。
func HeadersForLog(headers http.Header) http.Header {
	result := make(http.Header, len(headers))
	for name, values := range headers {
		copied := append([]string(nil), values...)
		if _, safe := productionHeaderAllowlist[strings.ToLower(name)]; !safe {
			for index := range copied {
				copied[index] = "******"
			}
		}
		result[http.CanonicalHeaderKey(name)] = copied
	}
	return result
}
