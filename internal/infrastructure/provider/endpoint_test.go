package provider

import "testing"

func TestBaseURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://api.example.com/anthropic/":                "https://api.example.com/anthropic",
		"https://dashscope.aliyuncs.com/compatible-mode/v1": "https://dashscope.aliyuncs.com/compatible-mode/v1",
	} {
		got, ok := BaseURL(raw)
		if !ok || got != want {
			t.Fatalf("valid endpoint rejected: %q => %q, %v", raw, got, ok)
		}
	}

	for _, raw := range []string{
		"http://api.example.com",
		"https://localhost",
		"https://127.0.0.1",
		"https://10.0.0.1",
		"https://key@api.example.com",
		"https://api.example.com/v1?token=secret",
		"https://api.example.com/path/../escape",
	} {
		if _, ok := BaseURL(raw); ok {
			t.Fatalf("unsafe endpoint accepted: %q", raw)
		}
	}
}
