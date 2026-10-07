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
ON CONFLICT DO NOTHING;

-- name: BillingChargeExists :one
SELECT EXISTS (
    SELECT 1
    FROM billing_document
    WHERE billing_type = sqlc.arg(billing_type)::text
      AND document_type = 'CHARGE'
      AND status = 'CONFIRMED'
      AND provider_credential_id = sqlc.arg(provider_credential_id)::bigint
      AND period_start = sqlc.arg(period_start)::timestamptz
      AND period_end = sqlc.arg(period_end)::timestamptz
      AND (sqlc.arg(billing_type)::text = 'SUBSCRIPTION' OR currency = sqlc.arg(currency)::text)
);

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
       SUM(document.total_tokens)::bigint AS total_tokens
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
       SUM(item.usage_tokens)::bigint AS tokens,
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

-- name: BillingUsageRatingCandidates :many
SELECT usage.id AS usage_record_id,
       usage.principal_id,
       usage.provider_credential_id,
       usage.provider_model_id,
       usage.started_at AS usage_started_at,
       usage.input_tokens::bigint AS input_tokens,
       usage.cached_input_tokens::bigint AS cached_input_tokens,
       usage.output_tokens::bigint AS output_tokens,
       price.id AS model_price_id,
       price.currency,
       price.input_price,
       price.cached_input_price,
       price.output_price,
       COALESCE(latest.id, 0)::bigint AS latest_rating_id,
       COALESCE(latest.revision, 0)::int AS latest_revision
FROM usage_record usage
JOIN provider_credential credential
  ON credential.id = usage.provider_credential_id
 AND credential.auth_type = 'API_KEY'
JOIN LATERAL (
    SELECT candidate.*
    FROM provider_credential_model_price candidate
    WHERE candidate.provider_credential_id = usage.provider_credential_id
      AND candidate.provider_model_id = usage.provider_model_id
      AND candidate.effective_at <= usage.started_at
    ORDER BY candidate.effective_at DESC, candidate.id DESC
    LIMIT 1
) price ON TRUE
LEFT JOIN LATERAL (
    SELECT rating.*
    FROM usage_rating rating
    WHERE rating.usage_record_id = usage.id
    ORDER BY rating.revision DESC, rating.id DESC
    LIMIT 1
) latest ON TRUE
WHERE usage.id > sqlc.arg(after_id)::bigint
  AND usage.status = 'SUCCESS'
  AND usage.input_tokens IS NOT NULL
  AND usage.cached_input_tokens IS NOT NULL
  AND usage.output_tokens IS NOT NULL
  AND usage.cached_input_tokens <= usage.input_tokens
  AND (
      latest.id IS NULL
      OR latest.model_price_id <> price.id
      OR latest.currency <> price.currency
      OR latest.input_price <> price.input_price
      OR latest.cached_input_price <> price.cached_input_price
      OR latest.output_price <> price.output_price
  )
ORDER BY usage.id
LIMIT sqlc.arg(page_limit)::int;

-- name: BillingInsertUsageRating :execrows
INSERT INTO usage_rating (
    id, usage_record_id, revision, supersedes_rating_id, principal_id,
    provider_credential_id, provider_model_id, model_price_id, usage_started_at,
    input_tokens, cached_input_tokens, output_tokens,
    input_price, cached_input_price, output_price,
    input_cost, cached_input_cost, output_cost, total_cost, currency, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(usage_record_id), sqlc.arg(revision), sqlc.narg(supersedes_rating_id),
    sqlc.arg(principal_id), sqlc.arg(provider_credential_id), sqlc.arg(provider_model_id),
    sqlc.arg(model_price_id), sqlc.arg(usage_started_at), sqlc.arg(input_tokens),
    sqlc.arg(cached_input_tokens), sqlc.arg(output_tokens), sqlc.arg(input_price),
    sqlc.arg(cached_input_price), sqlc.arg(output_price), sqlc.arg(input_cost),
    sqlc.arg(cached_input_cost), sqlc.arg(output_cost), sqlc.arg(total_cost),
    sqlc.arg(currency), sqlc.arg(created_at)
)
ON CONFLICT (usage_record_id, revision) DO NOTHING;

-- name: BillingAPIKeyPeriods :many
WITH current_rating AS (
    SELECT DISTINCT ON (usage_record_id)
           provider_credential_id, usage_started_at, currency
    FROM usage_rating
    ORDER BY usage_record_id, revision DESC, id DESC
)
SELECT provider_credential_id,
       date_trunc('month', usage_started_at, 'UTC')::timestamptz AS period_start,
       (date_trunc('month', usage_started_at, 'UTC') + interval '1 month')::timestamptz AS period_end,
       currency
FROM current_rating
WHERE date_trunc('month', usage_started_at, 'UTC') + interval '1 month' <= sqlc.arg(cutoff)::timestamptz
GROUP BY provider_credential_id, period_start, period_end, currency
ORDER BY period_start, provider_credential_id, currency;

-- name: BillingAPIKeyUsage :many
WITH current_rating AS (
    SELECT DISTINCT ON (usage_record_id)
           id, principal_id, provider_credential_id, usage_started_at,
           input_tokens, output_tokens, total_cost, currency
    FROM usage_rating
    ORDER BY usage_record_id, revision DESC, id DESC
)
SELECT id AS usage_rating_id,
       principal_id,
       (input_tokens + output_tokens)::bigint AS tokens,
       total_cost
FROM current_rating
WHERE provider_credential_id = sqlc.arg(provider_credential_id)::bigint
  AND usage_started_at >= sqlc.arg(period_start)::timestamptz
  AND usage_started_at < sqlc.arg(period_end)::timestamptz
  AND currency = sqlc.arg(currency)::text
ORDER BY principal_id, id;

-- name: BillingAPIKeyCurrentDocument :one
WITH base AS (
    SELECT id, total_amount, total_tokens
    FROM billing_document
    WHERE billing_type = 'API_KEY'
      AND document_type = 'CHARGE'
      AND status = 'CONFIRMED'
      AND provider_credential_id = sqlc.arg(provider_credential_id)::bigint
      AND period_start = sqlc.arg(period_start)::timestamptz
      AND period_end = sqlc.arg(period_end)::timestamptz
      AND currency = sqlc.arg(currency)::text
)
SELECT base.id AS original_document_id,
       (base.total_amount + COALESCE(SUM(adjustment.total_amount), 0::numeric))::numeric AS total_amount,
       (base.total_tokens + COALESCE(SUM(adjustment.total_tokens), 0::numeric))::bigint AS total_tokens
FROM base
LEFT JOIN billing_document adjustment
  ON adjustment.original_document_id = base.id
 AND adjustment.document_type = 'ADJUSTMENT'
 AND adjustment.status = 'CONFIRMED'
GROUP BY base.id, base.total_amount, base.total_tokens;

-- name: BillingAPIKeyCurrentItems :many
WITH base AS (
    SELECT id
    FROM billing_document
    WHERE billing_type = 'API_KEY'
      AND document_type = 'CHARGE'
      AND status = 'CONFIRMED'
      AND provider_credential_id = sqlc.arg(provider_credential_id)::bigint
      AND period_start = sqlc.arg(period_start)::timestamptz
      AND period_end = sqlc.arg(period_end)::timestamptz
      AND currency = sqlc.arg(currency)::text
), documents AS (
    SELECT document.id
    FROM billing_document document, base
    WHERE document.id = base.id
       OR (document.original_document_id = base.id
           AND document.document_type = 'ADJUSTMENT'
           AND document.status = 'CONFIRMED')
)
SELECT item.principal_id,
       SUM(item.usage_tokens)::bigint AS tokens,
       SUM(item.amount)::numeric AS amount
FROM billing_document_item item
JOIN documents ON documents.id = item.billing_document_id
GROUP BY item.principal_id
ORDER BY item.principal_id;

-- name: BillingInsertDocumentUsageRating :exec
INSERT INTO billing_document_usage_rating (billing_document_id, usage_rating_id, created_at)
VALUES (sqlc.arg(billing_document_id), sqlc.arg(usage_rating_id), sqlc.arg(created_at))
ON CONFLICT (usage_rating_id) DO NOTHING;

-- name: BillingDocuments :many
WITH filtered AS (
    SELECT document.id,
           document.billing_type,
           document.document_type,
           document.status,
           document.provider_credential_id,
           credential.resource_name AS credential_name,
           document.original_document_id,
           document.period_start,
           document.period_end,
           document.total_tokens,
           document.total_amount,
           document.currency,
           document.created_at
    FROM billing_document document
    JOIN provider_credential credential ON credential.id = document.provider_credential_id
    WHERE document.period_start >= sqlc.arg(from_time)::timestamptz
      AND document.period_start < sqlc.arg(to_time)::timestamptz
      AND (sqlc.arg(billing_type)::text = '' OR document.billing_type = sqlc.arg(billing_type)::text)
      AND (sqlc.arg(document_type)::text = '' OR document.document_type = sqlc.arg(document_type)::text)
      AND (sqlc.arg(currency)::text = '' OR document.currency = sqlc.arg(currency)::text)
)
SELECT filtered.*,
       (SELECT COUNT(*) FROM filtered)::bigint AS total_count
FROM filtered
WHERE sqlc.arg(after_id)::bigint = 0 OR filtered.id < sqlc.arg(after_id)::bigint
ORDER BY filtered.id DESC
LIMIT sqlc.arg(page_limit)::int;

-- name: BillingDocument :one
SELECT document.id,
       document.billing_type,
       document.document_type,
       document.status,
       document.provider_credential_id,
       credential.resource_name AS credential_name,
       document.original_document_id,
       document.period_start,
       document.period_end,
       document.total_tokens,
       document.total_amount,
       document.currency,
       document.created_at
FROM billing_document document
JOIN provider_credential credential ON credential.id = document.provider_credential_id
WHERE document.id = sqlc.arg(id)::bigint;

-- name: BillingDocumentItems :many
SELECT item.id,
       item.principal_id,
       principal.name AS principal_name,
       principal.principal_type,
       item.usage_tokens,
       item.allocation_ratio,
       item.amount
FROM billing_document_item item
JOIN principal ON principal.id = item.principal_id
WHERE item.billing_document_id = sqlc.arg(billing_document_id)::bigint
ORDER BY item.amount DESC, item.principal_id;

-- name: BillingDocumentAdjustments :many
SELECT document.id,
       document.billing_type,
       document.document_type,
       document.status,
       document.provider_credential_id,
       credential.resource_name AS credential_name,
       document.original_document_id,
       document.period_start,
       document.period_end,
       document.total_tokens,
       document.total_amount,
       document.currency,
       document.created_at
FROM billing_document document
JOIN provider_credential credential ON credential.id = document.provider_credential_id
WHERE document.original_document_id = sqlc.arg(original_document_id)::bigint
ORDER BY document.id;

-- name: BillingDocumentRatingCount :one
SELECT COUNT(*)::bigint
FROM billing_document_usage_rating
WHERE billing_document_id = sqlc.arg(billing_document_id)::bigint;

-- name: BillingUnratedUsage :many
WITH candidates AS (
    SELECT usage.id AS usage_record_id,
           usage.principal_id,
           principal.name AS principal_name,
           principal.principal_type,
           usage.provider_credential_id,
           credential.resource_name AS credential_name,
           usage.provider_model_id,
           usage.started_at,
           usage.input_tokens,
           usage.cached_input_tokens,
           usage.output_tokens,
           usage.created_at AS waiting_since,
           CASE
               WHEN usage.input_tokens IS NULL
                 OR usage.cached_input_tokens IS NULL
                 OR usage.output_tokens IS NULL
                 OR usage.cached_input_tokens > usage.input_tokens
                   THEN 'INCOMPLETE_TOKENS'
               WHEN price.id IS NULL THEN 'MISSING_PRICE'
               WHEN latest.id IS NULL
                 OR latest.model_price_id <> price.id
                 OR latest.currency <> price.currency
                 OR latest.input_price <> price.input_price
                 OR latest.cached_input_price <> price.cached_input_price
                 OR latest.output_price <> price.output_price
                   THEN 'PENDING_RATING'
               ELSE NULL
           END::text AS reason
    FROM usage_record usage
    JOIN principal ON principal.id = usage.principal_id
    JOIN provider_credential credential
      ON credential.id = usage.provider_credential_id
     AND credential.auth_type = 'API_KEY'
    LEFT JOIN LATERAL (
        SELECT candidate.*
        FROM provider_credential_model_price candidate
        WHERE candidate.provider_credential_id = usage.provider_credential_id
          AND candidate.provider_model_id = usage.provider_model_id
          AND candidate.effective_at <= usage.started_at
        ORDER BY candidate.effective_at DESC, candidate.id DESC
        LIMIT 1
    ) price ON TRUE
    LEFT JOIN LATERAL (
        SELECT rating.*
        FROM usage_rating rating
        WHERE rating.usage_record_id = usage.id
        ORDER BY rating.revision DESC, rating.id DESC
        LIMIT 1
    ) latest ON TRUE
    WHERE usage.status = 'SUCCESS'
      AND usage.started_at >= sqlc.arg(from_time)::timestamptz
      AND usage.started_at < sqlc.arg(to_time)::timestamptz
), filtered AS (
    SELECT *
    FROM candidates
    WHERE reason IS NOT NULL
      AND (sqlc.arg(reason)::text = '' OR reason = sqlc.arg(reason)::text)
)
SELECT filtered.*,
       (SELECT COUNT(*) FROM filtered)::bigint AS total_count
FROM filtered
WHERE filtered.usage_record_id > sqlc.arg(after_id)::bigint
ORDER BY filtered.usage_record_id
LIMIT sqlc.arg(page_limit)::int;
