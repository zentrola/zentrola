package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zentrola/zentrola/internal/infrastructure/telemetry"
	"go.opentelemetry.io/otel"
)

func TestTracedClientPropagatesChildSpan(t *testing.T) {
	provider := telemetry.Setup()
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background(), provider) })
	ctx, parent := otel.Tracer("provider-test").Start(context.Background(), "parent")
	defer parent.End()

	var traceparent string
	base := &http.Client{Transport: tracedRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		traceparent = request.Header.Get("traceparent")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})}
	client, _ := TracedClient(base)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://provider.example/test", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 || parts[1] != parent.SpanContext().TraceID().String() || parts[2] == parent.SpanContext().SpanID().String() {
		t.Fatalf("outbound child trace context was not propagated: %q", traceparent)
	}
}

type tracedRoundTripFunc func(*http.Request) (*http.Response, error)

func (f tracedRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
