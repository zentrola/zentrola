# Client configuration

[English](client-configuration.md) | [简体中文](client-configuration.zh-CN.md)

Every client uses a member Virtual Key. The key identifies the member; model access comes from the allowlists of the Groups to which that member belongs. Clients use a logical-model code, not an upstream provider-model code.

## Endpoints

| Protocol             | Base URL                           | Supported paths                                                           | Authentication                        |
| -------------------- | ---------------------------------- | ------------------------------------------------------------------------- | ------------------------------------- |
| OpenAI compatible    | `https://<gateway-host>/v1`        | `GET /v1/models`, `POST /v1/chat/completions`, `POST /v1/responses`       | `Authorization: Bearer <virtual-key>` |
| Anthropic compatible | `https://<gateway-host>/anthropic` | `POST /anthropic/v1/messages`, `POST /anthropic/v1/messages/count_tokens` | `x-api-key: <virtual-key>` or Bearer  |

Do not send both `x-api-key` and `Authorization` to the Anthropic endpoint.

## Codex and OpenAI-compatible clients

Set the gateway base URL and Virtual Key in the shell that starts the client.

macOS/Linux:

```bash
printf 'Zentrola Virtual Key: '
read -r -s ZENTROLA_KEY
printf '\n'
export OPENAI_BASE_URL='https://<gateway-host>/v1'
export OPENAI_API_KEY="$ZENTROLA_KEY"
unset ZENTROLA_KEY
```

Windows PowerShell:

```powershell
$secureKey = Read-Host 'Zentrola Virtual Key' -AsSecureString
$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password
$env:OPENAI_BASE_URL = 'https://<gateway-host>/v1'
$env:OPENAI_API_KEY = $zentrolaKey
Remove-Variable secureKey, zentrolaKey
```

Select one of the logical-model codes returned by `GET /v1/models`. Client-specific configuration files and supported environment variables can change between client releases; prefer the client's current official documentation when persistent configuration is required.

## Claude Code and Anthropic-compatible clients

macOS/Linux:

```bash
printf 'Zentrola Virtual Key: '
read -r -s ZENTROLA_KEY
printf '\n'
export ANTHROPIC_BASE_URL='https://<gateway-host>/anthropic'
export ANTHROPIC_API_KEY="$ZENTROLA_KEY"
unset ZENTROLA_KEY
```

Windows PowerShell:

```powershell
$secureKey = Read-Host 'Zentrola Virtual Key' -AsSecureString
$zentrolaKey = [Net.NetworkCredential]::new('', $secureKey).Password
$env:ANTHROPIC_BASE_URL = 'https://<gateway-host>/anthropic'
$env:ANTHROPIC_API_KEY = $zentrolaKey
Remove-Variable secureKey, zentrolaKey
```

Configure the client to use a logical-model code granted to the member. Zentrola forwards supported Messages, streaming, and tool-use traffic. `count_tokens` requires an Anthropic-compatible upstream route; Zentrola does not estimate it through an OpenAI-only upstream.

## Direct verification

List authorized models:

```bash
curl https://<gateway-host>/v1/models \
  -H "Authorization: Bearer <virtual-key>"
```

Call Anthropic Messages:

```bash
curl https://<gateway-host>/anthropic/v1/messages \
  -H "x-api-key: <virtual-key>" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"<logical-model-code>","max_tokens":128,"messages":[{"role":"user","content":"Hello"}]}'
```

## Troubleshooting

- `401`: the Virtual Key is missing, invalid, expired, or revoked.
- `403`: the member is disabled or has not been granted the requested logical model.
- Route unavailable: confirm that the model, provider, mapping, and credential are enabled and that the credential is not blocked.
- Empty model list: confirm Group membership and model allowlists.
- Cross-origin Admin Web errors: update `CORS_ALLOWED_ORIGINS`; gateway API clients are not governed by browser CORS in the same way.

Use the response `X-Trace-ID`, `X-Span-ID`, or request ID to correlate a failed request with Backend logs without sharing credentials or request bodies.
