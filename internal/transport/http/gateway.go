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
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

const openAICompletionDrainLimit = 2 * time.Second

// maxRequestBodyPreallocation 是按客户端声明的 Content-Length 预分配请求体缓冲区的上限。
// 声明接近 GATEWAY_MAX_BODY_BYTES 但慢速发送的连接不能立即占用整块内存，超出部分按实际到达的数据增长。
const maxRequestBodyPreallocation = 1 << 20

type GatewayHandler struct {
	service  *gw.Service
	cfg      GatewayOptions
	logger   *slog.Logger
	writer   *usageapp.Writer
	protocol string
}

type GatewayOptions struct {
	MaxBodyBytes    int64
	RequestTimeout  time.Duration
	BodyReadTimeout time.Duration
	WriteTimeout    time.Duration
	Development     bool
}

func NewOpenAIGatewayHandler(service *gw.Service, cfg GatewayOptions, logger *slog.Logger, writer *usageapp.Writer) *GatewayHandler {
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

func NewGatewayHandler(service *gw.Service, cfg GatewayOptions, logger *slog.Logger, writers ...*usageapp.Writer) *GatewayHandler {
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

func failureWithRequestedModel(failure *gw.Failure, protocol string, body []byte) *gw.Failure {
	if failure.Code != gw.ErrModelUnknown.Code && failure.Code != gw.ErrRoute.Code {
		return failure
	}
	parse := gw.Parse
	if gw.IsOpenAIProtocol(protocol) {
		parse = gw.ParseOpenAI
	}
	parsed, err := parse(body)
	if err != nil {
		return failure
	}
	detailed := *failure
	model := strconv.Quote(parsed.Model)
	if failure.Code == gw.ErrModelUnknown.Code {
		detailed.Message = "The requested model " + model + " is not configured."
	} else {
		detailed.Message = "No active provider route is available for model " + model + ". Enable a provider and configure its endpoint and model mapping."
	}
	return &detailed
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
	if route.path != "/v1/models" {
		return false
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		reject(&gw.Failure{Code: "METHOD_NOT_ALLOWED", Type: "invalid_request_error", Message: "Method not allowed.", Status: 405})
		return true
	}
	if route.protocol == gw.AnthropicProtocol {
		page, ok := parseAnthropicModelPage(r.URL.RawQuery)
		if !ok {
			reject(gw.ErrInvalid)
			return true
		}
		g.logger.InfoContext(r.Context(), "gateway model list request started",
			"protocol", route.protocol, "principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
		g.serveAnthropicModelList(w, r, identity, page, reject)
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

// anthropicModelPage 对应 Anthropic GET /v1/models 的分页参数；limit 为 0 表示返回全部。
type anthropicModelPage struct {
	limit             int
	beforeID, afterID string
}

func parseAnthropicModelPage(rawQuery string) (anthropicModelPage, bool) {
	var page anthropicModelPage
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return page, false
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" {
			return page, false
		}
		switch key {
		case "limit":
			limit, err := strconv.Atoi(values[0])
			if err != nil || limit < 1 || limit > 1000 {
				return page, false
			}
			page.limit = limit
		case "before_id":
			page.beforeID = values[0]
		case "after_id":
			page.afterID = values[0]
		default:
			return page, false
		}
	}
	return page, page.beforeID == "" || page.afterID == ""
}

// apply 在按 ID 排序的模型列表上截取一页；游标 ID 不在列表中时返回 false。
func (p anthropicModelPage) apply(data []AnthropicModel) ([]AnthropicModel, bool, bool) {
	index := func(id string) int {
		return slices.IndexFunc(data, func(model AnthropicModel) bool { return model.ID == id })
	}
	start, end := 0, len(data)
	if p.afterID != "" {
		if start = index(p.afterID) + 1; start == 0 {
			return nil, false, false
		}
	}
	if p.beforeID != "" {
		if end = index(p.beforeID); end < 0 {
			return nil, false, false
		}
	}
	page := data[start:end]
	if p.limit == 0 || len(page) <= p.limit {
		return page, false, true
	}
	if p.beforeID != "" {
		return page[len(page)-p.limit:], true, true
	}
	return page[:p.limit], true, true
}

func (g *GatewayHandler) serveAnthropicModelList(w http.ResponseWriter, r *http.Request, identity appsec.PrincipalIdentity, page anthropicModelPage, reject func(*gw.Failure)) {
	models, err := g.service.AnthropicModels(r.Context(), identity)
	if err != nil {
		failure := gw.ErrUnavailable
		var known *gw.Failure
		if errors.As(err, &known) {
			failure = known
		}
		reject(failure)
		return
	}
	data := make([]AnthropicModel, 0, len(models))
	for _, model := range models {
		data = append(data, AnthropicModel{
			ID: model.Code, Type: "model", DisplayName: model.DisplayName,
			CreatedAt: model.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	data, hasMore, ok := page.apply(data)
	if !ok {
		reject(gw.ErrInvalid)
		return
	}
	var firstID, lastID *string
	if len(data) > 0 {
		firstID, lastID = &data[0].ID, &data[len(data)-1].ID
	}
	g.logger.InfoContext(r.Context(), "gateway model list request completed",
		"protocol", gw.AnthropicProtocol, "principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		"model_count", len(data))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(AnthropicModelsResponse{
		Data: data, HasMore: hasMore, FirstID: firstID, LastID: lastID,
	})
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
	request.body, err = readRequestBody(http.MaxBytesReader(w, r.Body, g.cfg.MaxBodyBytes), r.ContentLength, g.cfg.MaxBodyBytes)
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

// readRequestBody 在 Content-Length 可信且不超过上限时按声明长度预分配缓冲区，避免
// io.ReadAll 在大请求上多次扩容复制。预分配不超过 maxRequestBodyPreallocation，之后按
// 实际到达的数据倍增且不超过声明长度。实际长度仍由 reader（MaxBytesReader）约束。
func readRequestBody(reader io.Reader, contentLength, limit int64) ([]byte, error) {
	if contentLength <= 0 || contentLength > limit {
		return io.ReadAll(reader)
	}
	body := make([]byte, 0, min(contentLength, maxRequestBodyPreallocation))
	for int64(len(body)) < contentLength {
		if len(body) == cap(body) {
			body = slices.Grow(body, int(min(int64(cap(body)), contentLength-int64(len(body)))))
		}
		n, err := reader.Read(body[len(body):min(int64(cap(body)), contentLength)])
		body = body[:len(body)+n]
		if err != nil {
			// 与 io.ReadFull 一致：未读到任何数据时视为空请求体，读到部分数据后结束视为截断；
			// 最后一次 Read 可能同时返回剩余数据与 io.EOF，读满声明长度时不算截断。
			if errors.Is(err, io.EOF) {
				err = nil
				if len(body) > 0 && int64(len(body)) < contentLength {
					err = io.ErrUnexpectedEOF
				}
			}
			return body, err
		}
	}
	// 声明长度之后仍有数据时继续读取，由 MaxBytesReader 负责超限判定。
	rest, err := io.ReadAll(reader)
	if len(rest) > 0 {
		body = append(body, rest...)
	}
	return body, err
}

func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := g.resolveRoute(r)
	path, protocol, inferencePath := route.path, route.protocol, route.inferencePath
	identity, _ := r.Context().Value(principalIdentityKey{}).(appsec.PrincipalIdentity)
	var trace *usage.Event
	var observer *gw.UsageObserver
	if path == inferencePath && r.Method == http.MethodPost && identity.ID > 0 {
		traceID, spanID := traceIDs(r.Context())
		trace = &usage.Event{PrincipalID: identity.ID, RequestID: requestIDFromContext(r.Context()), TraceID: traceID, SpanID: spanID, RequestAt: time.Now().UTC(), Status: usage.Failed, ErrorType: "INVALID_REQUEST"}
		trace.ClientProtocol = protocol
		defer func() {
			trace.CompletedAt = time.Now().UTC()
			if a := trace.Attempt; a != nil {
				a.CompletedAt = trace.CompletedAt
				a.Status = trace.Status
				a.ErrorType = trace.ErrorType
				if observer != nil {
					a.InputTokens, a.OutputTokens, a.CachedInputTokens = observer.Tokens(trace.Status == usage.Success)
					a.CacheCreationInputTokens = observer.CacheCreationTokens()
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
	clientSessionID, clientAgentID := gatewayClientSession(r, protocol)
	g.logger.InfoContext(r.Context(), "gateway forwarding started",
		"protocol", protocol, "path", path,
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID)
	upstreamStarted := time.Now()
	upstream, err := g.service.Forward(ctx, identity, gw.Request{Path: path, Version: version, Beta: beta, BetaQuery: query, Development: g.cfg.Development, Body: body, RequestID: requestIDFromContext(r.Context()), ProtocolHeaders: nativeHeaders, Trace: trace, Protocol: protocol, ClientSessionID: clientSessionID, ClientAgentID: clientAgentID})
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
		failure = failureWithRequestedModel(failure, protocol, body)
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
	attributes := []any{
		"protocol", protocol, "upstream_status", upstream.Status,
		"principal_id", identity.ID, "access_key_id", identity.AccessKeyID,
		"upstream_headers_ms", time.Since(upstreamStarted).Milliseconds(),
	}
	if route, ok := upstream.RouteSelection(); ok {
		attributes = append(attributes,
			"provider_id", route.ProviderID,
			"provider_model_id", route.ProviderModelID,
			"resource_id", route.ResourceID,
			"auth_type", route.AuthType,
			"session_routed", clientSessionID != "",
		)
	}
	g.logger.InfoContext(r.Context(), "gateway upstream response opened", attributes...)
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
		"request_id", requestIDFromContext(ctx),
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
	if cacheCreation := observer.CacheCreationTokens(); cacheCreation != nil {
		addAccessLogFields(ctx, "cache_creation_input_tokens", *cacheCreation)
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

func gatewayClientSession(r *http.Request, protocol string) (string, string) {
	if protocol == gw.AnthropicProtocol {
		id := singleSessionHeader(r.Header.Values("X-Claude-Code-Session-Id"))
		if id == "" {
			return "", ""
		}
		return id, singleSessionHeader(r.Header.Values("X-Claude-Code-Agent-Id"))
	}
	if protocol == gw.OpenAIResponsesProtocol {
		return singleSessionHeader(r.Header.Values("Session-Id")), ""
	}
	return "", ""
}

func singleSessionHeader(values []string) string {
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 128 || strings.TrimSpace(values[0]) != values[0] {
		return ""
	}
	for _, char := range values[0] {
		if char < 33 || char > 126 {
			return ""
		}
	}
	return values[0]
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
