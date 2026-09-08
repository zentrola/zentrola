// Package telemetry 初始化 OpenTelemetry Trace，并保持导出器可选。
package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup 安装能够生成真实 trace/span ID 的 SDK Provider。
// 当前未配置 Exporter；Span 仍会通过 W3C Trace Context 在 HTTP 边界传播。
func Setup() *sdktrace.TracerProvider {
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return provider
}

func Shutdown(ctx context.Context, provider *sdktrace.TracerProvider) error {
	if provider == nil {
		return nil
	}
	return provider.Shutdown(ctx)
}
