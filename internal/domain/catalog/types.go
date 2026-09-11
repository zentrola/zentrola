// Package catalog 定义平台逻辑模型及供应方映射的业务语义。
package catalog

type ProviderType string
type Protocol string

const (
	Official  ProviderType = "OFFICIAL"
	Anthropic Protocol     = "ANTHROPIC"
	OpenAI    Protocol     = "OPENAI"
)

const (
	AnthropicOfficialCode = "anthropic-official"
	OpenAIOfficialCode    = "openai-official"
	DeepSeekOfficialCode  = "deepseek-official"
	ZhipuOfficialCode     = "zhipu-official"
	SonnetCode            = "claude-sonnet"
	OpusCode              = "claude-opus"
)
