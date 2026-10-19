package gateway

import (
	"strings"
	"testing"
)

func TestChatUsageChunkFilterRemovesOnlyUsageOnlyChunks(t *testing.T) {
	content := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}],\"usage\":null}\n\n"
	finish := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":null}\n\n"
	usageChunk := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\n"
	inlineUsage := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3}}\n\n"
	comment := ": keep-alive\n\n"
	done := "data: [DONE]\n\n"
	tests := []struct {
		name, input, want string
	}{
		{name: "standard", input: content + finish + usageChunk + done, want: content + finish + done},
		{name: "crlf", input: strings.ReplaceAll(content+usageChunk+done, "\n", "\r\n"), want: strings.ReplaceAll(content+done, "\n", "\r\n")},
		{name: "usage inside final choice kept", input: content + inlineUsage + done, want: content + inlineUsage + done},
		{name: "comments kept", input: comment + content + usageChunk + done, want: comment + content + done},
		{name: "unterminated tail flushed", input: content + "data: [DONE]", want: content + "data: [DONE]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, step := range []int{1, 7, 4096} {
				filter := NewChatUsageChunkFilter()
				var got strings.Builder
				for i := 0; i < len(test.input); i += step {
					got.Write(filter.Filter([]byte(test.input[i:min(i+step, len(test.input))])))
				}
				got.Write(filter.Flush())
				if got.String() != test.want {
					t.Fatalf("step=%d got %q want %q", step, got.String(), test.want)
				}
			}
		})
	}
}

func TestChatUsageChunkFilterPassesThroughOversizedEvent(t *testing.T) {
	large := "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("x", usageChunkFilterEventLimit*2) + "\"}}]}\n\n"
	filter := NewChatUsageChunkFilter()
	var got strings.Builder
	for i := 0; i < len(large); i += 32 << 10 {
		got.Write(filter.Filter([]byte(large[i:min(i+32<<10, len(large))])))
	}
	got.Write(filter.Flush())
	if got.String() != large || cap(filter.pending) > usageChunkFilterEventLimit+64<<10 {
		t.Fatalf("oversized event changed or buffered: got=%d pending cap=%d", got.Len(), cap(filter.pending))
	}
}
