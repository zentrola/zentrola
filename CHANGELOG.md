# Changelog / 变更记录

All notable changes to Zentrola are documented here. Dates use UTC. The project follows semantic versioning after `1.0.0`.

Zentrola 的重要变化记录在此。日期使用 UTC；从 `1.0.0` 起遵循语义化版本。

## [Unreleased]

### Changed / 变更

- Nothing yet / 暂无。

## [1.0.1] - 2026-10-04

### Changed / 变更

- Refined backend module boundaries, input validation, error diagnostics, and release quality checks.
- Improved proxy failure diagnostics while keeping credentials and other secrets out of logs.
- 优化后台模块边界、参数校验、错误诊断和发布质量检查。
- 改进代理故障诊断，同时避免在日志中输出凭据等敏感信息。

### Fixed / 修复

- Prevented concurrent resource-state writes and subscription credential refreshes from overwriting newer changes.
- Fixed the missing connection-test protocol label and a `ResizeObserver` error during rapid page navigation.
- 修复并发资源状态写回及订阅凭据刷新覆盖较新修改的问题。
- 修复连接测试结果中的协议文案缺失，以及快速切换页面时的 `ResizeObserver` 异常。

### Database / 数据库

- Added migration `00043_provider_credential_version.sql` for credential version checks.
- 新增 `00043_provider_credential_version.sql`，用于凭据版本校验。

## [1.0.0] - 2026-10-01

### Added / 新增

- Model-governance MVP with administrator, member, Group, provider, logical-model, credential, Virtual Key, usage, and operation-log management.
- OpenAI- and Anthropic-compatible gateways with protocol conversion, streaming, tool calls, and provider failover.
- Chinese and English Admin Web, standalone release bundles, and Docker Compose deployment.
- 模型治理 MVP，包括管理员、成员、分组、服务商、逻辑模型、凭据、Virtual Key、用量和操作日志管理。
- 支持协议转换、流式响应、工具调用和服务商故障切换的 OpenAI/Anthropic 兼容网关。
- 中英文管理界面、独立发布包和 Docker Compose 部署方式。

[Unreleased]: https://github.com/zentrola/zentrola/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/zentrola/zentrola/releases/tag/v1.0.1
[1.0.0]: https://github.com/zentrola/zentrola/releases/tag/v1.0.0
