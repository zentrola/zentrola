-- name: QuotaConfiguredPrincipals :many
SELECT id, monthly_token_limit
FROM principal
WHERE id=ANY(sqlc.arg(scope_ids)::bigint[])
  AND NOT is_deleted
  AND monthly_token_limit IS NOT NULL
ORDER BY id;

-- name: QuotaConfiguredGroups :many
SELECT id, monthly_token_limit
FROM principal_group
WHERE id=ANY(sqlc.arg(scope_ids)::bigint[])
  AND NOT is_deleted
  AND monthly_token_limit IS NOT NULL
ORDER BY id;

-- name: QuotaPrincipalUsedTokens :many
SELECT requested.scope_id::bigint AS scope_id,
       COALESCE(counter.used_tokens, 0)::bigint AS used_tokens
FROM unnest(sqlc.arg(scope_ids)::bigint[]) AS requested(scope_id)
LEFT JOIN monthly_token_usage counter
  ON counter.scope_type='PRINCIPAL' AND counter.scope_id=requested.scope_id
 AND counter.period_start=sqlc.arg(period_start)::timestamptz
ORDER BY requested.scope_id;

-- name: QuotaGroupUsedTokens :many
SELECT requested.scope_id::bigint AS scope_id,
       COALESCE(counter.used_tokens, 0)::bigint AS used_tokens
FROM unnest(sqlc.arg(scope_ids)::bigint[]) AS requested(scope_id)
LEFT JOIN monthly_token_usage counter
  ON counter.scope_type='GROUP' AND counter.scope_id=requested.scope_id
 AND counter.period_start=sqlc.arg(period_start)::timestamptz
ORDER BY requested.scope_id;
