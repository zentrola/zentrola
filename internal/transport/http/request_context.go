package http

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func requestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func traceIDs(ctx context.Context) (traceID, spanID string) {
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return "", ""
	}
	return span.TraceID().String(), span.SpanID().String()
}
