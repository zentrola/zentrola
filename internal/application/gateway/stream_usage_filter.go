package gateway

import (
	"bytes"
	"encoding/json"
)

const usageChunkFilterEventLimit = 64 << 10

// ChatUsageChunkFilter 剥离网关代客户端开启 stream_options.include_usage 后，
// 上游在 [DONE] 前额外返回的 usage chunk（choices 为空数组）。它只缓存当前尚未
// 结束的 SSE 事件；超过上限的事件不可能是 usage chunk，立即透传。
// usage 观察仍在剥离前完成，计量不受影响。
type ChatUsageChunkFilter struct {
	pending     []byte
	out         []byte
	lineLength  int
	passthrough bool
}

func NewChatUsageChunkFilter() *ChatUsageChunkFilter {
	return &ChatUsageChunkFilter{}
}

// Filter 返回可立即转发给客户端的字节。返回的切片在下一次调用前有效。
func (f *ChatUsageChunkFilter) Filter(p []byte) []byte {
	out := f.out[:0]
	start := 0
	for index, b := range p {
		if b == '\r' {
			continue
		}
		if b != '\n' {
			f.lineLength++
			continue
		}
		if f.lineLength > 0 {
			f.lineLength = 0
			continue
		}
		// 空行结束当前事件。
		event := p[start : index+1]
		start = index + 1
		if f.passthrough {
			out = append(out, event...)
			f.passthrough = false
			continue
		}
		f.pending = append(f.pending, event...)
		if !usageOnlyChunk(f.pending) {
			out = append(out, f.pending...)
		}
		clear(f.pending)
		f.pending = f.pending[:0]
	}
	rest := p[start:]
	if f.passthrough {
		out = append(out, rest...)
	} else {
		f.pending = append(f.pending, rest...)
		if len(f.pending) > usageChunkFilterEventLimit {
			out = append(out, f.pending...)
			clear(f.pending)
			f.pending = f.pending[:0]
			f.passthrough = true
		}
	}
	f.out = out
	return out
}

// Flush 返回流结束时尚未以空行结束的残余字节，原样交付客户端。
func (f *ChatUsageChunkFilter) Flush() []byte {
	rest := append([]byte(nil), f.pending...)
	clear(f.pending)
	f.pending = f.pending[:0]
	f.passthrough = false
	f.lineLength = 0
	return rest
}

// usageOnlyChunk 判断一个完整 SSE 事件是否为 choices 为空、仅携带 usage 的 chunk。
func usageOnlyChunk(event []byte) bool {
	var data []byte
	for _, line := range bytes.Split(event, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		if data != nil {
			data = append(data, '\n')
		}
		data = append(data, bytes.TrimPrefix(line[5:], []byte{' '})...)
	}
	if len(data) == 0 {
		return false
	}
	var chunk struct {
		Object  string          `json:"object"`
		Choices json.RawMessage `json:"choices"`
		Usage   json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &chunk) != nil || chunk.Object != "chat.completion.chunk" {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(chunk.Choices), []byte("[]")) &&
		bytes.HasPrefix(bytes.TrimSpace(chunk.Usage), []byte("{"))
}
