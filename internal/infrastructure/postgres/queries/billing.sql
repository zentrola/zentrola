-- name: BillingSubscriptionPrices :many
SELECT p.id, p.provider_credential_id, p.currency, p.period_amount,
       p.billing_period, p.effective_at,
       COALESCE(c.effective_at, p.effective_at)::timestamptz AS credential_effective_at,
       CASE WHEN c.is_deleted THEN c.updated_at ELSE c.expires_at END::timestamptz AS credential_end_at
FROM provider_credential_subscription_price p
JOIN provider_credential c ON c.id = p.provider_credential_id
WHERE c.auth_type = 'SUBSCRIPTION'
  AND c.subscription_type = 'PERSONAL'
ORDER BY p.provider_credential_id, p.effective_at, p.id;

-- name: BillingChargePeriods :many
SELECT provider_credential_id, period_start, period_end
FROM billing_document
WHERE billing_type = 'SUBSCRIPTION'
  AND document_type = 'CHARGE'
  AND status = 'CONFIRMED'
ORDER BY provider_credential_id, period_start;

-- name: BillingSubscriptionUsage :many
SELECT principal_id,
       SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0))::bigint AS tokens
FROM usage_record
WHERE provider_credential_id = sqlc.arg(provider_credential_id)::bigint
  AND status = 'SUCCESS'
  AND started_at >= sqlc.arg(period_start)::timestamptz
  AND started_at < sqlc.arg(period_end)::timestamptz
GROUP BY principal_id
HAVING SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0)) > 0
ORDER BY tokens DESC, principal_id;

-- name: BillingInsertDocument :execrows
INSERT INTO billing_document (
    id, status, billing_type, document_type, provider_credential_id,
    source_type, source_id, original_document_id, period_start, period_end,
    total_tokens, total_amount, currency, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(status), sqlc.arg(billing_type), sqlc.arg(document_type),
    sqlc.arg(provider_credential_id), sqlc.arg(source_type), sqlc.arg(source_id),
    sqlc.narg(original_document_id), sqlc.arg(period_start), sqlc.arg(period_end),
    sqlc.arg(total_tokens), sqlc.arg(total_amount), sqlc.arg(currency), sqlc.arg(created_at)
)
ON CONFLICT (billing_type, provider_credential_id, period_start, period_end)
    WHERE document_type = 'CHARGE' AND status = 'CONFIRMED'
DO NOTHING;

-- name: BillingInsertDocumentItem :exec
INSERT INTO billing_document_item (
    id, billing_document_id, principal_id, usage_tokens,
    allocation_ratio, amount, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(billing_document_id), sqlc.arg(principal_id),
    sqlc.arg(usage_tokens), sqlc.arg(allocation_ratio), sqlc.arg(amount), sqlc.arg(created_at)
);

-- name: BillingSubscriptionCorrections :many
WITH adjustment AS (
    SELECT original_document_id, SUM(total_amount) AS amount
    FROM billing_document
    WHERE billing_type = 'SUBSCRIPTION'
      AND document_type = 'ADJUSTMENT'
      AND status = 'CONFIRMED'
    GROUP BY original_document_id
)
SELECT base.id AS original_document_id,
       base.provider_credential_id,
       base.source_id AS source_price_id,
       base.period_start,
       base.period_end,
       base.currency,
       price.currency AS target_currency,
       (base.total_amount + COALESCE(adjustment.amount, 0::numeric))::numeric AS current_amount,
       price.period_amount AS target_amount
FROM billing_document base
JOIN provider_credential_subscription_price price
  ON base.source_type = 'SUBSCRIPTION_PRICE' AND price.id = base.source_id
LEFT JOIN adjustment ON adjustment.original_document_id = base.id
WHERE base.billing_type = 'SUBSCRIPTION'
  AND base.document_type = 'CHARGE'
  AND base.status = 'CONFIRMED'
  AND (
      base.currency <> price.currency
      OR base.total_amount + COALESCE(adjustment.amount, 0::numeric) <> price.period_amount
  )
ORDER BY base.id;

-- name: BillingOriginalUsage :many
SELECT principal_id, usage_tokens AS tokens
FROM billing_document_item
WHERE billing_document_id = sqlc.arg(billing_document_id)::bigint
  AND usage_tokens > 0
ORDER BY usage_tokens DESC, principal_id;

-- name: BillingLockDocument :one
SELECT id
FROM billing_document
WHERE id = sqlc.arg(id)::bigint
  AND billing_type = 'SUBSCRIPTION'
  AND document_type = 'CHARGE'
  AND status = 'CONFIRMED'
FOR UPDATE;

-- name: BillingDocumentNetAmount :one
SELECT (base.total_amount + COALESCE(SUM(adjustment.total_amount), 0::numeric))::numeric AS amount
FROM billing_document base
LEFT JOIN billing_document adjustment
  ON adjustment.original_document_id = base.id
 AND adjustment.document_type = 'ADJUSTMENT'
 AND adjustment.status = 'CONFIRMED'
WHERE base.id = sqlc.arg(id)::bigint
GROUP BY base.id, base.total_amount;

-- name: BillingStatisticsTotals :many
WITH item_total AS (
    SELECT billing_document_id, SUM(amount) AS amount
    FROM billing_document_item
    GROUP BY billing_document_id
)
SELECT document.currency,
       document.billing_type,
       SUM(document.total_amount)::numeric AS total_amount,
       SUM(COALESCE(item_total.amount, 0::numeric))::numeric AS allocated_amount,
       SUM(document.total_amount - COALESCE(item_total.amount, 0::numeric))::numeric AS unallocated_amount,
       SUM(CASE WHEN document.document_type = 'CHARGE' THEN document.total_tokens ELSE 0 END)::bigint AS total_tokens
FROM billing_document document
LEFT JOIN item_total ON item_total.billing_document_id = document.id
WHERE document.status = 'CONFIRMED'
  AND document.period_start >= sqlc.arg(from_time)::timestamptz
  AND document.period_start < sqlc.arg(to_time)::timestamptz
  AND (sqlc.arg(billing_type)::text = '' OR document.billing_type = sqlc.arg(billing_type)::text)
GROUP BY document.billing_type, document.currency
ORDER BY document.billing_type, document.currency;

-- name: BillingStatisticsPrincipals :many
SELECT item.principal_id,
       principal.name AS principal_name,
       principal.principal_type,
       document.billing_type,
       document.currency,
       SUM(item.amount)::numeric AS amount,
       SUM(CASE WHEN document.document_type = 'CHARGE' THEN item.usage_tokens ELSE 0 END)::bigint AS tokens,
       COUNT(*) OVER ()::bigint AS total_count
FROM billing_document_item item
JOIN billing_document document ON document.id = item.billing_document_id
JOIN principal ON principal.id = item.principal_id
WHERE document.status = 'CONFIRMED'
  AND document.period_start >= sqlc.arg(from_time)::timestamptz
  AND document.period_start < sqlc.arg(to_time)::timestamptz
  AND (sqlc.arg(billing_type)::text = '' OR document.billing_type = sqlc.arg(billing_type)::text)
GROUP BY item.principal_id, principal.name, principal.principal_type, document.billing_type, document.currency
ORDER BY amount DESC, item.principal_id, document.billing_type, document.currency
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::bigint;
