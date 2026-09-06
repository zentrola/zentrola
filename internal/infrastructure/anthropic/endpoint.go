package anthropic

import "strings"

// 只接受已实现认证方式的官方地址，含路径的 Base URL 必须完整匹配。
func allowedBaseURL(raw string) (string, bool) {
	base := strings.TrimSuffix(raw, "/")
	switch base {
	case "https://api.anthropic.com", "https://api.deepseek.com/anthropic":
		return base, true
	default:
		return "", false
	}
}
