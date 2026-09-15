// Package logging 使用 slog 原生接口，并把请求字段附加到日志。
package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func TraceIDs(ctx context.Context) (traceID, spanID string) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return "", ""
	}
	return spanContext.TraceID().String(), spanContext.SpanID().String()
}

func New(out io.Writer, format string, level slog.Level) *slog.Logger {
	return NewWithOptions(Options{Console: out, ConsoleFormat: format, Color: "never", Level: level})
}

// Options 配置同一条日志记录的控制台和 JSON 文件输出。
// File 为 nil 时只输出到控制台。
type Options struct {
	Console       io.Writer
	ConsoleFormat string
	Color         string
	File          io.Writer
	ErrorOutput   io.Writer
	Level         slog.Level
	AddSource     bool
}

func NewWithOptions(options Options) *slog.Logger {
	handlerOptions := &slog.HandlerOptions{Level: options.Level, AddSource: options.AddSource}
	var handlers []slog.Handler
	if writerConfigured(options.Console) {
		switch options.ConsoleFormat {
		case "json":
			handlers = append(handlers, slog.NewJSONHandler(options.Console, handlerOptions))
		case "pretty":
			handlers = append(handlers, newPrettyHandler(options.Console, options.Color, options.Level))
		default:
			handlers = append(handlers, slog.NewTextHandler(options.Console, handlerOptions))
		}
	}
	if writerConfigured(options.File) {
		fileOutput := options.File
		if options.ErrorOutput != nil {
			fileOutput = &reportingWriter{out: options.File, errors: options.ErrorOutput}
		}
		handlers = append(handlers, slog.NewJSONHandler(fileOutput, handlerOptions))
	}
	var handler slog.Handler
	if len(handlers) == 1 {
		handler = handlers[0]
	} else {
		handler = fanoutHandler(handlers)
	}
	return slog.New(contextHandler{Handler: handler})
}

// io.Writer 可能包含一个 typed nil 指针，此时接口值本身并不等于 nil。
func writerConfigured(writer io.Writer) bool {
	if writer == nil {
		return false
	}
	value := reflect.ValueOf(writer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	hasTraceID, hasSpanID := false, false
	record.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "trace_id":
			hasTraceID = true
		case "span_id":
			hasSpanID = true
		}
		return true
	})
	traceID, spanID := TraceIDs(ctx)
	if !hasTraceID {
		record.AddAttrs(slog.String("trace_id", traceID))
	}
	if !hasSpanID {
		record.AddAttrs(slog.String("span_id", spanID))
	}
	return h.Handler.Handle(ctx, record)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{Handler: h.Handler.WithGroup(name)}
}

type fanoutHandler []slog.Handler

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var result error
	for _, handler := range h {
		if handler.Enabled(ctx, record.Level) {
			result = errors.Join(result, handler.Handle(ctx, record.Clone()))
		}
	}
	return result
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := make(fanoutHandler, len(h))
	for i, handler := range h {
		result[i] = handler.WithAttrs(attrs)
	}
	return result
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	result := make(fanoutHandler, len(h))
	for i, handler := range h {
		result[i] = handler.WithGroup(name)
	}
	return result
}

type prettyHandler struct {
	out    io.Writer
	level  slog.Leveler
	color  bool
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newPrettyHandler(out io.Writer, color string, level slog.Leveler) slog.Handler {
	return &prettyHandler{out: out, level: level, color: useColor(out, color), mu: &sync.Mutex{}}
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *prettyHandler) Handle(_ context.Context, record slog.Record) error {
	var attributes []slog.Attr
	attributes = append(attributes, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attributes = append(attributes, attr)
		return true
	})
	traceID, spanID := "", ""
	for _, attr := range attributes {
		if attr.Value.Resolve().Kind() != slog.KindString {
			continue
		}
		switch attr.Key {
		case "trace_id":
			traceID = attr.Value.String()
		case "span_id":
			spanID = attr.Value.String()
		}
	}

	when := record.Time
	if when.IsZero() {
		when = time.Now()
	}
	component, source := sourceLabels(record.PC)
	level := strings.ToUpper(record.Level.String())
	levelText := fmt.Sprintf("%-5s", level)

	var line bytes.Buffer
	line.WriteString(paint(h.color, "90", when.Local().Format("2006-01-02 15:04:05.000 -07:00")))
	line.WriteByte(' ')
	line.WriteString(paint(h.color, levelColor(record.Level), levelText))
	line.WriteByte(' ')
	traceContext := singleLine(traceID)
	if spanID = singleLine(spanID); spanID != "" {
		if traceContext != "" {
			traceContext += ","
		}
		traceContext += spanID
	}
	line.WriteString(paint(h.color, "35", "["+traceContext+"]"))
	line.WriteByte(' ')
	line.WriteString(paint(h.color, "33", component))
	line.WriteByte(' ')
	line.WriteString(paint(h.color, "32", "["+source+"]"))
	line.WriteString(" - ")
	message, remaining := compactPrettyRecord(record.Message, attributes)
	line.WriteString(message)
	appendAttrs(&line, h.groups, remaining)
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(line.Bytes())
	return err
}

var compactAccessKeys = map[string]struct{}{
	"request": {}, "response": {}, "duration_ms": {},
	"protocol": {}, "upstream_status": {}, "upstream_headers_ms": {}, "first_byte_ms": {},
	"input_tokens": {}, "output_tokens": {}, "cached_input_tokens": {},
	"provider_id": {}, "resource_id": {}, "model_id": {}, "provider_model_id": {},
	"upstream_request_id": {},
}

// compactPrettyRecord 为访问日志提供固定的人类可读布局；JSON Handler 仍保留完整字段名。
func compactPrettyRecord(message string, attributes []slog.Attr) (string, []slog.Attr) {
	if message != "http request" {
		return singleLine(message), attributes
	}
	values := make(map[string]slog.Value, len(compactAccessKeys))
	remaining := make([]slog.Attr, 0, len(attributes))
	for _, attr := range attributes {
		if _, compact := compactAccessKeys[attr.Key]; compact {
			values[attr.Key] = attr.Value.Resolve()
			continue
		}
		remaining = append(remaining, attr)
	}

	var line strings.Builder
	appendCompactObject(&line, "request=", values["request"])
	appendCompactObject(&line, "response=", values["response"])
	appendCompactValue(&line, "cost=", values["duration_ms"], "ms")
	appendCompactValue(&line, "proto=", values["protocol"], "")
	appendCompactValue(&line, "upstream=", values["upstream_status"], "")
	appendCompactValue(&line, "headers=", values["upstream_headers_ms"], "ms")
	appendCompactValue(&line, "ttfb=", values["first_byte_ms"], "ms")
	appendCompactTokens(&line, values)
	appendCompactValue(&line, "provider=", values["provider_id"], "")
	appendCompactValue(&line, "resource=", values["resource_id"], "")
	appendCompactValue(&line, "model=", values["model_id"], "")
	appendCompactValue(&line, "provider_model=", values["provider_model_id"], "")
	appendCompactValue(&line, "upstream_request=", values["upstream_request_id"], "")
	if line.Len() == 0 {
		return "http request", remaining
	}
	return line.String(), remaining
}

func appendCompactValue(line *strings.Builder, prefix string, value slog.Value, suffix string) {
	if value.Kind() == slog.KindAny && value.Any() == nil {
		return
	}
	appendCompactText(line, prefix+formatValue(value)+suffix)
}

func appendCompactText(line *strings.Builder, value string) {
	if value == "" {
		return
	}
	if line.Len() > 0 {
		line.WriteString(" - ")
	}
	line.WriteString(value)
}

func appendCompactTokens(line *strings.Builder, values map[string]slog.Value) {
	input, inputOK := values["input_tokens"]
	output, outputOK := values["output_tokens"]
	cached, cachedOK := values["cached_input_tokens"]
	if !inputOK && !outputOK && !cachedOK {
		return
	}
	parts := []string{"-", "-", "-"}
	if inputOK {
		parts[0] = formatValue(input)
	}
	if outputOK {
		parts[1] = formatValue(output)
	}
	if cachedOK {
		parts[2] = formatValue(cached)
	}
	appendCompactText(line, "tokens="+strings.Join(parts, "/"))
}

func appendCompactObject(line *strings.Builder, prefix string, value slog.Value) {
	if value.Kind() == slog.KindAny && value.Any() == nil {
		return
	}
	encoded, err := json.Marshal(value.Any())
	if err != nil || bytes.Equal(encoded, []byte("null")) || bytes.Equal(encoded, []byte("{}")) {
		return
	}
	appendCompactText(line, prefix+singleLine(string(encoded)))
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func appendAttrs(line *bytes.Buffer, groups []string, attrs []slog.Attr) {
	for _, attr := range attrs {
		appendAttr(line, groups, attr)
	}
}

func appendAttr(line *bytes.Buffer, groups []string, attr slog.Attr) {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		nested := groups
		if attr.Key != "" {
			nested = append(append([]string(nil), groups...), attr.Key)
		}
		for _, child := range value.Group() {
			appendAttr(line, nested, child)
		}
		return
	}
	if attr.Key == "" || (len(groups) == 0 && (attr.Key == "trace_id" || attr.Key == "span_id")) {
		return
	}
	key := strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
	line.WriteByte(' ')
	line.WriteString(key)
	line.WriteByte('=')
	line.WriteString(formatValue(value))
}

func formatValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		text := singleLine(value.String())
		if text == "" || strings.ContainsAny(text, " \t=\"") {
			return strconv.Quote(text)
		}
		return text
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindAny:
		return strconv.Quote(singleLine(fmt.Sprint(value.Any())))
	default:
		return strconv.Quote(singleLine(value.String()))
	}
}

func sourceLabels(pc uintptr) (string, string) {
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if frame.Function == "" {
		return "-", "-"
	}
	component := strings.TrimPrefix(frame.Function, "github.com/zentrola/zentrola/")
	component = strings.TrimPrefix(component, "internal/")
	component = strings.TrimPrefix(component, "cmd/")
	return component, filepath.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
}

func singleLine(value string) string {
	var escaped strings.Builder
	for _, character := range value {
		switch character {
		case '\r':
			escaped.WriteString("\\r")
		case '\n':
			escaped.WriteString("\\n")
		case '\t':
			escaped.WriteString("\\t")
		default:
			if character < 0x20 || character == 0x7f {
				_, _ = fmt.Fprintf(&escaped, "\\x%02x", character)
			} else {
				escaped.WriteRune(character)
			}
		}
	}
	return escaped.String()
}

func useColor(out io.Writer, mode string) bool {
	switch mode {
	case "always", "true":
		return true
	case "never", "false":
		return false
	}
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func paint(enabled bool, code, value string) string {
	if !enabled {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func levelColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "31"
	case level >= slog.LevelWarn:
		return "33"
	case level < slog.LevelInfo:
		return "36"
	default:
		return "32"
	}
}

type reportingWriter struct {
	out    io.Writer
	errors io.Writer
	once   sync.Once
}

func (w *reportingWriter) Write(data []byte) (int, error) {
	n, err := w.out.Write(data)
	if err != nil {
		w.once.Do(func() {
			_, _ = fmt.Fprintf(w.errors, "logging file output failed: %v\n", err)
		})
	}
	return n, err
}
