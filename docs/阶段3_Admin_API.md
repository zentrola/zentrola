# 阶段 3：Admin API

已完成 P0-MVP 的管理 API 操作链，沿用阶段 1 表结构，无新增 Migration。范围包括 MEMBER、Group、Model、Anthropic Resource、Virtual Key 和 Operation Log；不建设完整 Provider / Model Catalog 管理、APPLICATION 流程或前端页面。

## 认证与响应

除登录及 [首次管理员初始化](首次管理员初始化.md) 外，下列接口都要求 `Authorization: Bearer <Admin JWT>`，每次检查有效管理员及组织。客户端不能通过请求体或查询参数指定组织，组织身份来自 JWT 和数据库验证。Access Key 不能作为管理凭证。

成功使用 HTTP 200，创建资源使用 201：

```json
{"code":"OK","data":{},"requestId":"req_..."}
```

错误仅返回稳定业务码和安全说明，不回显请求体、凭证或数据库异常：

```json
{"code":"NOT_FOUND","message":"Object not found.","requestId":"req_..."}
```

| HTTP Status | code | 含义 |
| --- | --- | --- |
| 400 | `INVALID_ARGUMENT` | 非法参数、ID、状态或 JSON，包含未知字段也被拒绝 |
| 401 | `UNAUTHENTICATED` | JWT 无效或管理员 / 组织不可用 |
| 403 | `CURRENT_PASSWORD_INCORRECT` | 修改登录密码时当前密码不正确 |
| 404 | `NOT_FOUND` | 对象不存在、已删除或不属于当前组织 |
| 409 | `CONFLICT` | Group 编码重复、关联对象状态不允许或同一 Provider 已有 ACTIVE Resource |
| 409 | `PROVIDER_UNAVAILABLE` | 资源所属 Provider 已停用 |
| 422 | `CREDENTIAL_UNRECOVERABLE` | 启用资源时凭证无法解密，需重新录入 |
| 503 | `SERVICE_UNAVAILABLE` | 数据库、审计或其他基础依赖不可用 |

全部管理响应使用 `Cache-Control: no-store`；Header `X-Request-ID` 与响应体一致。BIGINT ID 均为字符串；请求体中的 `providerId` 也必须为字符串，避免前端 Number 精度丢失。

## 接口目录

`POST /api/v1/auth/password` 用于当前管理员修改登录密码，请求体为 `{"currentPassword":"...","newPassword":"..."}`，必须携带 Admin JWT。新密码需为 12–72 UTF-8 字节、无空字符且不同于当前密码。当前密码错误复用登录失败计数与锁定策略，锁定返回 429 `ACCOUNT_LOCKED`。成功返回 `data.clearToken=true`，客户端需清除会话并重新登录；密码、凭证版本与审计日志在同一事务中保存，该账号全部旧 JWT 立即失效。

| 方法 | 路径 | 请求体或用途 |
| --- | --- | --- |
| POST | `/api/v1/auth/login` | `username / password` |
| GET | `/api/v1/me` | 当前管理员 |
| POST | `/api/v1/auth/logout` | 客户端清理 JWT |
| POST | `/api/v1/members` | `name`，可选 `remark` |
| GET | `/api/v1/members` | 分页成员列表 |
| GET | `/api/v1/members/{id}` | 成员详情 |
| PATCH | `/api/v1/members/{id}/status` | `status: ACTIVE / DISABLED` |
| POST | `/api/v1/members/{id}/keys` | `name`，可选 `expiresAt` |
| GET | `/api/v1/members/{id}/keys` | Key 元数据，只有前缀，没有完整值或 Hash |
| POST | `/api/v1/access-keys/{id}/revoke` | 撤销 Key |
| POST | `/api/v1/groups` | `code / name`，可选 `remark` |
| GET | `/api/v1/groups` | 分页 Group 列表 |
| GET | `/api/v1/groups/{id}` | Group 详情 |
| GET | `/api/v1/groups/{id}/members` | 当前成员关联 |
| PUT | `/api/v1/groups/{id}/members/{memberId}` | 成员加入 Group，无请求体 |
| DELETE | `/api/v1/groups/{id}/members/{memberId}` | 成员移出 Group |
| GET | `/api/v1/groups/{id}/models` | 当前显式模型授权 |
| PUT | `/api/v1/groups/{id}/models/{modelId}` | 增加模型 Allowlist 授权 |
| DELETE | `/api/v1/groups/{id}/models/{modelId}` | 撤销模型授权 |
| GET | `/api/v1/models` | 模型目录，包含发布方、输入输出类型及备注 |
| GET | `/api/v1/models/{id}` | 模型详情 |
| POST | `/api/v1/models` | 新增官方模型，默认停用，字段见模型目录管理文档 |
| PUT | `/api/v1/models/{id}` | 编辑完整模型元数据，保留 ID 和状态 |
| PATCH | `/api/v1/models/{id}/status` | `status: ACTIVE / DISABLED` |
| GET | `/api/v1/providers` | 只读已安装的 Anthropic Official / DeepSeek Official 元数据，供配置资源选择 |
| POST | `/api/v1/resources` | `providerId / name / credential` |
| GET | `/api/v1/resources` | 分页资源列表 |
| GET | `/api/v1/resources/{id}` | 资源详情，不包含密文、Nonce 或完整凭证 |
| PUT | `/api/v1/resources/{id}/credential` | `credential`，覆盖旧凭证 |
| PATCH | `/api/v1/resources/{id}/status` | `status: ACTIVE / DISABLED` |
| POST | `/api/v1/resources/{id}/test-connection` | 使用已保存的凭证测试连接，无请求体 |
| GET | `/api/v1/operation-logs` | 当前组织的审计记录与安全快照 |

所有列表支持 `?limit=50&after=<ID>`，默认 50，最大 100。成员列表及成员密钥列表按 ID 倒序，最新记录在前，后续页查询小于 after 的 ID；其他列表按 ID 升序。成员密钥列表可用 `limit=1` 获取最新签发的一条（包括已过期或已撤销的记录），列表只返回识别前缀，不返回完整密钥。返回 `data.items / data.nextCursor`；空列表是 `[]`。当本页恰好达到 limit 时返回最后一个 ID 作为下一页游标，因此最后一次请求可能返回空页。

成员和资源名称长度为 1～128 bytes，Group 编码为 1～64 bytes，备注最多 2000 bytes；不接受首尾空白、NUL 或无效 UTF-8。Credential 为 1～4096 bytes 可见 ASCII，不接受换行或空白。HTTP JSON 请求体上限为 16 KiB。

## 完整操作顺序

1. 登录，保存 `data.token`，后续添加 Admin Bearer Header。
2. `POST /members`，例如 `{"name":"开发者","remark":"研发成员"}`，保存成员 ID。
3. `POST /groups`，例如 `{"code":"engineering","name":"研发"}`，保存 Group ID。
4. `PUT /groups/{groupId}/members/{memberId}` 建立关联。
5. `GET /models` 获取逻辑模型 ID，然后 `PUT /groups/{groupId}/models/{modelId}` 授权。
6. `GET /providers` 获取 Anthropic Official ID；创建 Resource 时提交真实企业凭证：

   ```json
   {"providerId":"<Provider ID>","name":"企业 Anthropic","credential":"<真实 API Key>"}
   ```

7. 新资源默认 `DISABLED`。执行 `POST /resources/{resourceId}/test-connection`，检查 `data.ok`，再通过 PATCH 状态接口显式启用。
8. `POST /members/{memberId}/keys`，例如 `{"name":"Claude Code"}`。妥善保存这一次响应中的 `data.key`，后续接口不再返回完整 Key。
9. `GET /operation-logs` 查看本次操作链及其 requestId。

步骤中的短路径均以 `/api/v1` 开头。阶段 4 已接入 Gateway 原生转发，详见 [阶段 4 Anthropic Gateway](阶段4_Anthropic_Gateway.md)。真实 Claude Code 调用仍需配置有效 Resource 并完成验收。

## 业务与事务规则

- 只创建 MEMBER。成员名称不设业务唯一约束，Group code 按组织对有效记录唯一。
- 成员和模型的停用立即影响数据库权限判定；成员停用也立即使其所有 Access Key 无法认证。重新启用成员不恢复已撤销或过期的 Key。
- 新关联要求当前组织内有效、启用的 MEMBER / Group / Model；移除关联或撤销授权允许关联对象已停用，便于清理权限。
- 对同一关联重复 PUT / DELETE、对同一对象重复设置相同状态均幂等，实际没有变化时不重复写日志。
- 移除成员与撤销授权使用软删除；再次添加创建新 ID，不复活旧记录。多组模型权限取并集，无任何有效授权时默认拒绝。
- 资源创建立即 AES-256-GCM 加密，AAD 绑定组织、Provider 和 Resource。`credentialConfigured` 表示已保存凭证，不表示已经通过上游连通性验证。
- 覆盖凭证保留资源当前状态，不会自动启用。密文不可解密的资源必须先替换 Credential 才能启用。
- 每个组织、Provider 至多一个 ACTIVE Resource；启用冲突返回 409，不暗中停用已有资源。可以保存多个停用配置，但没有 Failover 能力。
- 所有管理写入和相应 Operation Log 同事务提交。P0 单组织管理写入使用组织行锁串行化，配合数据库唯一约束；审计失败则业务写入回滚。
- 列表和关联读取使用只读一致性事务，隔离组织、软删除及父对象存在性。数据库记录经显式映射后返回，不直接序列化。

## 连接测试语义

使用 Anthropic 官方 `GET /v1/models?limit=1`，携带 `x-api-key` 和 `anthropic-version: 2023-06-01`。该接口用于列出可用模型，测试不会发送 Messages 推理请求。[Anthropic API 文档](https://platform.claude.com/docs/en/api/overview)

按用户追加授权，另支持 DeepSeek Official，其 Base URL 固定为 `https://api.deepseek.com/anthropic`，连接检查使用 `GET https://api.deepseek.com/models` 和 Bearer 认证。不跟随重定向、不使用环境代理；客户端不能指定任意探测地址。连接测试的整体 HTTP 超时为 15 秒，响应读取上限 64 KiB，不记录上游原始响应或错误文本。

连接测试操作已执行并完成审计时，返回 HTTP 200，通过 `data.ok / data.code` 表示测试结果。例如：

```json
{"code":"OK","data":{"ok":false,"code":"UPSTREAM_AUTH_FAILED","httpStatus":401,"latencyMs":120},"requestId":"req_..."}
```

结果码包括 `OK / UPSTREAM_AUTH_FAILED / UPSTREAM_RATE_LIMITED / UPSTREAM_TIMEOUT / UPSTREAM_UNAVAILABLE / UPSTREAM_INVALID_RESPONSE / UPSTREAM_URL_REJECTED / CREDENTIAL_INVALID / CREDENTIAL_UNRECOVERABLE / PROVIDER_UNAVAILABLE / REQUEST_CANCELLED / RESOURCE_CHANGED`。无有效 HTTP 响应时不返回 httpStatus。

网络调用期间不持有数据库事务。调用结束后单独记录 `RESOURCE_CONNECTION_TEST`，失败测试以 FAILED 和对应错误码入审计。测试期间 Resource 被修改时返回 `RESOURCE_CHANGED`，不把旧凭证的成功结果当作当前配置可用。请求取消后仍以最多 3 秒尝试保存审计。

测试成功仅证明当前凭证能够访问官方模型列表，不证明所有模型具有推理权限、余额充足或完整 Claude Code 链路可用。

## 验证

- Windows 全量 `go test ./... -count=1`、`go vet ./...` 与二进制构建通过，覆盖阶段 1 / 2 / 3 PostgreSQL 集成测试。
- Linux `go test -race ./... -count=1` 通过，本次同时启用上述数据库集成测试。
- API 验证覆盖完整配置链、认证隔离、组织边界、分页、非法参数、幂等、关系软删除、多组并集、成员 / 模型停用、Key 撤销、资源凭证更新、不可解密凭证恢复、并发单 ACTIVE 约束、审计失败回滚及日志保密。
- 模拟上游测试覆盖模型列表请求协议、成功 / 认证失败 / 限流 / 错误、无效或超限响应、超时 / 取消、非法 URL / Credential 及重定向拒绝。
- 最新 Docker 镜像构建与 Compose 配置校验通过；本地程序使用原有管理员实测登录、readiness 和六类管理列表均返回 200。
- 测试业务数据仅创建在随机隔离 schema，结束自动清理。本地业务库未植入演示成员、Group 或 Resource；临时验收进程已停止。

以上为阶段 3 当时的验证记录。后续已配置真实 DeepSeek 资源并通过连通性和 Gateway 实测，见 [DeepSeek 接入与实测](DeepSeek_接入与实测.md)。[阶段 5 Usage](阶段5_Usage归属与可靠写入.md) 和 [阶段 6 Admin Web](阶段6_Admin_Web.md) 也已完成；真实 Anthropic 账号与 Claude Code 客户端尚未验证。
