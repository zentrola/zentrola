// Package provider 提供可配置上游服务商共享的网络边界校验。
package provider

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"path"
	"strings"
)

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fec0::/10"),
}

// BaseURL 接受不含凭证、查询参数和片段的 HTTPS 基础地址。
// 字面量内网地址与本地域名被拒绝，避免管理配置直接成为 SSRF 入口。
func BaseURL(raw string) (string, bool) {
	base := strings.TrimSuffix(raw, "/")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", false
	}
	if ip := net.ParseIP(host); ip != nil && !publicIP(ip) {
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

func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func resolvePublicAddresses(ctx context.Context, resolver ipResolver, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !publicIP(ip) {
			return nil, errors.New("endpoint resolved to a non-public address")
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
		if !publicIP(address.IP) {
			return nil, errors.New("endpoint resolved to a non-public address")
		}
	}
	return addresses, nil
}

// PublicDialContext 在连接前解析并固定公网地址，避免域名、CNAME 或 DNS rebinding
// 绕过 BaseURL 对字面量内网地址的校验。
func PublicDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	resolver := ipResolver(net.DefaultResolver)
	if dialer.Resolver != nil {
		resolver = dialer.Resolver
	}
	return publicDialContext(dialer, resolver)
}

func publicDialContext(dialer *net.Dialer, resolver ipResolver) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := resolvePublicAddresses(ctx, resolver, host)
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
