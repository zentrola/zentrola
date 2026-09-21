package catalog

import (
	"net"
	"net/netip"
)

const (
	NetworkScopePublic  = "PUBLIC"
	NetworkScopePrivate = "PRIVATE"
)

// ValidNetworkScope 判断服务商 Endpoint 的网络范围是否受支持。
func ValidNetworkScope(scope string) bool {
	return scope == NetworkScopePublic || scope == NetworkScopePrivate
}

// ValidProxyScheme 判断服务商出站代理协议是否受支持。
func ValidProxyScheme(scheme string) bool {
	switch scheme {
	case "http", "https":
		return true
	default:
		return SOCKSProxyScheme(scheme)
	}
}

// SOCKSProxyScheme 判断代理协议是否为 SOCKS5 或其远端 DNS 别名。
func SOCKSProxyScheme(scheme string) bool {
	return scheme == "socks5" || scheme == "socks5h"
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

var carrierGradeNATPrefix = netip.MustParsePrefix("100.64.0.0/10")

// EndpointIPAllowed 判断 IP 是否可由指定网络范围的 Endpoint 访问。私网范围
// 允许私有地址、回环地址及运营商级 NAT 地址，但始终拒绝链路本地等危险地址。
func EndpointIPAllowed(ip net.IP, scope string) bool {
	if !ValidNetworkScope(scope) {
		return false
	}
	if publicEndpointIP(ip) {
		return true
	}
	if scope != NetworkScopePrivate || ip == nil || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	return ip.IsPrivate() || ip.IsLoopback() || carrierGradeNATPrefix.Contains(address)
}

func publicEndpointIP(ip net.IP) bool {
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
