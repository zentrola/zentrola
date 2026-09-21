// Package provider 提供可配置上游服务商共享的网络边界校验。
package provider

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path"
	"strings"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// BaseURL 接受不含凭证、查询参数和片段的 HTTPS 基础地址。
// 字面量内网地址与本地域名被拒绝，避免管理配置直接成为 SSRF 入口。
func BaseURL(raw string) (string, bool) {
	return BaseURLForScope(raw, catalog.NetworkScopePublic)
}

// BaseURLForScope 按 Endpoint 的网络范围校验基础地址。公网地址强制使用
// HTTPS；管理员显式标记的私网地址允许 HTTP，以支持局域网内的模型服务。
func BaseURLForScope(raw, networkScope string) (string, bool) {
	networkScope = normalizedNetworkScope(networkScope)
	base := strings.TrimSuffix(raw, "/")
	u, err := url.Parse(base)
	if err != nil || !catalog.ValidNetworkScope(networkScope) || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(networkScope == catalog.NetworkScopePublic && u.Scheme != "https") ||
		(networkScope == catalog.NetworkScopePrivate && u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if networkScope == catalog.NetworkScopePublic &&
		(host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")) {
		return "", false
	}
	if ip := net.ParseIP(host); ip != nil && !endpointIPAllowed(ip, networkScope) {
		return "", false
	}
	cleaned := path.Clean(u.EscapedPath())
	if cleaned == "." {
		cleaned = ""
	}
	if cleaned != u.EscapedPath() {
		return "", false
	}
	return base, true
}

func normalizedNetworkScope(networkScope string) string {
	if networkScope == "" {
		return catalog.NetworkScopePublic
	}
	return networkScope
}

func endpointIPAllowed(ip net.IP, networkScope string) bool {
	return catalog.EndpointIPAllowed(ip, networkScope)
}

func publicIP(ip net.IP) bool {
	return catalog.EndpointIPAllowed(ip, catalog.NetworkScopePublic)
}

func resolvePublicAddresses(ctx context.Context, resolver ipResolver, host string) ([]net.IPAddr, error) {
	return resolveEndpointAddresses(ctx, resolver, host, catalog.NetworkScopePublic)
}

func resolveEndpointAddresses(ctx context.Context, resolver ipResolver, host, networkScope string) ([]net.IPAddr, error) {
	networkScope = normalizedNetworkScope(networkScope)
	if !catalog.ValidNetworkScope(networkScope) {
		return nil, errors.New("invalid endpoint network scope")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !endpointIPAllowed(ip, networkScope) {
			return nil, errors.New("endpoint resolved to a disallowed address")
		}
		return []net.IPAddr{{IP: ip}}, nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("endpoint did not resolve to an address")
	}
	for _, address := range addresses {
		if !endpointIPAllowed(address.IP, networkScope) {
			return nil, errors.New("endpoint resolved to a disallowed address")
		}
	}
	return addresses, nil
}

// PublicDialContext 在连接前解析并固定公网地址，避免域名、CNAME 或 DNS rebinding
// 绕过 BaseURL 对字面量内网地址的校验。
func PublicDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	return EndpointDialContext(dialer, catalog.NetworkScopePublic)
}

// EndpointDialContext 在连接前解析并固定经过对应网络范围校验的地址。
func EndpointDialContext(dialer *net.Dialer, networkScope string) func(context.Context, string, string) (net.Conn, error) {
	networkScope = normalizedNetworkScope(networkScope)
	resolver := ipResolver(net.DefaultResolver)
	if dialer.Resolver != nil {
		resolver = dialer.Resolver
	}
	return endpointDialContext(dialer, resolver, networkScope)
}

func publicDialContext(dialer *net.Dialer, resolver ipResolver) func(context.Context, string, string) (net.Conn, error) {
	return endpointDialContext(dialer, resolver, catalog.NetworkScopePublic)
}

func endpointDialContext(dialer *net.Dialer, resolver ipResolver, networkScope string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := resolveEndpointAddresses(ctx, resolver, host, networkScope)
		if err != nil {
			return nil, err
		}
		var failures []error
		for _, candidate := range addresses {
			connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			failures = append(failures, dialErr)
		}
		return nil, errors.Join(failures...)
	}
}
