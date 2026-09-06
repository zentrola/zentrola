# P0-MVP 开发清单｜Claude Code Governance Slice

> 依据：AI Coding Control Plane V2.6.6  
> 目标：验证“企业是否需要以 MEMBER / Group 为单位治理 Claude Code 的模型访问和 Usage”  
> 原则：**范围做 MVP，地基保持生产级；不实现清单外任何功能。**

> 2026-09-06 范围补充：用户授权使用 DeepSeek 完成真实模型验证。增加显式 `setup-deepseek` 目录初始化命令和固定 DeepSeek Anthropic 兼容入口；默认 Bootstrap 保留 Anthropic，不增加通用 Provider CRUD、动态选路或跨协议转换。详见 [DeepSeek 接入与实测](DeepSeek_接入与实测.md)。原清单中的真实 Claude Code 客户端验收仍需单独执行。

> 同日追加范围：用户要求支持 OpenAI，已实现 `/v1/chat/completions` 和 `/v1/models`，复用 DeepSeek 官方资源，完成真实普通响应、SSE、工具往返和 Usage 验证。模型映射唯一性调整为每个逻辑模型、每种协议最多一个 ACTIVE 映射；其他治理规则共用。详见 [OpenAI 兼容接口](OpenAI_兼容接口.md)。该次接入不含 Responses 或 OpenAI 官方 Provider；随后阶段 6 已在 `web/` 完成，见 [Admin Web 验收](阶段6_Admin_Web.md)。

---

## 阶段 0：项目初始化

- [ ] 初始化 Go module，并建立 `domain / application / infrastructure / transport` 四层目录骨架
- [ ] PostgreSQL 本地环境 + Docker Compose（仅 `app + postgres`，P0-MVP 不依赖 Redis）
- [ ] 选定并接入 Migration 工具，跑通第一个空 Migration
- [ ] `slog` 基础日志配置
  - [ ] `LOG_LEVEL` 可配置
  - [ ] 开发环境支持 text
  - [ ] Docker 默认 JSON
- [ ] 统一 `requestId` 中间件
  - [ ] 请求进入时生成
  - [ ] 写入 `X-Request-ID`
  - [ ] 注入 `context.Context`
  - [ ] 日志自动带 `requestId`
- [ ] Graceful Shutdown 骨架
  - [ ] 监听 `SIGINT / SIGTERM`
  - [ ] 停止接收新请求
  - [ ] 等待请求退出
  - [x] Usage Flush 已在阶段 5 接入
- [ ] Health Check
  - [ ] `GET /health/live`
  - [ ] `GET /health/ready`
  - [ ] `live` 只判断进程存活
  - [ ] `ready` 检查 PostgreSQL、Master Key、Bootstrap 状态
- [ ] CORS 基础中间件与配置
  - [ ] `CORS_ENABLED` 可配置
  - [ ] `CORS_ALLOWED_ORIGINS` 使用明确 Origin Allowlist，例如开发环境 `http://localhost:5173`
  - [ ] 支持 Admin Web 所需的 `GET / POST / PUT / PATCH / DELETE / OPTIONS`
  - [ ] 允许 `Authorization / Content-Type / X-Request-ID` 等必要请求 Header
  - [ ] 暴露响应 Header `X-Request-ID`
  - [ ] 正确处理浏览器 Preflight `OPTIONS`
  - [ ] 生产环境禁止默认使用 `*` 放开全部 Origin

### 验收

- [ ] `docker compose up` 可正常启动
- [ ] `/health/live` 返回正常
- [ ] `/health/ready` 在依赖正常时返回 Ready
- [ ] 任意请求都可获得 `X-Request-ID`
- [ ] Admin Web 从配置允许的开发 Origin 可正常调用 Admin API
- [ ] 非 Allowlist Origin 不被 CORS 放行
- [ ] `Ctrl+C / docker stop` 可优雅退出

---

# 阶段 1：核心数据模型与 Migration

## 1.1 数据库通用规范

可变业务表默认包含：

```text
id
is_deleted
created_by
updated_by
created_at
updated_at
```

规则：

- [ ] `id` 使用 `BIGINT`
- [ ] ID 由应用侧 64-bit ID Generator 生成
- [ ] 表名、字段名统一英文 `snake_case`
- [ ] 所有表必须写中文 `COMMENT`
- [ ] 所有字段必须写中文 `COMMENT`
- [ ] 状态 / 类型数据库值统一使用大写英文
- [ ] COMMENT 使用中文说明枚举值，例如：
  - `ACTIVE=启用`
  - `DISABLED=停用`
- [ ] 除非业务语义明确允许为空，否则字段默认 `NOT NULL`
- [ ] 必填字段不得通过空字符串或虚假默认值代替缺失数据
- [ ] 时间统一使用 `TIMESTAMPTZ`
- [ ] 数据库统一按 UTC 保存时间
- [ ] 金额使用 `NUMERIC(20,8)`，禁止使用 float
- [ ] Token / 计数使用 `BIGINT`
- [ ] PostgreSQL 不使用 Native ENUM，使用 `VARCHAR + Go typed constants`
- [ ] 不建立 Foreign Key Constraint
- [ ] 不使用 `ON DELETE CASCADE`
- [ ] 关系完整性、存在性校验和删除依赖检查由 Application / Domain 层负责
- [ ] 高频 `xxx_id` 关联字段按查询需要建立普通索引

### 逻辑删除规则

- [ ] 逻辑删除默认不可恢复
- [ ] 删除后重新创建同业务标识对象时，生成新记录、新 ID
- [ ] 业务唯一约束只约束 `is_deleted = false`
- [ ] PostgreSQL 使用 Partial Unique Index

示例：

```sql
CREATE UNIQUE INDEX uk_admin_user_username
ON admin_user (organization_id, username)
WHERE is_deleted = false;
```

---

## 1.2 organization

P0-MVP 只支持 Single Organization。

建议字段：

```text
organization_code
organization_name
status
remark
+ 通用字段
```

- [ ] 首次启动自动 Bootstrap 一条默认 Organization
- [ ] P0-MVP 不提供多 Organization 创建流程

---

## 1.3 admin_user

建议字段：

```text
organization_id
username
password_hash
display_name
status
failed_login_count
locked_until
last_login_at
+ 通用字段
```

规则：

- [ ] `username` 当前有效记录唯一
- [ ] 使用 Partial Unique Index：`WHERE is_deleted = false`
- [ ] `failed_login_count` 默认 `0`，记录连续登录失败次数
- [ ] `locked_until` 仅在账号被临时锁定时有值，未锁定允许为 `NULL`

---

## 1.4 principal

建议字段：

```text
organization_id
principal_type
name
remark
status
+ 通用字段
```

P0-MVP：

```text
MEMBER=成员
APPLICATION=应用
```

- [ ] 本阶段只实现 `MEMBER`
- [ ] `APPLICATION` 类型可保留常量 / 字段兼容，但不建设流程

---

## 1.5 access_key

建议字段：

```text
principal_id
key_hash
key_prefix
name
status
expires_at
last_used_at
revoked_at
+ 通用字段
```

规则：

- [ ] 完整 Access Key 只在创建时展示一次
- [ ] 数据库只保存 `key_hash + key_prefix`
- [ ] Access Key 使用密码学安全随机数生成
- [ ] P0 使用 SHA-256 摘要
- [ ] Access Key 只识别 Principal，不承载业务权限

---

## 1.6 ai_group / principal_group

不要使用 `group` 作为表名，避免 SQL 关键字歧义。

### ai_group

```text
organization_id
group_code
group_name
remark
status
+ 通用字段
```

### principal_group

```text
organization_id
principal_id
group_id
+ 通用字段
```

规则：

- [ ] Principal 与 Group 从第一版就是多对多
- [ ] 不采用 `principal.group_id` 临时单组方案
- [ ] `(principal_id, group_id)` 当前有效关系唯一
- [ ] 多 Group 的 Model Permission 取并集

---

## 1.7 ai_provider

建议字段：

```text
provider_code
provider_name
provider_type
anthropic_base_url
status
+ 通用字段
```

P0-MVP：

- [ ] 只 Bootstrap `Anthropic Official`
- [ ] 不做完整 Provider CRUD UI
- [ ] Provider 与 Model 保持解耦

---

## 1.8 ai_model

建议字段：

```text
model_code
display_name
model_type
status
+ 通用字段
```

P0-MVP 示例：

```text
claude-sonnet
claude-opus
```

- [ ] `model_code` 为 Control Plane 稳定逻辑模型标识
- [ ] 不要求等同 Anthropic 上游真实 model code

---

## 1.9 provider_model

建议字段：

```text
provider_id
model_id
upstream_model_code
protocol_type
status
+ 通用字段
```

关系：

```text
ai_provider N:M ai_model
```

P0-MVP：

- [ ] 一个逻辑 Model 只允许一个有效 Provider Model
- [ ] 不实现 Multi-Provider Routing

---

## 1.10 ai_resource

**Resource 归属于 Provider，不归属于 Provider Model。**

建议字段：

```text
organization_id
provider_id
resource_name

credential_ciphertext
credential_nonce
key_version

status
last_active_at
+ 通用字段
```

关系：

```text
ai_provider 1:N ai_resource
```

P0-MVP：

- [ ] Anthropic Official 下只允许一个 ACTIVE Resource
- [ ] 同一 Resource 可服务多个 Anthropic Model
- [ ] 不实现 Health Score
- [ ] 不实现 Resource Failover
- [ ] 不实现 principal_resource / group_resource

---

## 1.11 group_model_permission

建议字段：

```text
organization_id
group_id
model_id
+ 通用字段
```

P0-MVP 权限语义固定为：

```text
显式授权
→ 可使用

无授权
→ 不可使用

多个 Group
→ Model Permission 取并集
```

即：

> **P0-MVP 对 Model 使用 Default Deny + Group Allowlist。**

不实现：

```text
Principal Model Override
Deny Policy
Group Priority
复杂 Policy Expression
```

---

## 1.12 ai_request

表示用户视角的一次 AI 调用。

建议字段：

```text
organization_id
request_id
principal_id
model_id
usage_scene
client_protocol

request_at
completed_at
latency_ms

status
error_type
created_at
```

说明：

- [ ] `ai_request` 属于请求事实记录
- [ ] 不使用逻辑删除
- [ ] `request_id` 唯一

---

## 1.13 usage_record

表示一次真实 Provider Attempt。

建议字段：

```text
organization_id

request_id
attempt_no

principal_id
provider_id
provider_model_id
resource_id
model_id

usage_scene

input_tokens
output_tokens
cached_input_tokens

billing_unit
billing_quantity

cost_amount
cost_currency

started_at
completed_at
latency_ms

status
error_type

created_at
```

P0-MVP：

- [ ] `attempt_no` 固定为 `1`
- [ ] 预留 `(request_id, attempt_no)` 唯一约束
- [ ] 不实现 Retry / Failover
- [ ] `usage_record` 不逻辑删除

---

## 1.14 operation_log

操作日志为 Append Only。

建议字段：

```text
id

organization_id

operator_type
operator_id
operator_name

module
operation_type

target_type
target_id
target_name

request_id

request_method
request_path

ip_address
user_agent

result
error_code

before_data
after_data

remark

created_at
```

明确不包含：

```text
is_deleted
created_by
updated_by
updated_at
```

规则：

- [ ] 只允许 INSERT
- [ ] 不提供普通 UPDATE
- [ ] 不提供普通 DELETE
- [ ] 日志清理由未来 Retention Job 物理处理
- [ ] Credential、Password、Master Key、JWT、完整 Access Key 不得进入 `before_data / after_data`

### 阶段 1 验收

- [x] Migration 可完整执行
- [x] 所有表 / 字段有中文 COMMENT
- [x] 所有默认 NOT NULL 规则检查通过
- [x] Partial Unique Index 检查通过
- [x] 无数据库 Foreign Key
- [x] `ai_model / ai_provider / provider_model / ai_resource` 关系评审通过
- [x] `ai_resource.provider_id` 关系正确
- [x] Credential 字段可以完整保存 AES-GCM Ciphertext / Nonce / Key Version

> 2026-09-06：已通过阶段 1 数据结构验收，详见 [阶段1_数据基础.md](阶段1_数据基础.md)。验证包括隔离 PostgreSQL schema 集成测试和项目库 Migration；本节前面的行为清单不因此自动视为完成，完整密钥、安全认证、管理写入与 Usage 流程仍按后续阶段实现。

---

# 阶段 2：安全地基

## 2.1 Master Key

解析优先级：

```text
System ENV / External Secret
        ↓
/data/secrets/master.key
        ↓
首次生成
```

- [x] 使用 `crypto/rand` 生成 32 bytes
- [x] 使用持久化文件保存
- [x] 文件权限 `0600`（Windows 使用当前用户和 SYSTEM 专用 DACL）
- [x] Master Key 不进 PostgreSQL
- [x] Master Key 不写日志
- [x] Master Key 不硬编码
- [x] 已存在本地 Key 时自动读取

P0-MVP 不实现完整 Master Key Rotation，只预留 `key_version`。

---

## 2.2 AES-256-GCM

- [x] 实现 Encrypt
- [x] 实现 Decrypt
- [x] 单元测试覆盖：
  - [x] 正常加密 / 解密
  - [x] Wrong Key 解密失败
  - [x] Ciphertext 篡改后完整性校验失败
  - [x] Nonce 使用正确

不在 P0-MVP 实现复杂 Envelope Encryption。

---

## 2.3 Access Key

- [x] 高熵随机生成
- [x] SHA-256 摘要
- [x] 数据库只保存 hash + prefix
- [x] 完整值仅创建时返回一次
- [x] 支持 Revoke
- [x] 支持状态校验

---

## 2.4 Admin 登录

按用户追加约定，新安装系统通过网页设置首位管理员，不从环境变量预置账号。已有管理员记录时关闭初始化入口；并发创建与审计使用同一事务。详见 [首次管理员初始化](首次管理员初始化.md)。

```text
Username / Password
        ↓
Argon2id / bcrypt Verify
        ↓
JWT
```

- [x] JWT 使用 `Authorization: Bearer <JWT>`
- [x] JWT 包含：
  - [x] admin id
  - [x] organization id
  - [x] issued at
  - [x] expires at
  - [x] jti
- [x] P0-MVP 不做 Refresh Token
- [x] JWT 有效期固定一个合理默认值，例如 8h

### 登录暴力破解防护

P0-MVP 使用 PostgreSQL 中的 `admin_user.failed_login_count / locked_until` 实现最小防护，不引入 Redis。

默认策略：

```text
连续登录失败 < 5 次
→ 记录失败次数

连续登录失败达到 5 次
→ 临时锁定账号 15 分钟

登录成功
→ failed_login_count 清零
→ locked_until 清空
→ 更新 last_login_at
```

- [x] 登录失败统一返回认证失败语义，不通过错误信息暴露用户名是否存在
- [x] 已锁定账号在 `locked_until` 到期前拒绝登录
- [x] 连续失败阈值和锁定时间可配置
- [x] 登录成功后清零连续失败状态
- [x] 已知管理员账号的登录成功 / 失败 / 锁定事件写入 `operation_log`
- [x] Password、JWT 等敏感信息禁止写入日志和操作日志

推荐初始配置：

```text
ADMIN_LOGIN_MAX_FAILURES=5
ADMIN_LOGIN_LOCK_DURATION=15m
```

---

## 2.5 双认证入口隔离

```text
Admin API
→ Admin JWT

Gateway
→ Principal Access Key
```

必须保证：

- [x] Principal Access Key 不能访问 Admin API
- [x] Admin JWT 不能作为 Gateway 调用凭证

### 阶段 2 验收

- [x] Access Key → Admin API 被拒绝
- [x] Admin JWT → Gateway API 被拒绝
- [x] 连续错误密码达到阈值后账号进入临时锁定
- [x] 锁定期间正确密码也不能绕过锁定
- [x] 锁定到期后可重新登录
- [x] 登录成功后失败计数与锁定状态被清零
- [x] AES-GCM 测试全部通过
- [x] Master Key 不出现在日志 / DB / API Response

---

# 阶段 3：Admin API

Admin API 统一：

```text
/api/v1/*
```

---

## 3.1 登录

```text
POST /api/v1/auth/login
GET  /api/v1/me
POST /api/v1/auth/logout
```

P0-MVP `logout` 只负责客户端清理 JWT，不建设服务端 Session。

---

## 3.2 MEMBER 管理

- [x] 创建 MEMBER
- [x] 查看 MEMBER
- [x] 启用 / 停用
- [x] 创建 Virtual Key
- [x] Revoke Virtual Key

---

## 3.3 Group 管理

- [x] 创建 Group
- [x] 查看 Group
- [x] 成员加入 Group
- [x] 成员移出 Group
- [x] 配置 Group Model Allowlist

---

## 3.4 Model 管理

- [x] 查看 Bootstrap Model
- [x] 启用 / 停用
- [x] Group Model 授权

不实现完整 Model Catalog 管理能力。

---

## 3.5 Resource 管理

- [x] 查看 Resource
- [x] 创建 / 配置 Anthropic Resource
- [x] 输入 Credential 后立即 AES-GCM 加密
- [x] 后台禁止回显完整 Credential
- [x] 支持覆盖更新 Credential
- [x] 支持测试连接
- [x] 支持状态切换

---

## 3.6 Admin API Response Contract

成功：

```json
{
  "code": "OK",
  "data": {},
  "requestId": "req_xxx"
}
```

错误：

```json
{
  "code": "MODEL_PERMISSION_DENIED",
  "message": "Model permission denied.",
  "requestId": "req_xxx"
}
```

规则：

- [x] 正确使用 HTTP Status
- [x] Business Code 使用稳定大写英文字符串
- [x] 前端逻辑依赖 code，不依赖 message
- [x] `X-Request-ID` 与 Body `requestId` 一致

---

## 3.7 Operation Log

以下管理行为必须写入 `operation_log`：

```text
MEMBER_CREATE
MEMBER_STATUS_CHANGE
ACCESS_KEY_CREATE
ACCESS_KEY_REVOKE

GROUP_CREATE
GROUP_MEMBER_ADD
GROUP_MEMBER_REMOVE
GROUP_MODEL_GRANT
GROUP_MODEL_REVOKE

MODEL_STATUS_CHANGE

RESOURCE_CREATE
RESOURCE_CREDENTIAL_UPDATE
RESOURCE_STATUS_CHANGE
RESOURCE_CONNECTION_TEST

LOGIN_SUCCESS
LOGIN_FAILED
LOGIN_LOCKED
```

### 阶段 3 验收

已通过隔离数据库中的完整 API 链路验收；连接测试使用模拟上游覆盖，真实账号连通性待配置有效 Credential 后验证。接口与验证记录见 [阶段 3 Admin API](阶段3_Admin_API.md)。

管理员只通过 API 可以完成：

```text
创建 MEMBER
→ 创建 Group
→ MEMBER 加入 Group
→ 配置 Group Model Permission
→ 配置 Resource
→ 签发 Virtual Key
```

---

# 阶段 4：Anthropic Native Gateway

开发与模拟验证已完成，另已通过 DeepSeek 真实 Messages、短 SSE、两轮原生工具往返、count_tokens 及权限测试。真实 Claude Code 客户端会话、长 SSE 和客户端工具执行仍待验收。详见 [阶段 4 Gateway](阶段4_Anthropic_Gateway.md) 与 [DeepSeek 接入与实测](DeepSeek_接入与实测.md)。

## 4.1 Gateway Authentication

```text
Virtual Key
   ↓
SHA-256
   ↓
access_key
   ↓
Principal
```

校验：

- [x] Access Key 存在
- [x] Access Key ACTIVE
- [x] Access Key 未 Revoke
- [x] Principal 存在
- [x] Principal 为 MEMBER
- [x] Principal ACTIVE

---

## 4.2 Model Permission

请求：

```text
model
↓
ai_model
↓
MEMBER 所属 Groups
↓
group_model_permission
```

规则：

```text
任一 Group 显式授权
→ Allow

全部未授权
→ Deny
```

不实现 Principal Override。

---

## 4.3 Resource Resolver

P0-MVP Resolver：

```text
Model
↓
唯一 ACTIVE provider_model
↓
Provider
↓
唯一 ACTIVE Resource
```

明确不实现：

```text
Health Score
Failover
Weight
Price Routing
Multi Provider
Resource Permission
```

---

## 4.4 Anthropic Messages Compatible

- [x] Anthropic Native Messages 请求转发
- [x] Header 正确转换 / 注入
- [x] 使用解密后的 Anthropic Credential
- [x] Credential 仅存在于必要内存范围
- [x] 不进入普通日志

---

## 4.5 SSE

- [x] SSE 原生透传
- [x] 不套统一 Admin Response Wrapper
- [x] 保持事件顺序
- [x] 客户端断开时取消 Upstream Request
- [x] 不缓存整个 Streaming Response 后再输出

---

## 4.6 Tool Loop

必须使用真实 Claude Code 测试：

```text
Claude Code
→ tool_use
→ 本地工具执行
→ tool_result
→ Claude
→ 后续响应
```

P0-MVP 只做 Anthropic Native Tool Call / Tool Result 透传，不做跨协议 Tool ID 映射。

---

## 4.7 Access Key Revoke

- [x] Revoke 后下一次请求立即失效
- [x] 不允许因本地 Cache 导致长时间延迟失效

### 阶段 4 验收

真实 Claude Code：

- [ ] Base URL 指向 Control Plane
- [ ] 使用 Virtual Key
- [ ] 普通会话成功
- [ ] 长 SSE 成功
- [ ] Tool Loop 成功
- [ ] 未授权 Model 被拒绝
- [ ] Revoke Virtual Key 后立即拒绝

---

# 阶段 5：Usage 归属

开发、模拟故障验证和真实 DeepSeek Usage 核对已完成。容器 SIGINT / SIGTERM / docker stop 排空通过；Windows 终端 Ctrl+C 尚未实际按键测试。详见 [阶段 5 Usage](阶段5_Usage归属与可靠写入.md)。

## 5.1 Usage Writer

采用：

```text
Gateway
↓
Usage Event
↓
Buffered Channel
↓
固定 goroutine Worker
↓
Batch Insert PostgreSQL
```

规则：

- [x] 不使用 `go saveUsage()` 每请求创建新 goroutine
- [x] 固定 Worker 数量
- [x] Channel 有固定容量
- [x] 支持 Batch Insert
- [x] 支持定时 Flush

推荐初始参数：

```text
USAGE_QUEUE_SIZE=10000
USAGE_BATCH_SIZE=100
USAGE_FLUSH_INTERVAL=500ms
```

---

## 5.2 Queue Full

不得静默丢 Usage。

规则固定：

```text
Channel 可写
→ 异步入队

Channel 已满
→ 当前请求同步写 PostgreSQL
```

如同步写失败：

- [x] 写 ERROR Log
- [x] 增加失败 Metric
- [x] 不得伪造 Usage 成功

---

## 5.3 Graceful Shutdown

```text
SIGTERM / SIGINT
↓
停止接受新请求
↓
关闭 Usage Queue 输入
↓
Flush Channel
↓
等待 Usage Worker
↓
关闭 PostgreSQL
↓
退出
```

必须保证：

- [x] 正常 `Ctrl+C`
- [x] `docker stop`
- [x] `SIGTERM`

情况下尽量 Flush 完内存 Usage。

明确：

> `kill -9 / 进程 Crash / 宿主机断电` 时，P0-MVP 的内存 Channel 数据仍可能丢失，本阶段不承诺 Crash-safe Usage Queue。

---

## 5.4 Usage 数据

一次请求：

```text
ai_request
```

一次上游 Attempt：

```text
usage_record
attempt_no = 1
```

- [x] Usage 归属到 Principal
- [x] Usage 归属到 Model
- [x] Usage 归属到 Provider
- [x] Usage 归属到 Resource

---

## 5.5 Usage Query API

至少支持：

- [x] 按 MEMBER 查询
- [x] 按 Model 查询
- [x] 按 Resource 查询
- [x] 按时间范围查询

### 阶段 5 验收

- [x] 正常请求能生成 `ai_request`
- [x] 正常请求能生成 `usage_record`
- [x] 真实 MEMBER / Model / Resource 归属准确
- [x] 正常 Graceful Shutdown 后 Channel 中剩余 Usage 已 Flush
- [x] 不要求 `kill -9` 场景零丢失

---

# 阶段 6：最小 Admin Web

本阶段只做必要后台页面，不追求完整产品化。`web/` 已完成，开发及真实后端验收见 [阶段 6 Admin Web](阶段6_Admin_Web.md)。

- [x] 登录页
- [x] 成员页
  - [x] 列表
  - [x] 创建
  - [x] 状态
  - [x] Virtual Key 管理
- [x] Group 页
  - [x] 列表
  - [x] 创建
  - [x] 成员分配
  - [x] Model Allowlist
- [x] 模型页
  - [x] 查看
  - [x] 启停
  - [x] Group 授权
- [x] Resource 页
  - [x] 查看
  - [x] Credential 录入 / 覆盖
  - [x] 连接测试
  - [x] 状态
- [x] Usage 页
  - [x] 按成员查看
  - [x] 按模型查看
  - [x] 按 Resource 查看

P0-MVP 前端只要求 i18n-ready：

- [x] 状态值使用英文编码
- [x] 可见文本尽量使用 i18n key
- [x] 不要求完成完整 en-US 翻译

### 阶段 6 验收

管理员不依赖 Postman / curl，可以通过 Admin Web 完成：

```text
创建成员
→ 创建 Group
→ 分配成员
→ Group 授权 Model
→ 配置 Resource
→ 创建 Virtual Key
→ 查看 Usage
```

---

# 阶段 7：部署与基础可观测性

## Docker Compose

P0-MVP：

```text
app
postgres
```

不包含 Redis。

- [ ] 一键启动
- [ ] PostgreSQL 数据持久化
- [ ] Master Key 文件持久化
- [ ] 日志默认 stdout + JSON

---

## Logging

使用：

```text
log/slog
```

至少包含：

```text
request_id
principal_id
model_id
provider_id
resource_id
latency_ms
error_code
```

禁止记录：

```text
Master Key
Provider Credential
完整 Access Key
JWT
Admin Password
完整 Prompt / Response（默认）
```

---

## OpenTelemetry

P0-MVP 只建立接缝，不建设完整可观测性平台。

- [ ] HTTP request context 可接 OTel
- [ ] Provider request 可创建 Span
- [ ] 日志预留 `trace_id`
- [ ] 不要求部署 Jaeger / Tempo / Collector 作为 MVP Gate

---

# P0-MVP Gate

以下是实际使用验证标准，不是额外功能清单。

- [ ] 真实开发成员 ≥ 5，建议 5～10 人
- [ ] 连续真实使用 ≥ 14 天
- [ ] 累计真实请求 ≥ 500 次
- [ ] Claude Code 普通会话稳定
- [ ] 长 SSE 稳定
- [ ] 真实 Tool Loop 稳定
- [ ] Virtual Key Revoke 后立即失效
- [ ] MEMBER / Group / Model Permission 生效准确
- [ ] 无已知越权访问
- [ ] Usage 能准确归属到 MEMBER / Model / Resource
- [ ] 正常 Graceful Shutdown 不丢 Channel 中待写 Usage
- [ ] 无已知高危 Credential / Permission 问题
- [ ] 管理员真实完成过：
  - [ ] 创建 MEMBER
  - [ ] 创建 Group
  - [ ] 分配 MEMBER 到 Group
  - [ ] 配置 Group Model Permission
  - [ ] 配置 Anthropic Resource Credential
  - [ ] 签发 Virtual Key
  - [ ] Revoke Virtual Key
  - [ ] 查看 MEMBER Usage

---

# MVP 最终验证问题

在持续真实使用后，必须直接询问管理员：

> **如果撤掉 AI Coding Control Plane，你愿不愿意回到“每个人自己维护 API Key、缺少统一成员 / Group 治理和 Usage 归属”的状态？**

这个答案比功能完成度更重要。

---

# 明确不做

P0-MVP 禁止实现以下能力，防止范围蔓延：

```text
APPLICATION 完整闭环

完整 Provider CRUD
Provider 商业渠道
Provider Price 历史

Cost Dashboard
Billing
Balance / Prepaid

principal_resource
group_resource
完整 Resource Permission

Multi Provider Routing
Weighted Routing
Health Score
Resource Failover
Provider Failover

Budget
Rate Limit
Concurrency

Redis
Redis Stream
Redis Pub/Sub

OpenAI Native Path
Codex
Cross-Protocol Adapter
跨协议 Tool Call 映射

Knowledge
pgvector
Skill
Managed MCP

SSO
OIDC
LDAP
SCIM

多组织
Kubernetes
HA
Managed Cloud

完整 en-US 翻译
国际官网 / 国际发布物料
```

---

# 开发纪律

开发过程中如果发现某项“不在清单中但顺手做掉也不难”，先判断：

> **它是否直接影响当前 MVP 的验证结论？**

如果答案是否定的：

> **不做。**

P0-MVP 的成功标准不是“平台看起来完整”，而是：

> **Claude Code 在真实团队里被 MEMBER / Group / Model Permission / Virtual Key / Usage 这套治理闭环有效管理起来。**
