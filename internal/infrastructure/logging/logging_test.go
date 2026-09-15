package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestPrettyConsoleAndJSONFileShareRecord(t *testing.T) {
	var console bytes.Buffer
	var file bytes.Buffer
	logger := NewWithOptions(Options{
		Console:       &console,
		ConsoleFormat: "pretty",
		Color:         "never",
		File:          &file,
		Level:         slog.LevelInfo,
		AddSource:     true,
	})
	provider := sdktrace.NewTracerProvider()
	defer func() { _ = provider.Shutdown(context.Background()) }()
	ctx, span := provider.Tracer("logging-test").Start(context.Background(), "request")
	defer span.End()
	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()
	logger.InfoContext(ctx, "service started", "status", 200, "detail", "two words")

	pretty := console.String()
	for _, expected := range []string{
		" INFO  ", "[" + traceID + "," + spanID + "]", "[logging_test.go:",
		" - service started", "status=200", `detail="two words"`,
	} {
		if !strings.Contains(pretty, expected) {
			t.Fatalf("pretty output %q does not contain %q", pretty, expected)
		}
	}
	if strings.Contains(pretty, "\x1b[") {
		t.Fatalf("color disabled output contains ANSI escape: %q", pretty)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3} [+-]\d{2}:\d{2} `).MatchString(pretty) {
		t.Fatalf("pretty output timestamp is missing timezone offset: %q", pretty)
	}

	var entry map[string]any
	if err := json.Unmarshal(file.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log %q: %v", file.String(), err)
	}
	if entry["msg"] != "service started" || entry["trace_id"] != traceID || entry["span_id"] != spanID || entry["status"] != float64(200) || entry["detail"] != "two words" {
		t.Fatalf("JSON file did not receive the same record: %#v", entry)
	}
	if _, ok := entry["source"]; !ok {
		t.Fatalf("JSON file is missing source: %#v", entry)
	}
}

func TestPrettyCompactsHTTPAccessLog(t *testing.T) {
	var output bytes.Buffer
	logger := NewWithOptions(Options{Console: &output, ConsoleFormat: "pretty", Color: "never", Level: slog.LevelInfo})
	logger.Info("http request",
		"request", map[string]any{
			"method": "GET", "url": "/api/v1/auth/setup", "bytes": 0,
			"headers": map[string]string{"accept": "application/json", "content-type": "application/json"},
		},
		"response", map[string]any{
			"status": 200, "bytes": 91,
			"headers": map[string]string{"content-type": "application/json"},
			"body":    json.RawMessage(`{"code":"OK","data":{"required":"[REDACTED]"}}`),
		},
		"duration_ms", 3,
		"upstream_headers_ms", 2,
		"first_byte_ms", 3,
		"input_tokens", 12,
		"output_tokens", 8,
	)
	formatted := output.String()
	for _, expected := range []string{
		`request={"bytes":0,"headers":{"accept":"application/json","content-type":"application/json"},"method":"GET","url":"/api/v1/auth/setup"}`,
		`response={"body":{"code":"OK","data":{"required":"[REDACTED]"}},"bytes":91,"headers":{"content-type":"application/json"},"status":200}`,
		"cost=3ms",
		"headers=2ms - ttfb=3ms - tokens=12/8/-",
	} {
		if !strings.Contains(formatted, expected) {
			t.Fatalf("compact access log %q does not contain %q", formatted, expected)
		}
	}
	for _, redundant := range []string{"http request", "request_time=", "method=", "duration_ms=", "path=", "request_body=", "response_body=", `\"code\"`} {
		if strings.Contains(formatted, redundant) {
			t.Fatalf("compact access log contains redundant %q: %q", redundant, formatted)
		}
	}
}

func TestTypedNilFileWriterIsDisabled(t *testing.T) {
	var console bytes.Buffer
	var file *RotatingFile
	logger := NewWithOptions(Options{
		Console:       &console,
		ConsoleFormat: "pretty",
		Color:         "never",
		File:          file,
		Level:         slog.LevelInfo,
	})
	logger.Info("console only")
	if !strings.Contains(console.String(), "console only") {
		t.Fatalf("console output missing: %q", console.String())
	}
}

func TestPrettyColorAlways(t *testing.T) {
	var output bytes.Buffer
	logger := NewWithOptions(Options{Console: &output, ConsoleFormat: "pretty", Color: "always", Level: slog.LevelInfo})
	logger.Warn("attention")
	if !strings.Contains(output.String(), "\x1b[33mWARN ") {
		t.Fatalf("warning level is not colored: %q", output.String())
	}
	if !strings.Contains(output.String(), "[]") {
		t.Fatalf("missing trace context should be empty: %q", output.String())
	}
	if strings.Contains(output.String(), "trace_id=") || strings.Contains(output.String(), "span_id=") {
		t.Fatalf("pretty trace context should contain values only: %q", output.String())
	}
}

func TestPrettyDoesNotPadComponent(t *testing.T) {
	var output bytes.Buffer
	logger := NewWithOptions(Options{Console: &output, ConsoleFormat: "pretty", Color: "never", Level: slog.LevelInfo})
	logger.Info("compact")
	if strings.Contains(output.String(), "logging.TestPrettyDoesNotPadComponent        [") {
		t.Fatalf("component contains fixed-width padding: %q", output.String())
	}
	if !strings.Contains(output.String(), "TestPrettyDoesNotPadComponent [logging_test.go:") {
		t.Fatalf("source should follow component with one space: %q", output.String())
	}
}

func TestPrettyEscapesTerminalControlCharacters(t *testing.T) {
	var output bytes.Buffer
	logger := NewWithOptions(Options{Console: &output, ConsoleFormat: "pretty", Color: "never", Level: slog.LevelInfo})
	logger.Info("message\ncontinued", "untrusted", "\x1b[31mred")
	if strings.Contains(output.String(), "\ncontinued\n") || strings.Contains(output.String(), "\x1b[31m") {
		t.Fatalf("pretty output contains raw control characters: %q", output.String())
	}
	if !strings.Contains(output.String(), `message\ncontinued`) || !strings.Contains(output.String(), `\x1b[31mred`) {
		t.Fatalf("pretty output did not visibly escape control characters: %q", output.String())
	}
}

func TestFileFailureDoesNotStopConsoleAndIsReportedOnce(t *testing.T) {
	var console bytes.Buffer
	var failures bytes.Buffer
	logger := NewWithOptions(Options{
		Console:       &console,
		ConsoleFormat: "pretty",
		Color:         "never",
		File:          failingWriter{},
		ErrorOutput:   &failures,
		Level:         slog.LevelInfo,
	})
	logger.Info("first")
	logger.Info("second")
	if !strings.Contains(console.String(), "first") || !strings.Contains(console.String(), "second") {
		t.Fatalf("console output stopped after file failure: %q", console.String())
	}
	if strings.Count(failures.String(), "logging file output failed") != 1 {
		t.Fatalf("file failure should be reported once, got %q", failures.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("test failure")
}

func TestRotatingFileKeepsWholeRecordsAndBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zentrola.jsonl")
	writer := &RotatingFile{path: path, maxBytes: 12, maxBackups: 2}
	if err := writer.open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	for _, record := range []string{"record-one\n", "record-two\n", "record-three\n"} {
		if _, err := writer.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, path, "record-three\n")
	assertFileContent(t, path+".1", "record-two\n")
	assertFileContent(t, path+".2", "record-one\n")
}

func TestRotatingFileRejectsConcurrentProcessesAndReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.jsonl")
	first, err := OpenRotatingFile(path, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRotatingFile(path, 1, 1); !errors.Is(err, ErrFileInUse) {
		if err == nil {
			t.Fatal("second writer unexpectedly acquired the same log path")
		}
		t.Fatalf("second writer returned the wrong error: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenRotatingFile(path, 1, 1)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("got %q in %s, want %q", content, path, expected)
	}
}
