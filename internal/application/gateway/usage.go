package gateway

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// UsageObserver 旁路读取字节，绝不重编码 Gateway 响应或保留完整响应。
// SSE 仅缓存当前事件（最多 256 KiB）；超限的 Responses 事件改为流式扫描，
// 只抽取 type 与 response.usage。JSON 只捕获顶层 type / usage。
type UsageObserver struct {
	openai                        bool
	responses                     bool
	stream                        bool
	autoDetect, formatDetected    bool
	fallbackStream                bool
	prefix                        []byte
	line, data                    []byte
	lineMode                      sseLineMode
	scan                          *responsesEventScanner
	eventName                     string
	drop                          bool
	start, stop, failed, badUsage bool
	protocolInvalid               bool
	usageSeen                     bool
	blocks                        map[int]bool
	nextBlock                     int
	// input 保存上游原始输入计数。Anthropic 的 input_tokens 不含缓存，
	// cacheCreation / cached 分别保存 cache_creation / cache_read，产出时再归一。
	input, output, cached *int64
	cacheCreation         *int64
	// compatibility 仅用于 OpenAI 协议：经兼容转换的 Anthropic 上游缓存写入数。
	compatibility *CompatibilityUsage
	eventCount    int64
	lastEvent     string
	json          metadataJSON
}

// sseLineMode 描述当前 SSE 行的处理方式。超过单事件上限后每行只判定一次，
// 剩余字节直接流入扫描器或被跳过，不再逐字节重复判定。
type sseLineMode uint8

const (
	sseLineBuffered sseLineMode = iota
	sseLineStreaming
	sseLineDiscard
)

const sseEventLimit = 256 << 10

type UsageDiagnostics struct {
	Stream          bool
	Started         bool
	Stopped         bool
	Failed          bool
	ProtocolInvalid bool
	BadUsage        bool
	JSONComplete    bool
	JSONInvalid     bool
	InputSeen       bool
	OutputSeen      bool
	CachedSeen      bool
	EventCount      int64
	LastEvent       string
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
	for len(p) > 0 {
		newline := bytes.IndexByte(p, '\n')
		segment := p
		if newline >= 0 {
			segment = p[:newline]
		}
		switch o.lineMode {
		case sseLineStreaming:
			o.scan.feed(segment)
		case sseLineDiscard:
		default:
			if room := sseEventLimit - len(o.line); len(segment) > room {
				o.line = append(o.line, segment[:room]...)
				o.overflowLine()
				p = p[room:]
				continue
			}
			o.line = append(o.line, segment...)
		}
		if newline < 0 {
			return
		}
		p = p[newline+1:]
		o.endLine()
	}
}

func (o *UsageObserver) endLine() {
	mode := o.lineMode
	o.lineMode = sseLineBuffered
	switch mode {
	case sseLineBuffered:
		o.sseLine()
	case sseLineStreaming:
		// 与缓存路径保持一致：多条 data 行按 "\n" 拼接。
		o.scan.feed([]byte{'\n'})
	}
	clear(o.line)
	o.line = o.line[:0]
}

// overflowLine 在单行超过事件上限时调用，每行只调用一次。
func (o *UsageObserver) overflowLine() {
	o.lineMode = sseLineDiscard
	if o.drop {
		return
	}
	switch {
	case bytes.HasPrefix(o.line, []byte("data:")):
		if o.responses && o.scannableResponsesEvent() {
			o.startResponsesScan()
			o.scan.feed(bytes.TrimPrefix(o.line[5:], []byte{' '}))
			o.lineMode = sseLineStreaming
			return
		}
		o.dropCurrentSSEEvent()
	case bytes.HasPrefix(o.line, []byte("event:")):
		o.dropCurrentSSEEvent()
	}
	// 注释、id、retry 等字段不参与 usage 判定，超长时仅跳过该行。
}

func (o *UsageObserver) sseLine() {
	line := bytes.TrimSuffix(o.line, []byte{'\r'})
	if len(line) == 0 {
		o.endEvent()
		return
	}
	if o.drop {
		return
	}
	if bytes.HasPrefix(line, []byte("event:")) {
		name := strings.TrimSpace(string(line[6:]))
		if len(name) > 128 {
			o.dropCurrentSSEEvent()
			return
		}
		o.eventName = name
		return
	}
	if bytes.HasPrefix(line, []byte("data:")) {
		part := bytes.TrimPrefix(line[5:], []byte{' '})
		if o.scan == nil && len(o.data)+len(part)+1 > sseEventLimit {
			if !o.responses || !o.scannableResponsesEvent() {
				o.dropCurrentSSEEvent()
				return
			}
			o.startResponsesScan()
		}
		if o.scan != nil {
			o.scan.feed(part)
			o.scan.feed([]byte{'\n'})
			return
		}
		o.data = append(o.data, part...)
		o.data = append(o.data, '\n')
	}
}

func (o *UsageObserver) endEvent() {
	if !o.drop {
		if o.scan != nil {
			o.finishResponsesScan()
		} else if len(o.data) > 0 {
			o.event(o.data)
		}
	}
	clear(o.data)
	o.data = o.data[:0]
	o.drop = false
	o.scan = nil
	o.eventName = ""
}

func (o *UsageObserver) dropCurrentSSEEvent() {
	o.drop = true
	o.scan = nil
	// event 字段已声明为不携带 usage 或终止状态的 Responses 事件（如图像生成的
	// 数 MiB base64 片段）跳过观察即可；原始字节仍由 HTTP 转发链路直接交付客户端。
	if o.responses && !o.scannableResponsesEvent() {
		return
	}
	o.badUsage = true
}

// scannableResponsesEvent 判断超限事件是否可能决定开始、完成、失败或 usage。
// event 字段不是 SSE 必填项，未声明时无法预知类型，同样需要扫描。
func (o *UsageObserver) scannableResponsesEvent() bool {
	switch o.eventName {
	case "", "response.created", "response.in_progress", "response.completed", "response.done",
		"response.failed", "response.incomplete", "error":
		return true
	}
	return false
}

func (o *UsageObserver) startResponsesScan() {
	if o.scan != nil {
		return
	}
	o.scan = &responsesEventScanner{}
	o.scan.feed(o.data)
	clear(o.data)
	o.data = o.data[:0]
}

func (o *UsageObserver) finishResponsesScan() {
	kind, usage, ok := o.scan.result()
	if !ok {
		o.eventCount++
		o.lastEvent = "[invalid-json]"
		o.badUsage = true
		return
	}
	if o.scan.usageInvalid && kind == "response.completed" {
		o.badUsage = true
	}
	o.responsesEvent(kind, usage)
}

func (o *UsageObserver) responsesEvent(kind string, usage []byte) {
	o.eventCount++
	o.lastEvent = kind
	if len(o.lastEvent) > 128 {
		o.lastEvent = o.lastEvent[:128] + "..."
	}
	switch kind {
	case "response.created", "response.in_progress":
		o.start = true
	case "response.completed":
		o.start = true
		o.update(usage)
		o.stop = true
	case "error", "response.failed", "response.incomplete":
		o.failed = true
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
			}
			if json.Unmarshal(raw, &event) != nil {
				o.eventCount++
				o.lastEvent = "[invalid-json]"
				o.badUsage = true
				return
			}
			o.responsesEvent(event.Type, event.Response.Usage)
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
				o.usageSeen = true
				o.update(chunk.Usage)
			}
		}
		return
	}
	var e struct {
		Type    string          `json:"type"`
		Index   *int            `json:"index"`
		Usage   json.RawMessage `json:"usage"`
		Message struct {
			Usage json.RawMessage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &e) != nil {
		o.badUsage = true
		o.invalidateProtocol()
		return
	}
	switch e.Type {
	case "message_start":
		if o.start || o.stop {
			o.invalidateProtocol()
			return
		}
		o.start = true
		o.update(e.Message.Usage)
	case "content_block_start":
		if !o.start || o.stop || o.hasOpenBlocks() || e.Index == nil || *e.Index != o.nextBlock {
			o.invalidateProtocol()
			return
		}
		if o.blocks == nil {
			o.blocks = map[int]bool{}
		}
		if _, exists := o.blocks[*e.Index]; exists {
			o.invalidateProtocol()
			return
		}
		o.blocks[*e.Index] = true
		o.nextBlock++
	case "content_block_delta":
		if e.Index == nil || !o.blocks[*e.Index] {
			o.invalidateProtocol()
		}
	case "content_block_stop":
		if e.Index == nil || !o.blocks[*e.Index] {
			o.invalidateProtocol()
			return
		}
		o.blocks[*e.Index] = false
	case "message_delta":
		if !o.start || o.stop || o.hasOpenBlocks() {
			o.invalidateProtocol()
			return
		}
		o.update(e.Usage)
	case "message_stop":
		if !o.start || o.stop || o.hasOpenBlocks() {
			o.invalidateProtocol()
			return
		}
		o.stop = true
	case "error":
		o.failed = true
	}
}

func (o *UsageObserver) invalidateProtocol() {
	o.protocolInvalid = true
	o.failed = true
}

func (o *UsageObserver) hasOpenBlocks() bool {
	for _, open := range o.blocks {
		if open {
			return true
		}
	}
	return false
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
			// Anthropic 的三类输入计数互不包含，分别记录，Tokens 产出时再归一。
			switch key {
			case "input_tokens":
				dst = &o.input
			case "output_tokens":
				dst = &o.output
			case "cache_creation_input_tokens":
				dst = &o.cacheCreation
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
		complete := o.start && o.stop && !o.failed
		// Chat Completions 流式响应只在 usage chunk 中返回计量。收到 [DONE] 但缺少
		// usage 时按不完整处理，避免以 SUCCESS 记录 0 Token 而绕过配额与计费。
		if o.openai && !o.responses {
			complete = complete && o.usageSeen
		}
		return complete
	}
	return o.start && o.json.complete && !o.json.invalid && !o.failed
}

// Ended 表示流式响应已收到终止事件（[DONE]、message_stop 或 response.completed）。
// 终止后即使计量不完整也不会再有有效数据，调用方无需等待上游关闭连接。
func (o *UsageObserver) Ended() bool {
	return o != nil && o.stream && o.stop
}

func (o *UsageObserver) ErrorCode() string {
	if o.protocolInvalid {
		return "UPSTREAM_INVALID_RESPONSE"
	}
	if o.failed {
		return "UPSTREAM_STREAM_ERROR"
	}
	return "UPSTREAM_RESPONSE_INCOMPLETE"
}

// ProtocolInvalid 表示 Anthropic SSE 的 JSON 或 content block 生命周期不合法。
// 它与 usage 字段缺失或异常分离，避免仅因计费信息不可用而中断正常响应。
func (o *UsageObserver) ProtocolInvalid() bool {
	return o != nil && o.stream && !o.openai && o.protocolInvalid
}
func (o *UsageObserver) Tokens(complete bool) (*int64, *int64, *int64) {
	if o.badUsage || o.json.invalid {
		return nil, nil, nil
	}
	input := o.input
	if !o.openai && input != nil {
		// 下游计费与配额按 OpenAI 语义处理：input 为总输入且 cached <= input。
		total, ok := anthropicTotalInput(*input, o.cacheCreation, o.cached)
		if !ok {
			return nil, nil, nil
		}
		input = &total
	}
	if !complete {
		return input, nil, o.cached
	}
	return input, o.output, o.cached
}

// UseCompatibilityUsage 为 OpenAI 协议 observer 关联兼容转换层记录的 Anthropic 原始用量。
// 客户端走 OpenAI 协议但路由到 Anthropic 端点时，缓存写入数只能由此取得。
func (o *UsageObserver) UseCompatibilityUsage(usage *CompatibilityUsage) {
	if o != nil && o.openai {
		o.compatibility = usage
	}
}

// CacheCreationTokens 返回 Anthropic 的缓存写入 Token 数，它已包含在 Tokens 产出的总输入中。
// 仅在总输入可用时返回；原生 OpenAI 协议不区分缓存写入，此时为 nil，经兼容转换的
// Anthropic 上游则取转换前的原始计数。
func (o *UsageObserver) CacheCreationTokens() *int64 {
	if o == nil {
		return nil
	}
	cacheCreation := o.cacheCreation
	if o.openai {
		cacheCreation = o.compatibility.CacheCreationTokens()
	}
	if cacheCreation == nil {
		return nil
	}
	// 与 usage_record / usage_rating 的约束一致：缓存写入与缓存读取同属总输入，二者之和不得超过总输入。
	// 计数不一致时只放弃缓存写入（按普通输入计费），不能让整条用量记录因违反约束而写入失败。
	input, _, cached := o.Tokens(false)
	if input == nil || *cacheCreation < 0 {
		return nil
	}
	uncached := *input
	if cached != nil {
		uncached -= *cached
	}
	if *cacheCreation > uncached {
		return nil
	}
	value := *cacheCreation
	return &value
}

// anthropicTotalInput 将 Anthropic 互不包含的 input / cache_creation / cache_read
// 合并为总输入。溢出时视为计量不可靠。
func anthropicTotalInput(input int64, parts ...*int64) (int64, bool) {
	total := input
	for _, part := range parts {
		if part == nil {
			continue
		}
		if *part > math.MaxInt64-total {
			return 0, false
		}
		total += *part
	}
	return total, true
}

func (o *UsageObserver) Diagnostics() UsageDiagnostics {
	if o == nil {
		return UsageDiagnostics{}
	}
	return UsageDiagnostics{
		Stream:          o.stream,
		Started:         o.start,
		Stopped:         o.stop,
		Failed:          o.failed,
		ProtocolInvalid: o.protocolInvalid,
		BadUsage:        o.badUsage,
		JSONComplete:    o.json.complete,
		JSONInvalid:     o.json.invalid,
		InputSeen:       o.input != nil,
		OutputSeen:      o.output != nil,
		CachedSeen:      o.cached != nil,
		EventCount:      o.eventCount,
		LastEvent:       o.lastEvent,
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
