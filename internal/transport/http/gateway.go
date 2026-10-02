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

type gatewayRoute struct {
	path          string
	protocol      string
	inferencePath string
}

func (g *GatewayHandler) resolveRoute(r *http.Request) gatewayRoute {
	route := gatewayRoute{
		path: strings.TrimPrefix(r.URL.Path, "/anthropic"), protocol: gw.AnthropicProtocol, inferencePath: "/v1/messages",
	}
	if g.protocol != gw.OpenAIProtocol {
		return route
	}
	route.path = r.URL.Path
	switch route.path {
	case "/v1/responses":
		route.protocol, route.inferencePath = gw.OpenAIResponsesProtocol, "/v1/responses"
	case "/v1/images/generations":
		route.protocol, route.inferencePath = gw.OpenAIImagesProtocol, "/v1/images/generations"
	default:
		route.protocol, route.inferencePath = gw.OpenAIProtocol, "/v1/chat/completions"
	}
	return route
}

func (g *GatewayHandler) serveModelList(
	w http.ResponseWriter,
	r *http.Request,
	identity appsec.PrincipalIdentity,
	route gatewayRoute,
	reject func(*gw.Failure),
) bool {
	if !gw.IsOpenAIProtocol(route.protocol) || route.path != "/v1/models" {
		return false
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		reject(&gw.Failure{Code: "METHOD_NOT_ALLOWED", Type: "invalid_request_error", Message: "Method not allowed.", Status: 405})
		return true
	}
	if r.URL.RawQuery != "" {
		reject(gw.ErrInvalid)
		return true
	}
	g.logger.InfoContext(r.Context(), "gateway model list request started",
		"protocol", route.protocol, "principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
	models, err := g.service.Models(r.Context(), identity)
	if err != nil {
		failure := gw.ErrUnavailable
		var known *gw.Failure
		if errors.As(err, &known) {
			failure = known
		}
		reject(failure)
		return true
	}
	g.logger.InfoContext(r.Context(), "gateway model list request completed",
		"protocol", route.protocol, "principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		"model_count", len(models))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
	return true
}

type parsedGatewayRequest struct {
	version       string
	beta          string
	betaQuery     bool
	body          []byte
	nativeHeaders map[string][]string
}

func (g *GatewayHandler) parseForwardRequest(w http.ResponseWriter, r *http.Request, protocol string) (parsedGatewayRequest, *gw.Failure) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || (r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity") {
		return parsedGatewayRequest{}, &gw.Failure{Code: "UNSUPPORTED_MEDIA_TYPE", Type: "invalid_request_error", Message: "Use uncompressed application/json.", Status: 415}
	}
	request := parsedGatewayRequest{nativeHeaders: make(map[string][]string)}
	if protocol == gw.AnthropicProtocol {
		request.version, request.beta, err = protocolHeaders(r)
	}
	if err != nil {
		return parsedGatewayRequest{}, gw.ErrInvalid
	}
	for name, values := range r.Header {
		if protocol == gw.AnthropicProtocol && strings.HasPrefix(strings.ToLower(name), "anthropic-") {
			request.nativeHeaders[name] = append([]string(nil), values...)
		}
	}
	if protocol == gw.AnthropicProtocol {
		request.betaQuery, err = parseBetaQuery(r)
	} else if r.URL.RawQuery != "" {
		err = gw.ErrInvalid
	}
	if err != nil {
		return parsedGatewayRequest{}, gw.ErrInvalid
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(g.cfg.BodyReadTimeout))
	request.body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, g.cfg.MaxBodyBytes))
	_ = controller.SetReadDeadline(time.Time{})
	if err == nil {
		return request, nil
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return parsedGatewayRequest{}, &gw.Failure{Code: "REQUEST_TOO_LARGE", Type: "request_too_large", Message: "Request body too large.", Status: 413}
	}
	return parsedGatewayRequest{}, gw.ErrInvalid
}

func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := g.resolveRoute(r)
	path, protocol, inferencePath := route.path, route.protocol, route.inferencePath
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
		if gw.IsOpenAIProtocol(protocol) {
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
	if g.serveModelList(w, r, identity, route, reject) {
		return
	}
	if (protocol == gw.AnthropicProtocol && path != "/v1/messages" && path != "/v1/messages/count_tokens") ||
		(gw.IsOpenAIProtocol(protocol) && path != inferencePath) {
		reject(&gw.Failure{Code: "NOT_FOUND", Type: "not_found_error", Message: "Route not found.", Status: 404})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		reject(&gw.Failure{Code: "METHOD_NOT_ALLOWED", Type: "invalid_request_error", Message: "Method not allowed.", Status: 405})
		return
	}
	parsed, failure := g.parseForwardRequest(w, r, protocol)
	if failure != nil {
		reject(failure)
		return
	}
	version, beta, query, body, nativeHeaders := parsed.version, parsed.beta, parsed.betaQuery, parsed.body, parsed.nativeHeaders
	controller := http.NewResponseController(w)
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
			if gw.IsOpenAIProtocol(protocol) {
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
	(&gatewayResponsePump{
		handler: g, writer: w, request: r, upstream: upstream, controller: controller,
		protocol: protocol, trace: trace, observer: &observer, upstreamContext: ctx,
		cancel: cancel, retainCompletion: retainOpenAICompletion,
		completionDrainStarted: &completionDrainStarted, startCompletionDrain: startCompletionDrain,
		interrupted: interrupted,
	}).run()
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
