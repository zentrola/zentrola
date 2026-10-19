-- name: ManageModelPrices :many
SELECT id, provider_credential_id, provider_model_id, currency,
    input_price, output_price, cached_input_price, cache_creation_input_price,
    effective_at, created_at
FROM provider_credential_model_price
WHERE provider_credential_id = $1
ORDER BY provider_model_id, effective_at DESC, id DESC;

-- name: ManageSubscriptionPrices :many
SELECT id, provider_credential_id, currency, period_amount,
       billing_period, effective_at, created_at
FROM provider_credential_subscription_price
WHERE provider_credential_id = $1
ORDER BY effective_at DESC, id DESC;

-- name: ManageInsertModelPrice :exec
INSERT INTO provider_credential_model_price (
    id, provider_credential_id, provider_model_id, currency,
    input_price, output_price, cached_input_price, cache_creation_input_price,
    effective_at, created_by, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(provider_credential_id), sqlc.arg(provider_model_id),
    sqlc.arg(currency), sqlc.arg(input_price), sqlc.arg(output_price),
    sqlc.arg(cached_input_price), sqlc.narg(cache_creation_input_price), sqlc.arg(effective_at),
    sqlc.arg(created_by), sqlc.arg(created_at)
);

-- name: ManageUpdateModelPrice :execrows
UPDATE provider_credential_model_price
SET currency = sqlc.arg(currency),
    input_price = sqlc.arg(input_price),
    output_price = sqlc.arg(output_price),
    cached_input_price = sqlc.arg(cached_input_price),
    cache_creation_input_price = sqlc.narg(cache_creation_input_price),
    effective_at = sqlc.arg(effective_at)
WHERE id = sqlc.arg(id)
  AND provider_credential_id = sqlc.arg(provider_credential_id)
  AND provider_model_id = sqlc.arg(provider_model_id);

-- name: ManageDeleteModelPrice :execrows
DELETE FROM provider_credential_model_price
WHERE id = sqlc.arg(id)
  AND provider_credential_id = sqlc.arg(provider_credential_id)
  AND provider_model_id = sqlc.arg(provider_model_id);

-- name: ManageInsertSubscriptionPrice :exec
INSERT INTO provider_credential_subscription_price (
    id, provider_credential_id, currency, period_amount, billing_period,
    effective_at, created_by, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(provider_credential_id), sqlc.arg(currency),
    sqlc.arg(period_amount), sqlc.arg(billing_period), sqlc.arg(effective_at),
    sqlc.arg(created_by), sqlc.arg(created_at)
);

-- name: ManageUpdateSubscriptionPrice :execrows
UPDATE provider_credential_subscription_price
SET currency = sqlc.arg(currency), period_amount = sqlc.arg(period_amount),
    billing_period = sqlc.arg(billing_period), effective_at = sqlc.arg(effective_at)
WHERE id = sqlc.arg(id) AND provider_credential_id = sqlc.arg(provider_credential_id);

-- name: ManageDeleteSubscriptionPrice :execrows
DELETE FROM provider_credential_subscription_price
WHERE id = sqlc.arg(id) AND provider_credential_id = sqlc.arg(provider_credential_id);

-- name: ManageSubscriptionPriceReferenced :one
SELECT EXISTS (
    SELECT 1
    FROM billing_document
    WHERE source_type = 'SUBSCRIPTION_PRICE'
      AND source_id = sqlc.arg(price_id)::bigint
);

-- name: ManageSubscriptionChargedUntil :one
-- 凭据已出账订阅账期的最晚结束时间；没有账期时为 NULL。
SELECT MAX(period_end)::timestamptz AS charged_until
FROM billing_document
WHERE provider_credential_id = sqlc.arg(provider_credential_id)::bigint
  AND billing_type = 'SUBSCRIPTION'
  AND document_type = 'CHARGE'
  AND status = 'CONFIRMED';

-- name: ManageModelPriceReferenced :one
SELECT EXISTS (
    SELECT 1
    FROM usage_rating
    WHERE model_price_id = sqlc.arg(price_id)::bigint
);

-- name: ManageModelPriceOverridesRatedUsage :one
-- 与触发器 reject_model_price_history_override 口径一致：价格版本放在 effective_at 后，
-- 是否会成为某条已核算调用的有效价格而替换其最新核算引用的价格。
SELECT EXISTS (
    SELECT 1
    FROM usage_record usage
    JOIN LATERAL (
        SELECT rating.model_price_id
        FROM usage_rating rating
        WHERE rating.usage_record_id = usage.id
        ORDER BY rating.revision DESC, rating.id DESC
        LIMIT 1
    ) latest ON TRUE
    WHERE usage.provider_credential_id = sqlc.arg(provider_credential_id)::bigint
      AND usage.provider_model_id = sqlc.arg(provider_model_id)::bigint
      AND usage.started_at >= sqlc.arg(effective_at)::timestamptz
      AND latest.model_price_id <> sqlc.arg(price_id)::bigint
      AND NOT EXISTS (
          SELECT 1
          FROM provider_credential_model_price later
          WHERE later.id <> sqlc.arg(price_id)::bigint
            AND later.provider_credential_id = sqlc.arg(provider_credential_id)::bigint
            AND later.provider_model_id = sqlc.arg(provider_model_id)::bigint
            AND later.effective_at <= usage.started_at
            AND (later.effective_at > sqlc.arg(effective_at)::timestamptz
                OR (later.effective_at = sqlc.arg(effective_at)::timestamptz AND later.id > sqlc.arg(price_id)::bigint))
      )
);
