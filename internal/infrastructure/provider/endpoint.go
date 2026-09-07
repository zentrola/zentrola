// Package provider 提供可配置上游服务商共享的网络边界校验。
package provider

import (
	"net"
	"net/url"
	"path"
	"strings"
)

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
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()) {
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
