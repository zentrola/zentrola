package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/cors"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type CORSOptions struct {
	Enabled bool
	Origins []string
}

func NewRouter(logger *slog.Logger, readiness *health.Service, corsConfig CORSOptions, healthTimeout time.Duration, environment string, security ...*SecurityHandlers) http.Handler {
	r := chi.NewRouter()
	r.Use(otelhttp.NewMiddleware("zentrola.http.server"), requestID, accessLog(logger), recoverPanic(logger))
	if corsConfig.Enabled {
		r.Use(cors.New(cors.Options{
			AllowedOrigins: corsConfig.Origins,
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type", "X-Request-ID", "traceparent", "tracestate"},
			ExposedHeaders: []string{"X-Request-ID", "X-Trace-ID", "X-Span-ID"},
			MaxAge:         300,
		}).Handler)
	}
	mountSwagger(r, environment)
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Zentrola is ready"))
	})
	// @Summary 进程存活检查
	// @Tags 健康检查
	// @Produce json
	// @Success 200 {object} response{data=LiveResponse}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Router /health/live [get]
	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, response{Code: "OK", Data: LiveResponse{Status: "LIVE"}})
	})
	// @Summary 服务就绪检查
	// @Tags 健康检查
	// @Produce json
	// @Success 200 {object} response{data=health.Report}
	// @Header all {string} X-Request-ID "请求追踪 ID"
	// @Failure 503 {object} response{data=health.Report}
	// @Router /health/ready [get]
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()
		report := readiness.Ready(ctx)
		status, code := http.StatusOK, "OK"
		if report.Status != "READY" {
			status, code = http.StatusServiceUnavailable, "NOT_READY"
		}
		writeJSON(w, r, status, response{Code: code, Data: report})
	})
	if len(security) > 0 && security[0] != nil {
		security[0].logger = logger
		security[0].mount(r)
	}
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") {
			writeOpenAIError(w, &gw.Failure{Code: "NOT_FOUND", Type: "invalid_request_error", Message: "Route not found.", Status: 404})
			return
		}
		writeJSON(w, r, http.StatusNotFound, response{Code: "NOT_FOUND", Message: "Route not found."})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") {
			writeOpenAIError(w, &gw.Failure{Code: "METHOD_NOT_ALLOWED", Type: "invalid_request_error", Message: "Method not allowed.", Status: 405})
			return
		}
		writeJSON(w, r, http.StatusMethodNotAllowed, response{Code: "METHOD_NOT_ALLOWED", Message: "Method not allowed."})
	})
	return r
}
