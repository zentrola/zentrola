package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	ctx := WithRequestID(context.Background(), "request-123")
	logger.InfoContext(ctx, "service started", "status", 200, "detail", "two words")

	pretty := console.String()
	for _, expected := range []string{
		" INFO  ", "[request_id=request-123]", "[logging_test.go:",
		" - service started", "status=200", `detail="two words"`,
	} {
		if !strings.Contains(pretty, expected) {
			t.Fatalf("pretty output %q does not contain %q", pretty, expected)
		}
	}
	if strings.Contains(pretty, "\x1b[") {
		t.Fatalf("color disabled output contains ANSI escape: %q", pretty)
	}

	var entry map[string]any
	if err := json.Unmarshal(file.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON log %q: %v", file.String(), err)
	}
	if entry["msg"] != "service started" || entry["request_id"] != "request-123" || entry["status"] != float64(200) || entry["detail"] != "two words" {
		t.Fatalf("JSON file did not receive the same record: %#v", entry)
	}
	if _, ok := entry["source"]; !ok {
		t.Fatalf("JSON file is missing source: %#v", entry)
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
