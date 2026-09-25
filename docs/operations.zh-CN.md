# 部署与运维

[English](operations.md) | [简体中文](operations.zh-CN.md)

部署前完整检查 [`.env.example`](../.env.example)。Backend、Admin Web、数据库、Redis 和反向代理应按照各自的信任边界部署在合适的网络中。

## 进程管理

Backend 支持：

```text
zentrola start                         后台启动
zentrola restart                       重启受管进程
zentrola stop                          停止受管进程
zentrola status                        查看状态
zentrola serve                         前台运行
zentrola migrate                       执行数据库迁移
zentrola healthcheck                   检查 /health/live
zentrola password --username <name>    重置管理员密码
```

Windows 使用 `.\zentrola.exe`。后台运行状态和启动诊断保存在可执行文件旁的 `run/` 目录。

Admin Web 在前台运行。无人值守时，应交给操作系统服务管理器或容器进程管理器托管。

## 健康检查

```shell
curl -fsS http://127.0.0.1:9527/health/live
curl -i http://127.0.0.1:9527/health/ready
```

- `/health/live` 表示 Backend 能够响应 HTTP 请求。
- `/health/ready` 检查 PostgreSQL、Master Key 和管理员初始化状态。
- Readiness 通过不代表 Provider Credential 或上游模型可用。
- Redis 有意不纳入 readiness；缓存和路由冷却状态不可用时 Gateway 会 fail-open。

## 数据库迁移与升级

`MIGRATIONS_AUTO_APPLY=true` 会在 Backend 启动时执行前向迁移。生产环境如果需要受控变更，可以关闭自动迁移，并在启动新版本前执行 `zentrola migrate`。

从使用 `data/secrets/master.key` 的旧版本升级时，必须在首次启动新版前，将该文件的原内容完整写入所选通用配置文件的 `MASTER_KEY`。不要生成新值；确认新版可以读取已有 Provider Credential 后，再按秘密管理流程归档旧文件。

每次升级前：

1. 阅读 Release Notes。
2. 将 PostgreSQL 和当前 Master Key 作为同一恢复集备份。
3. 停止旧 Backend 实例或完成流量排空。
4. 执行前向迁移。
5. 如果版本修改了共享 API，应同时替换 Backend 和 Admin Web。
6. 验证 live、ready、管理员登录、服务商状态和一次已授权 Gateway 调用。

除非 Release Notes 明确要求并提供步骤，不要对生产数据执行破坏性迁移或 Down Migration。

## 备份与恢复

Master Key 仅保存在所选通用配置文件中，默认为：

```text
.env 中的 MASTER_KEY
```

PostgreSQL 中的 Provider Credential 必须使用原 Master Key 才能解密，因此有效恢复集包括：

- PostgreSQL 数据
- 加密 Provider Credential 时使用的原 Master Key
- 包含 `MASTER_KEY` 的原始部署配置

应在隔离环境定期验证恢复流程，并通过权限控制和加密保护备份。只有数据库而没有原 Master Key 时，相关 Provider Credential 必须重新录入。

仓库 Compose 服务栈停止时不会删除命名数据卷：

```shell
docker compose stop
```

`docker compose down -v` 会永久删除命名数据卷，不能作为日常停止命令使用。

## 日志

主要配置：

```dotenv
LOG_LEVEL=info
LOG_FILE_PATH=./runtime/logs
LOG_FILE_MAX_SIZE_MB=100
LOG_FILE_MAX_BACKUPS=10
LOG_FILE_RETENTION_DAYS=7
```

- 控制台固定使用自动着色的 `pretty` 格式；支持 ANSI 的 IDE Run Console（包括 GoLand）也会着色。设置标准环境变量 `NO_COLOR` 可关闭颜色。
- Docker 和 Kubernetes：将 stdout 交由容器运行时或日志 Agent 轮转并采集。
- 没有日志 Agent 的单机：设置 `LOG_FILE_PATH`，启用应用管理的轮转日志。发布包的 `.env.example` 默认设置为 `./runtime/logs`；留空可关闭文件日志。文件使用无颜色的 `pretty` 格式，每条记录换行输出。
- `LOG_FILE_PATH` 是日志根目录。日志按 UTC 日期建目录；例如 `./runtime/logs` 会写入 `./runtime/logs/2026-09-17/app-1.log` 和 `error-1.log`。
- `app-*.log` 包含达到 `LOG_LEVEL` 的完整日志；`error-*.log` 是其中 `ERROR` 及以上记录的附加副本。两类文件分别按 `LOG_FILE_MAX_SIZE_MB` 轮转，并分别保留当前文件之外最多 `LOG_FILE_MAX_BACKUPS` 个历史分卷。
- `LOG_FILE_RETENTION_DAYS` 默认是 `7`，表示保留最近 7 个 UTC 自然日（含当天）；设为 `0` 可关闭按天清理。清理在启动和 UTC 跨日轮转时执行，与分卷数量限制同时生效。
- 日志清理是尽力而为的后台维护：目录不存在视为无需清理；删除失败只向 stderr 报告一次，不阻止服务启动或后续日志写入。
- 升级前已有的 `log-日期-序号.log` 不会自动迁移或删除。
- 使用 `--config` 时，相对目录以配置文件所在目录为基准；否则以进程工作目录为基准。
- 多个进程不能写入同一个日志目录。
- `APP_ENV=dev` 和 `APP_ENV=test` 会记录未经脱敏、未经截断的完整请求和响应正文，其中可能包含凭据、prompt、消息和模型输出。不得对外发送这些日志；`APP_ENV=prod` 不记录请求和响应正文。

访问日志摘要会排除敏感认证 Header。可以使用响应中的 `X-Trace-ID` 和 `X-Span-ID` 关联请求。

## 安全基线

- 通过 HTTPS 暴露 Admin Web 和 Gateway。
- CORS 只允许实际使用的 Admin Web Origin。
- 不要将 PostgreSQL 和 Redis 暴露到公网。
- 将 `.env`、Master Key、Provider Credential、管理员密码和 Virtual Key 作为秘密保存。
- 成员离职或权限变化后及时撤销 Virtual Key。
- 修改凭据、数据库结构或部署前先备份。
- 怀疑存在漏洞时按照 [SECURITY.zh-CN.md](../SECURITY.zh-CN.md) 处理。
