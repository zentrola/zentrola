# OpenAI 兼容接口

2026-09-06 按用户追加授权实现 OpenAI Chat Completions 入口，并通过已有 DeepSeek 官方资源完成真实验证。原 Anthropic Messages 入口继续工作；两种入口分别调用上游的原生协议，不在平台内转换消息格式。

## 接入配置

| 配置 | 值 |
| --- | --- |
| 客户端 Base URL | `http://127.0.0.1:8080/v1` |
| API Key | 管理员为成员签发的平台 Virtual Key |
| 已实测逻辑模型 | `deepseek-v4-flash` |
| 另已配置模型 | `deepseek-v4-pro`，需要单独授权，本轮未实际推理 |
| 上游 | DeepSeek Official，`https://api.deepseek.com/chat/completions` |

客户端需支持配置 Base URL 并使用 Chat Completions。当前未实现 `/v1/responses`、Embeddings、图片、音频、Files 或 Batches；未接入 OpenAI 官方 Provider，也未验证依赖 Responses 的客户端。这里的 OpenAI 支持指协议兼容，不代表已提供 GPT 模型。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| POST | `/v1/chat/completions` | 普通 JSON、SSE、`tools / tool_calls / tool_call_id` 原生转发 |
| GET | `/v1/models` | 仅列出当前成员获授权且有有效 OpenAI 映射及 ACTIVE Resource 的逻辑模型 |

两个接口均要求 `Authorization: Bearer <Virtual Key>`。不接受 Admin JWT 或 `x-api-key`；携带 `x-api-key` 时拒绝，避免认证歧义。不支持查询参数；未知路由返回 404、错误方法返回 405。本地错误使用 `{"error":{"message":"...","type":"...","param":null,"code":"..."}}`，并返回平台 `X-Request-ID` 和稳定错误码；上游状态和错误响应透传。

PowerShell 调用示例（不把 Key 写入脚本）：

```powershell
$secret = Read-Host '平台 Virtual Key' -AsSecureString
$headers = @{ Authorization = 'Bearer ' + [Net.NetworkCredential]::new('', $secret).Password }
try {
    Invoke-RestMethod -Headers $headers 'http://127.0.0.1:8080/v1/models'
    $body = @{
        model = 'deepseek-v4-flash'
        messages = @(@{ role = 'user'; content = '你好' })
        max_tokens = 128
        thinking = @{ type = 'disabled' }
    } | ConvertTo-Json -Depth 10
    Invoke-RestMethod -Method Post -Headers $headers -ContentType 'application/json' `
        -Uri 'http://127.0.0.1:8080/v1/chat/completions' -Body $body
} finally {
    $headers.Clear()
    $secret.Dispose()
}
```

`thinking` 是 DeepSeek 扩展，示例关闭思考用于短文本验证；平台原样保留扩展字段。SSE 使用 `stream: true`，建议同时设置 `stream_options: {include_usage: true}`。客户端按 `data:` 事件增量消费，以 `[DONE]` 判断正常结束。工具调用的第二轮须保留上轮 assistant 消息及原始 `tool_call_id`。字段和能力以 [DeepSeek Chat Completions 文档](https://api-docs.deepseek.com/zh-cn/api/create-chat-completion/) 为准。

## 已有环境升级

使用新二进制，先停止使用同一 `ID_NODE` 的旧服务，再执行：

```powershell
go run ./cmd/server setup-deepseek
go run ./cmd/server
```

默认自动应用 `00004_openai_protocol.sql`。关闭自动迁移时先执行 `go run ./cmd/server migrate`。新迁移增加 `ai_provider.openai_base_url`，允许 `OPENAI` 协议，并将 ACTIVE 映射唯一约束改为“每个逻辑模型、每种协议最多一个”。已有 Anthropic 映射和 Usage 历史不改写。

`setup-deepseek` 为两种 DeepSeek 模型补齐 OpenAI 映射，复用原 Provider、逻辑 Model 和 Resource，不修改凭证、成员权限或已有记录的启停状态。缺失的新映射继承 Anthropic 映射的状态；原映射已软删除则新增映射为 DISABLED。重复执行不恢复已有 OpenAI 映射。出现冲突时整个目录事务回滚；普通 `serve` 不自动安装 DeepSeek 目录。

新环境仍需通过 [管理 API](阶段3_Admin_API.md) 创建和启用 DeepSeek Resource、为成员所在 Group 授权模型并签发 Virtual Key。已有模型授权对两个协议入口共用，无需重复添加；模型可用性还取决于对应协议映射。

当前开发库已完成迁移及映射安装，已有 DeepSeek 资源可复用。测试 Key 已撤销，未保留长期测试访问。

当数据库已有 OpenAI 映射或请求历史时，迁移 Down 显式拒绝回滚，避免删除历史或产生不兼容数据。

## 路由与 Usage

每次调用复核 Key、成员、组织、模型和 Group 授权，再按协议解析唯一映射及资源。OpenAI 请求不会误用 Anthropic 映射。仅允许固定 DeepSeek 官方上游地址，不接受任意 URL，不跟随重定向，不使用环境代理，不增加重试或 Failover。上游 Bearer 使用解密后的 Resource Credential，客户端 Virtual Key、Cookie 和任意扩展 Header 不转发。

仅替换请求顶层 `model`，保留其余请求字节、工具字段和数字精度。响应状态、JSON 和 SSE 字节透传；流式输出边读边 Flush。取消、超时或截断时中止传输，不伪造 `[DONE]`。

Usage 复用阶段 5 的有界队列、批量事务、同步兜底和停机排空。`GET /api/v1/usage` 每行新增 `clientProtocol`，值为 `ANTHROPIC` 或 `OPENAI`。已认证的 Chat Completions 调用记录请求事实；实际尝试上游才记录 Attempt。模型发现和认证失败不计量。

| OpenAI 上游字段 | Usage 字段 |
| --- | --- |
| `prompt_tokens` | `inputTokens` |
| `completion_tokens` | `outputTokens` |
| DeepSeek `prompt_cache_hit_tokens`，或标准 `prompt_tokens_details.cached_tokens` | `cachedInputTokens` |

仅捕获必要元数据，不保存对话和工具内容。流式计数取上游最后累计值，不逐块求和；支持独立 Usage chunk 和 DeepSeek 最后内容 chunk，`usage: null` 不清空已确认计数。未见 `[DONE]`、取消或流错误时，最终输出数保持未知。缺失用量为 NULL，不估算。

各协议保留上游计数语义：OpenAI 的缓存读取通常包含在输入总数内，不能把 `inputTokens + cachedInputTokens` 直接作为总量，也不能不区分 `clientProtocol` 就套用 Anthropic 的计数规则。当前不计算成本；其他可靠性边界见 [阶段 5 Usage](阶段5_Usage归属与可靠写入.md)。

## 验证记录

- Windows 全量 Go 测试、PostgreSQL 隔离 schema 集成测试、`go vet` 和构建通过；Linux 全量 `-race`（含数据库）通过。
- 模拟端到端测试覆盖 JSON、SSE、工具往返、精度保留、模型发现、认证和权限、协议隔离、上游限流、超时、截断、取消与 Usage 归属。
- 真实 DeepSeek：模型发现、非流式对话、SSE、强制加法工具调用和回传结果均通过。SSE 为 41 个 chunk，首文本增量约 767 ms、总时长约 1023 ms；仅为单次观察。
- 产生 5 条 OpenAI 请求事实（4 次成功推理、1 次权限拒绝）和 4 条 Attempt。输入 / 输出分别为 13 / 5、20 / 39、315 / 54、102 / 6，缓存读取均为 0，与上游逐项一致。
- 撤销 Key 后返回 401。临时成员停用、Key 撤销、组授权及关联清理，保留审计与 Usage 历史。

复测脚本需 PowerShell 7.4、默认本地 8080 服务和可用的 DeepSeek Resource。每次执行发起四次短推理，不读取上游密钥：

```powershell
$password = Read-Host '管理员密码' -AsSecureString
try {
    ./scripts/Test-OpenAI.ps1 -AdminPassword $password
} finally {
    $password.Dispose()
}
```

脱敏结果保存于被 Git 忽略的 `.cache/openai-live-results.json`。脚本正常结束或抛错时清理临时访问；强制终止可能无法执行 finally，临时 Key 设有 15 分钟过期时间。普通 Go 自动化测试仅使用模拟上游，不调用付费模型。
