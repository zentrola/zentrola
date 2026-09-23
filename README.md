# Zentrola

[English](README.md) | [简体中文](README.zh-CN.md)

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Zentrola is an enterprise AI coding control plane for governing and delivering models to development teams.

It sits between AI coding clients such as Codex and Claude Code and an organization's model subscriptions. Teams keep using their preferred clients, while administrators centrally manage providers, credentials, logical models, access policies, virtual keys, usage, and audit records.

> Zentrola is under active development. The current release focuses on the Model Governance MVP. Tool, knowledge, skill, cost, billing, SSO, and application-principal governance are not complete yet.

![Zentrola Admin Console](docs/assets/admin-console.png)

## Why Zentrola

- Keep existing AI coding clients and workflows.
- Protect real provider credentials behind member-specific Virtual Keys.
- Grant logical models through members and Groups instead of sharing upstream API keys.
- Change providers, upstream models, or credentials without changing the client-facing model code.
- Route OpenAI- and Anthropic-compatible traffic with protocol conversion and provider failover.
- Attribute usage to members, logical models, providers, and credentials.
- Audit important administrative operations.

## How it works

```text
Codex / Claude Code / OpenAI-compatible clients
                         │
                   Virtual Key
                         ▼
┌──────────────────── Zentrola ────────────────────┐
│ Authentication → Group policy → Logical model   │
│                → Provider route → Usage record   │
└──────────────────────────────────────────────────┘
                         │
          OpenAI / Anthropic / compatible providers
                         │
                  PostgreSQL / Redis
```

The management plane and gateway plane use separate authentication domains:

- Administrators use the Admin Web and Admin API with an Admin JWT.
- Members use a Virtual Key with the gateway APIs.
- An Admin JWT cannot call the gateway. A Virtual Key cannot call administrator management endpoints, but can call the member self-service endpoints under `/api/v1/me`.

## Current capabilities

- First-administrator setup, sign-in, sign-out, password changes, and administrator password reset
- Member lifecycle management
- Group membership and per-Group logical-model allowlists
- Provider initialization and editable OpenAI/Anthropic-compatible endpoints
- Encrypted API-key credentials and supported ChatGPT/Codex and Claude Code personal-subscription authentication
- Provider proxy settings and public/private endpoint network scopes
- Provider credential connection tests and manual model-catalog synchronization
- Logical models and prioritized provider-model mappings
- Member Virtual Key issuance, expiration, and revocation
- Anthropic Messages, Count Tokens, SSE, and tool-use traffic
- OpenAI model listing, Chat Completions, Responses, Images API, SSE, and tool calls
- Same-protocol routing, cross-protocol conversion, and provider failover
- Usage dashboards and operation logs
- Chinese and English Admin Web localization

Model-catalog synchronization currently has dedicated adapters for OpenAI, Google, DeepSeek, Zhipu AI, Moonshot AI, Qwen, and xAI. Other OpenAI- or Anthropic-compatible providers can be configured manually.

## Quick start

### Requirements

- A supported Windows, macOS, or Linux host
- PostgreSQL 17
- Redis is recommended for gateway caches and route cooldown state; gateway routing falls back to PostgreSQL when Redis is temporarily unavailable

Use a published bundle for your operating system and architecture from [GitHub Releases](https://github.com/zentrola/zentrola/releases) when one is available. A bundle contains the Backend, Admin Web, static assets, and `.env.example`.

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

Keep `ADMIN_JWT_SECRET`, `MASTER_KEY`, the database password, and provider credentials separate. `MASTER_KEY` is read only from `.env` and must be backed up with the database; existing provider credentials cannot be recovered if it is lost.

The repository's `compose.yaml` is an optional PostgreSQL-only example:

```shell
docker compose up -d postgres
```

It publishes PostgreSQL on port `15432` by default. Set `POSTGRES_PORT=15432` when using that example.

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
- [Security policy](SECURITY.md)
- [简体中文文档索引](docs/README.zh-CN.md)

In `dev` and `test` environments, the Backend also exposes Swagger UI at `/swagger/index.html`. Swagger routes are disabled in `prod`.

## Project status and scope

The current MVP is suitable for validating member- and Group-based model governance. The following areas remain future work:

- Application principals and App Keys
- Cost, budgets, billing, and dynamic cost/latency routing
- Enterprise knowledge, a Skill registry, and managed MCP
- SSO, OIDC, LDAP, and SCIM
- High-availability deployment guidance

Please review the current capabilities and limitations before production adoption.

## Contributing

Issues and pull requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing Go code, the Vue Admin Web, database migrations, or generated code.

## Security

Do not publish credentials, Virtual Keys, administrator sessions, prompts, or model output in issues, screenshots, or logs. See [SECURITY.md](SECURITY.md) for reporting guidance.

## License

Licensed under the [Apache License 2.0](LICENSE).
