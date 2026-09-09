package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
)

type GatewayHandler struct {
	service  *gw.Service
	cfg      config.Gateway
	logger   *slog.Logger
	writer   *usageapp.Writer
	protocol string
}

func NewOpenAIGatewayHandler(service *gw.Service, cfg config.Gateway, logger *slog.Logger, writer *usageapp.Writer) *GatewayHandler {
	h := NewGatewayHandler(service, cfg, logger, writer)
	h.protocol = gw.OpenAIProtocol
	return h
}
func writeOpenAIError(w http.ResponseWriter, f *gw.Failure) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Zentrola-Error-Code", f.Code)
	w.WriteHeader(f.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": f.Message, "type": f.Type, "param": nil, "code": f.Code}})
}

func NewGatewayHandler(service *gw.Service, cfg config.Gateway, logger *slog.Logger, writers ...*usageapp.Writer) *GatewayHandler {
	h := &GatewayHandler{service: service, cfg: cfg, logger: logger}
	if len(writers) > 0 {
		h.writer = writers[0]
	}
	return h
}
func writeGatewayError(w http.ResponseWriter, failure *gw.Failure) {
	w.Header().Set("X-Zentrola-Error-Code", failure.Code)
	writeProtocolError(w, failure.Status, failure.Type, failure.Message)
}
func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/anthropic")
	protocol := gw.AnthropicProtocol
	inferencePath := "/v1/messages"
	if g.protocol == gw.OpenAIProtocol {
		path = r.URL.Path
		if path == "/v1/responses" {
			protocol = gw.OpenAIResponsesProtocol
			inferencePath = "/v1/responses"
		} else {
			protocol = gw.OpenAIProtocol
			inferencePath = "/v1/chat/completions"
		}
	}
	identity, _ := r.Context().Value(principalIdentityKey{}).(appsec.PrincipalIdentity)
	var trace *usage.Event
	var observer *gw.UsageObserver
	if path == inferencePath && r.Method == http.MethodPost && identity.ID > 0 && g.writer != nil {
		traceID, spanID := logging.TraceIDs(r.Context())
		trace = &usage.Event{OrganizationID: identity.OrganizationID, PrincipalID: identity.ID, RequestID: logging.RequestID(r.Context()), TraceID: traceID, SpanID: spanID, RequestAt: time.Now().UTC(), Status: usage.Failed, ErrorType: "INVALID_REQUEST"}
		trace.ClientProtocol = protocol
		defer func() {
			trace.CompletedAt = time.Now().UTC()
			if a := trace.Attempt; a != nil {
				a.CompletedAt = trace.CompletedAt
				a.Status = trace.Status
				a.ErrorType = trace.ErrorType
				if observer != nil {
					a.InputTokens, a.OutputTokens, a.CachedInputTokens = observer.Tokens(trace.Status == usage.Success)
				}
			}
			if trace.Attempt != nil {
				_ = g.writer.Submit(*trace)
			}
		}()
	}
	reject := func(failure *gw.Failure) {
		if trace != nil {
			trace.ErrorType = failure.Code
		}
		if protocol == gw.OpenAIProtocol || protocol == gw.OpenAIResponsesProtocol {
			writeOpenAIError(w, failure)
		} else {
			writeGatewayError(w, failure)
		}
	}
	interrupted := func(code string) {
		if trace != nil {
			trace.Status = usage.Failed
			trace.ErrorType = code
			if r.Context().Err() != nil {
				trace.Status = usage.Cancelled
				trace.ErrorType = "REQUEST_CANCELLED"
			}
		}
	}
	if (protocol == gw.OpenAIProtocol || protocol == gw.OpenAIResponsesProtocol) && path == "/v1/models" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			reject(&gw.Failure{Code: "METHOD_NOT_ALLOWED", Type: "invalid_request_error", Message: "Method not allowed.", Status: 405})
			return
		}
		if r.URL.RawQuery != "" {
			reject(gw.ErrInvalid)
			return
		}
		models, err := g.service.Models(r.Context(), identity)
		if err != nil {
			failure := gw.ErrUnavailable
			var known *gw.Failure
			if errors.As(err, &known) {
				failure = known
			}
			reject(failure)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
		return
	}
	if (protocol == gw.AnthropicProtocol && path != "/v1/messages" && path != "/v1/messages/count_tokens") ||
		((protocol == gw.OpenAIProtocol || protocol == gw.OpenAIResponsesProtocol) && path != inferencePath) {
		reject(&gw.Failure{Code: "NOT_FOUND", Type: "not_found_error", Message: "Route not found.", Status: 404})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		reject(&gw.Failure{Code: "METHOD_NOT_ALLOWED", Type: "invalid_request_error", Message: "Method not allowed.", Status: 405})
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity") {
		reject(&gw.Failure{Code: "UNSUPPORTED_MEDIA_TYPE", Type: "invalid_request_error", Message: "Use uncompressed application/json.", Status: 415})
		return
	}
	version, beta := "", ""
	if protocol == gw.AnthropicProtocol {
		version, beta, err = protocolHeaders(r)
	}
	if err != nil {
		reject(gw.ErrInvalid)
		return
	}
	nativeHeaders := make(map[string][]string)
	for name, values := range r.Header {
		if protocol == gw.AnthropicProtocol && strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			nativeHeaders[name] = append([]string(nil), values...)
		}
	}
	query := false
	if protocol == gw.AnthropicProtocol {
		query, err = parseBetaQuery(r)
	} else if r.URL.RawQuery != "" {
		err = gw.ErrInvalid
	}
	if err != nil {
		reject(gw.ErrInvalid)
		return
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(g.cfg.BodyReadTimeout))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, g.cfg.MaxBodyBytes))
	_ = controller.SetReadDeadline(time.Time{})
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			reject(&gw.Failure{Code: "REQUEST_TOO_LARGE", Type: "request_too_large", Message: "Request body too large.", Status: 413})
		} else {
			reject(gw.ErrInvalid)
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), g.cfg.RequestTimeout)
	defer cancel()
	addAccessLogFields(r.Context(), "protocol", protocol)
	upstreamStarted := time.Now()
	upstream, err := g.service.Forward(ctx, identity, gw.Request{Path: path, Version: version, Beta: beta, BetaQuery: query, Development: g.cfg.Development, Body: body, RequestID: logging.RequestID(r.Context()), ProtocolHeaders: nativeHeaders, Trace: trace, Protocol: protocol})
	addAccessLogFields(r.Context(), "upstream_headers_ms", time.Since(upstreamStarted).Milliseconds())
	addUsageRouteFields(r.Context(), trace)
	if err != nil {
		failure := gw.ErrUnavailable
		var known *gw.Failure
		if errors.As(err, &known) {
			failure = known
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			failure = gw.ErrTimeout
		}
		attributes := []any{"error_code", failure.Code}
		if g.cfg.Development {
			parse := gw.Parse
			if protocol == gw.OpenAIProtocol || protocol == gw.OpenAIResponsesProtocol {
				parse = gw.ParseOpenAI
			}
			parsed, parseErr := parse(body)
			attributes = append(attributes,
				"protocol", protocol,
				"path", path,
				"organization_id", identity.OrganizationID,
				"principal_id", identity.ID,
				"access_key_id", identity.AccessKeyID,
			)
			if parseErr == nil {
				attributes = append(attributes, "requested_model", parsed.Model)
			}
			if trace != nil && trace.ModelID > 0 {
				attributes = append(attributes, "model_id", trace.ModelID)
			}
		}
		g.logger.WarnContext(r.Context(), "gateway request rejected", attributes...)
		reject(failure)
		if trace != nil && errors.Is(r.Context().Err(), context.Canceled) {
			trace.Status = usage.Cancelled
			trace.ErrorType = "REQUEST_CANCELLED"
		}
		return
	}
	defer upstream.Body.Close()
	addAccessLogFields(r.Context(), "upstream_status", upstream.Status)
	if upstreamRequestID := safeUpstreamRequestID(upstream.Headers); upstreamRequestID != "" {
		addAccessLogFields(r.Context(), "upstream_request_id", upstreamRequestID)
	}
	if trace != nil {
		trace.ErrorType = "UPSTREAM_HTTP_" + strconv.Itoa(upstream.Status)
		if upstream.Status >= 200 && upstream.Status < 300 {
			trace.ErrorType = "UPSTREAM_RESPONSE_INCOMPLETE"
			media, _, _ := mime.ParseMediaType(http.Header(upstream.Headers).Get("Content-Type"))
			encoding := http.Header(upstream.Headers).Get("Content-Encoding")
			if encoding == "" || encoding == "identity" {
				observer = gw.NewUsageObserver(media == "text/event-stream")
				if protocol == gw.OpenAIResponsesProtocol {
					observer = gw.NewOpenAIResponsesUsageObserver(media == "text/event-stream")
				} else if protocol == gw.OpenAIProtocol {
					observer = gw.NewOpenAIUsageObserver(media == "text/event-stream")
				}
			}
		}
	}
	copyUpstreamHeaders(w.Header(), upstream.Headers)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	defer controller.SetWriteDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Now().Add(g.cfg.WriteTimeout))
	w.WriteHeader(upstream.Status)
	if err := controller.Flush(); err != nil {
		interrupted("CLIENT_WRITE_FAILED")
		cancel()
		return
	}
	// 固定缓冲区逐块转发，包括 SSE 未知事件、工具 JSON 增量和非流式原生错误。
	buffer := make([]byte, 32<<10)
	firstByteObserved := false
	for {
		n, readErr := upstream.Body.Read(buffer)
		if n > 0 {
			if !firstByteObserved {
				firstByteObserved = true
				addAccessLogFields(r.Context(), "first_byte_ms", accessLogElapsed(r.Context(), time.Now()))
			}
			if observer != nil {
				observer.Feed(buffer[:n])
			}
			_ = controller.SetWriteDeadline(time.Now().Add(g.cfg.WriteTimeout))
			if _, err := w.Write(buffer[:n]); err != nil {
				interrupted("CLIENT_WRITE_FAILED")
				cancel()
				return
			}
			if err := controller.Flush(); err != nil {
				interrupted("CLIENT_WRITE_FAILED")
				cancel()
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				interrupted("UPSTREAM_STREAM_INTERRUPTED")
				if trace != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
					trace.Status = usage.Failed
					trace.ErrorType = "UPSTREAM_TIMEOUT"
				}
				g.logger.WarnContext(r.Context(), "gateway response interrupted", "error_code", "UPSTREAM_STREAM_INTERRUPTED")
				// HTTP 状态和流已经开始，不能追加另一份 JSON 或伪造结束事件。
				panic(http.ErrAbortHandler)
			}
			if trace != nil && observer != nil {
				if observer.Complete() {
					trace.Status = usage.Success
					trace.ErrorType = ""
				} else {
					interrupted(observer.ErrorCode())
				}
				addUsageTokenFields(r.Context(), trace, observer)
			}
			if trace != nil && observer == nil && upstream.Status >= 200 && upstream.Status < 300 {
				trace.Status = usage.Success
				trace.ErrorType = ""
			}
			return
		}
	}
}

func addUsageRouteFields(ctx context.Context, event *usage.Event) {
	if event == nil || event.Attempt == nil {
		return
	}
	attempt := event.Attempt
	addAccessLogFields(ctx,
		"model_id", attempt.ModelID,
		"provider_id", attempt.ProviderID,
		"provider_model_id", attempt.ProviderModelID,
		"resource_id", attempt.ResourceID,
	)
}

func addUsageTokenFields(ctx context.Context, event *usage.Event, observer *gw.UsageObserver) {
	if event == nil || observer == nil {
		return
	}
	input, output, cached := observer.Tokens(event.Status == usage.Success)
	if input != nil {
		addAccessLogFields(ctx, "input_tokens", *input)
	}
	if output != nil {
		addAccessLogFields(ctx, "output_tokens", *output)
	}
	if cached != nil {
		addAccessLogFields(ctx, "cached_input_tokens", *cached)
	}
}

func safeUpstreamRequestID(headers map[string][]string) string {
	value := http.Header(headers).Get("Request-Id")
	if value == "" {
		value = http.Header(headers).Get("X-Request-Id")
	}
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return ""
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return ""
		}
	}
	return value
}

func protocolHeaders(r *http.Request) (string, string, error) {
	for _, line := range r.Header.Values("Connection") {
		for _, name := range strings.Split(line, ",") {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "anthropic-") {
				return "", "", gw.ErrInvalid
			}
		}
	}
	total := 0
	for name, values := range r.Header {
		if !strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			continue
		}
		for _, value := range values {
			total += len(name) + len(value)
			for _, ch := range value {
				if ch < 32 || ch > 126 {
					return "", "", gw.ErrInvalid
				}
			}
		}
	}
	if total > 16<<10 {
		return "", "", gw.ErrInvalid
	}
	versions := r.Header.Values("Anthropic-Version")
	if len(versions) > 1 {
		return "", "", gw.ErrInvalid
	}
	version := ""
	if len(versions) == 1 {
		version = versions[0]
		if len(version) != 10 {
			return "", "", gw.ErrInvalid
		}
	}
	betas := r.Header.Values("Anthropic-Beta")
	beta := strings.Join(betas, ",")
	if len(beta) > 8192 {
		return "", "", gw.ErrInvalid
	}
	for _, value := range []string{version, beta} {
		for _, ch := range value {
			if ch < 32 || ch > 126 {
				return "", "", gw.ErrInvalid
			}
		}
	}
	return version, beta, nil
}
func parseBetaQuery(r *http.Request) (bool, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return false, gw.ErrInvalid
	}
	for key, vs := range values {
		if key != "beta" || len(vs) != 1 || vs[0] != "true" {
			return false, gw.ErrInvalid
		}
	}
	return values.Get("beta") == "true", nil
}
func copyUpstreamHeaders(dst http.Header, src map[string][]string) {
	blocked := map[string]bool{}
	for _, line := range http.Header(src).Values("Connection") {
		for _, name := range strings.Split(line, ",") {
			blocked[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	for key, values := range src {
		name := strings.ToLower(key)
		if blocked[name] {
			continue
		}
		if name == "content-type" || name == "content-encoding" || name == "retry-after" || name == "request-id" || strings.HasPrefix(name, "anthropic-ratelimit-") || strings.HasPrefix(name, "x-ratelimit-") {
			for _, value := range values {
				dst.Add(key, value)
			}
		}
	}
}
