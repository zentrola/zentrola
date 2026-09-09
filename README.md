# Zentrola

Zentrola 是面向研发团队的企业 AI Coding 能力治理平台（AI Coding Control Plane）。

它位于 Codex、Claude Code 等 AI Coding Client 与企业模型订阅之间，在不改变研发人员工作习惯的前提下，统一治理并分发企业提供的模型、工具、知识和 Skill，同时管理权限、用量、成本与操作记录。

```text
Codex / Claude Code / 其他 AI Coding Client
                      ↓
                  Zentrola
                      ↓
          模型 / 工具 / 知识 / Skill
```

Zentrola 不取代 AI Coding Client，也不是另一套聊天界面。管理员通过管理后台配置企业 AI 能力；研发人员继续使用熟悉的客户端，通过企业分配的 Virtual Key 使用授权能力。

> 当前版本聚焦 Model Governance MVP，已经形成成员、Group、模型订阅、Virtual Key、Gateway 和 Usage 的最小治理闭环。Tool、Knowledge、Skill、Cost 等属于产品后续能力，不代表当前版本已经实现。

## 为什么选择 Zentrola

### 保留现有工作方式

研发人员继续使用 Codex、Claude Code 或其他兼容客户端，不需要迁移到企业自建的聊天页面，也不需要改变日常 Coding Workflow。

### 统一管理模型订阅

企业可以在管理后台集中配置采购或接入的模型订阅，通过逻辑模型向成员分发能力。研发人员只选择被授权的模型，不需要理解 Provider、Provider Model 和 Resource。

### 隔离真实 Provider Credential

Provider Credential 由管理员统一配置并加密保存，不下发给普通成员。成员只持有独立 Virtual Key，离职、权限变化或密钥泄露时可以单独撤销。

### 以成员和 Group 为单位治理

模型权限不绑定在共享 API Key 上，而是通过 MEMBER Principal、Group 和 Model Allowlist 统一管理。同一个成员可以加入多个 Group，授权关系清晰可追踪。

### 解耦客户端与 Provider

客户端使用稳定的逻辑模型编码。管理员可以调整其背后的 Provider、上游模型或 Resource，而无需要求所有成员更换 Virtual Key 或修改客户端中的模型名称。

### 统一归属 Usage 与管理操作

模型请求可以归属到成员、逻辑模型和 Resource；管理员对成员、Group、模型订阅、Resource 和 Virtual Key 的主要操作进入 Operation Log。

### 不把通用 Gateway 当作最终产品

Gateway 是 Zentrola 的基础设施层。产品重点是企业如何把模型及后续的 Tool、Knowledge、Skill 安全、可控地交付给研发人员，而不只是转发 API Traffic。

## 总体架构

```text
                            管理员
                              │
                              ▼
                     Admin Web（独立运行）
                              │
                              ▼
                         Admin API
                              │
┌──────────────────────── Zentrola ────────────────────────┐
│                                                          │
│  Principal / Group / Permission / Model Governance       │
│  Access Key / Resource / Usage / Operation Log            │
│                                                          │
│  AI Coding Client ──▶ Gateway ──▶ Model / Resource 路由   │
└──────────────────────────────────────────────────────────┘
                              │
                              ▼
                 Anthropic / DeepSeek / Future Provider
                              │
                              ▼
                          PostgreSQL
```

管理面与调用面相互隔离：

- 管理面：管理员通过 Admin Web 和 Admin API 配置平台。
- 调用面：MEMBER 使用 Virtual Key 访问标准 Gateway API。
- Admin JWT 不能调用 Gateway。
- Virtual Key 不能访问 Admin API。

## 职责边界

| 参与方 | 主要职责 |
| --- | --- |
| AI Coding Client | 模型选择、会话与上下文管理、用户交互、Tool Call、MCP Client 和 Skill 执行 |
| Zentrola | Access Key 鉴权、Principal 与 Group 权限、逻辑模型解析、Resource 选择、Streaming 转发、Usage 归属和操作审计 |
| Provider | 执行真实模型推理并返回原生协议响应 |
| 管理员 | 配置模型订阅、Provider Credential、成员、Group、授权策略和 Virtual Key |
| 研发人员 | 在授权范围内选择逻辑模型，不接触真实 Provider Credential，也不指定 Resource |

Zentrola 遵循 Client-native First 和 Thin Gateway 原则：客户端已经具备的会话、上下文和工具能力不在平台中重复实现；Gateway 只承担治理、路由和必要的协议处理。

## 核心概念

| 概念 | 含义 |
| --- | --- |
| Model | 向研发人员暴露的稳定逻辑模型 |
| Provider | 提供模型服务的官方或兼容服务方 |
| Provider Model | Provider 实际提供的上游模型，以及它与逻辑 Model 的映射 |
| Resource | 企业持有的一份可用模型订阅或调用资源，包含加密保存的 Provider Credential |
| MEMBER Principal | 被治理的企业研发人员 |
| Group | 成员集合，也是当前模型授权的主要载体 |
| Virtual Key | 成员访问 Gateway 的身份凭证，只负责识别 Principal，不直接承载权限 |
| Usage | 一次模型调用及其 Token 等使用事实 |
| Operation Log | 管理员执行重要治理操作的追加记录 |

## 当前版本能力

当前版本可用于验证以成员和 Group 为单位的 AI Coding 模型治理：

- 首位管理员初始化、登录、退出、修改密码和密码重置
- MEMBER 创建、状态管理和删除
- Group 创建、成员分配和 Group Model Allowlist
- 首次初始化预置 OpenAI、Anthropic、Google Gemini、DeepSeek、智谱、Kimi 和通义千问官方服务商及协议地址
- Provider Credential 加密存储；保存 API Key 后从官方模型接口同步模型及上游映射
- Resource 启停、连接测试和模型目录手动重新同步
- Virtual Key 签发、有效期管理和撤销
- Anthropic Messages、Count Tokens、SSE 和原生 Tool Loop
- OpenAI 与 Anthropic 上游协议双向兼容；优先使用同协议端点，未配置时转换请求和响应后使用另一协议端点
- OpenAI Models、Chat Completions、SSE 和工具调用往返
- 按成员、逻辑模型和 Resource 记录 Usage
- 查看主要 Operation Log

当前尚未完成：

- Claude Code 和 Codex 正式客户端的完整 E2E 验收
- OpenAI Responses Native Path
- APPLICATION Principal 与 App Key 管理闭环
- 多 Provider Routing、Resource Health、Failover 和实时限流
- Cost、Budget、企业计费和账单
- Enterprise Knowledge、Skill Registry 和 Managed MCP
- SSO、OIDC、LDAP、SCIM、多组织和高可用部署

Provider 端点按 `OPENAI` 和 `ANTHROPIC` 两类上游协议保存；其中 `ANTHROPIC` 即 Claude Code 使用的 Anthropic Messages 兼容协议。Gateway 客户端入口分别处理 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages；路由优先选择与客户端相同的上游协议，缺少同协议端点时才转换为另一协议，并将响应转换回客户端原协议。正式 Codex 客户端的完整 E2E 仍需单独实现和验收。

## 发布包结构

Backend 和 Admin Web 是两个独立程序，但发布到同一个目标目录，共享配置模板和前端静态资源。

在仓库根目录运行跨平台 Go 发布工具，可一键生成统一发布目录：

```powershell
go run ./cmd/release
```

默认在隔离的临时目录按 lockfile 安装前端依赖，不会修改开发目录中的 `web/node_modules`。
不传参数时自动构建当前操作系统与 CPU 架构，输出到 `dist/<系统>/<架构>`。如果已有完整的
`web/node_modules`，可加 `-skip-npm-install` 直接使用。也可以选择其他目标，例如构建 Linux
ARM64 发布包：

```powershell
go run ./cmd/release -target-os linux -architecture arm64
```

Windows 也可继续使用兼容 Windows PowerShell 5.1 的快捷入口：

```powershell
.\scripts\Build-Release.ps1
```

脚本会依次选择发布内容（Backend、Admin Web 或全部）、目标操作系统和 CPU 架构；也可以通过参数直接构建，适合自动化使用：

```powershell
# 只构建 Windows AMD64 Backend
.\scripts\Build-Release.ps1 -Component backend -TargetOS windows -Architecture amd64

# 只构建 macOS ARM64 Admin Web，并复用已有的 web/node_modules
.\scripts\Build-Release.ps1 -Component web -TargetOS macos -Architecture arm64 -SkipNpmInstall
```

对应的 Go 发布工具也支持 `-component backend|web|all`。单独构建某一组件时，只替换该组件的可执行文件或静态资源，不会删除同一目标下已生成的另一组件。

统一发布目录：

```text
dist/<系统>/<架构>/
├── zentrola              # macOS/Linux Backend
├── zentrola.exe          # Windows Backend
├── zentrola-web          # macOS/Linux Admin Web
├── zentrola-web.exe      # Windows Admin Web
├── .env.example
└── dist/
    ├── index.html
    ├── config.js
    └── assets/
```

实际发布包只包含对应操作系统的两个可执行文件，不会同时包含无扩展名和 `.exe` 两组版本。

## 构建应用镜像

应用镜像直接使用 `dist/linux/<架构>` 的统一发布包，同时包含 Backend、Admin Web 和前端静态资源。先生成 Linux 发布包：

```powershell
.\scripts\Build-Release.ps1 -Component all -TargetOS linux -Architecture amd64
```

再在仓库根目录构建镜像：

```powershell
docker build --build-arg TARGETARCH=amd64 -t zentrola:latest .
```

镜像默认以前台模式运行 Backend；真实配置通过环境变量注入，不会把本地 `.env` 写入镜像：

```powershell
docker run -d --name zentrola-backend `
  -p 127.0.0.1:9527:9527 `
  --env-file .env `
  zentrola:latest backend
```

同一个镜像可以启动 Admin Web，镜像内的 `/app/dist` 由 `zentrola-web` 提供：

```powershell
docker run -d --name zentrola-web `
  -p 127.0.0.1:9528:9528 `
  zentrola:latest web --api https://api.zentrola.example.com
```

容器以非 root 用户运行。`backend` 不传后续参数时内部执行 `zentrola serve`，也可以执行一次性命令，例如 `zentrola:latest backend migrate`；`web` 后的参数原样传给 `zentrola-web`。

## 安装 Backend

### 1. 准备配置

进入对应系统与架构的统一发布目录，将示例配置改名为 `.env`。Backend 和 Admin Web 可以共享该配置文件。

MacOS/Linux：

```bash
mv .env.example .env
```

Windows PowerShell：

```powershell
Rename-Item .env.example .env
```

至少设置以下配置：

```dotenv
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_DB=zentrola
POSTGRES_USER=zentrola
POSTGRES_PASSWORD=请填写数据库密码

ADMIN_JWT_SECRET=请填写至少包含32随机字节的Base64字符串

CORS_ENABLED=true
CORS_ALLOWED_ORIGINS=http://127.0.0.1:9528
```

MacOS/Linux 可以使用 OpenSSL 生成 `ADMIN_JWT_SECRET`：

```bash
openssl rand -base64 32
```

Windows PowerShell：

```powershell
$jwtBytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($jwtBytes)
[Convert]::ToBase64String($jwtBytes)
$rng.Dispose()
```

`ADMIN_JWT_SECRET` 不得与数据库密码、Provider Credential 或 Master Key 共用。

### 2. 准备 PostgreSQL

如果已经有 PostgreSQL 17，只需创建 `.env` 中指定的数据库和用户，随后直接运行 Backend。

仓库根目录的 `compose.yaml` 是可选的 PostgreSQL 快速安装示例，只启动数据库，不运行 Zentrola Backend 或 Admin Web；如需使用，可单独复制到部署目录：

```shell
docker compose up -d postgres
```

示例会读取同目录 `.env` 中的 `POSTGRES_DB`、`POSTGRES_USER` 和 `POSTGRES_PASSWORD`，并将数据库映射到宿主机端口 `15432`。使用该示例时，只需将 Backend 的数据库端口调整为：

```dotenv
POSTGRES_PORT=15432
```

Docker 会自动创建 `zentrola_postgres_data` 命名卷，不需要预先创建。正式环境应在首次启动前确定 PostgreSQL 的备份、恢复和主机故障迁移方案。

### 3. 运行 Backend

MacOS/Linux：

```bash
chmod +x zentrola
./zentrola
```

Windows PowerShell：

```powershell
.\zentrola.exe
```

不带参数运行时，Backend 在当前终端前台运行并读取同目录的 `.env`。默认地址为 `http://127.0.0.1:9527`，按 `Ctrl+C` 可以优雅停止。

需要后台运行时：

MacOS/Linux：

```bash
./zentrola start
./zentrola status
```

Windows PowerShell：

```powershell
.\zentrola.exe start
.\zentrola.exe status
```

后台运行状态和日志保存在可执行文件旁的 `run` 目录。

## 安装 Admin Web

### 1. 准备配置

Admin Web 与 Backend 位于同一个发布目录，直接使用前面由 `.env.example` 创建的 `.env`。

配置监听端口和浏览器能够访问的 Backend 地址：

```dotenv
WEB_ADDR=:9528
WEB_API_BASE_URL=http://127.0.0.1:9527
```

远程部署时，`WEB_API_BASE_URL` 必须填写用户浏览器能够访问的地址，例如 `https://api.zentrola.example.com`。同时需要把 Admin Web 的实际 Origin 配置到 Backend 的 `CORS_ALLOWED_ORIGINS`。

### 2. 运行 Admin Web

MacOS/Linux：

```bash
chmod +x zentrola-web
./zentrola-web
```

Windows PowerShell：

```powershell
.\zentrola-web.exe
```

也可以通过命令行临时指定 Backend 地址；命令行参数优先于 `.env`，只传 `--api` 时
同时作为 Gateway 地址：

```powershell
.\zentrola-web.exe --api https://api.example.com
# API 与 Gateway 使用不同公网地址时：
.\zentrola-web.exe --api https://api.example.com --gateway https://gateway.example.com
```

Admin Web 默认地址为 `http://127.0.0.1:9528`。程序读取同目录的 `.env` 和 `dist`，启动后不需要 Node.js、npm 或 Vite。

## 首次使用

首次打开 Admin Web 时创建首位管理员。系统不提供默认用户名或密码，创建成功后初始化入口自动关闭。

首次初始化只写入公开的厂商信息和协议地址，不内置 API Key，也不会在没有凭据时访问厂商。管理员按以下顺序完成第一条治理链路：

1. 在“服务商”中为预置厂商填写 Provider Credential。保存后可通过服务商编码选择官方模型目录适配器并同步模型；当前支持 DeepSeek。同步只新增尚不存在的模型和上游映射，新模型默认停用。
2. 在“模型”中检查同步结果，并启用准备开放给客户端的逻辑模型；必要时再调整服务商映射。
3. 创建成员和 Group，将成员加入对应 Group。
4. 为 Group 授权可用逻辑模型。
5. 为成员签发 Virtual Key。
6. 将 Gateway Base URL、Virtual Key 和逻辑模型编码交给研发人员。
7. 在 Usage 和 Operation Log 中检查使用归属与治理操作。

普通成员不登录 Admin Web，也不能获取真实 Provider Credential、选择 Provider 或指定 Resource。

## Gateway 接入

Virtual Key 只负责识别 MEMBER Principal，实际权限来自成员所属 Group 的 Model Allowlist。

Gateway 路由遵循“同协议优先、异协议兜底”：Anthropic 客户端优先使用 `ANTHROPIC` 端点，否则使用 `OPENAI` 端点；OpenAI 客户端顺序相反。异协议调用会转换普通响应、SSE、工具调用、停止原因、错误和 Usage，客户端始终收到其请求协议的格式。上游已经开始调用后不会因超时或错误切换协议重试，避免同一请求被执行两次。Anthropic `count_tokens` 没有等价的 OpenAI 上游接口，因此仅配置 `OPENAI` 端点时返回路由不可用，不生成估算值。

### Anthropic Compatible

| 配置 | 值 |
| --- | --- |
| Base URL | `http://<backend-host>:9527/anthropic` |
| Messages | `POST /anthropic/v1/messages` |
| Count Tokens | `POST /anthropic/v1/messages/count_tokens` |
| 认证 | `x-api-key: <Virtual Key>` 或 `Authorization: Bearer <Virtual Key>` |

两种认证方式不要同时发送。

当逻辑模型路由到 DeepSeek 官方 Anthropic Endpoint 时，Gateway 会自动移除 DeepSeek 尚不支持的 `advisor_20260301` 服务端工具及其 `advisor-tool-2026-03-01` beta 标记；其他工具和原生 Anthropic Provider 请求保持透传。客户端不需要单独关闭 Claude Code Advisor。

### OpenAI Compatible

| 配置 | 值 |
| --- | --- |
| Base URL | `http://<backend-host>:9527/v1` |
| Models | `GET /v1/models` |
| Chat Completions | `POST /v1/chat/completions` |
| 认证 | `Authorization: Bearer <Virtual Key>` |

请求中的 `model` 使用管理员分配的逻辑模型编码，而不是上游 Provider Model 编码。

## 日常运维

Backend 后台进程管理：

MacOS/Linux：

```bash
./zentrola status
./zentrola restart
./zentrola stop
```

Windows PowerShell：

```powershell
.\zentrola.exe status
.\zentrola.exe restart
.\zentrola.exe stop
```

健康检查：

```shell
curl -fsS http://127.0.0.1:9527/health/live
curl -i http://127.0.0.1:9527/health/ready
```

`/health/live` 表示 Backend 能够响应请求。`/health/ready` 还会检查 PostgreSQL、Master Key、基础数据和管理员初始化状态，但不表示 Provider Resource 已经配置或上游账号可用。

管理员密码恢复：

MacOS/Linux：

```bash
./zentrola password --username <管理员账号>
```

Windows PowerShell：

```powershell
.\zentrola.exe password --username <管理员账号>
```

执行密码恢复前，应先停止使用相同 `ID_NODE` 的 Backend。新密码只显示一次，应保存到密码管理器，不要把命令输出写入共享日志。

## 数据与备份

Backend 默认在发布目录下生成：

```text
data/secrets/master.key
```

Master Key 用于加密 Provider Credential，不存储在 PostgreSQL 中。PostgreSQL 数据与 Master Key 必须作为一组备份和恢复；只有数据库而没有原 Master Key 时，已经保存的 Provider Credential 无法解密。

如果使用 PostgreSQL Docker 示例，停止数据库不会删除数据：

```shell
docker compose stop postgres
```

不要执行 `docker compose down -v`，除非明确要永久删除 PostgreSQL 数据卷。

## 安全说明

- 不要提交或对外发送 `.env`、Provider Credential、Virtual Key、管理员密码和 Master Key。
- Admin JWT 与 Virtual Key 用途不同，禁止混用。
- 正式环境应通过 HTTPS 暴露 Admin Web 和 Gateway。
- CORS 只允许实际使用的 Admin Web Origin，不使用通配符。
- 不在日志、Issue、截图或客户端配置示例中暴露任何 Credential。

## 请求日志

Backend 的 `pretty` 控制台访问日志使用固定紧凑格式，包含请求时间、Trace/Span ID、HTTP 方法、请求路径、`status`、`cost`，以及经过白名单过滤的请求与响应 Header 摘要。`Authorization`、Cookie、API Key 等敏感 Header 永不记录。请求和响应字节数仍保留在 JSON 结构化日志中，便于统计但不占用控制台位置。

`APP_ENV=dev` 时还会输出 `request_body` 和 `response_body` 调试摘要。JSON 中的密码、Token、Credential、Virtual Key、prompt、消息内容和模型输出会自动替换为 `[REDACTED]`；超过 8 KiB 的 JSON、SSE 和其他非 JSON 报文只记录类型及字节数。`APP_ENV=test` 或 `APP_ENV=prod` 时不采集请求和响应报文。

控制台与日志文件由同一个结构化日志记录生成，因此消息、级别、OpenTelemetry `trace_id` / `span_id` 和业务字段保持一致。控制台支持适合人工阅读的彩色 `pretty` 格式，日志文件固定使用一行一条记录的 JSON Lines：

```dotenv
LOG_LEVEL=info
LOG_CONSOLE_FORMAT=pretty
LOG_COLOR=auto
LOG_FILE_PATH=data/logs/zentrola.jsonl
LOG_FILE_MAX_SIZE_MB=100
LOG_FILE_MAX_BACKUPS=10
```

`LOG_COLOR=auto` 只在真实终端启用 ANSI 颜色；也可设置为 `always` 或 `never`。`LOG_FILE_PATH` 为空时不创建日志文件。文件达到 `LOG_FILE_MAX_SIZE_MB` 后依次轮转为 `.1`、`.2`，最多保留 `LOG_FILE_MAX_BACKUPS` 份。同一日志路径由进程独占；第二个进程配置相同路径时会拒绝启动，避免并发轮转破坏文件。多实例部署应使用不同路径，或统一输出 stdout JSON。

Backend 使用 OpenTelemetry 为每个入站 HTTP 请求创建 Server Span，并通过 W3C `traceparent` 延续客户端 Trace；调用 Anthropic、OpenAI 或 DeepSeek 时自动创建子 Client Span并向上游传播。响应头 `X-Trace-ID` 和 `X-Span-ID` 便于直接排障。原有 `X-Request-ID` 与响应体 `requestId` 作为客户端和数据库兼容字段继续保留，但不再作为日志主关联字段。

JSON 文件通过有界异步队列写入，请求 goroutine 不直接等待磁盘；关闭服务时会先 flush。队列满时保留同步控制台日志、丢弃对应文件副本，并向 `stderr` 输出一次明确告警。

推荐按部署环境选择日志出口：

- 本地开发：`LOG_CONSOLE_FORMAT=pretty`、`LOG_COLOR=auto`、`LOG_FILE_PATH=`，由终端或 IDE 保存 stdout。
- 单机生产：没有 journald 或日志 Agent 时启用 `LOG_FILE_PATH=data/logs/zentrola.jsonl`，使用应用内异步轮转文件；通过内置 `start` 命令后台运行时会关闭常规控制台副本，`run/server.log` 只保留启动和致命错误，避免双写。
- Docker：Backend 设置 `LOG_CONSOLE_FORMAT=json`、`LOG_COLOR=never`、`LOG_FILE_PATH=`。仓库当前的 PostgreSQL Compose 示例已定义带大小和份数限制的 `local` logging driver；以后把 Backend 加入 Compose 时复用同一个 `default-logging` anchor，日志通过 `docker logs` 查看。
- Kubernetes：使用与 Docker 相同的 stdout JSON 配置，由 kubelet 轮转，并通过节点日志 Agent 发送到 Loki、OpenSearch 或云日志平台；不要让多个 Pod 写共享日志文件。

stdout 是日志传输通道而不是“不保留日志”：容器运行时负责短期保存和轮转，集中日志系统负责长期保留、按 `trace_id` 检索和告警。旧的 `LOG_FORMAT=text|json|pretty` 仍兼容；未设置 `LOG_CONSOLE_FORMAT` 时会作为控制台格式使用。

## 接口文档

`APP_ENV=dev` 或 `APP_ENV=test` 时，Backend 提供：

- Swagger UI：`http://<backend-host>:<backend-port>/swagger/index.html`
- Swagger JSON：`http://<backend-host>:<backend-port>/swagger/doc.json`

`APP_ENV=prod` 时不注册 Swagger 路由。完整 Backend 配置见 [.env.example](.env.example)。
