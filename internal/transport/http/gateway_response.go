package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

// gatewayResponsePump owns the response-side state machine after the upstream
// headers have opened. Keeping it separate makes client disconnect, completion
// drain and protocol validation transitions testable without growing ServeHTTP.
type gatewayResponsePump struct {
	handler                *GatewayHandler
	writer                 http.ResponseWriter
	request                *http.Request
	upstream               *gw.Response
	controller             *http.ResponseController
	protocol               string
	trace                  *usage.Event
	observer               **gw.UsageObserver
	upstreamContext        context.Context
	cancel                 context.CancelFunc
	retainCompletion       bool
	completionDrainStarted *atomic.Bool
	startCompletionDrain   func(string)
	interrupted            func(string)
	downstreamOpen         bool
	upstreamBytes          int64
	firstByteObserved      bool
	usageFilter            *gw.ChatUsageChunkFilter
}

func (p *gatewayResponsePump) run() {
	defer p.controller.SetWriteDeadline(time.Time{})
	responseStream, contentType, mediaType, streamInferred := p.configureObserver()
	p.configureUsageFilter(responseStream)
	if p.retainCompletion {
		p.handler.logOpenAIResponsesDiagnostic(p.request.Context(), slog.LevelInfo, "upstream_opened", *p.observer,
			"upstream_status", p.upstream.Status,
			"content_type", contentType,
			"content_encoding", http.Header(p.upstream.Headers).Get("Content-Encoding"),
			"media_type", mediaType,
			"requested_stream", p.upstream.RequestedStream,
			"stream_inferred", streamInferred,
		)
	}
	p.openDownstream(responseStream)
	if !p.downstreamOpen && (!p.retainCompletion || *p.observer == nil) {
		return
	}
	p.copyBody()
}

func (p *gatewayResponsePump) configureObserver() (bool, string, string, bool) {
	contentType := http.Header(p.upstream.Headers).Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	responseStream := mediaType == "text/event-stream"
	streamInferred := false
	if contentType == "" && (p.protocol == gw.OpenAIResponsesProtocol || p.protocol == gw.OpenAIImagesProtocol) && p.upstream.RequestedStream {
		responseStream = true
		streamInferred = true
	}
	if p.trace == nil {
		return responseStream, contentType, mediaType, streamInferred
	}
	p.trace.ErrorType = p.upstream.ErrorType
	if p.trace.ErrorType == "" {
		p.trace.ErrorType = "UPSTREAM_HTTP_" + strconv.Itoa(p.upstream.Status)
	}
	if p.upstream.Status < 200 || p.upstream.Status >= 300 {
		return responseStream, contentType, mediaType, streamInferred
	}
	p.trace.ErrorType = "UPSTREAM_RESPONSE_INCOMPLETE"
	encoding := http.Header(p.upstream.Headers).Get("Content-Encoding")
	if encoding != "" && encoding != "identity" {
		return responseStream, contentType, mediaType, streamInferred
	}
	switch p.protocol {
	case gw.OpenAIResponsesProtocol:
		if contentType == "" {
			*p.observer = gw.NewAutoOpenAIResponsesUsageObserver(p.upstream.RequestedStream)
		} else {
			*p.observer = gw.NewOpenAIResponsesUsageObserver(responseStream)
		}
	case gw.OpenAIProtocol:
		*p.observer = gw.NewOpenAIUsageObserver(responseStream)
	case gw.AnthropicProtocol:
		*p.observer = gw.NewUsageObserver(responseStream)
	}
	// 经兼容转换的 Anthropic 上游：转换后的 OpenAI usage 不含缓存写入，需由转换层补齐。
	(*p.observer).UseCompatibilityUsage(p.upstream.CompatibilityUsage)
	return responseStream, contentType, mediaType, streamInferred
}

// configureUsageFilter 在 include_usage 由网关代为开启时剥离上游追加的 usage chunk。
// observer 仍读取原始字节，计量不受剥离影响。
func (p *gatewayResponsePump) configureUsageFilter(responseStream bool) {
	encoding := http.Header(p.upstream.Headers).Get("Content-Encoding")
	if !p.upstream.OmitUsageChunk || !responseStream || p.upstream.Status < 200 || p.upstream.Status >= 300 ||
		(encoding != "" && encoding != "identity") {
		return
	}
	p.usageFilter = gw.NewChatUsageChunkFilter()
}

func (p *gatewayResponsePump) openDownstream(responseStream bool) {
	copyUpstreamHeaders(p.writer.Header(), p.upstream.Headers)
	if responseStream && p.writer.Header().Get("Content-Type") == "" {
		p.writer.Header().Set("Content-Type", "text/event-stream")
	}
	p.writer.Header().Set("Cache-Control", "no-store")
	p.writer.Header().Set("X-Accel-Buffering", "no")
	_ = p.controller.SetWriteDeadline(time.Now().Add(p.handler.cfg.WriteTimeout))
	p.writer.WriteHeader(p.upstream.Status)
	p.downstreamOpen = true
	if err := p.controller.Flush(); err != nil {
		p.clientWriteFailed("initial_flush_failed")
	}
}

func (p *gatewayResponsePump) copyBody() {
	buffer := make([]byte, 32<<10)
	for {
		n, readErr := p.upstream.Body.Read(buffer)
		if n > 0 && p.consume(buffer[:n]) {
			return
		}
		if readErr != nil {
			if readErr == io.EOF {
				p.flushUsageFilter()
			}
			p.finish(readErr)
			return
		}
	}
}

func (p *gatewayResponsePump) consume(chunk []byte) bool {
	p.upstreamBytes += int64(len(chunk))
	if !p.firstByteObserved {
		p.firstByteObserved = true
		addAccessLogFields(p.request.Context(), "first_byte_ms", accessLogElapsed(p.request.Context(), time.Now()))
	}
	observer := *p.observer
	if observer != nil {
		observer.Feed(chunk)
	}
	if observer != nil && observer.ProtocolInvalid() {
		p.rejectInvalidStream(observer)
		return true
	}
	if p.downstreamOpen {
		output := chunk
		if p.usageFilter != nil {
			output = p.usageFilter.Filter(chunk)
		}
		p.writeDownstream(output)
		if !p.downstreamOpen && (!p.retainCompletion || observer == nil) {
			return true
		}
	}
	if p.trace != nil && observer != nil && observer.Complete() {
		p.flushUsageFilter()
		p.trace.Status = usage.Success
		p.trace.ErrorType = ""
		addUsageTokenFields(p.request.Context(), p.trace, observer)
		if p.retainCompletion {
			p.handler.logOpenAIResponsesDiagnostic(p.request.Context(), slog.LevelInfo, "completion_observed", observer,
				"upstream_bytes", p.upstreamBytes,
				"downstream_open", p.downstreamOpen,
				"request_context_cancelled", p.request.Context().Err() != nil,
			)
		}
		return true
	}
	if p.trace != nil && observer != nil && p.protocol == gw.OpenAIProtocol && observer.Ended() {
		// 已收到 [DONE] 但缺少 usage（上游忽略 include_usage）：按不完整记录，
		// 不再等待上游关闭连接。
		p.flushUsageFilter()
		p.interrupted(observer.ErrorCode())
		addUsageTokenFields(p.request.Context(), p.trace, observer)
		return true
	}
	return false
}

func (p *gatewayResponsePump) writeDownstream(output []byte) {
	if !p.downstreamOpen || len(output) == 0 {
		return
	}
	_ = p.controller.SetWriteDeadline(time.Now().Add(p.handler.cfg.WriteTimeout))
	if _, err := p.writer.Write(output); err != nil {
		p.clientWriteFailed("client_write_failed")
	} else if err := p.controller.Flush(); err != nil {
		p.clientWriteFailed("client_flush_failed")
	}
}

func (p *gatewayResponsePump) flushUsageFilter() {
	if p.usageFilter != nil {
		p.writeDownstream(p.usageFilter.Flush())
	}
}

func (p *gatewayResponsePump) clientWriteFailed(reason string) {
	p.interrupted("CLIENT_WRITE_FAILED")
	if p.retainCompletion && *p.observer != nil {
		p.downstreamOpen = false
		p.startCompletionDrain(reason)
		return
	}
	p.downstreamOpen = false
	p.cancel()
}

func (p *gatewayResponsePump) rejectInvalidStream(observer *gw.UsageObserver) {
	p.interrupted(observer.ErrorCode())
	p.handler.service.ReportInvalidStream(p.request.Context(), p.upstream)
	p.handler.logger.WarnContext(p.request.Context(), "gateway rejected invalid upstream stream",
		"error_code", observer.ErrorCode(),
		"protocol", p.protocol,
		"upstream_status", p.upstream.Status,
		"upstream_bytes", p.upstreamBytes,
	)
	if p.downstreamOpen {
		payload := []byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"The upstream model provider returned an invalid streaming response. [UPSTREAM_INVALID_RESPONSE]\"}}\n\n")
		_ = p.controller.SetWriteDeadline(time.Now().Add(p.handler.cfg.WriteTimeout))
		_, _ = p.writer.Write(payload)
		_ = p.controller.Flush()
	}
	p.cancel()
}

func (p *gatewayResponsePump) finish(readErr error) {
	observer := *p.observer
	if readErr != io.EOF {
		if p.retainCompletion && p.completionDrainStarted.Load() && errors.Is(p.upstreamContext.Err(), context.Canceled) {
			if p.request.Context().Err() != nil {
				p.interrupted("UPSTREAM_STREAM_INTERRUPTED")
			}
			addUsageTokenFields(p.request.Context(), p.trace, observer)
			p.handler.logOpenAIResponsesDiagnostic(p.request.Context(), slog.LevelWarn, "completion_drain_ended_without_completion", observer,
				"upstream_bytes", p.upstreamBytes,
				"downstream_open", p.downstreamOpen,
				"request_context_cancelled", p.request.Context().Err() != nil,
			)
			return
		}
		p.interrupted("UPSTREAM_STREAM_INTERRUPTED")
		// 请求级截止或上游流空闲超时都归为上游超时。
		if p.trace != nil && (errors.Is(p.upstreamContext.Err(), context.DeadlineExceeded) || errors.Is(readErr, gw.ErrTimeout)) {
			p.trace.Status = usage.Failed
			p.trace.ErrorType = "UPSTREAM_TIMEOUT"
		}
		p.handler.logger.WarnContext(p.request.Context(), "gateway response interrupted", "error_code", "UPSTREAM_STREAM_INTERRUPTED")
		if p.retainCompletion {
			p.handler.logOpenAIResponsesDiagnostic(p.request.Context(), slog.LevelWarn, "upstream_read_interrupted", observer,
				"upstream_bytes", p.upstreamBytes,
				"downstream_open", p.downstreamOpen,
				"request_context_cancelled", p.request.Context().Err() != nil,
				"upstream_context_cancelled", errors.Is(p.upstreamContext.Err(), context.Canceled),
				"upstream_context_timed_out", errors.Is(p.upstreamContext.Err(), context.DeadlineExceeded),
			)
		}
		// HTTP 状态和流已经开始，不能追加另一份 JSON 或伪造结束事件。
		panic(http.ErrAbortHandler)
	}
	if p.trace != nil && observer != nil {
		if observer.Complete() {
			p.trace.Status = usage.Success
			p.trace.ErrorType = ""
		} else {
			p.interrupted(observer.ErrorCode())
		}
		addUsageTokenFields(p.request.Context(), p.trace, observer)
	}
	if p.trace != nil && observer == nil && p.upstream.Status >= 200 && p.upstream.Status < 300 {
		p.trace.Status = usage.Success
		p.trace.ErrorType = ""
	}
	if p.retainCompletion {
		level, stage := slog.LevelInfo, "upstream_eof"
		if observer == nil || !observer.Complete() {
			level, stage = slog.LevelWarn, "upstream_eof_without_completion"
		}
		p.handler.logOpenAIResponsesDiagnostic(p.request.Context(), level, stage, observer,
			"upstream_bytes", p.upstreamBytes,
			"downstream_open", p.downstreamOpen,
			"request_context_cancelled", p.request.Context().Err() != nil,
		)
	}
}
