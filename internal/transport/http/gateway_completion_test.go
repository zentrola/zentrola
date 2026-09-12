package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

type completionStore struct{}

func (completionStore) Resolve(context.Context, appsec.PrincipalIdentity, string, ...string) (gw.Route, error) {
	return gw.Route{
		ModelID: 1, ProviderID: 2, ProviderModelID: 3, ResourceID: 4,
		UpstreamModel: "upstream-model",
	}, nil
}

type completionCipher struct{}

func (completionCipher) Decrypt(catalog.SealedCredential, catalog.CredentialOwner) ([]byte, error) {
	return []byte("secret"), nil
}

func (completionCipher) DecryptProviderProxy(catalog.SealedCredential, catalog.ProviderProxyOwner) ([]byte, error) {
	return nil, errors.New("unexpected proxy decryption")
}

type completionUpstream func(context.Context, gw.Route, gw.Request, []byte) (*gw.Response, error)

func (f completionUpstream) Open(ctx context.Context, route gw.Route, request gw.Request, credential []byte) (*gw.Response, error) {
	return f(ctx, route, request, credential)
}

type completedThenCancelledBody struct {
	payload []byte
	cancel  context.CancelFunc
	sent    bool
}

func (b *completedThenCancelledBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, b.payload), nil
	}
	b.cancel()
	return 0, context.Canceled
}

func (*completedThenCancelledBody) Close() error { return nil }

type completionAfterCancellationBody struct {
	ctx    context.Context
	cancel context.CancelFunc
	read   int
}

func (b *completionAfterCancellationBody) Read(p []byte) (int, error) {
	switch b.read {
	case 0:
		b.read++
		b.cancel()
		return copy(p, []byte("event: response.created\n"+
			"data: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n")), nil
	case 1:
		b.read++
		select {
		case <-b.ctx.Done():
			return 0, b.ctx.Err()
		default:
		}
		return copy(p, []byte("event: response.completed\n"+
			"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens\":7}}}\n\n")), nil
	default:
		return 0, io.EOF
	}
}

func (*completionAfterCancellationBody) Close() error { return nil }

func TestOpenAIResponsesCompletionWinsFollowingClientCancellation(t *testing.T) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	var trace *usage.Event
	upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		body := []byte("event: response.completed\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":7}}}\n\n")
		return &gw.Response{
			Status: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type": {"text/event-stream"},
			},
			Body: &completedThenCancelledBody{payload: body, cancel: cancelRequest},
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	handler := NewOpenAIGatewayHandler(service, config.Gateway{
		MaxBodyBytes:    1 << 20,
		RequestTimeout:  time.Second,
		BodyReadTimeout: time.Second,
		WriteTimeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
	request.Body = io.NopCloser(strings.NewReader(`{"model":"client-model","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 10, AccessKeyID: 11}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if trace == nil || trace.Status != usage.Success || trace.ErrorType != "" {
		t.Fatalf("completed Responses stream recorded incorrectly: %+v", trace)
	}
	if trace.Attempt == nil || trace.Attempt.InputTokens == nil || *trace.Attempt.InputTokens != 12 ||
		trace.Attempt.OutputTokens == nil || *trace.Attempt.OutputTokens != 7 {
		t.Fatalf("completed Responses usage missing: %+v", trace.Attempt)
	}
}

func TestOpenAIResponsesInfersStreamWhenUpstreamOmitsContentType(t *testing.T) {
	var trace *usage.Event
	upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		body := "event: response.created\n" +
			"data: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
			"event: response.completed\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens\":7}}}\n\n"
		return &gw.Response{
			Status:  http.StatusOK,
			Headers: map[string][]string{},
			Body:    io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	var logs bytes.Buffer
	handler := NewOpenAIGatewayHandler(service, config.Gateway{
		MaxBodyBytes:    1 << 20,
		RequestTimeout:  time.Second,
		BodyReadTimeout: time.Second,
		WriteTimeout:    time.Second,
		Development:     true,
	}, slog.New(slog.NewTextHandler(&logs, nil)), nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Body = io.NopCloser(strings.NewReader(`{"model":"client-model","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 10, AccessKeyID: 11}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type=%q; want text/event-stream", got)
	}
	if trace == nil || trace.Status != usage.Success || trace.ErrorType != "" {
		t.Fatalf("inferred Responses stream recorded incorrectly: %+v", trace)
	}
	if trace.Attempt == nil || trace.Attempt.InputTokens == nil || *trace.Attempt.InputTokens != 12 ||
		trace.Attempt.OutputTokens == nil || *trace.Attempt.OutputTokens != 7 ||
		trace.Attempt.CachedInputTokens == nil || *trace.Attempt.CachedInputTokens != 3 {
		t.Fatalf("inferred Responses stream usage missing: %+v", trace.Attempt)
	}
	for _, expected := range []string{
		"stream=true",
		"requested_stream=true",
		"stream_inferred=true",
		"stage=completion_observed",
		"last_event=response.completed",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("diagnostic log %q does not contain %q", logs.String(), expected)
		}
	}
}

func TestOpenAIResponsesAutoDetectsJSONWhenUpstreamOmitsContentType(t *testing.T) {
	var trace *usage.Event
	upstream := completionUpstream(func(_ context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		return &gw.Response{
			Status:  http.StatusOK,
			Headers: map[string][]string{},
			Body: io.NopCloser(strings.NewReader(
				`{"object":"response","usage":{"input_tokens":14,"input_tokens_details":{"cached_tokens":5},"output_tokens":8}}`,
			)),
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	var logs bytes.Buffer
	handler := NewOpenAIGatewayHandler(service, config.Gateway{
		MaxBodyBytes:    1 << 20,
		RequestTimeout:  time.Second,
		BodyReadTimeout: time.Second,
		WriteTimeout:    time.Second,
		Development:     true,
	}, slog.New(slog.NewTextHandler(&logs, nil)), nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	request.Body = io.NopCloser(strings.NewReader(`{"model":"client-model","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 10, AccessKeyID: 11}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if trace == nil || trace.Status != usage.Success || trace.ErrorType != "" {
		t.Fatalf("auto-detected JSON response recorded incorrectly: %+v", trace)
	}
	if trace.Attempt == nil || trace.Attempt.InputTokens == nil || *trace.Attempt.InputTokens != 14 ||
		trace.Attempt.OutputTokens == nil || *trace.Attempt.OutputTokens != 8 ||
		trace.Attempt.CachedInputTokens == nil || *trace.Attempt.CachedInputTokens != 5 {
		t.Fatalf("auto-detected JSON usage missing: %+v", trace.Attempt)
	}
	for _, expected := range []string{
		"stream=false",
		"requested_stream=true",
		"stream_inferred=true",
		"stage=completion_observed",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("diagnostic log %q does not contain %q", logs.String(), expected)
		}
	}
}

func TestOpenAIResponsesDrainsCompletionAfterClientCancellation(t *testing.T) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	var trace *usage.Event
	upstream := completionUpstream(func(ctx context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		return &gw.Response{
			Status: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type": {"text/event-stream"},
			},
			Body: &completionAfterCancellationBody{ctx: ctx, cancel: cancelRequest},
		}, nil
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	var logs bytes.Buffer
	handler := NewOpenAIGatewayHandler(service, config.Gateway{
		MaxBodyBytes:    1 << 20,
		RequestTimeout:  time.Second,
		BodyReadTimeout: time.Second,
		WriteTimeout:    100 * time.Millisecond,
		Development:     true,
	}, slog.New(slog.NewTextHandler(&logs, nil)), nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
	request.Body = io.NopCloser(strings.NewReader(`{"model":"client-model","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 10, AccessKeyID: 11}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if trace == nil || trace.Status != usage.Success || trace.ErrorType != "" {
		t.Fatalf("completed Responses stream recorded incorrectly: %+v", trace)
	}
	if trace.Attempt == nil || trace.Attempt.InputTokens == nil || *trace.Attempt.InputTokens != 12 ||
		trace.Attempt.OutputTokens == nil || *trace.Attempt.OutputTokens != 7 ||
		trace.Attempt.CachedInputTokens == nil || *trace.Attempt.CachedInputTokens != 3 {
		t.Fatalf("drained Responses usage missing: %+v", trace.Attempt)
	}
	for _, expected := range []string{
		"stage=completion_observed",
		"last_event=response.completed",
		"input_tokens=12",
		"output_tokens=7",
		"cached_input_tokens=3",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("diagnostic log %q does not contain %q", logs.String(), expected)
		}
	}
}

func TestOpenAIResponsesCancelsUpstreamBeforeResponseOpens(t *testing.T) {
	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	opened := make(chan struct{})
	upstreamCancelled := make(chan struct{})
	var trace *usage.Event
	upstream := completionUpstream(func(ctx context.Context, _ gw.Route, request gw.Request, _ []byte) (*gw.Response, error) {
		trace = request.Trace
		close(opened)
		<-ctx.Done()
		close(upstreamCancelled)
		return nil, gw.ErrCancelled
	})
	service := gw.New(completionStore{}, completionCipher{}, upstream)
	handler := NewOpenAIGatewayHandler(service, config.Gateway{
		MaxBodyBytes:    1 << 20,
		RequestTimeout:  time.Second,
		BodyReadTimeout: time.Second,
		WriteTimeout:    100 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)
	request.Body = io.NopCloser(strings.NewReader(`{"model":"client-model","stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(request.Context(), principalIdentityKey{}, appsec.PrincipalIdentity{ID: 10, AccessKeyID: 11}))
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(recorder, request)
		close(done)
	}()

	<-opened
	cancelRequest()
	select {
	case <-upstreamCancelled:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("upstream request remained active before a response was opened")
	}
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("handler did not return after early client cancellation")
	}
	if trace == nil || trace.Status != usage.Cancelled || trace.ErrorType != "REQUEST_CANCELLED" {
		t.Fatalf("early cancellation recorded incorrectly: %+v", trace)
	}
}
