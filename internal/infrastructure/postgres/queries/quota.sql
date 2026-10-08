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
       COALESCE((
           SELECT SUM(COALESCE(usage.input_tokens, 0) + COALESCE(usage.output_tokens, 0))
           FROM usage_record usage
           WHERE usage.principal_id=requested.scope_id
             AND usage.started_at>=sqlc.arg(period_start)::timestamptz
             AND usage.started_at<sqlc.arg(period_end)::timestamptz
       ), 0)::bigint AS used_tokens
FROM unnest(sqlc.arg(scope_ids)::bigint[]) AS requested(scope_id)
ORDER BY requested.scope_id;

-- name: QuotaGroupUsedTokens :many
SELECT requested.scope_id::bigint AS scope_id,
       COALESCE((
           SELECT SUM(COALESCE(usage.input_tokens, 0) + COALESCE(usage.output_tokens, 0))
           FROM usage_record usage
           WHERE usage.started_at>=sqlc.arg(period_start)::timestamptz
             AND usage.started_at<sqlc.arg(period_end)::timestamptz
             AND EXISTS (
                 SELECT 1
                 FROM principal_group_membership membership
                 WHERE membership.group_id=requested.scope_id
                   AND membership.principal_id=usage.principal_id
                   AND NOT membership.is_deleted
             )
             AND EXISTS (
                 SELECT 1
                 FROM principal_group_model_permission permission
                 WHERE permission.group_id=requested.scope_id
                   AND permission.model_id=usage.model_id
                   AND NOT permission.is_deleted
             )
       ), 0)::bigint AS used_tokens
FROM unnest(sqlc.arg(scope_ids)::bigint[]) AS requested(scope_id)
ORDER BY requested.scope_id;
