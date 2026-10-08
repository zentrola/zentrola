# Zentrola

[English](README.md) | [简体中文](README.zh-CN.md)

[![CI](https://github.com/zentrola/zentrola/actions/workflows/ci.yml/badge.svg)](https://github.com/zentrola/zentrola/actions/workflows/ci.yml)
[![Security](https://github.com/zentrola/zentrola/actions/workflows/security.yml/badge.svg)](https://github.com/zentrola/zentrola/actions/workflows/security.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Zentrola is an enterprise AI coding control plane for governing and delivering models to development teams.

It sits between AI coding clients such as Codex and Claude Code and an organization's model subscriptions. Teams keep using their preferred clients, while administrators centrally manage providers, credentials, logical models, access policies, user and application keys, Token quotas, usage costs, and audit records.

> Zentrola is under active development. The `1.1.0` release extends the Model Governance MVP with application access, usage-cost accounting, and monthly Token quotas. Tool, knowledge, skill, SSO, and complete high-availability guidance remain future work.

![Zentrola Admin Console](docs/assets/admin-console.png)

## Why Zentrola

- Keep existing AI coding clients and workflows.
- Protect real provider credentials behind member-specific Virtual Keys.
- Grant logical models through members and Groups instead of sharing upstream API keys.
- Change providers, upstream models, or credentials without changing the client-facing model code.
- Route OpenAI- and Anthropic-compatible traffic with protocol conversion and provider failover.
- Attribute usage and costs to users, applications, logical models, providers, and credentials.
- Control monthly Token consumption with independent user, application, and Group quotas.
- Audit important administrative operations.

## How it works

```text
Codex / Claude Code / OpenAI-compatible clients / applications
                              │
                     Virtual Key / App Key
                              ▼
┌───────────────────────── Zentrola ─────────────────────────┐
│ Authentication → Group policy → Token quota → Logical model │
│                → Provider route → Usage and cost records     │
└──────────────────────────────────────────────────────────────┘
                              │
               OpenAI / Anthropic / compatible providers
                              │
                       PostgreSQL / Redis
```

The management plane and gateway plane use separate authentication domains:

- Administrators use the Admin Web and Admin API with an Admin JWT.
- Users use a Virtual Key and applications use an App Key with the gateway APIs.
- An Admin JWT cannot call the gateway. Gateway keys cannot call administrator management endpoints; eligible user Virtual Keys can call the self-service endpoints under `/api/v1/me`.

## Current capabilities

- First-administrator setup, sign-in, sign-out, password changes, and administrator password reset
- Member lifecycle management
- Application lifecycle and App Key issuance, expiration, and revocation
- User and application Group membership with per-Group logical-model allowlists
- Provider initialization and editable OpenAI/Anthropic-compatible endpoints
- Encrypted API-key credentials and supported ChatGPT/Codex and Claude Code personal-subscription authentication
- Provider proxy settings and public/private endpoint network scopes
- Provider credential connection tests and manual model-catalog synchronization
- Logical models and prioritized provider-model mappings
- User Virtual Key issuance, expiration, and revocation
- Anthropic Messages, Count Tokens, SSE, and tool-use traffic
- OpenAI model listing, Chat Completions, Responses, Images API, SSE, and tool calls
- Same-protocol routing, cross-protocol conversion, and provider failover
- Usage dashboards for users and applications, plus operation logs
- Effective-dated personal-subscription fees and API-key per-million-token prices
- Current-month cost overview, subscription allocation, usage rankings, and filterable billing details with CSV export and per-call cost breakdowns
- Independent monthly Token quotas for users, applications, and Groups, with usage levels, gateway enforcement, audited quota increases, and removable limits
- Chinese and English Admin Web localization

Model-catalog synchronization currently has dedicated adapters for OpenAI, Google, DeepSeek, Zhipu AI, Moonshot AI, Qwen, and xAI. Other OpenAI- or Anthropic-compatible providers can be configured manually.

## Quick start

### Requirements

- A supported Windows, macOS, or Linux host
- PostgreSQL 17
- Redis is recommended for gateway caches and route cooldown state; gateway routing falls back to PostgreSQL when Redis is temporarily unavailable

Use a published bundle for your operating system and architecture from [GitHub Releases](https://github.com/zentrola/zentrola/releases) when one is available. A bundle contains the Backend, Admin Web, static assets, `.env.example`, project license files, and third-party license notices.

If a release bundle is not available for the target platform, see [CONTRIBUTING.md](CONTRIBUTING.md) to build the same bundle from source.

### 1. Configure Zentrola

Rename `.env.example` to `.env`, then set at least:

```dotenv
APP_ENV=prod

POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_DB=zentrola
POSTGRES_USER=zentrola
POSTGRES_PASSWORD=replace-with-a-strong-password

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_DB=2
REDIS_PASSWORD=replace-with-a-redis-password

ADMIN_JWT_SECRET=replace-with-at-least-32-random-bytes-in-base64
MASTER_KEY=replace-with-another-32-random-bytes-in-base64

CORS_ENABLED=true
CORS_ALLOWED_ORIGINS=http://127.0.0.1:9528

WEB_API_BASE_URL=http://127.0.0.1:9527
WEB_GATEWAY_BASE_URL=http://127.0.0.1:9527
```

Cost processing uses the following defaults. These settings are optional; add them to the selected configuration file only when the deployment needs different scan intervals, timeouts, or batch sizes:

```dotenv
# Settle ended personal-subscription and API-key billing periods every hour.
# Wait 10 minutes after period end and limit each scan to 5 minutes.
BILLING_SETTLEMENT_INTERVAL=1h
BILLING_SETTLEMENT_GRACE=10m
BILLING_SETTLEMENT_TIMEOUT=5m

# Rate individual API-key calls every minute, for at most 50 seconds per scan
# and up to 500 pending usage records per page.
BILLING_API_KEY_RATING_INTERVAL=1m
BILLING_API_KEY_RATING_TIMEOUT=50s
BILLING_API_KEY_RATING_PAGE_SIZE=500
```

Both workers use Redis locks so that only one Backend instance performs each task at a time. See [Deployment and operations](docs/operations.md#cost-settlement-and-usage-rating) for value constraints, failure behavior, and tuning guidance.

Keep `ADMIN_JWT_SECRET`, `MASTER_KEY`, the database password, and provider credentials separate. `MASTER_KEY` is read only from the Backend's selected common configuration file. Binary deployments use `.env` by default; Compose mounts the value entered at the top of `compose.yaml` as an inline `/app/.env` config. Back it up with the database because existing provider credentials cannot be recovered if it is lost.

The repository's `compose.yaml` starts the `longjianghu/zentrola:latest` Backend and Admin Web together with PostgreSQL and Redis. With Docker Compose 2.23.1 or later, edit the four passwords and keys under `x-required-settings` at the top of the file, then run:

```shell
docker compose up -d
```

The Admin Web is available at `http://127.0.0.1:9528`, the Backend at `http://127.0.0.1:9527`, and PostgreSQL is published on port `15432` by default.

### 2. Start the Backend and Admin Web

macOS/Linux:

```bash
chmod +x zentrola zentrola-web
./zentrola start
./zentrola-web
```

Windows PowerShell:

```powershell
.\zentrola.exe start
.\zentrola-web.exe
```

Open `http://127.0.0.1:9528`. On a new installation, the page asks you to create the first administrator; Zentrola does not ship a default username or password.

See the [Getting Started guide](docs/getting-started.md) for release layout, configuration, foreground operation, and remote deployment.

## First model call

After signing in to the Admin Web:

1. Initialize or create a provider.
2. Add a provider credential, test it, and synchronize or configure model mappings.
3. Enable the logical model that clients should use.
4. Create a member and a Group.
5. Add the member to the Group and grant the logical model to the Group.
6. Issue a Virtual Key for the member.
7. Give the member the gateway URL, Virtual Key, and logical-model code.

For service-to-service access, create an application instead, add it to one or more Groups, and issue an App Key. User and application access both inherit model permissions from their Groups.

Verify the OpenAI-compatible model list:

```bash
curl http://127.0.0.1:9527/v1/models \
  -H "Authorization: Bearer <virtual-key>"
```

Send an OpenAI-compatible chat request:

```bash
curl http://127.0.0.1:9527/v1/chat/completions \
  -H "Authorization: Bearer <virtual-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"<logical-model-code>","messages":[{"role":"user","content":"Hello"}]}'
```

For Anthropic-compatible clients, use `http://127.0.0.1:9527/anthropic` as the base URL. See [Client configuration](docs/client-configuration.md) for supported paths, headers, Codex, Claude Code, and generic SDK examples.

## Documentation

- [Getting Started](docs/getting-started.md)
- [Client configuration](docs/client-configuration.md)
- [Deployment and operations](docs/operations.md)
- [Environment variable reference](.env.example)
- [Contributing](CONTRIBUTING.md)
- [Code of conduct](CODE_OF_CONDUCT.md)
- [Security policy](SECURITY.md)
- [Support](SUPPORT.md)
- [Changelog](CHANGELOG.md)
- [Release process](RELEASING.md)
- [Third-party notices](THIRD_PARTY_NOTICES.md)
- [简体中文文档索引](docs/README.zh-CN.md)

In `dev` and `test` environments, the Backend also exposes Swagger UI at `/swagger/index.html`. Swagger routes are disabled in `prod`.

## Project status and scope

The current release supports user-, application-, and Group-based model governance, basic usage-cost accounting, and monthly Token controls. Cost figures are intended for internal visibility and allocation, not customer invoicing or general-ledger accounting. The following areas remain future work:

- Dynamic cost/latency-aware routing and broader resource-health automation
- Enterprise knowledge, a Skill registry, and managed MCP
- SSO, OIDC, LDAP, and SCIM
- High-availability deployment guidance

Please review the current capabilities and limitations before production adoption.

## Contributing

Issues and pull requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing Go code, the Vue Admin Web, database migrations, or generated code.

## Security

Do not publish credentials, Virtual Keys, administrator sessions, prompts, or model output in issues, screenshots, or logs. See [SECURITY.md](SECURITY.md) for reporting guidance.

## License

Copyright 2026 longjianghu. Licensed under the [Apache License 2.0](LICENSE); see [NOTICE](NOTICE) and [Third-Party Notices](THIRD_PARTY_NOTICES.md) for attribution information.
