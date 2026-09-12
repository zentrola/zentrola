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
	"sync"
	"sync/atomic"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
)

const openAICompletionDrainLimit = 2 * time.Second

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
	message := failure.Message
	if failure.Code != "" {
		message += " [" + failure.Code + "]"
	}
	writeProtocolError(w, failure.Status, failure.Type, message)
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
	if path == inferencePath && r.Method == http.MethodPost && identity.ID > 0 {
		traceID, spanID := logging.TraceIDs(r.Context())
		trace = &usage.Event{PrincipalID: identity.ID, RequestID: logging.RequestID(r.Context()), TraceID: traceID, SpanID: spanID, RequestAt: time.Now().UTC(), Status: usage.Failed, ErrorType: "INVALID_REQUEST"}
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
			if g.writer != nil && (trace.Attempt != nil || len(trace.Attempts) > 0) {
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
		g.logger.InfoContext(r.Context(), "gateway model list request started",
			"protocol", protocol, "principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
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
		g.logger.InfoContext(r.Context(), "gateway model list request completed",
			"protocol", protocol, "principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
			"model_count", len(models))
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
	// Responses 客户端可能在收到工具调用后立即关闭下游连接，而最终 usage
	// 位于紧随其后的 response.completed。上游上下文因此不能被客户端取消
	// 直接截断；响应尚未打开时仍立即取消，打开后只保留一个很短的收尾窗口。
	retainOpenAICompletion := protocol == gw.OpenAIResponsesProtocol
	upstreamParent := r.Context()
	if retainOpenAICompletion {
		upstreamParent = context.WithoutCancel(r.Context())
	}
	ctx, cancel := context.WithTimeout(upstreamParent, g.cfg.RequestTimeout)
	defer cancel()
	var upstreamOpened, completionDrainStarted atomic.Bool
	var startCompletionDrain func(string)
	if retainOpenAICompletion {
		drainTimeout := min(g.cfg.WriteTimeout, openAICompletionDrainLimit)
		if drainTimeout <= 0 {
			drainTimeout = openAICompletionDrainLimit
		}
		var drainOnce sync.Once
		startCompletionDrain = func(reason string) {
			drainOnce.Do(func() {
				completionDrainStarted.Store(true)
				g.logOpenAIResponsesDiagnostic(r.Context(), slog.LevelWarn, "completion_drain_started", nil,
					"reason", reason,
					"upstream_opened", upstreamOpened.Load(),
					"drain_timeout_ms", drainTimeout.Milliseconds(),
					"observer_snapshot_deferred", true,
				)
				time.AfterFunc(drainTimeout, cancel)
			})
		}
		stopClientCancellation := context.AfterFunc(r.Context(), func() {
			if upstreamOpened.Load() {
				startCompletionDrain("request_context_cancelled")
				return
			}
			g.logOpenAIResponsesDiagnostic(r.Context(), slog.LevelWarn, "cancelled_before_upstream_opened", nil)
			cancel()
		})
		defer stopClientCancellation()
	}
	addAccessLogFields(r.Context(), "protocol", protocol)
	g.logger.InfoContext(r.Context(), "gateway forwarding started",
		"protocol", protocol, "path", path,
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
	upstreamStarted := time.Now()
	upstream, err := g.service.Forward(ctx, identity, gw.Request{Path: path, Version: version, Beta: beta, BetaQuery: query, Development: g.cfg.Development, Body: body, RequestID: logging.RequestID(r.Context()), ProtocolHeaders: nativeHeaders, Trace: trace, Protocol: protocol})
	if retainOpenAICompletion && err == nil && upstream != nil {
		upstreamOpened.Store(true)
	}
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
		attributes := []any{
			"error_code", failure.Code, "protocol", protocol,
			"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		}
		if g.cfg.Development {
			parse := gw.Parse
			if protocol == gw.OpenAIProtocol || protocol == gw.OpenAIResponsesProtocol {
				parse = gw.ParseOpenAI
			}
			parsed, parseErr := parse(body)
			attributes = append(attributes,
				"path", path,
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
	g.logger.InfoContext(r.Context(), "gateway upstream response opened",
		"protocol", protocol, "upstream_status", upstream.Status,
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		"upstream_headers_ms", time.Since(upstreamStarted).Milliseconds())
	addAccessLogFields(r.Context(), "upstream_status", upstream.Status)
	if upstreamRequestID := safeUpstreamRequestID(upstream.Headers); upstreamRequestID != "" {
		addAccessLogFields(r.Context(), "upstream_request_id", upstreamRequestID)
	}
	requestedStream := upstream.RequestedStream
	responseContentType := http.Header(upstream.Headers).Get("Content-Type")
	responseMedia, _, _ := mime.ParseMediaType(responseContentType)
	responseStream := responseMedia == "text/event-stream"
	streamInferred := false
	if responseContentType == "" && protocol == gw.OpenAIResponsesProtocol && requestedStream {
		responseStream = true
		streamInferred = true
	}
	if trace != nil {
		trace.ErrorType = "UPSTREAM_HTTP_" + strconv.Itoa(upstream.Status)
		if upstream.Status >= 200 && upstream.Status < 300 {
			trace.ErrorType = "UPSTREAM_RESPONSE_INCOMPLETE"
			encoding := http.Header(upstream.Headers).Get("Content-Encoding")
			if encoding == "" || encoding == "identity" {
				observer = gw.NewUsageObserver(responseStream)
				if protocol == gw.OpenAIResponsesProtocol {
					if responseContentType == "" {
						observer = gw.NewAutoOpenAIResponsesUsageObserver(requestedStream)
					} else {
						observer = gw.NewOpenAIResponsesUsageObserver(responseStream)
					}
				} else if protocol == gw.OpenAIProtocol {
					observer = gw.NewOpenAIUsageObserver(responseStream)
				}
			}
		}
	}
	if retainOpenAICompletion {
		g.logOpenAIResponsesDiagnostic(r.Context(), slog.LevelInfo, "upstream_opened", observer,
			"upstream_status", upstream.Status,
			"content_type", responseContentType,
			"content_encoding", http.Header(upstream.Headers).Get("Content-Encoding"),
			"media_type", responseMedia,
			"requested_stream", requestedStream,
			"stream_inferred", streamInferred,
		)
	}
	copyUpstreamHeaders(w.Header(), upstream.Headers)
	if responseStream && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	defer controller.SetWriteDeadline(time.Time{})
	_ = controller.SetWriteDeadline(time.Now().Add(g.cfg.WriteTimeout))
	w.WriteHeader(upstream.Status)
	downstreamOpen := true
	if err := controller.Flush(); err != nil {
		interrupted("CLIENT_WRITE_FAILED")
		if retainOpenAICompletion && observer != nil {
			downstreamOpen = false
			startCompletionDrain("initial_flush_failed")
		} else {
			cancel()
			return
		}
	}
	// 固定缓冲区逐块转发，包括 SSE 未知事件、工具 JSON 增量和非流式原生错误。
	buffer := make([]byte, 32<<10)
	firstByteObserved := false
	var upstreamBytes int64
	for {
		n, readErr := upstream.Body.Read(buffer)
		if n > 0 {
			upstreamBytes += int64(n)
			if !firstByteObserved {
				firstByteObserved = true
				addAccessLogFields(r.Context(), "first_byte_ms", accessLogElapsed(r.Context(), time.Now()))
			}
			if observer != nil {
				observer.Feed(buffer[:n])
			}
			if downstreamOpen {
				_ = controller.SetWriteDeadline(time.Now().Add(g.cfg.WriteTimeout))
				if _, err := w.Write(buffer[:n]); err != nil {
					interrupted("CLIENT_WRITE_FAILED")
					if retainOpenAICompletion && observer != nil {
						downstreamOpen = false
						startCompletionDrain("client_write_failed")
					} else {
						cancel()
						return
					}
				} else if err := controller.Flush(); err != nil {
					interrupted("CLIENT_WRITE_FAILED")
					if retainOpenAICompletion && observer != nil {
						downstreamOpen = false
						startCompletionDrain("client_flush_failed")
					} else {
						cancel()
						return
					}
				}
			}
			// 流式协议的完成事件本身就是响应终点。客户端收到终点后可能立即
			// 关闭连接，使下一次上游读取返回 context canceled；不要因此把
			// 已完整交付的调用覆盖成 CANCELLED。
			if trace != nil && observer != nil && observer.Complete() {
				trace.Status = usage.Success
				trace.ErrorType = ""
				addUsageTokenFields(r.Context(), trace, observer)
				if retainOpenAICompletion {
					g.logOpenAIResponsesDiagnostic(r.Context(), slog.LevelInfo, "completion_observed", observer,
						"upstream_bytes", upstreamBytes,
						"downstream_open", downstreamOpen,
						"request_context_cancelled", r.Context().Err() != nil,
					)
				}
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				if retainOpenAICompletion && completionDrainStarted.Load() && errors.Is(ctx.Err(), context.Canceled) {
					if r.Context().Err() != nil {
						interrupted("UPSTREAM_STREAM_INTERRUPTED")
					}
					addUsageTokenFields(r.Context(), trace, observer)
					g.logOpenAIResponsesDiagnostic(r.Context(), slog.LevelWarn, "completion_drain_ended_without_completion", observer,
						"upstream_bytes", upstreamBytes,
						"downstream_open", downstreamOpen,
						"request_context_cancelled", r.Context().Err() != nil,
					)
					return
				}
				interrupted("UPSTREAM_STREAM_INTERRUPTED")
				if trace != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
					trace.Status = usage.Failed
					trace.ErrorType = "UPSTREAM_TIMEOUT"
				}
				g.logger.WarnContext(r.Context(), "gateway response interrupted", "error_code", "UPSTREAM_STREAM_INTERRUPTED")
				if retainOpenAICompletion {
					g.logOpenAIResponsesDiagnostic(r.Context(), slog.LevelWarn, "upstream_read_interrupted", observer,
						"upstream_bytes", upstreamBytes,
						"downstream_open", downstreamOpen,
						"request_context_cancelled", r.Context().Err() != nil,
						"upstream_context_cancelled", errors.Is(ctx.Err(), context.Canceled),
						"upstream_context_timed_out", errors.Is(ctx.Err(), context.DeadlineExceeded),
					)
				}
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
			if retainOpenAICompletion {
				level, stage := slog.LevelInfo, "upstream_eof"
				if observer == nil || !observer.Complete() {
					level, stage = slog.LevelWarn, "upstream_eof_without_completion"
				}
				g.logOpenAIResponsesDiagnostic(r.Context(), level, stage, observer,
					"upstream_bytes", upstreamBytes,
					"downstream_open", downstreamOpen,
					"request_context_cancelled", r.Context().Err() != nil,
				)
			}
			return
		}
	}
}

func (g *GatewayHandler) logOpenAIResponsesDiagnostic(ctx context.Context, level slog.Level, stage string, observer *gw.UsageObserver, fields ...any) {
	if !g.cfg.Development {
		return
	}
	attributes := []any{
		"request_id", logging.RequestID(ctx),
		"stage", stage,
		"observer_enabled", observer != nil,
	}
	if observer != nil {
		diagnostic := observer.Diagnostics()
		attributes = append(attributes,
			"stream", diagnostic.Stream,
			"started", diagnostic.Started,
			"stopped", diagnostic.Stopped,
			"failed", diagnostic.Failed,
			"bad_usage", diagnostic.BadUsage,
			"json_complete", diagnostic.JSONComplete,
			"json_invalid", diagnostic.JSONInvalid,
			"input_seen", diagnostic.InputSeen,
			"output_seen", diagnostic.OutputSeen,
			"cached_seen", diagnostic.CachedSeen,
			"event_count", diagnostic.EventCount,
			"last_event", diagnostic.LastEvent,
			"complete", observer.Complete(),
		)
		input, output, cached := observer.Tokens(observer.Complete())
		if input != nil {
			attributes = append(attributes, "input_tokens", *input)
		}
		if output != nil {
			attributes = append(attributes, "output_tokens", *output)
		}
		if cached != nil {
			attributes = append(attributes, "cached_input_tokens", *cached)
		}
	}
	attributes = append(attributes, fields...)
	g.logger.Log(ctx, level, "openai responses diagnostic", attributes...)
}

func addUsageRouteFields(ctx context.Context, event *usage.Event) {
	if event == nil {
		return
	}
	attempt := event.Attempt
	if attempt == nil && len(event.Attempts) > 0 {
		attempt = &event.Attempts[len(event.Attempts)-1]
	}
	if attempt == nil {
		return
	}
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
