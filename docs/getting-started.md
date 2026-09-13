# Getting Started

[English](getting-started.md) | [简体中文](getting-started.zh-CN.md)

This guide installs a Zentrola release bundle and completes the first administrative setup. It does not cover building from source.

## 1. Prepare dependencies

Zentrola requires PostgreSQL 17. Redis is recommended for authentication and route caches and for temporary provider cooldown state. A temporary Redis outage does not prevent readiness and gateway routing falls back to PostgreSQL, but production installations should still provide Redis.

The repository includes a PostgreSQL-only Compose example:

```shell
docker compose up -d postgres
```

The example publishes PostgreSQL on `127.0.0.1:15432` and stores data in the `zentrola_postgres_data` volume. It does not start Redis, the Backend, or Admin Web.

## 2. Prepare the release directory

A release bundle has this layout:

```text
zentrola              # Backend on macOS/Linux
zentrola.exe          # Backend on Windows
zentrola-web          # Admin Web server on macOS/Linux
zentrola-web.exe      # Admin Web server on Windows
.env.example
dist/
  index.html
  config.js
  assets/
```

Only the executables for the selected operating system are included. Rename `.env.example` to `.env` in the same directory.

## 3. Configure the Backend

Set a PostgreSQL connection and an independent Admin JWT signing secret:

```dotenv
APP_ENV=prod
HTTP_ADDR=:9527

POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_DB=zentrola
POSTGRES_USER=zentrola
POSTGRES_PASSWORD=replace-with-a-strong-password
POSTGRES_SSLMODE=disable

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_DB=2
REDIS_PASSWORD=replace-with-a-redis-password

ADMIN_JWT_SECRET=replace-with-at-least-32-random-bytes-in-base64
```

Use `POSTGRES_PORT=15432` with the repository's Compose example. Use TLS and `POSTGRES_SSLMODE` appropriate to the production database.

When neither `ACP_MASTER_KEY` nor `ACP_MASTER_KEY_FILE` is provided, the Backend creates `data/secrets/master.key`. This key encrypts provider credentials and must be backed up with the database.

## 4. Configure Admin Web

```dotenv
WEB_ADDR=:9528
WEB_API_BASE_URL=http://127.0.0.1:9527
WEB_GATEWAY_BASE_URL=http://127.0.0.1:9527

CORS_ENABLED=true
CORS_ALLOWED_ORIGINS=http://127.0.0.1:9528
```

`WEB_API_BASE_URL` and `WEB_GATEWAY_BASE_URL` must be URLs that users' browsers and clients can reach. For remote deployment, use the public HTTPS URLs rather than an internal container hostname. Add the actual Admin Web origin to `CORS_ALLOWED_ORIGINS`.

## 5. Start the services

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

The Backend runs in the background when started with `start`. Run `./zentrola` or `.\zentrola.exe` with no command to keep it in the foreground.

Open `http://127.0.0.1:9528` and create the first administrator. There is no default account. Creating the first administrator closes the setup endpoint.

## 6. Configure the first governance path

1. Open **Providers** and initialize the built-in provider metadata or create a provider manually.
2. Add an API-key credential or a supported ChatGPT subscription credential.
3. Test the credential, synchronize supported model catalogs, or add provider-model mappings manually.
4. Enable the logical model and its provider mapping.
5. Create a member and a Group.
6. Add the member to the Group and grant the logical model to the Group.
7. Issue a Virtual Key for the member and store it immediately; complete secret values are not shown again.

Continue with [Client configuration](client-configuration.md).

## Next steps

- Review every setting in [`.env.example`](../.env.example) before production deployment.
- Configure HTTPS before exposing Admin Web or Gateway outside a trusted network.
- Read [Deployment and operations](operations.md) before storing real provider credentials.
