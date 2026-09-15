# 仓库指南

## 项目结构与模块组织

Zentrola 由 Go 后端和 Vue 管理界面组成。程序入口位于 `cmd/server` 和 `cmd/web`。后端按层组织在 `internal/` 下：`domain` 存放核心类型，`application` 存放用例，`infrastructure` 包含 PostgreSQL 与上游服务适配器，`transport/http` 负责 HTTP 处理与路由。数据库迁移、手写查询和 sqlc 生成代码分别位于 `internal/infrastructure/postgres/{migrations,queries,dbgen}`。Vue 3/TypeScript 源码位于 `web/src`，路由页面放在 `pages/`，共享组件放在 `components/`，Playwright 测试位于 `web/tests`。运维脚本统一放在 `scripts/`。

## 构建、测试与开发命令

- `go run ./cmd/server`：启动本地 API，默认监听 9527 端口。
- `go test ./...`：运行 Go 测试；PostgreSQL 集成测试需设置 `ZENTROLA_INTEGRATION=1`。
- `go build ./cmd/server ./cmd/web`：编译两个 Go 程序。
- `sqlc generate`：在迁移或查询变更后重新生成 `dbgen`；固定使用 sqlc 1.30.0。
- `cd web && npm ci`：按 lockfile 安装依赖；要求 Node 22.18+，推荐 Node 24 LTS。
- `cd web && npm run dev`：在 9528 端口启动 Vite，并将 `/api` 代理到后端。
- `cd web && npm run build`：执行类型检查并生成 `web/dist`。
- `cd web && npm test`：运行使用模拟 API 的 Playwright 测试。

## 编码风格与命名约定

Go 代码必须经过 `gofmt`；包名使用简短的小写单词，导出标识符使用 PascalCase，测试以 `*_test.go` 命名并与实现放在同一包中。保持依赖方向：transport 调用 application 服务，domain 不得依赖 infrastructure。不要手动修改 `dbgen`。前端遵循 `.prettierrc.json`：不使用分号、使用单引号、每行最多 100 字符并保留尾随逗号。Vue 组件使用 PascalCase（如 `MemberKeys.vue`），TypeScript 工具函数使用 camelCase。

### 前端本地化占位符

界面字段没有记录或没有可展示的值时，无论中文还是英文界面，都必须统一使用半角连字符 `-` 作为占位符。新增或修改页面、组件、国际化文案及相关测试时都必须遵循此规则，不得使用全角破折号 `—` 表示空值。

### 时间与时区

所有后端接口的日期时间参数与响应字段必须统一使用 UTC RFC 3339 格式，并以 `Z` 表示 UTC（例如 `2026-09-15T03:20:00Z`）；客户端不得传入本地时间或其他时区偏移。服务端生成业务时间必须使用 `time.Now().UTC()`，接收、计算、持久化及返回 `time.Time` 前必须确保其为 UTC；PostgreSQL 时间字段统一使用 `TIMESTAMPTZ`。禁止依据服务器本地时区计算日期边界或格式化接口响应，本地化展示只能在客户端展示层处理。仅用于耗时、超时和 Unix 时间戳计算的时间值不受格式约束；面向人工阅读的本地时间日志必须显式包含时区偏移，不能输出无时区信息的本地时间。

## 测试规范

验证逻辑和服务行为应编写聚焦的 Go 表驱动测试。Playwright 文件命名为 `*.spec.ts`，默认浏览器测试必须使用模拟 API。提交前运行 `go test ./...`、`npm run build` 和 `npm test`。不得将真实服务凭据写入测试数据、截图或日志。

## Commit 与 Pull Request 规范

近期 commit 使用简洁、祈使语气的中文摘要，不添加类型前缀，例如 `完善命令行进程管理`。每个 commit 只包含一项完整变更。Pull Request 应说明行为及配置变化、列出验证命令、关联 issue，并为可见的 UI 变更附截图。明确标注数据库迁移、生成文件、兼容性风险和必要的环境配置更新。

## 安全与配置

从合适的 `.env.*.example` 复制本地配置；禁止提交 `.env`、凭据、JWT 或测试产物。不要通过 `VITE_*` 暴露秘密。数据库破坏性命令及 `docker compose down -v` 属于不可逆操作，仅在意图明确时执行。
