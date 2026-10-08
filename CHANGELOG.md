# Changelog / 变更记录

All notable changes to Zentrola are documented here. Dates use UTC. The project follows semantic versioning after `1.0.0`.

Zentrola 的重要变化记录在此。日期使用 UTC；从 `1.0.0` 起遵循语义化版本。

## [1.1.0] - 2026-10-08

### Added / 新增

- Added application principals, App Key lifecycle management, Group membership, usage attribution, and administrative audit records for service-to-service model access.
- Added effective-dated credential pricing: per-million-token input, cached-input, and output prices for API keys, plus recurring period fees for personal subscriptions.
- Added automated personal-subscription allocation and API-key usage rating, with correction records when effective prices or subscription fees change.
- Added configurable background workers for billing-period settlement and asynchronous API-key usage rating, coordinated across Backend instances through Redis locks.
- Added a cost overview for the current month, cost allocation by user and application, token-usage rankings, and billing details with multidimensional filters, CSV export, and per-call cost breakdowns.
- Added independent monthly Token quotas for users, applications, and Groups. The Admin Web shows used and remaining Tokens; quota progress changes from blue to amber at 50% and to red at 80%, while requests are rejected when any applicable quota reaches 100%.
- Added audited quota adjustments, including adding quota, removing a quota limit without deleting usage history, common amount presets, and optional adjustment reasons.
- 新增应用主体、App Key 生命周期、Group 归属、用量归属和管理操作审计，支持应用以独立身份调用模型。
- 新增带生效时间的凭证价格历史：API Key 支持输入、缓存输入和输出的每百万 Token 单价，个人订阅支持周期费用。
- 新增个人订阅费用自动分摊和 API Key 用量核算；价格或订阅费用变更后可生成差额调整记录。
- 新增可配置的账期结算和 API Key 异步核算后台任务，通过 Redis 锁协调多个 Backend 实例。
- 新增默认展示当月的成本概览、按用户和应用划分的费用分摊、Token 用量排行，以及支持多维筛选、CSV 导出和单次调用成本拆解的计费明细。
- 新增用户、应用和 Group 相互独立的月度 Token 配额。管理端展示已用量和剩余量，进度条在 50% 时由蓝色变为橙色、80% 时变为红色；任一适用配额达到 100% 后拒绝请求。
- 新增带审计记录的配额调整，支持增加额度、保留历史用量并取消额度限制、常用额度快捷值和选填调整原因。

### Changed / 变更

- Updated usage and cost analytics to include applications alongside users, models, providers, credentials, and Groups.
- Unified row actions in user, application, Group, model, and provider tables under a vertical-more menu, with consistent fixed-column hover behavior and responsive column widths.
- Changed Docker Compose images from the pinned `1.0.1` tag to `latest`.
- 用量与成本分析增加应用维度，并继续支持用户、模型、服务商、凭证和 Group 等维度。
- 用户、应用、Group、模型和服务商列表统一使用竖向更多操作菜单，并统一固定列悬停样式和响应式列宽。
- Docker Compose 镜像由固定的 `1.0.1` 标签调整为 `latest`。

### Fixed / 修复

- Prevented row-action popovers from expanding table overflow, limited the interface to one open row menu at a time, and removed unnecessary horizontal scrolling from user and application lists at common desktop widths.
- Fixed current-month subscription costs and estimated allocation so configured active subscription fees are visible before final period settlement.
- 修复行操作浮层撑开表格滚动区域的问题，同一时间仅允许展开一个行菜单，并消除用户和应用列表在常见桌面宽度下不必要的横向滚动。
- 修复当月订阅费用及预计分摊展示，使已配置且生效的订阅费用在账期最终结算前也能进入成本概览。

### Database / 数据库

- Added migrations `00044` through `00050` for application principals, credential price histories, billing documents and API-key usage ratings, monthly Token quotas, and removable quota limits.
- 新增 `00044` 至 `00050` 迁移，用于应用主体、凭证价格历史、成本单据与 API Key 用量核算、月度 Token 配额及可取消的配额限制。

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

[1.1.0]: https://github.com/zentrola/zentrola/releases/tag/v1.1.0
[1.0.1]: https://github.com/zentrola/zentrola/releases/tag/v1.0.1
[1.0.0]: https://github.com/zentrola/zentrola/releases/tag/v1.0.0
