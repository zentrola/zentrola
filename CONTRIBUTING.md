# Contributing to Zentrola

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh-CN.md)

Thank you for helping improve Zentrola. Keep each change focused, preserve the separation between the management and gateway planes, and never include real credentials or user data.

## Before you start

- Search existing issues before opening a new one.
- Discuss large behavior, schema, protocol, or compatibility changes before implementation.
- Report security vulnerabilities privately according to [SECURITY.md](SECURITY.md), not in a public issue.
- Keep one pull request focused on one complete change.

## Development requirements

- Go 1.26
- PostgreSQL 17
- Redis for cache and failover-state testing
- Node.js 22.18 or newer; Node.js 24 LTS is recommended
- npm using the committed lockfile
- sqlc 1.30.0 when queries or migrations change

## Repository layout

```text
cmd/server/                              Backend entry point
cmd/web/                                 Admin Web server entry point
internal/domain/                         Core domain types and rules
internal/application/                    Use cases
internal/infrastructure/                 PostgreSQL and provider adapters
internal/transport/http/                 HTTP handlers and routes
internal/infrastructure/postgres/
  migrations/                            Database migrations
  queries/                               Hand-written SQL queries
  dbgen/                                 sqlc-generated code
web/src/pages/                           Vue route pages
web/src/components/                      Shared Vue components
web/tests/                               Playwright tests
scripts/                                 Operational and release scripts
docs/                                    Published user documentation
development-docs/                        Local implementation notes, ignored by Git
```

Dependencies should point inward: transport calls application services, and domain code must not depend on infrastructure. Do not edit `dbgen` manually.

## Local development

Copy `.env.dev.example` or `.env.example` to `.env`, then provide local PostgreSQL, Redis, and JWT settings. The Compose file can start an isolated PostgreSQL instance:

```shell
docker compose up -d postgres
```

Run the Backend:

```shell
go run ./cmd/server
```

Run Admin Web in another terminal:

```shell
cd web
npm ci
npm run dev
```

The defaults are Backend `http://127.0.0.1:9527` and Admin Web `http://127.0.0.1:9528`.

## Build a release bundle

Build Backend, Admin Web, static assets, and the configuration template into `dist/<os>/<architecture>`:

```shell
go run ./cmd/release
```

The release tool installs frontend dependencies from the lockfile in an isolated temporary directory. If `web/node_modules` is already complete, use `-skip-npm-install`. Select another target with `-target-os` and `-architecture`, or one component with `-component backend|web|all`.

On Windows, `scripts/Build-Release.ps1` provides an interactive PowerShell wrapper around the same release tool.

## Code style

### Go

- Run `gofmt` on changed Go files.
- Use short lowercase package names and PascalCase exported identifiers.
- Keep tests next to implementations in `*_test.go` files.
- Prefer focused table-driven tests for validation and service behavior.

### Vue and TypeScript

- Follow `.prettierrc.json`: no semicolons, single quotes, 100-character lines, and trailing commas.
- Use PascalCase Vue component names and camelCase TypeScript functions.
- Use the ASCII hyphen `-` for empty-value placeholders in every locale; do not use the em dash `—`.
- Do not put secrets in `VITE_*` variables.

### Database

- Add forward migrations; do not rewrite migrations that may already be deployed.
- Update hand-written SQL under `internal/infrastructure/postgres/queries/`.
- Run `sqlc generate` with sqlc 1.30.0 after migration or query changes.
- Review generated changes but do not edit them manually.

## Verification

Run the checks relevant to the change and, before requesting review, aim to run the complete suite:

```shell
go test ./...
go build ./cmd/server ./cmd/web
cd web
npm run build
npm test
```

PostgreSQL integration tests require `ZENTROLA_INTEGRATION=1`. Default Playwright tests use mocked APIs and must not contact real providers.

## Documentation

- Update both English and Simplified Chinese versions when user-visible behavior changes.
- Keep the root README concise and move detailed user guidance to `docs/`.
- Keep design notes, phase reports, real-provider test notes, and implementation checklists in `development-docs/`.
- Never include credentials, JWTs, Virtual Keys, private prompts, model responses, or sensitive screenshots.

## Commits and pull requests

Use a concise imperative Chinese commit summary without a type prefix, following the repository's existing convention. A pull request should:

- explain behavior and configuration changes;
- list verification commands and results;
- link related issues;
- include screenshots for visible UI changes;
- call out migrations, generated files, compatibility risks, and required environment changes.

By contributing, you confirm that you have the right to submit the change. Unless explicitly stated otherwise, contributions intentionally submitted for inclusion in Zentrola are provided under the [Apache License 2.0](LICENSE), without additional terms or conditions.
