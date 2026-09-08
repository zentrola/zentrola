package provider

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TracedClient 为单次调用创建 OTel Client Span，并保留实际发送请求的 Header，
// 以便响应体关闭时及时清除短期驻留的 Provider Credential。
func TracedClient(base *http.Client) (*http.Client, *SentHeaders) {
	client := *base
	capture := &SentHeaders{base: client.Transport}
	if capture.base == nil {
		capture.base = http.DefaultTransport
	}
	client.Transport = otelhttp.NewTransport(capture)
	return &client, capture
}

type SentHeaders struct {
	base    http.RoundTripper
	headers http.Header
}

func (c *SentHeaders) RoundTrip(request *http.Request) (*http.Response, error) {
	c.headers = request.Header
	return c.base.RoundTrip(request)
}

func (c *SentHeaders) Delete(name string) {
	if c != nil && c.headers != nil {
		c.headers.Del(name)
	}
}
