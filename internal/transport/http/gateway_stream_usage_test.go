package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

func TestOpenAIChatStreamUsageInjection(t *testing.T) {
	content := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}],\"usage\":null}\n\n"
	finish := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":null}\n\n"
	usageChunk := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":4}}}\n\n"
	done := "data: [DONE]\n\n"
	tests := []struct {
		name, requestBody, upstreamStream, wantClient string
		wantStatus                                    usage.Status
		wantError                                     string
		wantTokens                                    bool
	}{
		{
			name:           "client without include_usage gets transparent stream",
			requestBody:    `{"model":"client-model","stream":true,"messages":[]}`,
			upstreamStream: content + finish + usageChunk + done,
			wantClient:     content + finish + done,
			wantStatus:     usage.Success, wantTokens: true,
		},
		{
			name:           "client requested usage keeps usage chunk",
			requestBody:    `{"model":"client-model","stream":true,"stream_options":{"include_usage":true},"messages":[]}`,
			upstreamStream: content + finish + usageChunk + done,
			wantClient:     content + finish + usageChunk + done,
			wantStatus:     usage.Success, wantTokens: true,
		},
		{
			name:           "upstream ignores include_usage",
			requestBody:    `{"model":"client-model","stream":true,"messages":[]}`,
			upstreamStream: content + finish + done,
			wantClient:     content + finish + done,
			wantStatus:     usage.Failed, wantError: "UPSTREAM_RESPONSE_INCOMPLETE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var trace *usage.Event
			var upstreamBody string
			upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
				trace = request.Trace
				upstreamBody = string(request.Body)
				return &gw.Response{
					Status:  http.StatusOK,
					Headers: map[string][]string{"Content-Type": {"text/event-stream"}},
					Body:    io.NopCloser(strings.NewReader(test.upstreamStream)),
				}, nil
			})
			service := gw.New(completionStore{}, completionCipher{}, upstream)
			handler := NewOpenAIGatewayHandler(service, GatewayOptions{
				MaxBodyBytes: 1 << 20, RequestTimeout: time.Second, BodyReadTimeout: time.Second, WriteTimeout: time.Second,
			}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(test.requestBody))
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if !strings.Contains(upstreamBody, `"include_usage":true`) || !strings.Contains(upstreamBody, `"upstream-model"`) {
				t.Fatalf("upstream request did not force include_usage: %s", upstreamBody)
			}
			if recorder.Code != http.StatusOK || recorder.Body.String() != test.wantClient {
				t.Fatalf("client stream mismatch: status=%d body=%q", recorder.Code, recorder.Body.String())
			}
			if trace == nil || trace.Status != test.wantStatus || trace.ErrorType != test.wantError || trace.Attempt == nil {
				t.Fatalf("trace mismatch: %+v", trace)
			}
			attempt := trace.Attempt
			if test.wantTokens && (attempt.InputTokens == nil || *attempt.InputTokens != 12 || attempt.OutputTokens == nil ||
				*attempt.OutputTokens != 7 || attempt.CachedInputTokens == nil || *attempt.CachedInputTokens != 4) {
				t.Fatalf("usage not recorded: %+v", attempt)
			}
		})
	}
}

// payloadThenBlockBody 发送完 payload 后阻塞，直到被关闭，模拟上游 [DONE] 后不关闭连接。
type payloadThenBlockBody struct {
	payload []byte
	sent    bool
	closed  chan struct{}
}

func (b *payloadThenBlockBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, b.payload), nil
	}
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *payloadThenBlockBody) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

func TestOpenAIChatStreamWithoutUsageStopsAtDone(t *testing.T) {
	stream := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	var trace *usage.Event
	upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		return &gw.Response{
			Status:  http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"text/event-stream"}},
			Body:    &payloadThenBlockBody{payload: []byte(stream), closed: make(chan struct{})},
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	handler := NewOpenAIGatewayHandler(service, GatewayOptions{
		MaxBodyBytes: 1 << 20, RequestTimeout: time.Minute, BodyReadTimeout: time.Second, WriteTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-model","stream":true,"messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}))
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(recorder, request)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler waited for upstream EOF after [DONE]")
	}
	if recorder.Body.String() != stream || trace == nil || trace.Status != usage.Failed || trace.ErrorType != "UPSTREAM_RESPONSE_INCOMPLETE" {
		t.Fatalf("body=%q trace=%+v", recorder.Body.String(), trace)
	}
}

func TestOpenAIResponsesOversizedCompletedEventRecordsUsage(t *testing.T) {
	stream := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"" +
		strings.Repeat("QUJD", 1<<20) + "\"}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens\":7}}}\n\n"
	var trace *usage.Event
	upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		return &gw.Response{
			Status:  http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"text/event-stream"}},
			Body:    io.NopCloser(strings.NewReader(stream)),
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	handler := NewOpenAIGatewayHandler(service, GatewayOptions{
		MaxBodyBytes: 1 << 20, RequestTimeout: time.Second, BodyReadTimeout: time.Second, WriteTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"client-model","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Body.String() != stream {
		t.Fatalf("stream changed: bytes=%d", recorder.Body.Len())
	}
	if trace == nil || trace.Status != usage.Success || trace.Attempt == nil || trace.Attempt.InputTokens == nil ||
		*trace.Attempt.InputTokens != 12 || trace.Attempt.OutputTokens == nil || *trace.Attempt.OutputTokens != 7 ||
		trace.Attempt.CachedInputTokens == nil || *trace.Attempt.CachedInputTokens != 3 {
		t.Fatalf("oversized completed usage missing: %+v", trace)
	}
}

type failAfterReader struct {
	data io.Reader
	err  error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	n, err := r.data.Read(p)
	if err == io.EOF {
		return n, r.err
	}
	return n, err
}

func TestStreamIdleTimeoutRecordedAsUpstreamTimeout(t *testing.T) {
	content := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}],\"usage\":null}\n\n"
	var trace *usage.Event
	upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		return &gw.Response{
			Status:  http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"text/event-stream"}},
			Body:    io.NopCloser(&failAfterReader{data: strings.NewReader(content), err: gw.ErrTimeout}),
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	handler := NewOpenAIGatewayHandler(service, GatewayOptions{
		MaxBodyBytes: 1 << 20, RequestTimeout: time.Second, BodyReadTimeout: time.Second, WriteTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-model","stream":true,"messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 1, AccessKeyID: 2}))
	func() {
		defer func() { _ = recover() }()
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()
	if trace == nil || trace.Status != usage.Failed || trace.ErrorType != "UPSTREAM_TIMEOUT" {
		t.Fatalf("idle timeout trace = %+v", trace)
	}
}
