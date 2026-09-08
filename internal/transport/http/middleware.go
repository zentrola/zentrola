package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	"go.opentelemetry.io/otel/trace"
)

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var entropy [16]byte
		// Go 的 crypto/rand.Read 保证填满缓冲区，无法获取安全随机数时终止进程。
		_, _ = rand.Read(entropy[:])
		id := "req_" + hex.EncodeToString(entropy[:])
		w.Header().Set("X-Request-ID", id)
		if spanContext := trace.SpanContextFromContext(r.Context()); spanContext.IsValid() {
			w.Header().Set("X-Trace-ID", spanContext.TraceID().String())
			w.Header().Set("X-Span-ID", spanContext.SpanID().String())
		}
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

const accessLogBodyLimit = 8 << 10

type accessLogDetailsKey struct{}

type accessLogDetails struct {
	started time.Time
	mu      sync.Mutex
	fields  map[string]any
}

func withAccessLogDetails(ctx context.Context, started time.Time) context.Context {
	return context.WithValue(ctx, accessLogDetailsKey{}, &accessLogDetails{started: started, fields: make(map[string]any)})
}

func addAccessLogFields(ctx context.Context, fields ...any) {
	details, _ := ctx.Value(accessLogDetailsKey{}).(*accessLogDetails)
	if details == nil {
		return
	}
	details.mu.Lock()
	defer details.mu.Unlock()
	for index := 0; index+1 < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if ok && key != "" {
			details.fields[key] = fields[index+1]
		}
	}
}

func accessLogElapsed(ctx context.Context, since time.Time) int64 {
	if details, _ := ctx.Value(accessLogDetailsKey{}).(*accessLogDetails); details != nil && !details.started.IsZero() {
		return since.Sub(details.started).Milliseconds()
	}
	return 0
}

func accessLogExtraFields(ctx context.Context) []any {
	details, _ := ctx.Value(accessLogDetailsKey{}).(*accessLogDetails)
	if details == nil {
		return nil
	}
	details.mu.Lock()
	defer details.mu.Unlock()
	keys := make([]string, 0, len(details.fields))
	for key := range details.fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		result = append(result, key, details.fields[key])
	}
	return result
}

type bodyCapture struct {
	data      bytes.Buffer
	total     int64
	truncated bool
}

func (c *bodyCapture) Write(p []byte) (int, error) {
	c.total += int64(len(p))
	remaining := accessLogBodyLimit - c.data.Len()
	if remaining > 0 {
		written := len(p)
		if written > remaining {
			written = remaining
		}
		_, _ = c.data.Write(p[:written])
	}
	if c.total > accessLogBodyLimit {
		c.truncated = true
	}
	return len(p), nil
}

type captureReadCloser struct {
	io.ReadCloser
	capture *bodyCapture
}

func (r *captureReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		_, _ = r.capture.Write(p[:n])
	}
	return n, err
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

func sensitiveJSONField(name string) bool {
	if safeJSONField(name) {
		return false
	}
	name = nonAlphaNumeric.ReplaceAllString(strings.ToLower(name), "")
	for _, marker := range []string{
		"password", "secret", "token", "credential", "authorization", "apikey", "virtualkey", "jwt",
		"content", "message", "prompt", "input", "output", "system", "instruction", "thinking", "conversation", "context", "query", "response", "text",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return name == "key"
}

func safeJSONField(name string) bool {
	name = nonAlphaNumeric.ReplaceAllString(strings.ToLower(name), "")
	switch name {
	case "id", "requestid", "modelid", "providerid", "providermodelid", "resourceid", "organizationid", "principalid", "accesskeyid",
		"model", "stream", "maxtokens", "temperature", "topp", "type", "role", "code", "status", "object", "stopreason", "finishreason",
		"usage", "inputtokens", "outputtokens", "cachedinputtokens", "prompttokens", "completiontokens", "totaltokens",
		"preview", "keypreview", "credentialconfigured", "enabled":
		return true
	default:
		return false
	}
}

func redactJSON(value any) any {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if sensitiveJSONField(key) {
				value[key] = "[REDACTED]"
				continue
			}
			switch child.(type) {
			case map[string]any, []any:
				value[key] = redactJSON(child)
			default:
				if !safeJSONField(key) {
					value[key] = "[REDACTED]"
				}
			}
		}
	case []any:
		for i := range value {
			switch value[i].(type) {
			case map[string]any, []any:
				value[i] = redactJSON(value[i])
			default:
				value[i] = "[REDACTED]"
			}
		}
	default:
		return "[REDACTED]"
	}
	return value
}

func bodyLogValue(capture *bodyCapture, contentType string) string {
	if capture == nil || capture.total == 0 {
		return ""
	}
	media, _, _ := mime.ParseMediaType(contentType)
	if media != "application/json" {
		return fmt.Sprintf("[OMITTED content_type=%s bytes=%d]", media, capture.total)
	}
	if capture.truncated {
		return fmt.Sprintf("[OMITTED truncated_json bytes=%d captured=%d]", capture.total, capture.data.Len())
	}
	decoder := json.NewDecoder(bytes.NewReader(capture.data.Bytes()))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Sprintf("[OMITTED invalid_json bytes=%d]", capture.total)
	}
	redacted, err := json.Marshal(redactJSON(value))
	if err != nil {
		return fmt.Sprintf("[OMITTED unencodable_json bytes=%d]", capture.total)
	}
	return string(redacted)
}

func accessLog(logger *slog.Logger, environments ...string) func(http.Handler) http.Handler {
	development := len(environments) > 0 && environments[0] == "dev"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			r = r.WithContext(withAccessLogDetails(r.Context(), start))
			// chi 包装器保留 Flusher 等接口，避免影响后续 SSE。
			wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			var requestBody, responseBody *bodyCapture
			if development {
				requestBody = &bodyCapture{}
				responseBody = &bodyCapture{}
				if r.Body != nil {
					r.Body = &captureReadCloser{ReadCloser: r.Body, capture: requestBody}
				}
				wrapped.Tee(responseBody)
			}
			defer func() {
				status := wrapped.Status()
				if status == 0 {
					status = http.StatusOK
				}
				attributes := []any{
					"request_time", start.UTC().Format(time.RFC3339Nano),
					"method", r.Method,
					"duration_ms", time.Since(start).Milliseconds(),
					"path", r.URL.Path,
					"status", status,
					"request_bytes", func() int64 {
						if requestBody != nil {
							return requestBody.total
						}
						return r.ContentLength
					}(),
					"response_bytes", wrapped.BytesWritten(),
				}
				if development {
					attributes = append(attributes,
						"request_body", bodyLogValue(requestBody, r.Header.Get("Content-Type")),
						"response_body", bodyLogValue(responseBody, wrapped.Header().Get("Content-Type")),
					)
				}
				attributes = append(attributes, accessLogExtraFields(r.Context())...)
				logger.InfoContext(r.Context(), "http request", attributes...)
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
