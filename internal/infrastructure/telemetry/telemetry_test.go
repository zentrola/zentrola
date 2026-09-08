package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestSetupCreatesValidTraceContext(t *testing.T) {
	provider := Setup()
	t.Cleanup(func() { _ = Shutdown(context.Background(), provider) })
	ctx, span := otel.Tracer("telemetry-test").Start(context.Background(), "request")
	defer span.End()
	spanContext := span.SpanContext()
	if !spanContext.IsValid() || !spanContext.TraceID().IsValid() || !spanContext.SpanID().IsValid() {
		t.Fatalf("invalid OpenTelemetry context: %v", spanContext)
	}
	if ctx == nil {
		t.Fatal("trace context was not returned")
	}
}
