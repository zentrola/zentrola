# 阶段 4：Anthropic Native Gateway

原生转发、安全校验和模拟测试已完成。2026-09-06 按用户追加授权接入 DeepSeek，真实接口验证结果见 [DeepSeek 接入与实测](DeepSeek_接入与实测.md)。没有执行真实 Claude Code 客户端会话或本地工具，所以清单中的真实客户端验收保持未勾选。

本页描述 Anthropic 入口。随后新增的 `/v1/chat/completions` 和 `/v1/models` 见 [OpenAI 兼容接口](OpenAI_兼容接口.md)，两种入口按各自协议解析模型映射。

## 已实现入口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/anthropic/v1/messages` | 原生 Messages，支持 JSON 和 SSE |
| POST | `/anthropic/v1/messages/count_tokens` | 原生输入 Token 估算，应用相同授权和资源解析规则 |

接受可选查询参数 `?beta=true`，其余查询参数返回 400。未知路径返回原生 404，方法不匹配返回 405。不实现 Models Discovery、Files、Batches、Bedrock、Vertex 或其他协议端点。

Gateway 使用一个 Virtual Key：`x-api-key: <Virtual Key>` 或 `Authorization: Bearer <Virtual Key>`。不接受 Admin JWT；同时携带两种认证 Header 时拒绝，保持阶段 2 的认证歧义防护。因此当前不支持会同时发送两个凭证 Header 的 apiKeyHelper 配置。

## 每次请求的判断过程

1. 对 Virtual Key 做 SHA-256，读取数据库，检查 Key 撤销 / 过期、MEMBER 和组织状态。
2. 读取有界 JSON 请求体，识别顶层 `model / stream`。重复顶层字段、非法 UTF-8、非对象 JSON、多个 JSON 文档被拒绝。
3. 在只读一致性事务中复核身份，并精确匹配平台逻辑模型。模型停用时拒绝。
4. 检查有效 Group 的显式模型授权，按多组并集计算，没有授权即拒绝。Key 自身不存权限。
5. 解析唯一 ACTIVE Provider Model、有效 Anthropic Official / DeepSeek Official Provider 以及当前组织该 Provider 唯一 ACTIVE Resource。
6. 结束数据库事务，解密 Credential，替换请求顶层逻辑模型编码，向官方上游发送请求。
7. 原样传输响应状态与内容，关闭响应体和上游请求生命周期。

身份、权限和路由不缓存。撤销 Key、停用成员、撤销模型授权或停用资源，在修改提交后的下一次请求生效。已经开始的请求使用本次解析结果，不因中途撤销而主动中断；不实现会话级追踪和在途请求撤销。

不做健康评分、Failover、动态多 Provider 选路、加权或价格路由。不添加应用层重试，并禁用 POST 请求体自动重放，避免重复上游调用。

## 原生协议保留

只替换顶层 `model` 的 JSON 字符串，其余请求体字节保持不变，包括字段顺序、数字精度、system 数组、工具 ID、tool_use / tool_result、cache_control、thinking 及未知字段。响应中的上游 `model` 编码不再替换回逻辑编码。

客户端的 `anthropic-*` 请求 Header 保留，包括 `anthropic-beta` 以及未知扩展；不对 beta 能力值做固定枚举。未提供 `anthropic-version` 时注入 `2023-06-01`。总协议 Header 大小限定 16 KiB，beta 限定 8 KiB，并拒绝控制字符或被 Connection 声明为逐跳字段的协议 Header。这与官方要求保留扩展能力、系统内容及原生格式的方向一致。[Claude Code 网关兼容指南](https://code.claude.com/docs/en/llm-gateway-protocol)

上游认证使用解密后的 Resource Credential，注入 `x-api-key`；不会把客户端 Authorization、Virtual Key、Cookie 或任意自定义 Header 复制到上游。Base URL 白名单为 `https://api.anthropic.com` 和 `https://api.deepseek.com/anthropic`，不使用环境代理、不跟随重定向。凭证明文字节使用完毕后清理，HTTP 事务关闭后移除请求 Header 中的凭证引用；不承诺 Go 内存的物理擦除。

响应仅转发 Content-Type、Content-Encoding、Retry-After、官方 Request-Id 和 Anthropic 限流 Header，并剔除 Connection 指定的逐跳字段。平台自己的 `X-Request-ID` 不被上游覆盖，设置 `Cache-Control: no-store / X-Accel-Buffering: no`。

SSE 使用固定 32 KiB 缓冲区边读取边写入并 Flush，不缓存完整流，不重新编码事件，不更改顺序，包括未知事件、ping 和工具 JSON 增量。客户端断开会取消上游 context；流已经开始后遇到传输失败，中断连接，不追加管理 JSON、不伪造 message_stop。[Anthropic 流式协议](https://platform.claude.com/docs/en/build-with-claude/streaming)

`count_tokens` 同样使用逻辑模型映射；其返回值是上游输入 Token 估算，不作为真实 Usage 落库。[Token counting 文档](https://platform.claude.com/docs/en/build-with-claude/token-counting)

## 错误响应

本地生成错误使用 Anthropic 形状，不包含管理面 `code / data / requestId` 包装：

```json
{"type":"error","error":{"type":"permission_error","message":"Model permission denied."}}
```

稳定业务码通过 `X-Zentrola-Error-Code` 返回，全链路 ID 通过 `X-Request-ID` 返回。

| HTTP Status | 业务码 | 含义 |
| --- | --- | --- |
| 400 | `INVALID_REQUEST` | JSON、模型字段、协议 Header 或查询参数非法 |
| 401 | `UNAUTHENTICATED` | 调用凭证或主体无效 |
| 403 | `MODEL_DISABLED / MODEL_PERMISSION_DENIED` | 模型停用或无授权 |
| 404 | `MODEL_NOT_FOUND / NOT_FOUND` | 未知逻辑模型或路径 |
| 405 | `METHOD_NOT_ALLOWED` | 非 POST 调用 |
| 413 | `REQUEST_TOO_LARGE` | 请求体超过限制 |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | 非 JSON 或压缩请求体 |
| 503 | `MODEL_ROUTE_UNAVAILABLE / RESOURCE_UNAVAILABLE` | 没有唯一有效路由或资源 |
| 503 | `CREDENTIAL_UNRECOVERABLE / DEPENDENCY_UNAVAILABLE` | 资源密文或基础依赖不可用 |
| 502 | `UPSTREAM_UNAVAILABLE` | 上游网络错误或重定向被拒绝 |
| 504 | `UPSTREAM_TIMEOUT` | 上游或请求总时限到期，且响应尚未开始 |

已收到的上游 4xx / 5xx，包括 429，其 HTTP 状态、原始响应体及 Retry-After 直接转发，不改成成功、不重试。未知异常使用安全诊断，日志不记录 Prompt、工具内容、Token 或 Credential。发生响应中断时日志标记 `UPSTREAM_STREAM_INTERRUPTED`；HTTP 已开始后的状态不能再改成 504。

## 配置

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `GATEWAY_MAX_BODY_BYTES` | `33554432` | JSON 请求体 32 MiB，上限允许配置到 128 MiB |
| `GATEWAY_REQUEST_TIMEOUT` | `15m` | 从请求体读取完毕后开始，覆盖路由查询及上游响应传输 |
| `GATEWAY_HEADER_TIMEOUT` | `120s` | 上游响应 Header 等待上限 |
| `GATEWAY_BODY_READ_TIMEOUT` | `30s` | 客户端请求体读取期限 |
| `GATEWAY_WRITE_TIMEOUT` | `30s` | 每次写入 / Flush 到客户端的期限 |

所有时限必须为正数。HTTP 服务仍不设置全局 WriteTimeout；上述写入期限逐块更新，支持长 SSE，同时限制慢接收端。请求必须使用未压缩的 `application/json`，结构和非路由字段的业务有效性由 Anthropic 验证。

## 后续真实 Claude Code 验收

准备有效 Anthropic Resource、启用它，并按阶段 3 给 MEMBER 授权逻辑模型及签发 Key。以下仅为待执行的接入示例，本轮未修改用户的 Claude Code 配置：

```powershell
# 在启动 Claude Code 的当前终端中设置，使用平台 Virtual Key。
$env:ANTHROPIC_BASE_URL = 'http://127.0.0.1:8080/anthropic'
Remove-Item Env:ANTHROPIC_AUTH_TOKEN -ErrorAction SilentlyContinue
$env:ANTHROPIC_API_KEY = [Net.NetworkCredential]::new('', (Read-Host 'Virtual Key' -AsSecureString)).Password
$env:ANTHROPIC_DEFAULT_SONNET_MODEL = 'claude-sonnet'
$env:ANTHROPIC_DEFAULT_OPUS_MODEL = 'claude-opus'
# MVP 仅预置两种逻辑模型，后台轻量调用也走 Sonnet，并受其授权约束。
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = 'claude-sonnet'
claude --model claude-sonnet
```

Base URL 包含 `/anthropic`，不包含 `/v1`。通过 `/status` 核实实际 Base URL 和凭证来源；已有配置文件可能覆盖当前 shell 设置。认证变量分别对应 Bearer 或 x-api-key，当前示例只使用 x-api-key。[Claude Code 接入指南](https://code.claude.com/docs/en/llm-gateway-connect)

Sonnet / Opus / Haiku 环境变量控制客户端模型别名，此处将它们映射到平台逻辑编码；上游模型编码仍由数据库 Provider Model 决定。[Claude Code 模型配置](https://code.claude.com/docs/en/model-config)

待真实执行并记录：普通会话、长 SSE、工具读取一个明确指定的测试文件并返回结果、未授权模型拒绝，以及撤销 Virtual Key 后下一次请求拒绝。没有真实测试前不将这些验收项标记完成。

## 本轮验证与后续边界

- 全量 Go 测试、阶段 1～4 PostgreSQL 隔离 schema 集成测试、`go vet`、Windows 构建通过。
- Linux `go test -race ./... -count=1 -timeout=90s` 通过，包含数据库集成与流式测试。
- Docker 构建与 Compose 配置检查通过；本地登录、readiness 和 Gateway 拒绝 Admin JWT 均通过，临时进程已停止。
- 模拟上游验证：逻辑模型替换、原始 JSON 字节和大整数保留、原生工具往返、count_tokens、限流错误透传、重定向拒绝及 Header 凭证隔离。
- 实际本地 HTTP 测试验证：首段事件无需等待完整响应、超过缓冲区的大段 SSE 字节及顺序一致、客户端断开取消上游、截断响应中止、超时、请求大小限制、无授权时不调用上游、撤销后立即拒绝。
- 没有读取真实业务 Prompt、调用真实 Anthropic API 或在业务库植入模拟成员 / Resource。

后续阶段 5 已接入 `ai_request / usage_record`、有界队列、批量写入、同步兜底、退出 Flush 和查询接口，见 [阶段 5 Usage](阶段5_Usage归属与可靠写入.md)。[阶段 6 管理界面](阶段6_Admin_Web.md) 也已完成。
