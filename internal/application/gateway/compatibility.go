package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
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
		endpointProtocol == OpenAIEndpoint && (inboundProtocol == OpenAIProtocol || inboundProtocol == OpenAIResponsesProtocol)

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
	switch targetProtocol {
	case AnthropicProtocol:
		if stream {
			transform = openAIStreamToAnthropic
		} else {
			transform = openAIJSONToAnthropic
		}
	case OpenAIProtocol:
		if stream {
			transform = anthropicStreamToOpenAIChat
		} else {
			transform = anthropicJSONToOpenAIChat
		}
	case OpenAIResponsesProtocol:
		if stream {
			transform = anthropicStreamToOpenAIResponses
		} else {
			transform = anthropicJSONToOpenAIResponses
		}
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
	return &Response{Status: response.Status, Headers: headers, Body: body}, nil
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
	if targetProtocol != OpenAIProtocol && targetProtocol != OpenAIResponsesProtocol {
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
