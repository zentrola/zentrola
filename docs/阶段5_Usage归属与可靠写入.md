# 阶段 5：Usage 归属与可靠写入

已接入 Anthropic / DeepSeek Gateway：请求完成后生成不含业务内容的 Usage Event，经有界队列和固定 Worker 写入 PostgreSQL。真实 DeepSeek 非流式、SSE 和两轮工具调用的归属与 Token 数已验证。

后续增加的 [OpenAI Chat Completions 入口](OpenAI_兼容接口.md) 复用同一写入链路，已完成真实验证。请求事实和管理查询通过 `clientProtocol` 区分 `ANTHROPIC / OPENAI`；下文 Messages 的事实规则同样适用于 Chat Completions，`GET /v1/models` 不计量。

## 请求事实与上游 Attempt

| 情况 | ai_request | usage_record |
| --- | --- | --- |
| 已认证 MEMBER 的 Messages 调用，实际尝试上游 | 一条 | 一条，attempt_no=1 |
| 未授权、模型不存在、资源不可用、非法 Messages 请求 | 一条失败事实，已识别模型时保留 model_id | 无，不伪造上游调用 |
| 凭证解密失败、上游地址等本地校验失败 | 一条失败事实 | 无 |
| 网络连接失败、超时、上游 4xx / 5xx、响应截断 | 一条失败事实 | 一条失败 Attempt |
| 客户端取消进行中的调用 | CANCELLED | 若已尝试上游则记录 CANCELLED |
| 未通过 Virtual Key 认证 | 无；未确定有效 Principal，只保留安全请求日志 | 无 |
| count_tokens、健康检查、管理请求或未知路由 | 不计入模型推理事实 | 无 |

“尝试上游”包含发起 HTTP 连接但未收到有效响应的情形，不表示上游一定已计费。一次工具往返包含两次 Messages 请求，所以生成两条请求事实和两条 Attempt。

请求事实保存全链路 `request_id`、组织、Principal、逻辑 Model、时间、耗时、状态及稳定错误码。Attempt 额外保存实际 Provider、Provider Model、Resource 和上游 Token 数。关联 ID 来自本次权限及路由解析，不从客户端声明的 ID 取值，不因后续成员或资源停用而修改历史归属。

记录不保存 Prompt、响应文本、工具参数、工具结果、Virtual Key 或上游 Credential。HTTP 原始响应和 SSE 事件继续透传，Usage 写入失败不向已经开始的响应追加错误或伪造结束事件。

## Token 语义与响应解析

以下字段描述 Anthropic 协议。OpenAI 将 `prompt_tokens / completion_tokens` 映射到输入 / 输出数，缓存读取取 `prompt_cache_hit_tokens` 或 `prompt_tokens_details.cached_tokens`，按最终累计值保存，SSE 以 `[DONE]` 结束。各协议保持上游语义：OpenAI 输入总数可能包含缓存命中，不应将输入和缓存再次相加。跨协议查询时使用 `clientProtocol` 区分。

- `input_tokens` 保存上游同名字段；`output_tokens` 保存上游最终累计值。
- `cached_input_tokens` 对应上游 `cache_read_input_tokens`；独立于普通输入字段保存。
- 缺失、非法、重复或无法可靠解析的计数保留 NULL；明确返回的 0 保留 0。
- SSE 从 `message_start.message.usage` 和后续 `message_delta.usage` 更新计数。delta 的计数是累计值，采用覆盖，不逐段累加。[Anthropic 流式文档](https://platform.claude.com/docs/en/build-with-claude/streaming)
- SSE 需看到开始与结束事件且无 error 事件，才视为完整响应。取消、截断和超时保留已确认的输入及缓存读取数，最终输出数置 NULL。
- 非流式 JSON 只捕获顶层 `type / usage`，忽略内容中的同名字段，支持跨读取块和大整数；不保存完整响应。
- SSE 只缓存当前行及事件，各限制 256 KiB。JSON 元数据限制 64 KiB、嵌套深度 256；超过边界或解析失败时不估算 Token，不影响原生字节转发。压缩上游响应仍透传，用量保持未知。

本阶段沿用既有三种 Token 字段，不另外存储 `cache_creation_input_tokens`；也不把各字段相加充当完整计费总量。`billing_unit=TOKEN`，`billing_quantity / cost_amount / cost_currency` 保持 NULL，不计算 Cost 或客户账单。此次功能验证 Usage 归属，不是与上游账单的完整对账。

## 写入与故障处理

固定一个 Worker，固定容量 channel。达到批量大小或定时器到期后，用 `jsonb_to_recordset` 在一个事务中分别批量插入 `ai_request / usage_record`。第二张表失败时第一张表同样回滚。

每个 Event 的 ID 在提交时生成一次；数据库以 `request_id` 和 `(request_id, attempt_no)` 唯一键保证重复提交不会重复记账，不覆盖已提交事实。整个批次失败后，Worker 对每条 Event 各尝试一次独立写入，隔离坏记录；仍失败则记录 ERROR 和失败计数。

channel 满时由当前请求同步写入 PostgreSQL，记录 Warning 并增加 `fallback`。同步失败增加 `failed / writeErrors` 并写 ERROR，不报告持久化成功。写入使用独立、有时限的 context，不因客户端取消而取消本次保存尝试。

这不是永久重试队列。数据库持续故障、ID 生成失败或停机 Flush 超时可能导致事实无法保存，失败通过日志与计数明确报告；没有磁盘队列、DLQ 或自动补偿。进程崩溃、`kill -9`、宿主机断电仍可能丢失尚未落库的内存事件，这是 P0 清单明确接受的边界。

## 配置与退出顺序

| 环境变量 | 默认值 | 含义 |
| --- | --- | --- |
| USAGE_QUEUE_SIZE | 10000 | 等待队列容量，最多 1000000 |
| USAGE_BATCH_SIZE | 100 | 每批最多事件数，最多 10000 |
| USAGE_FLUSH_INTERVAL | 500ms | 部分批次的定时 Flush 周期 |
| USAGE_WRITE_TIMEOUT | 3s | 每次批量或同步数据库写入时限 |
| USAGE_SHUTDOWN_TIMEOUT | 5s | 停止输入后，等待 Writer 排空的时限 |

数量和时限都必须为正值。配置支持 System ENV > `.env` > 默认值，Compose 同步暴露上述配置。

退出顺序：停止 HTTP 接收 → 等待正在处理的请求完成；HTTP 超时则关闭连接并等待 Handler 退出 → 关闭 Usage 输入 → 排空 channel / 当前批次，并等待同步写入 → 关闭 PostgreSQL。Writer Flush 超时会取消剩余写入并记录未完成数量；存在已失败事件或排空超时，进程退出码为非零。

默认 HTTP Shutdown 为 20s，Writer 为 5s，Compose 给予 30s 停机窗口。运行环境应给出足够停机宽限期；强制终止不属于正常排空保障。

## 管理查询 API

两个接口都需要 Admin JWT，Virtual Key 无管理权限；查询复核管理员和组织有效性。

### GET /api/v1/usage

返回请求事实列表，并关联可能存在的 Attempt，因此可同时看到成功调用和本地拒绝。支持组合参数：

| 参数 | 规则 |
| --- | --- |
| memberId | 正整数 ID，按历史 Principal 筛选 |
| modelId | 正整数逻辑 Model ID |
| resourceId | 正整数 Resource ID；筛选时没有 Attempt 的请求不会命中 |
| from / to | RFC3339 时间，范围 `[from,to)`；默认最近 24 小时，最大跨度 366 天 |
| after | 非负请求事实 ID，按 ID 升序分页 |
| limit | 1～100，默认 50 |

无匹配结果返回空列表；重复参数、未知参数及非法范围返回 400。分页时建议固定 from / to；入库有短暂异步延迟，查询不强制 Flush。

响应格式：`{code:"OK",data:{items:[...],nextCursor:...},requestId:...}`。关联 ID 使用字符串；未识别的模型、未发起的 Attempt 和未知 Token 字段返回 null。单行包括 `id / requestId / clientProtocol / principalId / modelId / requestAt / completedAt / latencyMs / status / errorType / usageId / attemptNo / providerId / providerModelId / resourceId / inputTokens / outputTokens / cachedInputTokens / attemptStatus / attemptErrorType`。

```powershell
# $headers 使用登录接口获得的 Admin JWT；ID 取自管理接口。
Invoke-RestMethod -Headers $headers 'http://127.0.0.1:8080/api/v1/usage?limit=50'
Invoke-RestMethod -Headers $headers 'http://127.0.0.1:8080/api/v1/usage?memberId=123&from=2026-09-06T00:00:00Z&to=2026-09-07T00:00:00Z'
```

新增迁移 `00003_usage_query_indexes.sql` 为请求事实补充组织时间和模型时间索引，不回写已执行的初始表结构。

### GET /api/v1/usage/writer

返回本进程计数器：`queued`（异步接收）、`persisted`（写入成功的 Event 提交数）、`fallback`（同步兜底数）、`failed`（最终保存失败数）、`writeErrors`（数据库写入失败次数）、`pending`（尚未处理完成数）。这些是运行指标，不是账本聚合；进程重启后重新计数，重复幂等提交也可能增加 persisted。账本查询以数据库事实为准。

## 验证记录

- Windows 全量 Go 测试、阶段 1～5 PostgreSQL 隔离 schema 集成测试、`go vet` 和构建通过。
- Linux race 检查覆盖队列满同步兜底、同步失败、批失败隔离、定时 Flush、事件指针快照、并发 Submit / Close、停机超时，以及数据库集成。
- 端到端模拟测试覆盖普通 / SSE 用量、取消、超时、截断、限流、缺失用量、本地拒绝无 Attempt、count_tokens 不计量、组合筛选、分页、时间范围和组织隔离。
- 真实 DeepSeek 调用生成 5 条请求事实：4 条成功推理 + 1 条模型权限拒绝；生成 4 条 Attempt。逐项核对上游与数据库的输入 / 输出 / 缓存读取数，全部一致。
- 四次真实调用的输入 / 输出数分别为 13 / 5、22 / 59、315 / 54、102 / 6，缓存读取均为 0。这只是当次测试结果，不代表后续调用固定用量。
- 专用容器设置 `USAGE_FLUSH_INTERVAL=1h`，分别测试 `docker stop`、SIGINT、SIGTERM：每轮停机前 3 条请求事实仍待写、数据库为 0；退出后落库 3 条，退出码为 0。SIGINT 在 Linux 容器验证，未实际按 Windows 终端 Ctrl+C。
- 测试成员停用、临时 Key 撤销，操作审计和真实 Usage 历史保留；临时验证容器停止并清理。

复测真实 DeepSeek（需要已有 ACTIVE Resource，最多四次短推理）：

```powershell
$password = Read-Host '管理员密码' -AsSecureString
try {
    ./scripts/Test-DeepSeek.ps1 -AdminPassword $password -VerifyUsage
} finally {
    $password.Dispose()
}
```

结果写入 `.cache/deepseek-live-results.json`，不包含凭证或业务文本。正常自动化测试使用模拟上游和隔离数据库，不调用付费 API。

后续 [阶段 6 Admin Web](阶段6_Admin_Web.md) 已完成，用量可通过网页查询。真实 Claude Code 客户端会话与本地工具执行仍需单独验收。
