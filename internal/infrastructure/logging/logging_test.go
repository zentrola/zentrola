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
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestPrettyConsoleAndFileShareFormattedLine(t *testing.T) {
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

	if file.String() != pretty {
		t.Fatalf("file output differs from console output:\nconsole: %q\nfile: %q", pretty, file.String())
	}
}

func TestFileUsesConsoleFormatWithoutANSIColor(t *testing.T) {
	for _, format := range []string{"pretty", "text", "json"} {
		t.Run(format, func(t *testing.T) {
			var console bytes.Buffer
			var file bytes.Buffer
			logger := NewWithOptions(Options{
				Console:       &console,
				ConsoleFormat: format,
				Color:         "always",
				File:          &file,
				Level:         slog.LevelInfo,
				AddSource:     true,
			})
			logger.Info("one line", "status", 200)

			if strings.Count(file.String(), "\n") != 1 || !strings.HasSuffix(file.String(), "\n") {
				t.Fatalf("file output should contain one newline-terminated record: %q", file.String())
			}
			if strings.Contains(file.String(), "\x1b[") {
				t.Fatalf("file output contains ANSI color: %q", file.String())
			}
			if format != "pretty" && file.String() != console.String() {
				t.Fatalf("%s file output differs from console output:\nconsole: %q\nfile: %q", format, console.String(), file.String())
			}
		})
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

func TestPrettyColorAutoSupportsIDEConsole(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "")
	var output bytes.Buffer
	logger := NewWithOptions(Options{Console: &output, ConsoleFormat: "pretty", Color: "auto", Level: slog.LevelInfo})
	logger.Info("IDE console")
	if !strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("auto color should support an ANSI-capable piped console: %q", output.String())
	}
}

func TestPrettyColorAutoHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "")
	var output bytes.Buffer
	logger := NewWithOptions(Options{Console: &output, ConsoleFormat: "pretty", Color: "auto", Level: slog.LevelInfo})
	logger.Info("plain console")
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("NO_COLOR output contains ANSI color: %q", output.String())
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
	directory := t.TempDir()
	writer := &RotatingFile{
		directory: directory, maxBytes: 12, maxBackups: 2,
		now: func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.Local) },
	}
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

	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-1.log"), "record-one\n")
	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-2.log"), "record-two\n")
	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-3.log"), "record-three\n")
}

func TestRotatingFileRejectsConcurrentProcessesAndReleasesLock(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	first, err := OpenRotatingFile(directory, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRotatingFile(directory, 1, 1); !errors.Is(err, ErrFileInUse) {
		if err == nil {
			t.Fatal("second writer unexpectedly acquired the same log path")
		}
		t.Fatalf("second writer returned the wrong error: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenRotatingFile(directory, 1, 1)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRotatingFileStartsNewDateAtFirstSegment(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 9, 17, 23, 59, 0, 0, time.Local)
	writer := &RotatingFile{
		directory: directory, maxBytes: 1024, maxBackups: 10,
		now: func() time.Time { return now },
	}
	if err := writer.open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Write([]byte("before midnight\n")); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 9, 18, 0, 1, 0, 0, time.Local)
	if _, err := writer.Write([]byte("after midnight\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-1.log"), "before midnight\n")
	assertFileContent(t, filepath.Join(directory, "log-2026-09-18-1.log"), "after midnight\n")
}

func TestRotatingFileResumesLatestSegmentForCurrentDate(t *testing.T) {
	directory := t.TempDir()
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.Local) }
	first := &RotatingFile{directory: directory, maxBytes: 1024, maxBackups: 10, now: now}
	if err := first.open(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte("before restart\n")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := &RotatingFile{directory: directory, maxBytes: 1024, maxBackups: 10, now: now}
	if err := second.open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if _, err := second.Write([]byte("after restart\n")); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-1.log"), "before restart\nafter restart\n")
}

func TestRotatingFileRetainsConfiguredHistoryAndIgnoresOtherFiles(t *testing.T) {
	directory := t.TempDir()
	otherPath := filepath.Join(directory, "notes.txt")
	if err := os.WriteFile(otherPath, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	writer := &RotatingFile{
		directory: directory, maxBytes: 4, maxBackups: 1,
		now: func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.Local) },
	}
	if err := writer.open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	for _, record := range []string{"one\n", "two\n", "three\n"} {
		if _, err := writer.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(directory, "log-2026-09-17-1.log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oldest log segment should be removed, got %v", err)
	}
	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-2.log"), "two\n")
	assertFileContent(t, filepath.Join(directory, "log-2026-09-17-3.log"), "three\n")
	assertFileContent(t, otherPath, "keep")
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
