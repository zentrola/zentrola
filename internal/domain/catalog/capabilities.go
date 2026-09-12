package catalog

// ProviderCapabilities 定义服务商支持的功能特性。
type ProviderCapabilities struct {
	SupportsAdvisor bool // 是否支持 advisor_20260301 工具
}

// GetProviderCapabilities 返回指定服务商的能力特性。
// 当服务商不支持某功能时，Gateway 层会自动过滤相关请求内容。
func GetProviderCapabilities(baseURL string) ProviderCapabilities {
	switch baseURL {
	case "https://api.anthropic.com":
		// Anthropic 官方完整支持所有功能
		return ProviderCapabilities{
			SupportsAdvisor: true,
		}
	case "https://api.deepseek.com/anthropic":
		// DeepSeek 不支持 advisor 工具
		return ProviderCapabilities{
			SupportsAdvisor: false,
		}
	default:
		// 未知服务商默认不支持新特性（保守策略）
		// 避免向不支持的端点发送未知工具导致报错
		return ProviderCapabilities{
			SupportsAdvisor: false,
		}
	}
}
