package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	"github.com/zentrola/zentrola/internal/infrastructure/telemetry"
)

func TestAnthropicGatewayErrorIncludesActionableReasonAndStableCode(t *testing.T) {
	for _, test := range []struct {
		failure *gw.Failure
		reason  string
	}{
		{gw.ErrRoute, "configure its endpoint and model mapping"},
		{gw.ErrRouteCooldown, "temporarily cooling down"},
		{gw.ErrResource, "Configure or enable the provider API key"},
		{gw.ErrCredential, "Reconfigure the provider API key"},
		{gw.ErrProxy, "Reconfigure the provider proxy"},
		{gw.ErrProxyServer, "proxy server"},
	} {
		t.Run(test.failure.Code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeGatewayError(recorder, test.failure)

			if recorder.Code != test.failure.Status {
				t.Fatalf("status=%d; want %d", recorder.Code, test.failure.Status)
			}
			if recorder.Header().Get("X-Zentrola-Error-Code") != test.failure.Code {
				t.Fatalf("missing error code header: %s", recorder.Header().Get("X-Zentrola-Error-Code"))
			}
			body := recorder.Body.String()
			if !strings.Contains(body, test.reason) || !strings.Contains(body, "["+test.failure.Code+"]") {
				t.Fatalf("response is not actionable: %s", body)
			}
			if strings.Contains(body, `"code"`) {
				t.Fatalf("Anthropic error contract gained a non-standard code field: %s", body)
			}
		})
	}
}

func TestHealthAndRequestCorrelation(t *testing.T) {
	provider := telemetry.Setup()
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background(), provider) })
	var logs bytes.Buffer
	service := health.New(
		health.Check{Name: "postgres", Run: func(context.Context) error { return errors.New("database-secret") }},
		health.Check{Name: "master_key", Run: health.Pending},
	)
	router := NewRouter(logging.New(&logs, "json", slog.LevelInfo), service, CORSOptions{}, time.Second, "prod")
	seen := map[string]bool{}
	for path, want := range map[string]int{"/health/live": 200, "/health/ready": 503, "/missing": 404} {
		req := httptest.NewRequest("GET", path+"?token=query-secret", nil)
		req.Header.Set("X-Request-ID", "untrusted-client-id")
		req.Header.Set("Authorization", "Bearer header-secret")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s: got %d, want %d", path, rec.Code, want)
		}
		var body response
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		id := rec.Header().Get("X-Request-ID")
		if !strings.HasPrefix(id, "req_") || id != body.RequestID || seen[id] {
			t.Fatalf("bad request ID: %q", id)
		}
		seen[id] = true
		traceID, spanID := rec.Header().Get("X-Trace-ID"), rec.Header().Get("X-Span-ID")
		if traceID == "" || spanID == "" || !strings.Contains(logs.String(), traceID) || !strings.Contains(logs.String(), spanID) {
			t.Fatal("OpenTelemetry trace context missing in response or log")
		}
		if wantID := "req_" + traceID + "_" + spanID; id != wantID {
			t.Fatalf("request ID %q does not match trace context; want %q", id, wantID)
		}
		if strings.Contains(rec.Body.String(), "database-secret") {
			t.Fatal("dependency secret leaked in response")
		}
	}
	for _, secret := range []string{"database-secret", "query-secret", "header-secret", "untrusted-client-id"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("secret leaked: %s", secret)
		}
	}
}

func TestRequestIDDistinguishesServerSpansWithinTrace(t *testing.T) {
	provider := telemetry.Setup()
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background(), provider) })
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(), CORSOptions{}, time.Second, "prod")
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const traceparent = "00-" + traceID + "-00f067aa0ba902b7-01"

	requestIDs := map[string]bool{}
	serverSpanIDs := map[string]bool{}
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
		req.Header.Set("traceparent", traceparent)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		serverSpanID := rec.Header().Get("X-Span-ID")
		requestID := rec.Header().Get("X-Request-ID")
		if got := rec.Header().Get("X-Trace-ID"); got != traceID {
			t.Fatalf("trace ID=%q; want %q", got, traceID)
		}
		if want := "req_" + traceID + "_" + serverSpanID; requestID != want {
			t.Fatalf("request ID=%q; want %q", requestID, want)
		}
		if serverSpanIDs[serverSpanID] || requestIDs[requestID] {
			t.Fatalf("server span did not uniquely identify request: span=%q request=%q", serverSpanID, requestID)
		}
		serverSpanIDs[serverSpanID] = true
		requestIDs[requestID] = true
	}
}

func TestRootReturnsReadyMessage(t *testing.T) {
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(), CORSOptions{}, time.Second, "prod")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("got Content-Type %q", got)
	}
	if got := rec.Body.String(); got != "Zentrola is ready" {
		t.Fatalf("got body %q", got)
	}
	if got := rec.Header().Get("X-Request-ID"); !strings.HasPrefix(got, "req_") {
		t.Fatalf("bad request ID: %q", got)
	}
}

func TestIncomingTraceparentContinuesTraceWithNewServerSpan(t *testing.T) {
	provider := telemetry.Setup()
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background(), provider) })
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const parentSpanID = "00f067aa0ba902b7"
	router := NewRouter(slog.New(slog.NewJSONHandler(io.Discard, nil)), health.New(), CORSOptions{}, time.Second, "prod")
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-"+parentSpanID+"-01")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Header().Get("X-Trace-ID") != traceID {
		t.Fatalf("trace was not continued: %q", rec.Header().Get("X-Trace-ID"))
	}
	if spanID := rec.Header().Get("X-Span-ID"); spanID == "" || spanID == parentSpanID {
		t.Fatalf("server span ID was not generated: %q", spanID)
	}
}

func TestReadyWhenAllDependenciesReady(t *testing.T) {
	service := health.New(health.Check{Name: "postgres", Run: func(context.Context) error { return nil }})
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), service, CORSOptions{}, time.Second, "prod")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/health/ready", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"READY"`) {
		t.Fatal(rec.Body.String())
	}
}

func TestCORSAllowlist(t *testing.T) {
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(),
		CORSOptions{Enabled: true, Origins: []string{"http://127.0.0.1:3000"}}, time.Second, "prod")
	for _, tt := range []struct {
		name, origin, method, headers string
		allowed                       bool
	}{
		{"allowed", "http://127.0.0.1:3000", "POST", "authorization,content-type,x-request-id", true},
		{"foreign origin", "https://untrusted.example", "POST", "authorization", false},
		{"unknown method", "http://127.0.0.1:3000", "TRACE", "", false},
		{"unknown header", "http://127.0.0.1:3000", "POST", "x-unapproved", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("OPTIONS", "/api/v1/members", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Access-Control-Request-Method", tt.method)
			req.Header.Set("Access-Control-Request-Headers", tt.headers)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			allowed := rec.Header().Get("Access-Control-Allow-Origin") == tt.origin
			if allowed != tt.allowed {
				t.Fatalf("unexpected CORS headers: %v", rec.Header())
			}
			if rec.Header().Get("X-Request-ID") == "" {
				t.Fatal("preflight missing request ID")
			}
		})
	}
}

func TestMiddlewarePreservesStreamingAndCancellation(t *testing.T) {
	logger := logging.New(io.Discard, "json", slog.LevelInfo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handler := requestID(accessLog(logger)(recoverPanic(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !errors.Is(r.Context().Err(), context.Canceled) {
			t.Fatal("cancellation lost")
		}
		if requestIDFromContext(r.Context()) == "" {
			t.Fatal("request context missing ID")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: test\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Fatal(err)
		}
	}))))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/stream", nil).WithContext(ctx))
	if !rec.Flushed || rec.Body.String() != "data: test\n\n" {
		t.Fatal("stream was altered or not flushed")
	}
}

func TestAccessLogRedactsBodiesInEveryEnvironment(t *testing.T) {
	provider := telemetry.Setup()
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background(), provider) })
	for _, environment := range []string{"dev", "test", "prod"} {
		t.Run(environment, func(t *testing.T) {
			var logs bytes.Buffer
			logger := logging.New(&logs, "json", slog.LevelInfo)
			handler := requestID(accessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Debug-Response", "response-header-value")
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":{"code":"MODEL_PERMISSION_DENIED","message":"Model permission denied."},"accessToken":"response-secret"}`)
			})))
			req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?token=query-secret", strings.NewReader(`{"model":"claude-sonnet","messages":[{"role":"user","content":"private-prompt"}],"password":"request-secret","input_text":"future-schema-secret","conversation":"unknown-content-secret"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer development-access-key")
			req.Header.Set("X-Debug-Request", "request-header-value")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			output := logs.String()
			for _, required := range []string{
				"time", "trace_id", "span_id", "duration_ms", "request", "response", "method", "url", "status",
				"headers", "body", "bytes", "******", rec.Header().Get("X-Request-ID"),
			} {
				if !strings.Contains(output, required) {
					t.Fatalf("non-production access log missing %s: %s", required, output)
				}
			}
			for _, secret := range []string{
				"request-secret", "response-secret", "development-access-key", "private-prompt",
				"future-schema-secret", "unknown-content-secret", "query-secret",
				"request-header-value", "response-header-value",
			} {
				if strings.Contains(output, secret) {
					t.Fatalf("non-production access log leaked %s: %s", secret, output)
				}
			}
			if strings.Count(output, `"trace_id"`) != 1 || strings.Count(output, `"span_id"`) != 1 {
				t.Fatalf("trace and span IDs must each appear once: %s", output)
			}
		})
	}
}

func TestAccessLogOmitsLargeBodies(t *testing.T) {
	requestBody := `{"prompt":"` + strings.Repeat("large-private-prompt-", 10_000) + `"}`
	responseBody := "event: response.completed\ndata: private-stream-response\n\n"
	for _, environment := range []string{"dev", "test"} {
		t.Run(environment, func(t *testing.T) {
			var logs bytes.Buffer
			logger := logging.New(&logs, "json", slog.LevelInfo)
			handler := accessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, responseBody)
			}))
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(httptest.NewRecorder(), req)

			var record map[string]any
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			request, requestOK := record["request"].(map[string]any)
			response, responseOK := record["response"].(map[string]any)
			if !requestOK || !responseOK || request["body"] != "******" || response["body"] != "******" ||
				request["bytes"] != float64(len(requestBody)) || response["bytes"] != float64(len(responseBody)) ||
				strings.Contains(logs.String(), "large-private-prompt") || strings.Contains(logs.String(), "private-stream-response") {
				t.Fatalf("%s access log exposed a body or lost byte counts: %s", environment, logs.String())
			}
		})
	}
}

func TestAccessLogCountsBytesReadFromUnknownLengthBody(t *testing.T) {
	const body = "private-request-body"
	for _, test := range []struct {
		name string
		read int64
		want int64
	}{
		{name: "entire_body", want: int64(len(body))},
		{name: "partial_body", read: 7, want: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := accessLog(logging.New(&logs, "json", slog.LevelInfo))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				if test.read > 0 {
					_, err = io.CopyN(io.Discard, r.Body, test.read)
				} else {
					_, err = io.Copy(io.Discard, r.Body)
				}
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
			}))
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", io.NopCloser(strings.NewReader(body)))
			request.ContentLength = -1
			handler.ServeHTTP(httptest.NewRecorder(), request)
			var record struct {
				Request struct {
					Bytes int64  `json:"bytes"`
					Body  string `json:"body"`
				} `json:"request"`
			}
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record.Request.Bytes != test.want || record.Request.Body != "******" || strings.Contains(logs.String(), body) {
				t.Fatalf("unexpected request log: bytes=%d body=%q log=%s", record.Request.Bytes, record.Request.Body, logs.String())
			}
		})
	}
}

func TestNonProductionAccessLogOmitsResponsesSSEBody(t *testing.T) {
	const responseBody = "event: response.output_text.delta\ndata: private-stream-response\n\n"
	for _, environment := range []string{"dev", "test"} {
		t.Run(environment, func(t *testing.T) {
			var logs bytes.Buffer
			logger := logging.New(&logs, "json", slog.LevelInfo)
			handler := accessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
				_, _ = io.WriteString(w, responseBody)
			}))
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","stream":true}`))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(httptest.NewRecorder(), req)

			var record map[string]any
			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			response, ok := record["response"].(map[string]any)
			if !ok || response["bytes"] != float64(len(responseBody)) {
				t.Fatalf("%s Responses SSE metadata missing: %v", environment, record)
			}
			if response["body"] != "******" || strings.Contains(logs.String(), "private-stream-response") {
				t.Fatalf("%s Responses SSE body was logged: %s", environment, logs.String())
			}
		})
	}
}

func TestProductionAccessLogMasksBodies(t *testing.T) {
	var logs bytes.Buffer
	logger := logging.New(&logs, "json", slog.LevelInfo)
	handler := requestID(accessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Set-Cookie", "session=response-cookie")
		_, _ = io.WriteString(w, `{"value":"response-value"}`)
	})))
	rec := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/test?token=query-secret", strings.NewReader(`{"value":"request-value"}`))
	request.Header.Set("Authorization", "Bearer request-secret")
	handler.ServeHTTP(rec, request)
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	loggedRequest, requestOK := record["request"].(map[string]any)
	response, responseOK := record["response"].(map[string]any)
	requestHeaders, requestHeadersOK := loggedRequest["headers"].(map[string]any)
	responseHeaders, responseHeadersOK := response["headers"].(map[string]any)
	if !requestOK || !responseOK || !requestHeadersOK || !responseHeadersOK || loggedRequest["url"] != "/test?token=******" ||
		requestHeaders["authorization"] != "******" || responseHeaders["set-cookie"] != "******" ||
		response["status"] != float64(http.StatusOK) || loggedRequest["body"] != "******" || response["body"] != "******" {
		t.Fatalf("unexpected production access log: %v", record)
	}
	output := logs.String()
	for _, secret := range []string{"query-secret", "request-secret", "response-cookie", "request-value", "response-value"} {
		if strings.Contains(output, secret) {
			t.Fatalf("production access log leaked %q: %s", secret, output)
		}
	}
	if strings.Contains(output, "%2A") {
		t.Fatalf("production access log encoded the star mask: %s", output)
	}
}

func TestAccessLogHeadersUseSafeAllowlist(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	headers.Set("User-Agent", "zentrola-client/1.0")
	headers.Set("Authorization", "Bearer authorization-secret")
	headers.Set("Cookie", "session=cookie-secret")
	headers.Set("X-Api-Key", "api-key-secret")

	encoded, err := json.Marshal(accessLogHeaders(headers, requestHeaderAllowlist))
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, expected := range []string{"content-type", "accept", "user-agent", "zentrola-client/1.0"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("safe header %q missing from %s", expected, output)
		}
	}
	for _, secret := range []string{"authorization-secret", "cookie-secret", "api-key-secret", "authorization", "cookie", "x-api-key"} {
		if strings.Contains(strings.ToLower(output), secret) {
			t.Fatalf("sensitive header %q leaked in %s", secret, output)
		}
	}
}

func TestAccessLogAcceptsOnlySafeUpstreamRequestID(t *testing.T) {
	for _, test := range []struct {
		name, value, expected string
	}{
		{"request id", "upstream-123", "upstream-123"},
		{"fallback x request id", "fallback-456", "fallback-456"},
		{"control character", "unsafe\nvalue", ""},
		{"too long", strings.Repeat("x", 257), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := make(http.Header)
			if test.name == "fallback x request id" {
				headers.Set("X-Request-Id", test.value)
			} else {
				headers.Set("Request-Id", test.value)
			}
			if got := safeUpstreamRequestID(headers); got != test.expected {
				t.Fatalf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestPanicRecoveryDoesNotLeakSecret(t *testing.T) {
	var logs bytes.Buffer
	logger := logging.New(&logs, "json", slog.LevelInfo)
	handler := requestID(accessLog(logger)(recoverPanic(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("panic-secret")
	}))))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/panic", nil))
	if rec.Code != 500 {
		t.Fatal("expected internal error")
	}
	if strings.Contains(logs.String()+rec.Body.String(), "panic-secret") {
		t.Fatal("panic secret leaked")
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/anthropic/v1/messages", nil))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), `"code"`) || !strings.Contains(rec.Body.String(), `"type":"api_error"`) {
		t.Fatal("gateway panic must retain the native error contract")
	}
}
