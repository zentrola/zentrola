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

	"github.com/zentrola/zentrola/internal/application/health"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
)

func TestHealthAndRequestCorrelation(t *testing.T) {
	var logs bytes.Buffer
	service := health.New(
		health.Check{Name: "postgres", Run: func(context.Context) error { return errors.New("database-secret") }},
		health.Check{Name: "master_key", Run: health.Pending},
	)
	router := NewRouter(logging.New(&logs, "json", slog.LevelInfo), service, config.CORS{}, time.Second, "prod")
	seen := map[string]bool{}
	for path, want := range map[string]int{"/health/live": 200, "/health/ready": 503, "/": 404, "/missing": 404} {
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
		if !strings.Contains(logs.String(), id) {
			t.Fatal("request ID missing in log")
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

func TestReadyWhenAllDependenciesReady(t *testing.T) {
	service := health.New(health.Check{Name: "postgres", Run: func(context.Context) error { return nil }})
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), service, config.CORS{}, time.Second, "prod")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/health/ready", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"READY"`) {
		t.Fatal(rec.Body.String())
	}
}

func TestCORSAllowlist(t *testing.T) {
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(),
		config.CORS{Enabled: true, Origins: []string{"http://127.0.0.1:3000"}}, time.Second, "prod")
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
		if logging.RequestID(r.Context()) == "" {
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
