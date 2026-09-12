package gateway

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// UsageObserver 旁路读取字节，绝不重编码 Gateway 响应或保留完整响应。
// SSE 仅缓存当前事件（最多 256 KiB）；JSON 只捕获顶层 type / usage。
type UsageObserver struct {
	openai                        bool
	responses                     bool
	stream                        bool
	autoDetect, formatDetected    bool
	fallbackStream                bool
	prefix                        []byte
	line, data                    []byte
	drop                          bool
	start, stop, failed, badUsage bool
	input, output, cached         *int64
	eventCount                    int64
	lastEvent                     string
	json                          metadataJSON
}

type UsageDiagnostics struct {
	Stream       bool
	Started      bool
	Stopped      bool
	Failed       bool
	BadUsage     bool
	JSONComplete bool
	JSONInvalid  bool
	InputSeen    bool
	OutputSeen   bool
	CachedSeen   bool
	EventCount   int64
	LastEvent    string
}

func NewOpenAIUsageObserver(stream bool) *UsageObserver {
	o := &UsageObserver{stream: stream, openai: true}
	o.json.kindKey = "object"
	o.json.allowNullUsage = true
	o.json.accept = func(key string, raw []byte) {
		if key == "object" {
			var object string
			if json.Unmarshal(raw, &object) != nil || object != "chat.completion" {
				o.failed = true
			} else {
				o.start = true
			}
		}
		if key == "usage" && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			o.update(raw)
		}
	}
	return o
}

func NewOpenAIResponsesUsageObserver(stream bool) *UsageObserver {
	o := &UsageObserver{stream: stream, openai: true, responses: true}
	o.json.kindKey = "object"
	o.json.allowNullUsage = true
	o.json.accept = func(key string, raw []byte) {
		if key == "object" {
			var object string
			if json.Unmarshal(raw, &object) != nil || object != "response" {
				o.failed = true
			} else {
				o.start = true
			}
		}
		if key == "usage" && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			o.update(raw)
		}
	}
	return o
}

// NewAutoOpenAIResponsesUsageObserver 在上游未声明 Content-Type 时，通过最多
// 4 KiB 的响应前缀识别 JSON 或 SSE。无法识别时使用请求声明作为兜底。
func NewAutoOpenAIResponsesUsageObserver(fallbackStream bool) *UsageObserver {
	o := NewOpenAIResponsesUsageObserver(fallbackStream)
	o.autoDetect = true
	o.fallbackStream = fallbackStream
	return o
}

func NewUsageObserver(stream bool) *UsageObserver {
	o := &UsageObserver{stream: stream}
	o.json.kindKey = "type"
	o.json.accept = func(key string, raw []byte) {
		if key == "type" {
			var kind string
			if json.Unmarshal(raw, &kind) != nil || kind != "message" {
				o.failed = true
			} else {
				o.start = true
			}
		}
		if key == "usage" {
			o.update(raw)
		}
	}
	return o
}
func (o *UsageObserver) Feed(p []byte) {
	if o.autoDetect && !o.formatDetected {
		o.detectAndFeed(p)
		return
	}
	o.feed(p)
}

const responseFormatDetectionLimit = 4 << 10

func (o *UsageObserver) detectAndFeed(p []byte) {
	remaining := responseFormatDetectionLimit - len(o.prefix)
	if remaining > len(p) {
		remaining = len(p)
	}
	o.prefix = append(o.prefix, p[:remaining]...)
	stream, detected := detectResponseStream(o.prefix, o.fallbackStream)
	if !detected && len(o.prefix) < responseFormatDetectionLimit {
		return
	}
	if !detected {
		stream = o.fallbackStream
	}
	o.stream = stream
	o.formatDetected = true
	prefix := o.prefix
	o.prefix = nil
	o.feed(prefix)
	if remaining < len(p) {
		o.feed(p[remaining:])
	}
}

func detectResponseStream(prefix []byte, fallbackStream bool) (stream, detected bool) {
	prefix = bytes.TrimLeft(prefix, " \r\n\t")
	if len(prefix) == 0 {
		return false, false
	}
	if prefix[0] == '{' {
		return false, true
	}
	if prefix[0] == ':' {
		return true, true
	}
	for _, marker := range [][]byte{[]byte("event:"), []byte("data:"), []byte("id:"), []byte("retry:")} {
		if bytes.HasPrefix(prefix, marker) {
			return true, true
		}
		if bytes.HasPrefix(marker, prefix) {
			return false, false
		}
	}
	return fallbackStream, true
}

func (o *UsageObserver) feed(p []byte) {
	if !o.stream {
		o.json.feed(p)
		return
	}
	for _, b := range p {
		if b == '\n' {
			o.sseLine()
			clear(o.line)
			o.line = o.line[:0]
			continue
		}
		if len(o.line) < 256<<10 {
			o.line = append(o.line, b)
		} else {
			o.drop = true
			o.badUsage = true
		}
	}
}
func (o *UsageObserver) sseLine() {
	line := bytes.TrimSuffix(o.line, []byte{'\r'})
	if len(line) == 0 {
		if !o.drop && len(o.data) > 0 {
			o.event(o.data)
		}
		clear(o.data)
		o.data = o.data[:0]
		o.drop = false
		return
	}
	if bytes.HasPrefix(line, []byte("data:")) {
		part := bytes.TrimPrefix(line[5:], []byte{' '})
		if len(o.data)+len(part)+1 > 256<<10 {
			o.drop = true
			o.badUsage = true
			return
		}
		o.data = append(o.data, part...)
		o.data = append(o.data, '\n')
	}
}
func (o *UsageObserver) event(raw []byte) {
	if o.openai {
		if o.responses {
			var event struct {
				Type     string `json:"type"`
				Response struct {
					Usage json.RawMessage `json:"usage"`
				} `json:"response"`
				Error json.RawMessage `json:"error"`
			}
			if json.Unmarshal(raw, &event) != nil {
				o.eventCount++
				o.lastEvent = "[invalid-json]"
				o.badUsage = true
				return
			}
			o.eventCount++
			o.lastEvent = event.Type
			if len(o.lastEvent) > 128 {
				o.lastEvent = o.lastEvent[:128] + "..."
			}
			switch event.Type {
			case "response.created", "response.in_progress":
				o.start = true
			case "response.completed":
				o.start = true
				o.update(event.Response.Usage)
				o.stop = true
			case "error", "response.failed", "response.incomplete":
				o.failed = true
			}
			return
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
			o.stop = true
			return
		}
		var chunk struct {
			Object string          `json:"object"`
			Usage  json.RawMessage `json:"usage"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(raw, &chunk) != nil {
			o.badUsage = true
			return
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			o.failed = true
			return
		}
		if chunk.Object == "chat.completion.chunk" {
			if o.stop {
				o.badUsage = true
			}
			o.start = true
			if len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
				o.update(chunk.Usage)
			}
		}
		return
	}
	var e struct {
		Type    string          `json:"type"`
		Usage   json.RawMessage `json:"usage"`
		Message struct {
			Usage json.RawMessage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &e) != nil {
		o.badUsage = true
		return
	}
	switch e.Type {
	case "message_start":
		if o.start {
			o.badUsage = true
		}
		o.start = true
		o.update(e.Message.Usage)
	case "message_delta":
		if !o.start || o.stop {
			o.badUsage = true
		}
		o.update(e.Usage)
	case "message_stop":
		o.stop = true
	case "error":
		o.failed = true
	}
}
func (o *UsageObserver) update(raw []byte) {
	if len(raw) == 0 {
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	first, err := dec.Token()
	if err != nil || first != json.Delim('{') {
		o.badUsage = true
		return
	}
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			o.badUsage = true
			return
		}
		key, ok := t.(string)
		if !ok || seen[key] {
			o.badUsage = true
			return
		}
		seen[key] = true
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			o.badUsage = true
			return
		}
		var dst **int64
		if o.openai {
			if o.responses {
				switch key {
				case "input_tokens":
					dst = &o.input
				case "output_tokens":
					dst = &o.output
				case "input_tokens_details":
					var details struct {
						Cached json.RawMessage `json:"cached_tokens"`
					}
					if json.Unmarshal(value, &details) != nil {
						o.badUsage = true
						continue
					}
					if len(details.Cached) == 0 {
						continue
					}
					dst = &o.cached
					value = details.Cached
				default:
					continue
				}
			} else {
				switch key {
				case "prompt_tokens":
					dst = &o.input
				case "completion_tokens":
					dst = &o.output
				case "prompt_cache_hit_tokens":
					dst = &o.cached
				case "prompt_tokens_details":
					var details struct {
						Cached json.RawMessage `json:"cached_tokens"`
					}
					if json.Unmarshal(value, &details) != nil {
						o.badUsage = true
						continue
					}
					if len(details.Cached) == 0 {
						continue
					}
					dst = &o.cached
					value = details.Cached
				default:
					continue
				}
			}
		} else {
			switch key {
			case "input_tokens":
				dst = &o.input
			case "output_tokens":
				dst = &o.output
			case "cache_read_input_tokens":
				dst = &o.cached
			default:
				continue
			}
		}
		if string(value) == "null" {
			*dst = nil
			continue
		}
		n, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil || n < 0 {
			o.badUsage = true
			continue
		}
		// message_delta 为累计计数，覆盖前值，绝不累加。回退计数视为不可靠。
		if *dst != nil && n < **dst {
			o.badUsage = true
			continue
		}
		*dst = &n
	}
	if _, err := dec.Token(); err != nil {
		o.badUsage = true
	}
}
func (o *UsageObserver) Complete() bool {
	if o.stream {
		return o.start && o.stop && !o.failed
	}
	return o.start && o.json.complete && !o.json.invalid && !o.failed
}
func (o *UsageObserver) ErrorCode() string {
	if o.failed {
		return "UPSTREAM_STREAM_ERROR"
	}
	return "UPSTREAM_RESPONSE_INCOMPLETE"
}
func (o *UsageObserver) Tokens(complete bool) (*int64, *int64, *int64) {
	if o.badUsage || o.json.invalid {
		return nil, nil, nil
	}
	if !complete {
		return o.input, nil, o.cached
	}
	return o.input, o.output, o.cached
}

func (o *UsageObserver) Diagnostics() UsageDiagnostics {
	if o == nil {
		return UsageDiagnostics{}
	}
	return UsageDiagnostics{
		Stream:       o.stream,
		Started:      o.start,
		Stopped:      o.stop,
		Failed:       o.failed,
		BadUsage:     o.badUsage,
		JSONComplete: o.json.complete,
		JSONInvalid:  o.json.invalid,
		InputSeen:    o.input != nil,
		OutputSeen:   o.output != nil,
		CachedSeen:   o.cached != nil,
		EventCount:   o.eventCount,
		LastEvent:    o.lastEvent,
	}
}

type metadataJSON struct {
	kindKey                                       string
	allowNullUsage                                bool
	stack                                         []byte
	quoted, escape, keyString, wantKey, wantValue bool
	keyRaw                                        []byte
	key                                           string
	capture                                       []byte
	capturing                                     bool
	seen                                          map[string]bool
	complete, invalid                             bool
	accept                                        func(string, []byte)
}

func (j *metadataJSON) feed(p []byte) {
	for _, b := range p {
		j.byte(b)
	}
}
func (j *metadataJSON) byte(b byte) {
	if j.invalid {
		return
	}
	if j.complete {
		if !strings.ContainsRune(" \r\n\t", rune(b)) {
			j.invalid = true
		}
		return
	}
	if j.capturing {
		if len(j.capture) >= 64<<10 {
			j.invalid = true
			return
		}
		j.capture = append(j.capture, b)
	}
	if j.quoted {
		if j.keyString {
			if len(j.keyRaw) > 128 {
				j.invalid = true
				return
			}
			j.keyRaw = append(j.keyRaw, b)
		}
		if j.escape {
			j.escape = false
			return
		}
		if b == '\\' {
			j.escape = true
			return
		}
		if b < 32 {
			j.invalid = true
			return
		}
		if b == '"' {
			j.quoted = false
			if j.keyString {
				if json.Unmarshal(j.keyRaw, &j.key) != nil {
					j.invalid = true
				}
				j.keyString = false
				j.wantKey = false
			}
			if j.capturing && len(j.stack) == 1 {
				j.finish()
			}
		}
		return
	}
	if strings.ContainsRune(" \r\n\t", rune(b)) {
		return
	}
	if len(j.stack) == 1 && j.wantValue {
		j.wantValue = false
		if j.key == "usage" || j.key == j.kindKey {
			if j.key == "usage" && b == 'n' && j.allowNullUsage {
				return
			}
			if (j.key == "usage" && b != '{') || (j.key == j.kindKey && b != '"') {
				j.invalid = true
				return
			}
			j.capturing = true
			j.capture = append(j.capture[:0], b)
		}
	}
	switch b {
	case '"':
		j.quoted = true
		if len(j.stack) == 1 && j.wantKey {
			j.keyString = true
			j.keyRaw = append(j.keyRaw[:0], b)
		}
	case '{', '[':
		if len(j.stack) == 0 {
			if b != '{' {
				j.invalid = true
				return
			}
			j.wantKey = true
		}
		if len(j.stack) >= 256 {
			j.invalid = true
			return
		}
		j.stack = append(j.stack, b)
	case '}', ']':
		if len(j.stack) == 0 {
			j.invalid = true
			return
		}
		last := j.stack[len(j.stack)-1]
		if (b == '}' && last != '{') || (b == ']' && last != '[') {
			j.invalid = true
			return
		}
		j.stack = j.stack[:len(j.stack)-1]
		if j.capturing && len(j.stack) == 1 {
			j.finish()
		}
		if len(j.stack) == 0 {
			j.complete = true
		}
	case ':':
		if len(j.stack) == 1 {
			j.wantValue = true
		}
	case ',':
		if len(j.stack) == 1 {
			j.wantKey = true
		}
	}
}
func (j *metadataJSON) finish() {
	if j.seen == nil {
		j.seen = map[string]bool{}
	}
	if j.seen[j.key] {
		j.invalid = true
		return
	}
	j.seen[j.key] = true
	if !json.Valid(j.capture) {
		j.invalid = true
		return
	}
	j.accept(j.key, j.capture)
	clear(j.capture)
	j.capture = j.capture[:0]
	j.capturing = false
}
