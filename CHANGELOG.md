# Changelog / 变更记录

All notable changes to Zentrola are documented here. Dates use UTC. The project follows semantic versioning after `1.0.0`.

Zentrola 的重要变化记录在此。日期使用 UTC；从 `1.0.0` 起遵循语义化版本。

## [1.2.1] - 2026-10-19

### Added / 新增

- Anthropic official model catalog sync: model synchronization reads `/v1/models` with an API Key, or with a Claude Code subscription token when no supported API Key is configured. Deprecated models are skipped, and image input support is recorded from the catalog.
- OpenAI model catalog sync with Codex subscriptions: when no API Key is available, model synchronization reads the ChatGPT Codex catalog with a Codex subscription credential. Hidden models are skipped, image input support is recorded, and model names follow the same rules as API Key sync. Expired access tokens are not refreshed during sync and return `SUBSCRIPTION_CREDENTIAL_EXPIRED`.
- Manual quota refresh for Claude subscriptions: the credential list can send one minimal request (1 output token, preferring a Haiku model mapping) and read only the unified rate limits from the response headers. It uses a tiny amount of subscription quota, may start a new 5-hour window, and does not change the credential runtime status. Refreshes are audited as `RESOURCE_QUOTA_REFRESH`.
- `CLAUDE_CODE_VERSION` sets the client version declared by all Claude subscription requests (`User-Agent: claude-cli/<version> (external, cli)`); it defaults to a built-in version when empty, and must use the `x.y.z` format. When the upstream rejects a Claude subscription request, the backend logs the upstream `error.type` and truncated `error.message`.
- Anthropic 官方模型目录同步：优先使用 API Key 读取 `/v1/models`，没有可用 API Key 时使用 Claude Code 订阅 Token；已弃用的模型不会同步，并按目录记录是否支持图片输入。
- OpenAI 模型目录支持 Codex 订阅同步：没有可用 API Key 时，使用 Codex 订阅凭据读取 ChatGPT Codex 模型目录；跳过隐藏模型，记录是否支持图片输入，模型名称与 API Key 同步规则一致。同步时不刷新已过期的访问令牌，并返回 `SUBSCRIPTION_CREDENTIAL_EXPIRED`。
- Claude 订阅手动刷新额度：凭据列表可发送一次最小请求（输出 1 个 Token，优先使用 Haiku 模型映射），只从响应头读取统一限额。会消耗极少量订阅额度，可能开启新的 5 小时窗口，不改变凭据运行状态；刷新记录为审计事件 `RESOURCE_QUOTA_REFRESH`。
- 新增 `CLAUDE_CODE_VERSION`，统一设置所有 Claude 订阅请求声明的客户端版本（`User-Agent: claude-cli/<版本> (external, cli)`）；留空时使用内置默认版本，必须为 `x.y.z` 格式。Claude 订阅请求被上游拒绝时，后端日志会记录上游的 `error.type` 和截断后的 `error.message`。
- `GATEWAY_STREAM_IDLE_TIMEOUT` (default `120s`, `0` disables it) stops an upstream response, including SSE, that sends no data for longer than the configured time; the request is recorded as `UPSTREAM_TIMEOUT`.
- Operation log APIs (`/api/v1/operation-logs` and `/api/v1/applications/{id}/operations`) accept `keyword`, `type`, and `result` filters. The Admin Web searches operation logs on the server and adds operation type and result filters.
- 新增 `GATEWAY_STREAM_IDLE_TIMEOUT`（默认 `120s`，`0` 表示关闭）：上游响应（含 SSE）连续无数据超过该时长即中断，并记录为 `UPSTREAM_TIMEOUT`。
- 操作日志接口（`/api/v1/operation-logs` 与 `/api/v1/applications/{id}/operations`）支持 `keyword`、`type`、`result` 筛选；管理端改为服务端搜索操作日志，并新增操作类型与结果筛选。

### Changed / 变更

- Claude subscription connection tests now send one minimal request to the selected model instead of reading the usage endpoint. Quota status, reset time, and check time are updated from the unified 5-hour and 7-day rate limits returned in the response. Enabling a provider with a Claude subscription also requires choosing a model mapping.
- Claude subscription quota data is also synced from gateway responses, at most once every 5 minutes per credential unless the quota status or reset time changes. When the quota is exhausted, the gateway stops routing to that credential until the reset time.
- Provider-level model sync skips credentials whose runtime status is `BLOCKED`, so a failing API Key no longer prevents a healthy subscription credential from being used. If every credential is blocked, the previous order (API Key first, then priority) still applies so the real upstream error is returned.
- Claude subscription requests always use the version declared by `CLAUDE_CODE_VERSION` and no longer forward the client's own `User-Agent`.
- Claude 订阅连接测试改为向所选模型发送一次最小请求，不再读取额度接口；额度状态、重置时间和检查时间改为根据响应中的 5 小时、7 天统一限额更新。使用 Claude 订阅启用服务商时需要选择模型映射。
- 网关转发 Claude 订阅请求时也会同步额度：额度状态或重置时间不变时，每个凭据最多每 5 分钟写入一次；额度耗尽后网关在重置前不再路由到该凭据。
- 服务商级模型同步会跳过运行状态为 `BLOCKED` 的凭据，避免失效的 API Key 导致正常的订阅凭据始终用不上；全部凭据都被封禁时，仍按 API Key 优先、再按优先级的顺序选择，以返回上游的真实错误。
- Claude 订阅请求统一使用 `CLAUDE_CODE_VERSION` 声明的版本，不再透传客户端的 `User-Agent`。
- Anthropic usage is recorded with OpenAI semantics: input tokens now include cache writes and cache reads, and cached input tokens are cache reads. Monthly Token quotas therefore count cached prompts, and cache writes are rated at the input price.
- Streaming OpenAI Chat requests always ask the upstream for usage (`stream_options.include_usage`). If the client did not request it, the extra usage chunk is removed from the response; a stream that ends without usage is recorded as `FAILED` with `UPSTREAM_RESPONSE_INCOMPLETE`.
- Subscription prices whose effective time is earlier than the end of an already billed period can no longer be added, moved, or deleted; amount-only changes are still allowed and settled through adjustments. Model price changes rejected by billing history return `409 CONFLICT` instead of `503`.
- Model sync no longer overwrites the display name or publisher of an existing model.
- The current-month cost allocation prorates subscription fees by the overlap between the query window and the current billing period, instead of counting the full period fee.
- Codex app-server processes only inherit an allowlist of environment variables, and app-server errors no longer block a subscription permanently.
- Proxy connections are reused per proxy configuration, and per-request proxy logs are written at Debug level. Upstream credentials may be up to 16 KiB.
- The Admin Web shows a UTC marker next to date ranges, shows price effective dates in UTC, and uses the same provider health rules on the home page and the provider page.
- Anthropic 用量改按 OpenAI 口径记录：输入 Token 包含缓存写入与缓存读取，缓存输入 Token 为缓存读取。月度 Token 配额因此会计入缓存提示，缓存写入暂按输入单价核算。
- OpenAI Chat 流式请求统一向上游请求用量（`stream_options.include_usage`）；客户端未请求时会从响应中剥离额外的用量分片。流结束仍没有用量时记为 `FAILED`（`UPSTREAM_RESPONSE_INCOMPLETE`）。
- 生效时间早于已出账账期结束时间的订阅价格不能再新增、移动或删除；只改金额仍然允许，通过调整单结算。被计费历史拒绝的模型价格变更返回 `409 CONFLICT`，不再返回 `503`。
- 模型同步不再覆盖已有模型的显示名称和发布方。
- 当月预计费用分摊按查询窗口与当前账期的重叠时长折算订阅费用，不再计入整期费用。
- Codex app-server 子进程只继承白名单环境变量；app-server 返回的错误不再导致订阅被永久封禁。
- 代理连接按代理配置复用，逐请求的代理日志降为 Debug 级别；上游凭据长度上限放宽到 16 KiB。
- 管理端在日期范围旁标注 UTC，价格生效日期按 UTC 展示，首页与服务商页使用同一套服务商运行状态判定。

### Fixed / 修复

- Gateway requests routed to Claude Code subscription credentials no longer fail with HTTP 429: the gateway prepends the Claude Code system prompt when it is missing (keeping the client's system prompt and `cache_control`), merges the `claude-code-20250219` beta, and applies the same handling to `/v1/messages/count_tokens`. API Key routing is unchanged.
- Claude subscription connection tests no longer fail with HTTP 429: the probe now identifies itself as Claude Code and accepts responses that contain only a signed thinking block.
- Claude subscription OAuth tokens are sent only to `https://api.anthropic.com` by the gateway and connection tests, even when the provider endpoint points to another host.
- Enabling a provider with a Claude subscription rechecks that the tested model mapping is still valid, matching the API Key behavior.
- 网关路由到 Claude Code 订阅凭据的请求不再返回 HTTP 429：缺少 Claude Code 系统提示时，网关会将其插入到最前面（保留客户端原有系统提示和 `cache_control`），合并 `claude-code-20250219` beta，`/v1/messages/count_tokens` 也按相同方式处理。API Key 路由行为不变。
- Claude 订阅连接测试不再返回 HTTP 429：探测请求会声明 Claude Code 身份，并接受只包含带签名思考块的响应。
- 网关和连接测试只会把 Claude 订阅 OAuth Token 发往 `https://api.anthropic.com`，即使服务商端点指向其他主机。
- 使用 Claude 订阅启用服务商时，会与 API Key 一样复核被测试的模型映射仍然有效。
- Anthropic calls that used prompt caching were classified as `INCOMPLETE_TOKENS` and not billed; they are now rated normally. Cross-protocol conversions keep cached Token fields.
- Backfilling an earlier subscription price could create billing periods that overlapped issued charges; settlement and the database now reject overlapping periods.
- Usage rating and API Key settlement no longer rescan all history on every run, a failing record no longer blocks the rest of the run, and billing workers log sanitized error details.
- Responses streams whose final event exceeds 256 KiB, such as image generation results, no longer lose usage or get recorded as failed.
- A client disconnect no longer marks Redis as unavailable or invalidates gateway caches on every instance.
- Concurrent failover in the same session no longer discards a successful upstream response with `503`, and a busy subscription refresh lock no longer cools down a credential that is still valid.
- When converting OpenAI streams to Anthropic, an upstream stream that ends early is reported as an error instead of a normal completion. Non-standard upstream 4xx bodies keep their status code and no longer cool down every candidate credential.
- Codex app-server timeouts are classified as `UPSTREAM_TIMEOUT`, and the whole app-server process tree is stopped. Management probes and reset-credit redemption hold the subscription refresh lock and keep rotated credentials on version conflicts.
- In the Admin Web, lists no longer show stale results after filters change during loading, quota status no longer fails for more than 100 rows, deleted credentials no longer reappear after concurrent reloads, price dates no longer show one day off west of UTC, and CSV export works reliably for long ranges. Missing English error text and full-width punctuation on English pages were fixed.
- A client disconnect during a Codex subscription refresh no longer stops the app-server midway, so a rotated refresh token is no longer lost; refresh failures after the client has gone no longer cool down the credential.
- Redis dial timeouts are no longer mistaken for a client disconnect, so route state and gateway caches stop calling an unreachable Redis for a short time as intended.
- OpenAI Chat and Responses requests routed to an Anthropic endpoint now record cache write tokens, so cache writes are rated at the cache write price instead of the input price. Total input tokens still include cache writes and cache reads.
- 使用提示缓存的 Anthropic 调用此前被归为 `INCOMPLETE_TOKENS` 而不计费，现已正常核算；跨协议转换保留缓存 Token 字段。
- 补录更早生效的订阅价格可能生成与已出账单据重叠的账期；结算与数据库现在都会拒绝重叠账期。
- 用量核算和 API Key 结算不再每轮重扫全部历史，单条失败不再阻塞整轮，计费任务日志记录脱敏后的错误详情。
- Responses 流的终止事件超过 256 KiB（如图像生成结果）时，不再丢失用量或被记为失败。
- 客户端断开连接不再被当作 Redis 不可用，也不再导致所有实例的网关缓存失效。
- 同一会话并发故障转移时不再以 `503` 丢弃已成功的上游响应；订阅刷新锁被占用时不再冷却仍然有效的凭据。
- OpenAI 流转换为 Anthropic 时，上游提前结束会返回错误事件，不再伪装成正常完成；非标准格式的上游 4xx 保留原状态码，不再冷却所有候选凭据。
- Codex app-server 超时归类为 `UPSTREAM_TIMEOUT`，并终止整棵进程树；管理端订阅探测与重置卡兑换持有订阅刷新锁，版本冲突时保留轮换后的凭据。
- 管理端修复：加载中切换筛选后列表仍显示旧结果、超过 100 行时额度状态查询失败、并发刷新后已删除的凭据重新出现、西半球时区价格日期差一天、长时间范围 CSV 导出不稳定，以及英文界面缺失错误文案和混用全角标点。
- Codex 订阅刷新期间客户端断开连接不再中途终止 app-server，已轮换的 refresh token 不会因此丢失；客户端已断开后的刷新失败不再冷却该凭据。
- Redis 拨号超时不再被误判为客户端断开，路由状态与网关缓存在 Redis 不可达时会按预期短暂停止访问 Redis。
- 经 OpenAI Chat 或 Responses 协议路由到 Anthropic 端点的请求会记录缓存写入 Token，缓存写入按缓存写入单价核算，不再按输入单价计费；总输入 Token 仍包含缓存写入与缓存读取。

### Database / 数据库

- Added migration `00055` to allow the `RESOURCE_QUOTA_REFRESH` audit event. Rolling it back is refused once quota refresh audit records exist.
- 新增 `00055` 迁移，允许记录 `RESOURCE_QUOTA_REFRESH` 审计事件；已存在额度刷新审计记录时拒绝回滚。
- Added migration `00056` with indexes for usage rating scans, a price-consistency SQL function, and a trigger that rejects partially overlapping subscription charges. Existing overlapping charges are not changed.
- 新增 `00056` 迁移：增加用量核算扫描索引、价格一致性 SQL 函数，以及拒绝订阅账期部分重叠的触发器；不修改已存在的重叠单据。

### Upgrade notes / 升级说明

- No configuration changes are required: `CLAUDE_CODE_VERSION` and `GATEWAY_STREAM_IDLE_TIMEOUT` are optional. Upgrading needs migrations `00055` and `00056`, which are applied at startup when `MIGRATIONS_AUTO_APPLY=true`; otherwise run `zentrola migrate` before starting the new version. Migration `00056` builds indexes on `usage_record` and `usage_rating` and blocks writes to them while it runs, so upgrade large deployments during low traffic.
- Anthropic input Token counts and monthly quota usage increase after the upgrade because cached prompts are now counted. OpenAI-compatible upstreams that reject `stream_options` will fail streaming Chat requests.
- 无需修改现有配置，`CLAUDE_CODE_VERSION` 与 `GATEWAY_STREAM_IDLE_TIMEOUT` 均为可选项。升级时需执行迁移 `00055` 和 `00056`：`MIGRATIONS_AUTO_APPLY=true` 时 Backend 启动会自动执行，否则请在启动新版本前运行 `zentrola migrate`。`00056` 会在 `usage_record` 与 `usage_rating` 上建索引，执行期间阻塞这两张表的写入，数据量大的部署请在低峰期升级。
- 升级后 Anthropic 输入 Token 与月度配额用量会因计入缓存提示而增加；不支持 `stream_options` 的 OpenAI 兼容上游会导致 Chat 流式请求失败。

## [1.2.0] - 2026-10-13

### Added / 新增

- Provider priority and failover: each model uses providers in configured priority order, with lower numbers taking precedence. If the current provider is unavailable, requests try the next available provider.
- Session-level account distribution: within the selected provider, available subscription accounts take precedence. If none are available, requests use an API Key. New sessions are distributed across multiple subscription accounts, and later requests in the same session keep using the selected account. If that account is deleted or becomes unavailable, a request that is safe to retry can switch to another available account; the session binding updates after a successful response.
- 服务商优先级与故障切换：按模型配置服务商的使用顺序，优先级数字越小越先使用；当前服务商不可用时，自动尝试下一个可用服务商。
- 会话级账号分流：在选中的服务商内，优先使用可用订阅账号；没有可用订阅账号时使用 API Key 兜底。多个新会话会在订阅账号之间分流，同一会话继续使用已选账号。已选账号被删除或不可用时，允许安全重试的请求可切换到其他可用账号；新账号成功响应后才更新会话绑定。

## [1.1.0] - 2026-10-08

### Added / 新增

- Added application principals, App Key lifecycle management, Group membership, usage attribution, and administrative audit records for service-to-service model access.
- Added effective-dated credential pricing: per-million-token input, cached-input, and output prices for API keys, plus recurring period fees for personal subscriptions.
- Added automated personal-subscription allocation and API-key usage rating, with correction records when effective prices or subscription fees change.
- Added configurable background workers for billing-period settlement and asynchronous API-key usage rating, coordinated across Backend instances through Redis locks.
- Added a cost overview for the current month, cost allocation by user and application, token-usage rankings, and billing details with filters, CSV export, and per-call cost breakdowns.
- Added independent monthly Token quotas for users, applications, and Groups. The Admin Web shows used and remaining Tokens; quota progress changes from blue to amber at 50% and to red at 80%, while requests are rejected when any applicable quota reaches 100%.
- Added audited quota adjustments, including adding quota, removing a quota limit without deleting usage history, common amount presets, and optional adjustment reasons.
- Added a provider activation flow that lets administrators choose a credential and, where applicable, a protocol and model mapping, then enables the provider only after a successful connection test.
- Added `expiry=all|unexpired` filtering to user and application key APIs and list controls; the Admin Web shows valid keys by default.
- 新增应用主体、App Key 生命周期、Group 归属、用量归属和管理操作审计，支持应用以独立身份调用模型。
- 新增带生效时间的凭证价格历史：API Key 支持输入、缓存输入和输出的每百万 Token 单价，个人订阅支持周期费用。
- 新增个人订阅费用自动分摊和 API Key 用量核算；价格或订阅费用变更后可生成差额调整记录。
- 新增可配置的账期结算和 API Key 异步核算后台任务，通过 Redis 锁协调多个 Backend 实例。
- 新增默认展示当月的成本概览、按用户和应用划分的费用分摊、Token 用量排行，以及支持筛选、CSV 导出和单次调用成本拆解的计费明细。
- 新增用户、应用和 Group 相互独立的月度 Token 配额。管理端展示已用量和剩余量，进度条在 50% 时由蓝色变为橙色、80% 时变为红色；任一适用配额达到 100% 后拒绝请求。
- 新增带审计记录的配额调整，支持增加额度、保留历史用量并取消额度限制、常用额度快捷值和选填调整原因。
- 新增服务商启用前的连接选择与验证流程，可指定凭证及适用的协议、模型映射，测试成功后才启用服务商。
- 用户和应用密钥接口及列表新增 `expiry=all|unexpired` 筛选；管理端默认展示有效密钥。

### Changed / 变更

- Updated usage and cost analytics to include applications alongside users, models, providers, credentials, and Groups.
- Unified row actions in user, application, Group, model, and provider tables under a vertical-more menu, with consistent fixed-column hover behavior and responsive column widths.
- Changed Docker Compose images from the pinned `1.0.1` tag to `latest`.
- Simplified billing-detail filters to date range, caller, model, and billing type; added user and application name suggestions and moved search actions beside the filters.
- Distinguished personal-subscription allocation from per-token billing in billing details, removed redundant summary cards, and simplified the detail title and layout.
- Localized principal types, protocols, call statuses, billing types, and rating results in cost CSV exports according to the Admin Web language. Requests without a supported `Accept-Language` retain raw enum codes.
- Hid the dashboard's current-model-provider section when empty, while keeping an error and retry action visible if loading fails.
- Kept actions visible at the bottom of common dialogs.
- Removed the Dockerfile's dependency on a remote syntax image.
- 用量与成本分析增加应用维度，并继续支持用户、模型、服务商、凭证和 Group 等维度。
- 用户、应用、Group、模型和服务商列表统一使用竖向更多操作菜单，并统一固定列悬停样式和响应式列宽。
- Docker Compose 镜像由固定的 `1.0.1` 标签调整为 `latest`。
- 计费明细筛选精简为日期范围、调用方、模型和计费方式；用户与应用支持名称联想选择，查询操作移至筛选项旁。
- 计费明细区分个人订阅分摊与按 Token 调用计费，移除重复的汇总卡片，并精简详情标题和布局。
- 成本 CSV 根据管理端语言格式化主体类型、协议、调用状态、计费方式和计费处理结果；未指定受支持语言时保留原始枚举值。
- 仪表盘无当前模型服务商时隐藏区块；加载失败时仍显示错误和重试入口。
- 常用弹窗的操作按钮固定在底部。
- 移除 Dockerfile 对远程语法镜像的依赖。

### Fixed / 修复

- Prevented row-action popovers from expanding table overflow, limited the interface to one open row menu at a time, and removed unnecessary horizontal scrolling from user and application lists at common desktop widths.
- Fixed current-month subscription costs and estimated allocation so configured active subscription fees are visible before final period settlement.
- Aligned key-dialog content widths and action columns, displayed `-` for unavailable key actions, refreshed valid-key lists when a key expires, and omitted empty codes from usage rankings.
- Made key revocation set the expiry to the revocation time and aligned historical revoked-key expiry values through database migrations.
- 修复行操作浮层撑开表格滚动区域的问题，同一时间仅允许展开一个行菜单，并消除用户和应用列表在常见桌面宽度下不必要的横向滚动。
- 修复当月订阅费用及预计分摊展示，使已配置且生效的订阅费用在账期最终结算前也能进入成本概览。
- 修正密钥弹窗内容宽度和操作列对齐，无可用操作时显示 `-`；密钥在弹窗打开期间过期时刷新有效列表；用量排行不再显示空编码占位符。
- 撤销密钥时将到期时间设为撤销时间，并通过数据库迁移修正历史撤销记录。

### Database / 数据库

- Added migrations `00044` through `00051` for application principals, credential price histories, billing documents and API-key usage ratings, monthly Token quotas, removable quota limits, and transactional quota totals with call-time Group attribution.
- Added migrations `00052` and `00053` to align revoked-key expiry with its revocation time, including historical records with a pre-existing expiry.
- 新增 `00044` 至 `00051` 迁移，用于应用主体、凭证价格历史、成本单据与 API Key 用量核算、月度 Token 配额、取消配额限制，以及调用时 Group 归属和事务内累计用量。
- 新增 `00052` 和 `00053` 迁移，使撤销密钥的到期时间与撤销时间一致，包括原本设置过到期时间的历史记录。

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

[1.2.0]: https://github.com/zentrola/zentrola/releases/tag/v1.2.0
[1.1.0]: https://github.com/zentrola/zentrola/releases/tag/v1.1.0
[1.0.1]: https://github.com/zentrola/zentrola/releases/tag/v1.0.1
[1.0.0]: https://github.com/zentrola/zentrola/releases/tag/v1.0.0
