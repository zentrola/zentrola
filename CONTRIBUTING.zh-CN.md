# 参与 Zentrola 开发

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh-CN.md)

感谢你帮助改进 Zentrola。每次变更应聚焦一个完整目标，保持管理面与调用面的边界，并且不得包含真实凭据或用户数据。

## 开始之前

- 新建 Issue 前先搜索是否已有相同问题。
- 大型行为变更、数据库结构调整、协议变更或兼容性调整应先讨论再实现。
- 安全漏洞必须按照 [SECURITY.zh-CN.md](SECURITY.zh-CN.md) 私下报告，不要提交公开 Issue。
- 每个 Pull Request 只包含一项完整变更。

## 开发环境要求

- Go 1.26
- PostgreSQL 17
- Redis，用于缓存和故障切换状态测试
- Node.js 22.18 或更高版本，推荐 Node.js 24 LTS
- npm，严格使用仓库中的 lockfile
- 修改查询或迁移时使用 sqlc 1.30.0

## 仓库结构

```text
cmd/server/                              Backend 入口
cmd/web/                                 Admin Web 服务入口
internal/domain/                         核心领域类型和规则
internal/application/                    应用用例
internal/infrastructure/                 PostgreSQL 和 Provider 适配器
internal/transport/http/                 HTTP Handler 和路由
internal/infrastructure/postgres/
  migrations/                            数据库迁移
  queries/                               手写 SQL
  dbgen/                                 sqlc 生成代码
web/src/pages/                           Vue 路由页面
web/src/components/                      Vue 共享组件
web/tests/                               Playwright 测试
scripts/                                 运维和发布脚本
docs/                                    公开用户文档
dev-docs/                                 本地实现资料，由 Git 忽略
```

依赖应保持向内：transport 调用 application 服务，domain 不得依赖 infrastructure。不要手动修改 `dbgen`。

## 本地开发

将 `.env.dev.example` 或 `.env.example` 复制为 `.env`，配置本地 PostgreSQL、Redis 和 JWT。可以用 Compose 启动隔离的 PostgreSQL：

```shell
docker compose up -d postgres
```

启动 Backend：

```shell
go run ./cmd/server
```

在另一个终端启动 Admin Web：

```shell
cd web
npm ci
npm run dev
```

默认 Backend 地址为 `http://127.0.0.1:9527`，Admin Web 地址为 `http://127.0.0.1:9528`。

## 构建发布包

将 Backend、Admin Web、静态资源和配置模板构建到 `dist/<os>/<architecture>`：

```shell
go run ./cmd/release
```

发布工具会在隔离的临时目录中按照 lockfile 安装前端依赖。如果 `web/node_modules` 已完整安装，可以使用 `-skip-npm-install`。通过 `-target-os` 和 `-architecture` 选择其他目标，通过 `-component backend|web|all` 选择构建组件。

Windows 还可以使用 `scripts/Build-Release.ps1` 这个交互式 PowerShell 封装。

## 编码风格

### Go

- 修改后的 Go 文件必须执行 `gofmt`。
- 包名使用简短的小写单词，导出标识符使用 PascalCase。
- 测试与实现放在同一包，文件名使用 `*_test.go`。
- 验证逻辑和服务行为优先编写聚焦的表驱动测试。

### Vue 和 TypeScript

- 遵循 `.prettierrc.json`：不使用分号、使用单引号、每行最多 100 字符并保留尾随逗号。
- Vue 组件使用 PascalCase，TypeScript 函数使用 camelCase。
- 所有语言的空值占位符统一使用半角连字符 `-`，不能使用全角破折号 `—`。
- 不要在 `VITE_*` 环境变量中放置秘密。

### 数据库

- 只新增前向迁移，不要重写可能已经部署的历史迁移。
- 手写 SQL 放在 `internal/infrastructure/postgres/queries/`。
- 迁移或查询修改后，使用 sqlc 1.30.0 执行 `sqlc generate`。
- 检查生成结果，但不要手动修改生成代码。

## 验证

至少运行与变更相关的检查；提交评审前应尽量完成全部测试：

```shell
go test ./...
go build ./cmd/server ./cmd/web
cd web
npm run build
npm test
```

PostgreSQL 集成测试需要设置 `ZENTROLA_INTEGRATION=1`。默认 Playwright 测试使用模拟 API，不得访问真实 Provider。

## 文档

- 用户可见行为变化时，同时更新英文和简体中文版本。
- 根 README 保持精简，详细用户说明放入 `docs/`。
- 设计记录、阶段报告、真实服务商测试记录和实现清单保留在 `dev-docs`。
- 文档不得包含 Credential、JWT、Virtual Key、私有 prompt、模型输出或敏感截图。

## Commit 与 Pull Request

Commit 使用简洁、祈使语气的中文摘要，不添加类型前缀。Pull Request 应：

- 说明行为与配置变化；
- 列出验证命令及结果；
- 关联相关 Issue；
- 可见 UI 变更附截图；
- 明确标注迁移、生成文件、兼容性风险和必要的环境配置更新。

提交贡献即表示你有权提交相关内容。除非另有明确声明，有意提交并纳入 Zentrola 的贡献均按照 [Apache License 2.0](LICENSE) 提供，不附加其他条款或条件。
