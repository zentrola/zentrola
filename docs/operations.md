# Deployment and operations

[English](operations.md) | [简体中文](operations.zh-CN.md)

Review [`.env.example`](../.env.example) before deployment. Keep the Backend, Admin Web, database, Redis, and reverse proxy on networks appropriate to their trust boundaries.

## Process management

The Backend supports:

```text
zentrola start                         Start in the background
zentrola restart                       Restart the managed process
zentrola stop                          Stop the managed process
zentrola status                        Show status
zentrola serve                         Run in the foreground
zentrola migrate                       Apply database migrations
zentrola healthcheck                   Check /health/live
zentrola password --username <name>    Reset an administrator password
```

Use `.\zentrola.exe` instead of `zentrola` on Windows. Background state and startup diagnostics are stored under `run/` next to the executable.

Admin Web runs in the foreground. Place it under the operating system's service manager or a container supervisor for unattended operation.

## Health checks

```shell
curl -fsS http://127.0.0.1:9527/health/live
curl -i http://127.0.0.1:9527/health/ready
```

- `/health/live` confirms that the Backend can answer HTTP requests.
- `/health/ready` checks PostgreSQL, the master key, and administrator initialization.
- Readiness does not prove that a provider credential or upstream model is available.
- Redis is intentionally not part of readiness; gateway caches and cooldown state fail open.

## Database migrations and upgrades

`MIGRATIONS_AUTO_APPLY=true` applies forward migrations when the Backend starts. For controlled production changes, disable automatic migration and run `zentrola migrate` before starting the new version.

Before every upgrade:

1. Read the release notes.
2. Back up PostgreSQL and the active master key as one recovery set.
3. Stop or drain old Backend instances.
4. Apply forward migrations.
5. Replace Backend and Admin Web together when a release changes their shared API.
6. Verify live, ready, administrator login, provider status, and one authorized gateway call.

Do not run destructive or down migrations against production data unless the release explicitly requires and documents them.

## Backup and recovery

The default master-key path is:

```text
data/secrets/master.key
```

Provider credentials in PostgreSQL cannot be decrypted without the original master key. A valid recovery set therefore includes:

- PostgreSQL data
- the exact master key used to encrypt provider credentials
- the deployment configuration, stored through an approved secret-management process

Test restoration in an isolated environment. Protect backups with access controls and encryption. If only PostgreSQL is restored without the key, affected provider credentials must be entered again.

The repository's Compose example keeps PostgreSQL data when stopped:

```shell
docker compose stop postgres
```

`docker compose down -v` permanently removes the named data volume and must not be used as a routine stop command.

## Logging

Important settings:

```dotenv
LOG_LEVEL=info
LOG_CONSOLE_FORMAT=json
LOG_COLOR=never
LOG_FILE_PATH=
LOG_FILE_MAX_SIZE_MB=100
LOG_FILE_MAX_BACKUPS=10
```

- Containers and Kubernetes: write JSON to stdout and let the runtime or logging agent rotate and ship it.
- A single host without a logging agent: set `LOG_FILE_PATH` to enable JSON Lines with application-managed rotation.
- Do not let multiple processes write the same log file.
- `APP_ENV=dev` may log unredacted JSON request and response summaries. They can contain credentials, prompts, messages, and model output. Never enable this behavior in production or share those logs.

Sensitive authentication headers are excluded from access-log summaries. Response `X-Trace-ID` and `X-Span-ID` values can be used to correlate requests.

## Security baseline

- Expose Admin Web and Gateway through HTTPS.
- Restrict CORS to actual Admin Web origins.
- Keep PostgreSQL and Redis off public networks.
- Store `.env`, the master key, provider credentials, administrator passwords, and Virtual Keys as secrets.
- Revoke Virtual Keys when members leave or access changes.
- Back up before credential, schema, or deployment changes.
- Follow [SECURITY.md](../SECURITY.md) when a vulnerability is suspected.
