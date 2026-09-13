# Security Policy

[English](SECURITY.md) | [简体中文](SECURITY.zh-CN.md)

Zentrola handles provider credentials, Virtual Keys, administrator sessions, prompts, and model output. Treat a suspected vulnerability or accidental secret disclosure as sensitive.

## Supported versions

Zentrola has not declared a stable support matrix yet. Security fixes target the latest released version. Upgrade to the latest release before reporting a problem that may already be fixed.

## Reporting a vulnerability

Do not open a public issue with vulnerability details, credentials, exploit steps, private logs, prompts, or model responses.

The repository does not yet publish a dedicated security contact. Until one is configured, use GitHub's private vulnerability reporting feature for `zentrola/zentrola` if it is available. If private reporting is unavailable, contact the maintainers through an established private channel and share only enough information to arrange a secure exchange.

A useful report includes:

- the affected version or commit;
- the affected component and configuration;
- impact and reproducible steps using synthetic data;
- whether exploitation has been observed;
- suggested mitigation, if known.

Remove or replace all real provider credentials, Virtual Keys, JWTs, cookies, database connection strings, prompts, model responses, and personal data.

## Response expectations

The project does not currently promise a formal response SLA. Maintainers should acknowledge a valid private report, coordinate verification and remediation, prepare release notes without premature disclosure, and credit the reporter when requested and appropriate.

## Operational security

- Use HTTPS for Admin Web and Gateway traffic.
- Keep PostgreSQL and Redis on private networks.
- Use independent secrets for Admin JWT signing, PostgreSQL, Redis, and provider authentication.
- Back up PostgreSQL together with the exact master key used to encrypt provider credentials.
- Never enable `APP_ENV=dev` request/response body diagnostics in production.
- Revoke exposed or unnecessary Virtual Keys immediately.
- Rotate any secret that may have appeared in a log, screenshot, issue, or chat; redaction alone does not make an exposed secret safe again.

For deployment details, see [Deployment and operations](docs/operations.md).
