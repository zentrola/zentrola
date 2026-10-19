package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// CompatibleUpstream 按实际 endpoint 协议选择上游，并在入站协议与 endpoint
// 协议不一致时转换请求及响应。路由选择已完成，因此它不会在上游失败后重试另一协议。
type CompatibleUpstream struct {
	anthropic Upstream
	openAI    Upstream
}

func NewCompatibleUpstream(anthropic, openAI Upstream) *CompatibleUpstream {
	return &CompatibleUpstream{anthropic: anthropic, openAI: openAI}
}

func (u *CompatibleUpstream) Open(ctx context.Context, route Route, request Request, credential []byte) (*Response, error) {
	endpointProtocol := route.EndpointProtocol
	if endpointProtocol == "" {
		endpointProtocol = OpenAIEndpoint
		if request.Protocol == AnthropicProtocol {
			endpointProtocol = AnthropicEndpoint
		}
	}
	inboundProtocol := request.Protocol
	sameProtocol := endpointProtocol == AnthropicEndpoint && inboundProtocol == AnthropicProtocol ||
		endpointProtocol == OpenAIEndpoint && IsOpenAIProtocol(inboundProtocol)

	upstream := u.openAI
	if endpointProtocol == AnthropicEndpoint {
		upstream = u.anthropic
	}
	if upstream == nil {
		return nil, ErrRoute
	}
	if sameProtocol {
		return upstream.Open(ctx, route, request, credential)
	}

	adapted, err := convertGatewayRequest(request, endpointProtocol)
	if err != nil {
		return nil, err
	}
	response, err := upstream.Open(ctx, route, adapted, credential)
	if err != nil {
		return nil, err
	}
	return convertGatewayResponse(response, inboundProtocol)
}

func convertGatewayResponse(response *Response, targetProtocol string) (*Response, error) {
	if response == nil || response.Body == nil {
		return nil, ErrUpstream
	}
	headers := cloneHeaders(response.Headers)
	translateCompatibilityHeaders(headers, targetProtocol)
	stream := strings.Contains(strings.ToLower(http.Header(headers).Get("Content-Type")), "text/event-stream")
	var transform func(io.Reader, io.Writer) error
	var compatibilityUsage *CompatibilityUsage
	if response.Status >= 400 && (targetProtocol == AnthropicProtocol || targetProtocol == OpenAIProtocol || targetProtocol == OpenAIResponsesProtocol) {
		// 错误响应保留原状态码并包装为目标协议的标准错误体，供 failover 按原生
		// 路径的状态码语义判定是否冷却或重试。
		stream = false
		transform = compatibilityErrorTransform(targetProtocol, response.Status)
	}
	switch {
	case transform != nil:
	case targetProtocol == AnthropicProtocol:
		if stream {
			transform = openAIStreamToAnthropic
		} else {
			transform = openAIJSONToAnthropic
		}
	case targetProtocol == OpenAIProtocol:
		compatibilityUsage = &CompatibilityUsage{}
		convert := anthropicJSONToOpenAIChat
		if stream {
			convert = anthropicStreamToOpenAIChat
		}
		transform = func(reader io.Reader, writer io.Writer) error { return convert(reader, writer, compatibilityUsage) }
	case targetProtocol == OpenAIResponsesProtocol:
		compatibilityUsage = &CompatibilityUsage{}
		convert := anthropicJSONToOpenAIResponses
		if stream {
			convert = anthropicStreamToOpenAIResponses
		}
		transform = func(reader io.Reader, writer io.Writer) error { return convert(reader, writer, compatibilityUsage) }
	default:
		response.Body.Close()
		return nil, ErrInvalid
	}
	body := newConvertedBody(response.Body, transform)
	headers.Del("Content-Length")
	headers.Del("Content-Encoding")
	if stream {
		headers.Set("Content-Type", "text/event-stream")
	} else {
		headers.Set("Content-Type", "application/json")
	}
	return &Response{Status: response.Status, Headers: headers, Body: body, CompatibilityUsage: compatibilityUsage}, nil
}

// CompatibilityUsage 由 Anthropic→OpenAI 兼容转换层写入转换前的原始用量细节。
// OpenAI 语义的 usage 不区分缓存写入，转换后缓存写入只能并入总输入；observer 通过它
// 取回 cache_creation_input_tokens，使缓存写入能够单独计价。转换协程写入、响应处理
// 协程读取，因此使用原子值。
type CompatibilityUsage struct {
	cacheCreation atomic.Pointer[int64]
}

// record 在写出转换后的 usage 之前调用，保证 observer 读到最终 usage 时已能取得对应的缓存写入数。
func (u *CompatibilityUsage) record(counts anthropicUsageCounts) {
	if u == nil || !counts.cacheCreationSeen {
		return
	}
	value := counts.cacheCreation
	u.cacheCreation.Store(&value)
}

// CacheCreationTokens 返回上游 Anthropic 报告的缓存写入 Token 数；上游未报告时为 nil。
func (u *CompatibilityUsage) CacheCreationTokens() *int64 {
	if u == nil {
		return nil
	}
	value := u.cacheCreation.Load()
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func translateCompatibilityHeaders(headers http.Header, targetProtocol string) {
	var mappings map[string]string
	if targetProtocol == AnthropicProtocol {
		mappings = map[string]string{
			"X-Ratelimit-Limit-Requests":     "Anthropic-Ratelimit-Requests-Limit",
			"X-Ratelimit-Remaining-Requests": "Anthropic-Ratelimit-Requests-Remaining",
			"X-Ratelimit-Reset-Requests":     "Anthropic-Ratelimit-Requests-Reset",
			"X-Ratelimit-Limit-Tokens":       "Anthropic-Ratelimit-Tokens-Limit",
			"X-Ratelimit-Remaining-Tokens":   "Anthropic-Ratelimit-Tokens-Remaining",
			"X-Ratelimit-Reset-Tokens":       "Anthropic-Ratelimit-Tokens-Reset",
		}
		for source, target := range mappings {
			if value := headers.Get(source); value != "" {
				headers.Set(target, value)
			}
		}
		for key := range headers {
			if strings.HasPrefix(strings.ToLower(key), "x-ratelimit-") {
				headers.Del(key)
			}
		}
		return
	}
	if !IsOpenAIProtocol(targetProtocol) {
		return
	}
	mappings = map[string]string{
		"Anthropic-Ratelimit-Requests-Limit":     "X-Ratelimit-Limit-Requests",
		"Anthropic-Ratelimit-Requests-Remaining": "X-Ratelimit-Remaining-Requests",
		"Anthropic-Ratelimit-Requests-Reset":     "X-Ratelimit-Reset-Requests",
		"Anthropic-Ratelimit-Tokens-Limit":       "X-Ratelimit-Limit-Tokens",
		"Anthropic-Ratelimit-Tokens-Remaining":   "X-Ratelimit-Remaining-Tokens",
		"Anthropic-Ratelimit-Tokens-Reset":       "X-Ratelimit-Reset-Tokens",
	}
	for source, target := range mappings {
		if value := headers.Get(source); value != "" {
			headers.Set(target, value)
		}
	}
	for key := range headers {
		if strings.HasPrefix(strings.ToLower(key), "anthropic-ratelimit-") {
			headers.Del(key)
		}
	}
}

func cloneHeaders(source map[string][]string) http.Header {
	result := make(http.Header, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

type convertedBody struct {
	*io.PipeReader
	source io.ReadCloser
	once   sync.Once
}

func newConvertedBody(source io.ReadCloser, transform func(io.Reader, io.Writer) error) *convertedBody {
	reader, writer := io.Pipe()
	body := &convertedBody{PipeReader: reader, source: source}
	go func() {
		err := transform(source, writer)
		source.Close()
		writer.CloseWithError(err)
	}()
	return body
}

func (b *convertedBody) Close() error {
	var err error
	b.once.Do(func() {
		err = b.PipeReader.Close()
		if closeErr := b.source.Close(); err == nil {
			err = closeErr
		}
	})
	return err
}
