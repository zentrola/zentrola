# Zentrola

[English](README.md) | [简体中文](README.zh-CN.md)

[![CI](https://github.com/zentrola/zentrola/actions/workflows/ci.yml/badge.svg)](https://github.com/zentrola/zentrola/actions/workflows/ci.yml)
[![Security](https://github.com/zentrola/zentrola/actions/workflows/security.yml/badge.svg)](https://github.com/zentrola/zentrola/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Zentrola 是面向研发团队的企业 AI Coding 能力治理平台（AI Coding Control Plane）。

它位于 Codex、Claude Code 等 AI Coding Client 与企业模型订阅之间。研发人员继续使用熟悉的客户端，管理员则统一管理服务商、凭据、逻辑模型、访问权限、用户与应用密钥、Token 配额、用量成本和操作记录。

> Zentrola 正在持续开发。`1.1.0` 版本在 Model Governance MVP 基础上增加了应用接入、用量成本核算和月度 Token 配额。Tool、Knowledge、Skill、SSO 和完整高可用方案仍属于后续工作。

![Zentrola 管理控制台](docs/assets/admin-console.png)

## 为什么使用 Zentrola

- 保留现有 AI Coding Client 和工作习惯。
- 使用成员独立的 Virtual Key，避免向研发人员下发真实 Provider Credential。
- 通过成员、Group 和逻辑模型授权，不再共享上游 API Key。
- 更换服务商、上游模型或凭据时，无需修改客户端使用的逻辑模型编码。
- 统一承接 OpenAI 与 Anthropic 兼容流量，并提供协议转换和服务商故障切换。
- 将调用用量和成本归属到用户、应用、逻辑模型、服务商和凭据。
- 通过相互独立的用户、应用和 Group 月度配额控制 Token 消耗。
- 记录重要管理操作，便于追踪和审计。

## 工作方式

```text
Codex / Claude Code / OpenAI 兼容客户端 / 应用
                         │
                Virtual Key / App Key
                         ▼
┌──────────────────── Zentrola ────────────────────┐
│ 身份认证 → Group 权限 → Token 配额 → 逻辑模型    │
│          → Provider 路由 → Usage 与成本记录      │
└──────────────────────────────────────────────────┘
                         │
             OpenAI / Anthropic / 兼容服务商
                         │
                  PostgreSQL / Redis
```

管理面与调用面使用相互独立的认证域：

- 管理员使用 Admin Web 和 Admin API，通过 Admin JWT 登录。
- 用户使用 Virtual Key、应用使用 App Key 调用 Gateway API。
- Admin JWT 不能调用 Gateway。Gateway Key 不能访问管理员管理接口；符合条件的用户 Virtual Key 可以访问 `/api/v1/me` 下的成员自助接口。

## 当前能力

- 首位管理员初始化、登录、退出、修改密码和管理员密码重置
- MEMBER 创建、状态管理和删除
- 应用生命周期管理，以及 App Key 的签发、有效期管理和撤销
- 用户与应用的 Group 归属和 Group Model Allowlist
- 服务商初始化，以及 OpenAI/Anthropic 兼容端点维护
- API Key 加密存储，以及受支持的 ChatGPT/Codex 和 Claude Code 个人订阅认证
- Provider 出站代理配置和 Endpoint 公网/私网范围
- Provider Credential 连接测试和模型目录手动同步
- 逻辑模型与带优先级的 Provider Model 映射
- Virtual Key 签发、有效期管理和撤销
- Anthropic Messages、Count Tokens、SSE 和工具调用
- OpenAI Models、Chat Completions、Responses、Images API、SSE 和工具调用
- 同协议优先、异协议转换和服务商故障切换
- 用户与应用 Usage 仪表盘和 Operation Log
- 带生效时间的个人订阅周期费用和 API Key 每百万 Token 单价
- 默认展示当月的成本概览、订阅费用分摊、用量排行，以及支持筛选、CSV 导出和单次调用成本拆解的计费明细
- 用户、应用和 Group 相互独立的月度 Token 配额，包括用量等级、Gateway 准入控制、带审计的额度增加和取消限制
- Admin Web 中英文界面

模型目录同步目前为 OpenAI、Google、DeepSeek、智谱 AI、月之暗面、通义千问和 xAI 提供专用适配器。其他 OpenAI 或 Anthropic 兼容服务商可以手动配置。

## 快速开始

### 环境要求

- Windows、macOS 或 Linux 主机
- PostgreSQL 17
- 推荐使用 Redis 保存 Gateway 缓存和路由冷却状态；Redis 暂时不可用时，Gateway 会回退到 PostgreSQL

如果 [GitHub Releases](https://github.com/zentrola/zentrola/releases) 已提供目标操作系统和 CPU 架构的发布包，请优先使用。发布包包含 Backend、Admin Web、静态资源、`.env.example`、项目许可证和第三方许可证声明。

如果目标平台暂时没有发布包，请参考 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md) 从源代码构建相同结构的发布包。

### 1. 配置 Zentrola

将 `.env.example` 重命名为 `.env`，至少配置：

```dotenv
APP_ENV=prod

POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_DB=zentrola
POSTGRES_USER=zentrola
POSTGRES_PASSWORD=请替换为高强度密码

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_DB=2
REDIS_PASSWORD=请替换为Redis密码

ADMIN_JWT_SECRET=请替换为至少32个随机字节的Base64字符串
MASTER_KEY=请替换为另一组32个随机字节的Base64字符串

CORS_ENABLED=true
CORS_ALLOWED_ORIGINS=http://127.0.0.1:9528

WEB_API_BASE_URL=http://127.0.0.1:9527
WEB_GATEWAY_BASE_URL=http://127.0.0.1:9527
```

成本处理默认使用以下配置。这些配置不是必填项；只有需要调整扫描间隔、超时时间或批量大小时，才需要将其加入所选配置文件：

```dotenv
# 个人订阅与 API Key 月度结算：每小时扫描一次，账期结束 10 分钟后结算，单轮最长 5 分钟。
BILLING_SETTLEMENT_INTERVAL=1h
BILLING_SETTLEMENT_GRACE=10m
BILLING_SETTLEMENT_TIMEOUT=5m

# API Key 单次调用异步核算：每分钟扫描，每轮最长 50 秒，每页最多处理 500 条待核算记录。
BILLING_API_KEY_RATING_INTERVAL=1m
BILLING_API_KEY_RATING_TIMEOUT=50s
BILLING_API_KEY_RATING_PAGE_SIZE=500
```

两个后台任务都使用 Redis 锁，确保每类任务同一时间只有一个 Backend 实例执行。取值约束、故障行为和调优建议见[部署与运维](docs/operations.zh-CN.md#成本结算与用量核算)。

`ADMIN_JWT_SECRET`、`MASTER_KEY`、数据库密码和 Provider Credential 必须彼此独立。`MASTER_KEY` 只从 Backend 选定的通用配置文件读取；二进制部署默认使用 `.env`，Compose 会将文件顶部填写的值以内联配置挂载为 `/app/.env`。该值必须与数据库一起备份；丢失后已有 Provider Credential 无法恢复。

仓库中的 `compose.yaml` 会使用 `longjianghu/zentrola:latest` 启动 Backend、Admin Web、PostgreSQL 和 Redis。使用 Docker Compose 2.23.1 或更高版本，直接修改文件顶部 `x-required-settings` 中的四个密码和密钥，然后运行：

```shell
docker compose up -d
```

Admin Web 地址为 `http://127.0.0.1:9528`，Backend 地址为 `http://127.0.0.1:9527`，PostgreSQL 默认发布到宿主机端口 `15432`。

### 2. 启动 Backend 和 Admin Web

macOS/Linux：

```bash
chmod +x zentrola zentrola-web
./zentrola start
./zentrola-web
```

Windows PowerShell：

```powershell
.\zentrola.exe start
.\zentrola-web.exe
```

打开 `http://127.0.0.1:9528`。新安装会要求创建首位管理员；Zentrola 不提供默认用户名或密码。

发布目录、完整配置、前台运行和远程部署说明见[快速开始](docs/getting-started.zh-CN.md)。

## 完成第一次模型调用

登录 Admin Web 后：

1. 初始化或创建服务商。
2. 添加 Provider Credential，测试连接并同步或配置模型映射。
3. 启用准备开放给客户端的逻辑模型。
4. 创建成员和 Group。
5. 将成员加入 Group，并向 Group 授权逻辑模型。
6. 为成员签发 Virtual Key。
7. 将 Gateway 地址、Virtual Key 和逻辑模型编码交给成员。

如需为系统间调用提供独立身份，可以改为创建应用，将应用加入一个或多个 Group 后签发 App Key。用户与应用都会继承所属 Group 的模型权限。

验证 OpenAI 兼容模型列表：

```bash
curl http://127.0.0.1:9527/v1/models \
  -H "Authorization: Bearer <virtual-key>"
```

发送 OpenAI 兼容对话请求：

```bash
curl http://127.0.0.1:9527/v1/chat/completions \
  -H "Authorization: Bearer <virtual-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"<logical-model-code>","messages":[{"role":"user","content":"你好"}]}'
```

Anthropic 兼容客户端使用 `http://127.0.0.1:9527/anthropic` 作为 Base URL。支持的路径、认证 Header、Codex、Claude Code 和通用 SDK 示例见[客户端接入](docs/client-configuration.zh-CN.md)。

## 文档

- [快速开始](docs/getting-started.zh-CN.md)
- [客户端接入](docs/client-configuration.zh-CN.md)
- [部署与运维](docs/operations.zh-CN.md)
- [环境变量参考](.env.example)
- [参与贡献](CONTRIBUTING.zh-CN.md)
- [社区行为准则](CODE_OF_CONDUCT.md)
- [安全策略](SECURITY.zh-CN.md)
- [获取支持](SUPPORT.md)
- [变更记录](CHANGELOG.md)
- [发布流程](RELEASING.md)
- [第三方软件声明](THIRD_PARTY_NOTICES.md)
- [English documentation index](docs/README.md)

在 `dev` 和 `test` 环境中，Backend 还会在 `/swagger/index.html` 提供 Swagger UI；`prod` 环境不会注册 Swagger 路由。

## 项目状态与边界

当前版本支持以用户、应用和 Group 为单位的模型治理、基础用量成本核算和月度 Token 管控。成本数据用于企业内部掌握和分摊支出，不定位为客户计费、开票或财务总账系统。以下能力仍属于后续工作：

- 按成本和延迟动态路由，以及更完整的资源健康自动化
- Enterprise Knowledge、Skill Registry 和 Managed MCP
- SSO、OIDC、LDAP 和 SCIM
- 完整的高可用部署方案

在用于正式生产环境前，请先确认当前能力和限制符合组织要求。

## 参与贡献

欢迎提交 Issue 和 Pull Request。修改 Go、Vue Admin Web、数据库迁移或生成代码前，请先阅读[参与贡献](CONTRIBUTING.zh-CN.md)。

## 安全

不要在 Issue、截图或日志中公开 Provider Credential、Virtual Key、管理员会话、prompt 或模型输出。漏洞报告方式见[安全策略](SECURITY.zh-CN.md)。

## License

Copyright 2026 longjianghu。本项目使用 [Apache License 2.0](LICENSE) 开源许可证，归属信息见 [NOTICE](NOTICE) 和[第三方软件声明](THIRD_PARTY_NOTICES.md)。
