# 客户端接入

[English](client-configuration.md) | [简体中文](client-configuration.zh-CN.md)

所有客户端都使用成员 Virtual Key。Virtual Key 只负责识别成员，模型权限来自该成员所属 Group 的 Model Allowlist。客户端填写逻辑模型编码，而不是上游 Provider Model 编码。

## 接口地址

| 协议                 | Base URL                           | 支持的路径                                                                | 认证方式                              |
| -------------------- | ---------------------------------- | ------------------------------------------------------------------------- | ------------------------------------- |
| OpenAI Compatible    | `https://<gateway-host>/v1`        | `GET /v1/models`、`POST /v1/chat/completions`、`POST /v1/responses`、`POST /v1/images/generations` | `Authorization: Bearer <virtual-key>` |
| Anthropic Compatible | `https://<gateway-host>/anthropic` | `POST /anthropic/v1/messages`、`POST /anthropic/v1/messages/count_tokens` | `x-api-key: <virtual-key>` 或 Bearer  |

调用 Anthropic 接口时不要同时发送 `x-api-key` 和 `Authorization`。

## Codex 和 OpenAI 兼容客户端

在启动客户端的终端设置 Gateway Base URL 和 Virtual Key。

macOS/Linux：

```bash
printf 'Zentrola Virtual Key: '
read -r -s ZENTROLA_KEY
printf '\n'
export OPENAI_BASE_URL='https://<gateway-host>/v1'
export OPENAI_API_KEY="$ZENTROLA_KEY"
unset ZENTROLA_KEY
```

Windows PowerShell：

```powershell
$secureKey = Read-Host 'Zentrola Virtual Key' -AsSecureString
$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password
$env:OPENAI_BASE_URL = 'https://<gateway-host>/v1'
$env:OPENAI_API_KEY = $zentrolaKey
Remove-Variable secureKey, zentrolaKey
```

从 `GET /v1/models` 返回结果中选择一个逻辑模型编码。客户端配置文件及支持的环境变量可能随客户端版本变化；需要持久化配置时，以客户端当前官方文档为准。

## Claude Code 和 Anthropic 兼容客户端

macOS/Linux：

```bash
printf 'Zentrola Virtual Key: '
read -r -s ZENTROLA_KEY
printf '\n'
export ANTHROPIC_BASE_URL='https://<gateway-host>/anthropic'
export ANTHROPIC_API_KEY="$ZENTROLA_KEY"
unset ZENTROLA_KEY
```

Windows PowerShell：

```powershell
$secureKey = Read-Host 'Zentrola Virtual Key' -AsSecureString
$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password
$env:ANTHROPIC_BASE_URL = 'https://<gateway-host>/anthropic'
$env:ANTHROPIC_API_KEY = $zentrolaKey
Remove-Variable secureKey, zentrolaKey
```

将客户端模型设置为已向成员授权的逻辑模型编码。Zentrola 会转发受支持的 Messages、Streaming 和工具调用。`count_tokens` 必须路由到 Anthropic 兼容上游；Zentrola 不会通过只有 OpenAI 接口的上游生成估算值。

## 会话与账号分流

Zentrola 先按服务商模型映射及协议优先级选路；在同一映射下，个人订阅优先于 API Key。有多个同类账号时，Codex 的 `session-id` 和 Claude Code 的 `x-claude-code-session-id` 用于分配新会话，并将后续请求固定到同一账号。Claude Code 子代理的 `x-claude-code-agent-id` 会得到独立绑定。绑定在连续 30 天没有请求后过期。

当前账号不可用时，网关会在可以安全重试的情况下切换账号并更新绑定。如果请求带有 `previous_response_id`，网关必须找到原绑定，不会将该 ID 发给另一个账号；找不到时返回 `SESSION_ROUTE_UNAVAILABLE`。不带上述会话头的客户端仍按原有优先级和故障转移规则选路。

## 直接验证

查看已授权模型：

```bash
curl https://<gateway-host>/v1/models \
  -H "Authorization: Bearer <virtual-key>"
```

调用 Anthropic Messages：

```bash
curl https://<gateway-host>/anthropic/v1/messages \
  -H "x-api-key: <virtual-key>" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"<logical-model-code>","max_tokens":128,"messages":[{"role":"user","content":"你好"}]}'
```

## 常见问题

- `401`：Virtual Key 缺失、错误、过期或已撤销。
- `403`：成员已停用，或未获得请求的逻辑模型权限。
- 路由不可用：检查模型、服务商、映射和凭据是否启用，以及凭据是否被阻断。
- 模型列表为空：检查成员所属 Group 及 Group Model Allowlist。
- Admin Web 跨域失败：更新 `CORS_ALLOWED_ORIGINS`；普通 Gateway API Client 不受浏览器 CORS 的同类限制。

可以使用响应中的 `X-Trace-ID`、`X-Span-ID` 或 request ID 关联 Backend 日志，不要为排障发送凭据或请求正文。
