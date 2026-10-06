-- name: ManageModelPrices :many
SELECT id, provider_credential_id, provider_model_id, currency,
    input_price, output_price, cached_input_price, effective_at, created_at
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
    input_price, output_price, cached_input_price, effective_at,
    created_by, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(provider_credential_id), sqlc.arg(provider_model_id),
    sqlc.arg(currency), sqlc.arg(input_price), sqlc.arg(output_price),
    sqlc.arg(cached_input_price), sqlc.arg(effective_at),
    sqlc.arg(created_by), sqlc.arg(created_at)
);

-- name: ManageUpdateModelPrice :execrows
UPDATE provider_credential_model_price
SET currency = sqlc.arg(currency),
    input_price = sqlc.arg(input_price),
    output_price = sqlc.arg(output_price),
    cached_input_price = sqlc.arg(cached_input_price),
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
