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

When upgrading from a version that used `data/secrets/master.key`, copy that file's exact content into `MASTER_KEY` in the selected common configuration file before the first new-version startup. Do not generate a replacement. Archive the old file through the approved secret-management process only after confirming that the new version can read existing provider credentials.

Before every upgrade:

1. Read the release notes.
2. Back up PostgreSQL and the active master key as one recovery set.
3. Stop or drain old Backend instances.
4. Apply forward migrations.
5. Replace Backend and Admin Web together when a release changes their shared API.
6. Verify live, ready, administrator login, provider status, and one authorized gateway call.

Do not run destructive or down migrations against production data unless the release explicitly requires and documents them.

## Backup and recovery

The master key is stored only in the selected common configuration. A binary deployment defaults to:

```text
MASTER_KEY in .env
```

The repository's Compose deployment uses the value under `master-key-file` at the top of `compose.yaml` and mounts it as `/app/.env` inside the container.

Provider credentials in PostgreSQL cannot be decrypted without the original master key. A valid recovery set therefore includes:

- PostgreSQL data
- the exact master key used to encrypt provider credentials
- the original deployment configuration containing `MASTER_KEY`

Test restoration in an isolated environment. Protect backups with access controls and encryption. If only PostgreSQL is restored without the key, affected provider credentials must be entered again.

The repository's Compose stack keeps its named data volumes when stopped:

```shell
docker compose stop
```

`docker compose down -v` permanently removes the named data volume and must not be used as a routine stop command.

## Logging

Important settings:

```dotenv
LOG_LEVEL=info
LOG_FILE_PATH=./runtime/logs
LOG_FILE_MAX_SIZE_MB=100
LOG_FILE_MAX_BACKUPS=10
LOG_FILE_RETENTION_DAYS=7
```

- Console logs always use the automatically colored `pretty` format, including ANSI-capable IDE run consoles such as GoLand. Set the standard `NO_COLOR` environment variable to disable colors.
- Containers and Kubernetes: let the container runtime or logging agent rotate and ship stdout.
- A single host without a logging agent: set `LOG_FILE_PATH` to enable application-managed log rotation. The release `.env.example` enables it with `./runtime/logs`; leave it empty to disable file logging. Files use uncolored `pretty` format and write one record per line.
- `LOG_FILE_PATH` is the log root directory. Logs use UTC date directories; for example, `./runtime/logs` writes `./runtime/logs/2026-09-17/app-1.log` and `error-1.log`.
- `app-*.log` contains the complete timeline at or above `LOG_LEVEL`; `error-*.log` is an additional copy of its `ERROR` and higher records. Each stream rotates independently at `LOG_FILE_MAX_SIZE_MB` and retains up to `LOG_FILE_MAX_BACKUPS` historical segments in addition to its active file.
- `LOG_FILE_RETENTION_DAYS` defaults to `7`, retaining the latest seven UTC calendar dates including today. Set it to `0` to disable age-based cleanup. Cleanup runs at startup and UTC date rollover, alongside the segment-count limit.
- Log cleanup is best effort: a missing directory needs no cleanup, and deletion failures are reported to stderr once without preventing service startup or subsequent log writes.
- Existing `log-date-index.log` files from older versions are not migrated or deleted automatically.
- With `--config`, a relative directory is resolved from the configuration file's directory; otherwise it is resolved from the process working directory.
- Do not let multiple processes write the same log directory.
- `APP_ENV=dev` and `APP_ENV=test` log complete request and response bodies without redaction or truncation. They can contain credentials, prompts, messages, and model output, so do not share those logs. `APP_ENV=prod` does not log request or response bodies.

Sensitive authentication headers are excluded from access-log summaries. Response `X-Trace-ID` and `X-Span-ID` values can be used to correlate requests.

## Security baseline

- Expose Admin Web and Gateway through HTTPS.
- Restrict CORS to actual Admin Web origins.
- Keep PostgreSQL and Redis off public networks.
- Store `.env`, the master key, provider credentials, administrator passwords, and Virtual Keys as secrets.
- Revoke Virtual Keys when members leave or access changes.
- Back up before credential, schema, or deployment changes.
- Follow [SECURITY.md](../SECURITY.md) when a vulnerability is suspected.
