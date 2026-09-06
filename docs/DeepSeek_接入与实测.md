# DeepSeek 接入与实测

2026-09-06 首次按用户授权，在现有 Anthropic Native Gateway 增加 DeepSeek 官方兼容接口。本页描述 Anthropic Messages 接入与当时的测试。随后已按用户追加授权实现独立的 [OpenAI 兼容接口](OpenAI_兼容接口.md)，两种入口均原生转发，zentrola 不做 OpenAI / Anthropic 格式转换。

官方资料：[Anthropic 兼容说明](https://api-docs.deepseek.com/zh-cn/guides/anthropic_api/)、[模型列表](https://api-docs.deepseek.com/zh-cn/api/list-models/)、[Claude Code 接入](https://api-docs.deepseek.com/zh-cn/quick_start/agent_integrations/claude_code/)。

## 路由与凭证

| 项目 | 配置 |
| --- | --- |
| Provider | `deepseek-official` / DeepSeek Official |
| 上游 Base URL | `https://api.deepseek.com/anthropic` |
| 逻辑模型 → 上游模型 | `deepseek-v4-flash` → `deepseek-v4-flash` |
| 逻辑模型 → 上游模型 | `deepseek-v4-pro` → `deepseek-v4-pro` |
| 客户端 Base URL | `http://127.0.0.1:8080/anthropic` |
| Messages / count_tokens 认证 | 上游 `x-api-key`，来源于资源解密 |
| 资源连接检查 | `GET https://api.deepseek.com/models`，Bearer 认证 |

平台精确匹配模型编码，不依赖 DeepSeek 对未知名称的自动回退。Claude Sonnet / Opus 的原有 Provider Model 完整保留。每个逻辑模型、每种协议最多一个 ACTIVE 映射，每个组织、Provider 只有一个 ACTIVE Resource；不增加 Failover、重试或动态选路。

上游地址仅允许完整匹配 Anthropic 官方地址或上述 DeepSeek 兼容地址，可带一个末尾 `/`。拒绝额外路径、查询串、用户信息、伪造主机名和重定向；不使用环境代理。客户端 Virtual Key 只用于平台认证，不发送给上游。

凭证通过现有 Resource API 加密保存，AAD 继续绑定组织、Provider 和 Resource。管理接口不回传明文；日志不记录凭证、请求内容或工具内容。本次没有把上游密钥写进源码、脚本或配置文件。

## 新环境配置

DeepSeek 目录由数据库维护，不再提供专用命令写入目录。已有数据库中的 Provider、Model、Provider Model、Resource、权限和审计记录保持不变；服务启动不会自动安装或重置 DeepSeek 目录。

新环境应先通过数据库目录维护流程配置 `ai_provider`、`ai_model` 和 `provider_model`：一个 DeepSeek Provider、所需逻辑模型，以及各模型的 Anthropic/OpenAI 协议映射。逻辑模型的名称、编码、模态和启停状态可在模型管理页面维护；Provider 与协议映射目前没有配置界面，仍需通过数据库维护。维护时遵循现有业务 ID、必填元数据、软删除和唯一性约束，不恢复停用或已删除记录。

管理员随后通过 [阶段 3 管理 API](阶段3_Admin_API.md) 完成：

1. `GET /api/v1/providers` 选择 `deepseek-official`。
2. `POST /api/v1/resources` 保存 `{providerId, name, credential}`，默认 DISABLED。
3. `POST /api/v1/resources/{id}/test-connection` 检查账号；成功后 PATCH 状态为 ACTIVE。
4. 创建 MEMBER / Group，添加成员并显式授权所需 DeepSeek 模型，然后签发 Virtual Key。

当前开发库已经完成目录安装和“DeepSeek 开发资源”配置，资源处于 ACTIVE。实测临时成员已停用，Key 已撤销，组关联与授权已软删除；审计记录保留。未签发长期使用的 Key，需要使用时通过管理 API 为实际成员创建。

## 真实接口验证结果

本次测试经过本地 zentrola Gateway，使用真实 DeepSeek `deepseek-v4-flash`，不是模拟上游。输入仅为短文本与固定加法工具，不包含项目源码或用户业务内容。

| 检查 | 结果 |
| --- | --- |
| 官方模型列表连接检查 | HTTP 200 |
| 非流式 Messages | HTTP 200，返回模型 `deepseek-v4-flash`，内容包含指定标记 |
| SSE | HTTP 200，65 个事件，首个文本增量约 217 ms，完整流约 570 ms |
| 工具第一轮 | HTTP 200，`stop_reason=tool_use`，正确请求 2 + 3 |
| 工具第二轮 | 带原始 `tool_use_id` 回传 5，HTTP 200、`end_turn`，回答正确 |
| count_tokens | HTTP 200，返回正整数 `input_tokens` |
| 未授权 `deepseek-v4-pro` | 平台返回 403 |
| 撤销 Virtual Key 后请求 | 平台返回 401 |
| 测试访问清理 | 临时 Key 撤销、成员停用、组授权和成员关联移除 |

时间为单次观察，不是性能承诺。Pro 目录已配置，但本次未发起 Pro 推理。测试工具只把固定结果回传给模型，未执行 Claude Code 的文件读取或命令工具；真实客户端会话、长 SSE 和客户端工具执行验收仍待完成。

复测脚本只接受管理员密码，不需要获取上游密钥。需 PowerShell 7.4 或以上，服务使用默认本地 8080 端口，且已有可用 DeepSeek Resource。每次显式执行最多产生四次短文本推理，并创建随后停用的测试成员，保留审计记录。

```powershell
$password = Read-Host '管理员密码' -AsSecureString
try {
    ./scripts/Test-DeepSeek.ps1 -AdminPassword $password
} finally {
    $password.Dispose()
}
```

脱敏结果写入被 Git 忽略的 `.cache/deepseek-live-results.json`。脚本 finally 清理临时访问；进程被强制终止时 finally 可能不执行，临时 Key 自身设有 15 分钟过期时间。

回归验证：Windows 全量 Go 测试、PostgreSQL 隔离 schema 集成测试、`go vet` 和构建通过；Linux `go test -race ./... -count=1 -timeout=90s` 通过。新增测试覆盖 DeepSeek 完整 URL 校验、认证方式、原生路径和事件保留、目录幂等、不恢复已删除记录以及冲突时事务回滚。

## Claude Code 后续配置

安装客户端并签发正式 Virtual Key 后，在启动 Claude Code 的终端设置：

```powershell
$env:ANTHROPIC_BASE_URL = 'http://127.0.0.1:8080/anthropic'
Remove-Item Env:ANTHROPIC_AUTH_TOKEN -ErrorAction SilentlyContinue
$env:ANTHROPIC_API_KEY = [Net.NetworkCredential]::new('', (Read-Host '平台 Virtual Key' -AsSecureString)).Password
$env:ANTHROPIC_DEFAULT_SONNET_MODEL = 'deepseek-v4-flash'
$env:ANTHROPIC_DEFAULT_OPUS_MODEL = 'deepseek-v4-pro'
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = 'deepseek-v4-flash'
$env:CLAUDE_CODE_SUBAGENT_MODEL = 'deepseek-v4-flash'
claude --model deepseek-v4-flash
```

成员需要分别获得 Flash / Pro 授权。环境变量中填平台 Virtual Key；上游 DeepSeek Key 只保留在平台资源中。本次未安装 Claude Code、修改它的持久配置或标记其验收完成。

兼容能力以 DeepSeek 为准：官方说明 `anthropic-version`、Messages 的 `anthropic-beta`、`cache_control` 和 `thinking.budget_tokens` 等部分字段被忽略；不能把 Anthropic 格式兼容理解为所有 Claude 能力完全等价。平台保持原生转发，不伪造这些能力。

以上是最初接入时的测试记录。当时未写 Usage；后续阶段 5 已完成并再次使用真实 DeepSeek 核对归属及 Token 数，见 [阶段 5 Usage](阶段5_Usage归属与可靠写入.md)。可为复测脚本增加 `-VerifyUsage` 一并验证落库。[阶段 6 管理界面](阶段6_Admin_Web.md) 也已完成。
