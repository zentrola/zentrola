package http

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
)

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var entropy [16]byte
		// Go 的 crypto/rand.Read 保证填满缓冲区，无法获取安全随机数时终止进程。
		_, _ = rand.Read(entropy[:])
		id := "req_" + hex.EncodeToString(entropy[:])
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// chi 包装器保留 Flusher 等接口，避免影响后续 SSE。
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
				}
				logger.InfoContext(r.Context(), "http request",
					"method", r.Method, "path", r.URL.Path,
					"status", status, "latency_ms", time.Since(start).Milliseconds())
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}

func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					if value == http.ErrAbortHandler {
						panic(value)
					}
					// panic 值可能包含凭证，不直接写入响应或日志。
					logger.ErrorContext(r.Context(), "http handler panic", "error_code", "INTERNAL_ERROR")
					if tracked, ok := w.(chimiddleware.WrapResponseWriter); ok && tracked.Status() != 0 {
						// 已开始的流不能追加管理面 JSON，交由 net/http 中止连接。
						panic(http.ErrAbortHandler)
					}
					if r.URL.Path == "/anthropic" || strings.HasPrefix(r.URL.Path, "/anthropic/") {
						writeProtocolError(w, http.StatusInternalServerError, "api_error", "Internal server error.")
					} else if r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") {
						writeOpenAIError(w, &gw.Failure{Code: "INTERNAL_ERROR", Type: "api_error", Message: "Internal server error.", Status: 500})
					} else {
						writeJSON(w, r, http.StatusInternalServerError, response{Code: "INTERNAL_ERROR", Message: "Internal server error."})
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
