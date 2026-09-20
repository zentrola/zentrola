package catalog

import "testing"

func TestGetProviderCapabilities(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		wantAdvisor bool
		description string
	}{
		{
			name:        "anthropic_official",
			baseURL:     "https://api.anthropic.com",
			wantAdvisor: true,
			description: "Anthropic 官方支持所有功能",
		},
		{
			name:        "deepseek_official",
			baseURL:     "https://api.deepseek.com/anthropic",
			wantAdvisor: false,
			description: "DeepSeek 不支持 advisor",
		},
		{
			name:        "unknown_provider",
			baseURL:     "https://api.unknown.com/v1",
			wantAdvisor: false,
			description: "未知服务商默认不支持新特性（保守策略）",
		},
		{
			name:        "empty_url",
			baseURL:     "",
			wantAdvisor: false,
			description: "空 URL 视为未知服务商",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := GetProviderCapabilities(tt.baseURL)
			if caps.SupportsAdvisor != tt.wantAdvisor {
				t.Errorf("%s: got SupportsAdvisor=%v, want %v",
					tt.description, caps.SupportsAdvisor, tt.wantAdvisor)
			}
		})
	}
}

func TestCapabilitiesAreConsistent(t *testing.T) {
	// 验证能力判断的一致性：同一 URL 多次查询应返回相同结果
	baseURL := "https://api.deepseek.com/anthropic"
	caps1 := GetProviderCapabilities(baseURL)
	caps2 := GetProviderCapabilities(baseURL)

	if caps1.SupportsAdvisor != caps2.SupportsAdvisor {
		t.Error("capability query should be deterministic")
	}
}
