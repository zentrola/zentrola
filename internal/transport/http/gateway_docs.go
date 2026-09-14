package http

import gw "github.com/zentrola/zentrola/internal/application/gateway"

// Gateway 请求由原生协议解析和透传，文档不强制限制上游扩展字段。

// @Summary Anthropic 消息推理
// @Tags Anthropic Gateway
// @Description 使用成员 Access Key。请求体示例：{"model":"claude-sonnet","max_tokens":64,"messages":[{"role":"user","content":"Hello"}],"stream":false}。model 为平台逻辑模型编码，须先配置模型授权及可用资源。原生扩展字段透传；stream=true 返回 text/event-stream，建议用 curl 或 SDK 验证 SSE。执行会调用真实上游并可能产生费用。
// @Accept json
// @Produce json,text/event-stream
// @Security GatewayKey
// @Security GatewayBearer
// @Param anthropic-version header string false "Anthropic API 版本" default(2023-06-01)
// @Param anthropic-beta header string false "上游 Beta 功能标记"
// @Param beta query string false "启用 Beta 路径" Enums(true)
// @Param body body map[string]interface{} true "Anthropic 原生 Messages 请求；至少包含 model、max_tokens、messages"
// @Success 200 {object} map[string]interface{} "上游原生响应；stream=true 时为 SSE 事件流，不包装 code/data"
// @Failure 400,401,403,404,413,415,429,502,503,504 {object} AnthropicErrorResponse
// @Header all {string} X-Request-ID "平台请求追踪 ID"
// @Router /anthropic/v1/messages [post]

// @Summary Anthropic Token 计数
// @Tags Anthropic Gateway
// @Description 请求体示例：{"model":"claude-sonnet","messages":[{"role":"user","content":"Hello"}]}。使用成员 Access Key，原生字段透传；是否支持此接口取决于配置的上游。
// @Accept json
// @Produce json
// @Security GatewayKey
// @Security GatewayBearer
// @Param anthropic-version header string false "Anthropic API 版本" default(2023-06-01)
// @Param anthropic-beta header string false "上游 Beta 功能标记"
// @Param beta query string false "启用 Beta 路径" Enums(true)
// @Param body body map[string]interface{} true "Anthropic 原生 count_tokens 请求"
// @Success 200 {object} TokenCountResponse
// @Failure 400,401,403,404,413,415,429,502,503,504 {object} AnthropicErrorResponse
// @Router /anthropic/v1/messages/count_tokens [post]

// @Summary OpenAI Chat Completions
// @Tags OpenAI Gateway
// @Description 使用成员 Access Key 的 Bearer 认证。请求体示例：{"model":"deepseek-chat","messages":[{"role":"user","content":"Hello"}],"stream":false}。model 从 GET /v1/models 获取，原生扩展字段透传；stream=true 返回 SSE，建议用 curl 或 SDK 验证流式响应。执行会调用真实上游并可能产生费用。
// @Accept json
// @Produce json,text/event-stream
// @Security GatewayBearer
// @Param body body map[string]interface{} true "OpenAI 原生 Chat Completions 请求；至少包含 model、messages"
// @Success 200 {object} map[string]interface{} "上游原生响应；stream=true 时为 SSE 事件流"
// @Failure 400,401,403,404,413,415,429,502,503,504 {object} OpenAIErrorResponse
// @Header all {string} X-Request-ID "平台请求追踪 ID"
// @Router /v1/chat/completions [post]

// @Summary OpenAI Responses
// @Tags OpenAI Gateway
// @Description 使用成员 Access Key 的 Bearer 认证。model 为平台逻辑模型编码，请求和响应按 OpenAI Responses 协议透传。
// @Accept json
// @Produce json,text/event-stream
// @Security GatewayBearer
// @Param body body map[string]interface{} true "OpenAI Responses 请求；至少包含 model、input"
// @Success 200 {object} map[string]interface{} "上游原生响应；stream=true 时为 SSE 事件流"
// @Failure 400,401,403,404,413,415,429,502,503,504 {object} OpenAIErrorResponse
// @Header all {string} X-Request-ID "平台请求追踪 ID"
// @Router /v1/responses [post]

// @Summary OpenAI 图片生成
// @Tags OpenAI Gateway
// @Description 使用成员 Access Key 的 Bearer 认证。model 为平台逻辑模型编码，请求和响应按 OpenAI Images Generations 协议透传；仅路由到使用 API Key 的 OpenAI 协议上游。stream=true 时透传 SSE。执行会调用真实上游并可能产生费用。
// @Accept json
// @Produce json,text/event-stream
// @Security GatewayBearer
// @Param body body map[string]interface{} true "OpenAI Images Generations 请求；至少包含 model、prompt"
// @Success 200 {object} map[string]interface{} "上游原生图片响应；stream=true 时为 SSE 事件流"
// @Failure 400,401,403,404,413,415,429,502,503,504 {object} OpenAIErrorResponse
// @Header all {string} X-Request-ID "平台请求追踪 ID"
// @Router /v1/images/generations [post]

// @Summary 可用的 OpenAI 逻辑模型
// @Tags OpenAI Gateway
// @Description 返回当前成员被授权且有可用 OpenAI 资源的逻辑模型；不接受查询参数。
// @Produce json
// @Security GatewayBearer
// @Success 200 {object} OpenAIModelsResponse
// @Failure 400,401,503 {object} OpenAIErrorResponse
// @Router /v1/models [get]

type OpenAIModelsResponse struct {
	Object string     `json:"object" example:"list"`
	Data   []gw.Model `json:"data"`
}

type TokenCountResponse struct {
	InputTokens int64 `json:"input_tokens" example:"12"`
}

type AnthropicErrorResponse struct {
	Type  string `json:"type" example:"error"`
	Error struct {
		Type    string `json:"type" example:"authentication_error"`
		Message string `json:"message" example:"Authentication failed."`
	} `json:"error"`
}

type OpenAIErrorResponse struct {
	Error struct {
		Message string `json:"message" example:"Authentication failed."`
		Type    string `json:"type" example:"authentication_error"`
		Param   any    `json:"param"`
		Code    string `json:"code" example:"UNAUTHENTICATED"`
	} `json:"error"`
}
