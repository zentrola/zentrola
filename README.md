# zentrola

企业 AI Coding 能力治理平台。当前开发范围为 P0-MVP Claude Code Governance Slice，按用户追加授权扩展 DeepSeek 和 OpenAI 兼容接口。

## 当前状态

阶段 0～3 已实现：Go 四层结构、14 张业务表、sqlc / Goose、安全认证，以及成员、分组、模型、资源和操作日志管理 API。管理员可通过 API 完成创建成员、入组、授权模型、配置资源和签发 Key 的操作链。

模型目录现支持手动新增和编辑官方名称、编码、JSONB 输入/输出类型及备注，详见 [模型目录管理](docs/模型目录管理.md)。

首次启动会原子创建默认 Organization、Anthropic Official、两个逻辑 Model 及对应 Provider Model。管理员由首次使用者在网页中设置，创建成功后关闭初始化入口；后续启动不会覆盖配置或恢复软删除记录，不预置 Resource Credential。
阶段 4 Gateway 和阶段 5 Usage 已实现，另已按用户授权接入 DeepSeek 并通过真实 Messages、SSE、Tool Loop、count_tokens、权限和 Usage 归属验证。Usage 支持固定 Worker、批量写入、队列满同步兜底、停机 Flush 和管理查询。阶段 6 管理界面已在 `web/` 完成并通过真实后端联调；真实 Claude Code 客户端会话尚未验收。

OpenAI 兼容入口已实现：`POST /v1/chat/completions`、`GET /v1/models`，支持普通响应、SSE 和工具往返，并通过真实 DeepSeek 及 Usage 验证。当前上游为 DeepSeek，未提供 OpenAI 官方 GPT 模型或 Responses API。

数据库、Master Key、基础数据和管理员均正常时，`/health/ready` 返回 200；这不代表已有可调用的 Resource 或上游账号已通过验证。

完整管理接口、请求示例和操作顺序见 [阶段 3 Admin API](docs/阶段3_Admin_API.md)。
Gateway 行为、超时配置与后续 Claude Code 接入步骤见 [阶段 4 Anthropic Gateway](docs/阶段4_Anthropic_Gateway.md)。
DeepSeek 目录安装、资源配置、真实测试结果与客户端配置见 [DeepSeek 接入与实测](docs/DeepSeek_接入与实测.md)。
Usage 归属、计数语义、查询接口、可靠性边界和停机验证见 [阶段 5 Usage](docs/阶段5_Usage归属与可靠写入.md)。
OpenAI 客户端配置、已有环境升级和真实测试结果见 [OpenAI 兼容接口](docs/OpenAI_兼容接口.md)。
页面功能和验收记录见 [阶段 6 Admin Web](docs/阶段6_Admin_Web.md)，前端开发命令见 [web/README](web/README.md)。

## 工程结构

```text
cmd/server/                         启动、依赖装配与进程生命周期
internal/
  domain/                           当前领域的机器语义和 ID 生成接口
  application/
    bootstrap/                      首次数据初始化编排
    health/                         就绪检查编排
    security/                       管理员认证和 Access Key 用例
    management/                     成员、分组、模型、资源和审计用例
    gateway/                        模型路由、权限及原生请求转发编排
    usage/                          有界写入队列、固定 Worker 与管理查询
  infrastructure/
    config/                         System ENV > .env.{环境} > .env > 默认值
    logging/                        slog 与请求上下文
    idgen/                          Sonyflake 正数 int64 ID
    security/                       bcrypt、JWT、Master Key 与 AES-GCM
    anthropic/                      官方连接检查与 Messages 上游 HTTP 客户端
    openai/                         DeepSeek 原生 Chat Completions 上游 HTTP 客户端
    postgres/
      migrations/                   嵌入到二进制的 SQL Migration
      queries/                      sqlc SQL 源文件
      dbgen/                        sqlc 生成的 pgx/v5 类型安全代码
  transport/http/                   chi 路由、中间件、HTTP 响应
docs/                               产品方案与当前开发清单
web/                                Vue 3 管理界面、Vite 构建及浏览器测试
```

Domain 不依赖数据库或 HTTP；Application 编排用例；Infrastructure 实现外部依赖；Transport 负责协议。使用显式构造函数装配，不引入 DI 容器。sqlc 数据库记录不直接暴露为 API 响应，管理写入和完整关联校验随对应 Application 用例实现。

## 本地运行

需要 Go 1.26 或以上版本，以及 PostgreSQL 17。建议使用与 Windows 系统匹配的 amd64 Go 安装包。

```powershell
Copy-Item .env.example .env
# 编辑 .env：填入数据库密码和 ADMIN_JWT_SECRET；管理员在网页首次设置。
# 确认 zentrola 数据库已创建；保留已有 .env 时不要重新复制覆盖。
go mod download
go run ./cmd/server
```

程序只迁移指定数据库，不自动创建数据库或覆盖已有业务数据。连接参数分别配置，密码中的特殊字符无需手工 URL 编码。
本地 `.env` 和 `.env.dev/test/prod` 已被 Git 和 Docker 构建上下文忽略，禁止提交；仓库仅保留不含真实凭证的 `.env*.example`。不要在日志中打印配置对象、连接字符串或原始凭证。

### 运行环境与配置覆盖

`APP_ENV` 支持 `dev / test / prod`，默认 `dev`。先从系统环境变量、再从通用 `.env` 中选择环境，随后只读取同目录下对应的 `.env.dev`、`.env.test` 或 `.env.prod`。保留旧值 `development / production` 的兼容，分别归一化为 `dev / prod`，使用短名称的环境文件。

配置优先级：**系统环境变量 > 当前环境文件 > 通用 `.env` > 代码默认值**。环境文件可新增配置或覆盖通用值，未定义的键继承低优先级配置；显式空值也会覆盖，必填项为空时启动报错。环境文件不得定义 `APP_ENV`，避免加载过程中切换环境。非法环境名称和已存在但无法读取或解析的配置文件会使启动失败；文件不存在则跳过，最终配置仍须通过校验。加载过程不修改进程环境变量。

```powershell
# 本地开发：.env 中 APP_ENV=dev，按需添加开发环境覆盖。
Copy-Item .env.dev.example .env.dev
go run ./cmd/server

# 测试部署：使用 .env 通用配置，再叠加 .env.test。
Copy-Item .env.test.example .env.test
$env:APP_ENV = 'test'
go run ./cmd/server
Remove-Item Env:APP_ENV
```

已有环境文件时不要重复复制覆盖。生产环境可设系统环境变量 `APP_ENV=prod`，配合 `.env.prod` 或完全由系统环境变量提供配置。`test` 表示测试部署环境，`go test` 不会自动设置它。配置在启动时读取，修改后需重启。

### Swagger 接口文档

`APP_ENV=dev/test` 时，启动后访问 [Swagger 页面](http://localhost:8080/swagger/index.html)；原始文档为 `/swagger/doc.json`，也可访问 `/swagger` 自动跳转。文档覆盖健康检查、认证、成员、分组授权、资源、操作日志、用量及 Anthropic/OpenAI Gateway。地址随服务端口变化，文档请求与调试使用当前访问域名，不固定 localhost。UI 静态资源与文档 JSON 都随二进制打包，不依赖 CDN。

`APP_ENV=prod` 时不注册页面、文档 JSON 和静态资源路由，访问返回 404。环境开关在启动时判断，不在业务请求中逐次判断；文档依赖和资源仍包含在二进制中。程序启动不执行生成器，不扫描源码。

管理接口先调用 `POST /api/v1/auth/login`，将响应中的 `data.token` 以 `Bearer <token>` 形式填入右上角 **Authorize → AdminBearer**。Gateway 使用成员 Access Key：OpenAI 填入 **GatewayBearer**，Anthropic 可选 **GatewayKey**（Key 原文）或 **GatewayBearer**，两者不要同时使用。页面不持久保存授权；真实推理与连接测试会调用已配置的上游。Gateway 的原生 JSON 请求示例见各接口描述；SSE 建议使用 curl 或 SDK 验证。

接口旁的 Swaggo 注释是文档来源，生成器版本由 `go.mod` 的 tool 依赖固定。修改接口或 DTO 后，在构建、重启前重新生成：

```powershell
# 从项目根目录执行；脚本优先使用 PATH 中的 Go，也支持项目缓存工具链。
./scripts/Generate-ApiDocs.ps1

# 等价命令（需要 go 已加入 PATH）
go tool swag init -d ./cmd/server,./internal/transport/http -g main.go --parseInternal --parseDependencyLevel 1 --parseFuncBody --outputTypes json -o ./internal/transport/http/apidocs
go run ./cmd/server
```

生成的 `internal/transport/http/apidocs/swagger.json` 与源代码一起提交。`.swaggo` 将审计快照的 `json.RawMessage` 映射为 JSON 对象；请求 DTO 与实际 Handler 共用，分页响应也复用同一泛型结构。Docker 构建会重新生成文档。路由测试检查文档覆盖、Schema 引用、业务 ID 类型以及各环境的页面访问行为。

新环境打开管理页面，自行设置首位管理员的账号和密码，没有默认用户名或密码。已有管理员时直接登录，升级不修改旧账号。JWT 签名密钥仍需至少 32 随机 bytes 的 Base64，且与 Master Key 独立。流程与并发防护见 [首次管理员初始化](docs/首次管理员初始化.md)。

```powershell
Invoke-RestMethod http://localhost:8080/health/live
# 管理员初始化完成且依赖正常后应返回 HTTP 200 / READY。
curl.exe -i http://localhost:8080/health/ready
```

请求进入时生成新的 `X-Request-ID`，不信任客户端传入的 ID。健康检查响应中的 `requestId` 与 Header 一致；日志字段使用 `request_id`。

## Migration

采用 [Goose Provider](https://pressly.github.io/goose/documentation/provider/)，通过 pgx 连接 PostgreSQL，并使用数据库会话锁串行执行迁移。
Migration SQL 随二进制嵌入：`00001` 为空迁移，`00002` 创建清单内 14 张业务表、中文 COMMENT、CHECK 约束和索引，`00003` 补充 Usage 请求事实查询索引，`00004` 增加 OpenAI 上游地址、协议约束及按协议唯一的模型映射，`00005` 增加首位管理员初始化审计及查询索引，`00006` 增加模型发布方、JSONB 输入输出类型、备注和模型新增编辑审计，`00007` 移除模型发布方字段。

```powershell
go run ./cmd/server migrate
```

默认启动自动执行未应用的 Migration；如需独立迁移，设置 `MIGRATIONS_AUTO_APPLY=false`。
已执行的 Migration 不回写修改；变更使用新版本文件。业务表使用应用侧 BIGINT ID、中文 COMMENT、Partial Unique Index 和无外键规范；Goose 自身的版本表属于工具元数据。

`migrate` 只迁移 Schema，正常 `serve` 启动再执行首次 Bootstrap。预置逻辑模型 `claude-sonnet / claude-opus` 与真实上游编码分离，首次默认映射为 `claude-sonnet-5 / claude-opus-5`，依据 [Anthropic 模型文档](https://platform.claude.com/docs/en/models/overview)。可在首次启动前设置 `BOOTSTRAP_SONNET_MODEL / BOOTSTRAP_OPUS_MODEL`；初始化后运行时映射以数据库为准。预置映射不是上游账号可用性测试，实际连通性留到 Resource 和 Gateway 阶段验证。

`ID_NODE` 默认 1，范围 1～65535。Sonyflake 使用固定 epoch `2026-01-01 UTC`，进程内共享同一生成器；同一数据库的并行写入进程必须分配不同节点号。P0-MVP 按单实例部署，不自动分配节点号或建设分布式协调服务。

数据字段与阶段边界说明见 [阶段 1 数据基础](docs/阶段1_数据基础.md)。

## Docker Compose

本地开发前端时，先启动 Go 后端，然后在 `web/` 执行 `npm.cmd ci`、`npm.cmd run dev`，访问 `http://127.0.0.1:5173`。执行 `npm.cmd run build` 后，Go 可直接提供管理页面，默认入口 `http://127.0.0.1:8080/`；从仓库根目录启动并保留 `web/dist`。

```powershell
# 先配置 .env 中的数据库密码及独立 JWT 签名密钥，启动后在网页设置管理员。
docker compose up --build -d
docker compose logs -f app
docker compose stop
```

Compose 仅包含 `app + postgres`，使用独立 PostgreSQL 数据卷，宿主机默认端口 `15432`，与本地已有 `5432` 实例隔离。切换本地程序连接到 Compose 数据库时，将 `.env` 中的 `POSTGRES_PORT` 改为 `15432`。

Docker 多阶段构建会安装前端锁定依赖并生成静态页面，最终镜像只保留 Go 二进制和页面产物，不运行 Node 服务。管理界面与 API 同源，访问应用根地址即可登录。

当前 Compose 将应用环境固定为 `prod`，通过 `environment` 注入配置；镜像不包含 `.env` 文件。应用的分层加载不会自动让 Compose 读取宿主机 `.env.prod`。如需将其中的值用于 Compose 变量插值，使用 `docker compose --env-file .env --env-file .env.prod up --build -d`；只有 `compose.yaml` 中映射的变量会传入容器，固定值仍以 Compose 定义为准。

镜像默认输出 JSON 日志，以非 root 身份和只读根文件系统运行。`secrets_data` 持久卷保存 `/data/secrets/master.key`，文件权限为 `0600`，拥有者为 UID 65532；重建应用容器复用原 Key。容器健康检查使用 liveness，业务就绪状态通过 `/health/ready` 检查。

本地直接运行时，Master Key 默认保存于 `data/secrets/master.key`；Windows 使用当前用户和 SYSTEM 专用 DACL。Master Key 不存数据库，须与数据库一起保留。连接同一数据库的不同运行方式必须使用同一 Master Key；本地文件与 Compose 卷不会自动同步。

SIGINT / SIGTERM 触发停止接收请求、等待在途请求结束、关闭并排空 Usage Queue，最后关闭数据库。HTTP 不设全局 WriteTimeout，以支持长 SSE。

## 开发检查

```powershell
go test ./...
go vet ./...
go build -o bin/zentrola.exe ./cmd/server

# PostgreSQL 集成测试：仅在随机隔离 schema 中验证并清理，不修改 public 业务表。
$env:ZENTROLA_INTEGRATION = '1'
go test ./internal/infrastructure/postgres -count=1 -v
Remove-Item Env:ZENTROLA_INTEGRATION

# 修改 Migration 或查询之后，使用 sqlc 1.30.0 重新生成。
sqlc generate
# 无本地 sqlc 时，在项目根目录使用固定版本镜像：
docker run --rm --mount "type=bind,source=$($PWD.Path),target=/src" --workdir /src sqlc/sqlc:1.30.0 generate
```

支持 race detector 的 Go 工具链上可执行 `go test -race ./...`。32 位 Windows Go 不支持 race detector。

## 开发基线

- [P0-MVP 开发清单](docs/P0-MVP_开发清单_Claude_Code_Governance_Slice_V1.0.md)：当前实现范围与验收依据。
- [总体方案 V2.6.6](docs/AI_Coding_Control_Plane_企业AI_Coding能力治理平台_V2.6.6.md)：长期架构和工程规范。

按用户追加授权支持 Anthropic / DeepSeek 两个固定官方 Provider，不建设通用多 Provider 路由。暂不实现 Redis、Failover、Cost、Knowledge、Skill、MCP 或 APPLICATION 闭环。
