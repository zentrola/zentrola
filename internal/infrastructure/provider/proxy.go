package provider

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// ClientWithProxy 为单次代理调用克隆 Transport，避免不同服务商之间串用代理配置。
// 返回的 cleanup 必须在响应体关闭时调用。
func ClientWithProxy(base *http.Client, proxy *catalog.OutboundProxy) (*http.Client, func(), error) {
	if proxy == nil {
		return base, func() {}, nil
	}
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil || proxyURL.Hostname() == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") ||
		proxyURL.RawQuery != "" || proxyURL.Fragment != "" || (proxyURL.EscapedPath() != "" && proxyURL.EscapedPath() != "/") {
		return nil, nil, errors.New("invalid proxy URL")
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
	return &client, transport.CloseIdleConnections, nil
}
