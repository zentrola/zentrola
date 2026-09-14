-- name: InsertUsageAttempts :exec
INSERT INTO usage_record(id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,started_at,completed_at,latency_ms,status,error_type,created_at)
SELECT nextval('zentrola_global_id_seq'),request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,'MODEL_GATEWAY',client_protocol,input_tokens,output_tokens,cached_input_tokens,started_at,completed_at,latency_ms,status,error_type,completed_at
FROM jsonb_to_recordset(sqlc.arg(payload)::jsonb) AS x(request_id text,attempt_no integer,client_protocol text,principal_id bigint,provider_id bigint,provider_model_id bigint,provider_credential_id bigint,model_id bigint,input_tokens bigint,output_tokens bigint,cached_input_tokens bigint,started_at timestamptz,completed_at timestamptz,latency_ms bigint,status text,error_type text)
ON CONFLICT(request_id,attempt_no) DO NOTHING;

-- name: QueryUsage :many
SELECT u.id,u.request_id,u.client_protocol,u.principal_id,p.name AS principal_name,u.model_id,u.started_at,u.completed_at,u.latency_ms,u.status,u.error_type,
attempt_no,provider_id,provider_model_id,provider_credential_id AS resource_id,input_tokens,output_tokens,cached_input_tokens
FROM usage_record u
JOIN principal p ON p.id=u.principal_id
WHERE (u.id<sqlc.arg(after_id)::bigint OR sqlc.arg(after_id)::bigint=0)
AND u.started_at>=sqlc.arg(from_time)::timestamptz AND u.started_at<sqlc.arg(to_time)::timestamptz
AND (sqlc.narg(principal_id)::bigint IS NULL OR u.principal_id=sqlc.narg(principal_id))
AND (sqlc.narg(model_id)::bigint IS NULL OR u.model_id=sqlc.narg(model_id))
AND (sqlc.narg(provider_id)::bigint IS NULL OR u.provider_id=sqlc.narg(provider_id))
AND (sqlc.narg(resource_id)::bigint IS NULL OR u.provider_credential_id=sqlc.narg(resource_id))
ORDER BY u.id DESC LIMIT sqlc.arg(page_limit)::int;

-- name: CountUsage :one
SELECT COUNT(*)::bigint
FROM usage_record u
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz AND u.started_at<sqlc.arg(to_time)::timestamptz
AND (sqlc.narg(principal_id)::bigint IS NULL OR u.principal_id=sqlc.narg(principal_id))
AND (sqlc.narg(model_id)::bigint IS NULL OR u.model_id=sqlc.narg(model_id))
AND (sqlc.narg(provider_id)::bigint IS NULL OR u.provider_id=sqlc.narg(provider_id))
AND (sqlc.narg(resource_id)::bigint IS NULL OR u.provider_credential_id=sqlc.narg(resource_id));

-- name: UsageMemberStatistics :many
SELECT u.principal_id AS entity_id,
       p.name,
       ''::text AS code,
       COUNT(DISTINCT u.request_id)::bigint AS metric_count,
       COUNT(DISTINCT u.request_id) FILTER (WHERE u.status='SUCCESS') AS successful,
       COALESCE(SUM(u.input_tokens), 0)::bigint AS input_tokens,
       COALESCE(SUM(u.output_tokens), 0)::bigint AS output_tokens,
       COALESCE(SUM(u.cached_input_tokens), 0)::bigint AS cached_input_tokens,
       COALESCE(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)), 0)::bigint AS tokens,
       SUM(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0))) OVER ()::bigint AS overall_tokens,
       COALESCE(ROUND(AVG(u.latency_ms)), 0)::bigint AS average_latency_ms,
       COUNT(*) OVER ()::bigint AS total_count
FROM usage_record u
JOIN principal p ON p.id=u.principal_id
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY u.principal_id, p.name
ORDER BY tokens DESC, metric_count DESC, u.principal_id DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::bigint;

-- name: UsageModelStatistics :many
SELECT m.id AS entity_id,
       m.display_name AS name,
       m.model_code AS code,
       COUNT(DISTINCT u.request_id)::bigint AS metric_count,
       COUNT(DISTINCT u.request_id) FILTER (WHERE u.status='SUCCESS') AS successful,
       COALESCE(SUM(u.input_tokens), 0)::bigint AS input_tokens,
       COALESCE(SUM(u.output_tokens), 0)::bigint AS output_tokens,
       COALESCE(SUM(u.cached_input_tokens), 0)::bigint AS cached_input_tokens,
       COALESCE(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)), 0)::bigint AS tokens,
       SUM(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0))) OVER ()::bigint AS overall_tokens,
       COALESCE(ROUND(AVG(u.latency_ms)), 0)::bigint AS average_latency_ms,
       COUNT(*) OVER ()::bigint AS total_count
FROM usage_record u
JOIN model m ON m.id=u.model_id
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY m.id, m.display_name, m.model_code
ORDER BY metric_count DESC, tokens DESC, m.id DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::bigint;

-- name: UsageProviderStatistics :many
SELECT p.id AS entity_id,
       p.provider_name AS name,
       p.provider_code AS code,
       COUNT(*)::bigint AS metric_count,
       COUNT(*) FILTER (WHERE u.status='SUCCESS') AS successful,
       COALESCE(SUM(u.input_tokens), 0)::bigint AS input_tokens,
       COALESCE(SUM(u.output_tokens), 0)::bigint AS output_tokens,
       COALESCE(SUM(u.cached_input_tokens), 0)::bigint AS cached_input_tokens,
       COALESCE(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)), 0)::bigint AS tokens,
       SUM(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0))) OVER ()::bigint AS overall_tokens,
       COALESCE(ROUND(AVG(u.latency_ms)), 0)::bigint AS average_latency_ms,
       COUNT(*) OVER ()::bigint AS total_count
FROM usage_record u
JOIN provider p ON p.id=u.provider_id
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY p.id, p.provider_name, p.provider_code
ORDER BY metric_count DESC, tokens DESC, p.id DESC
LIMIT sqlc.arg(page_limit)::int OFFSET sqlc.arg(page_offset)::bigint;

-- name: UsageDashboardCounts :one
SELECT
    (SELECT COUNT(*)::bigint FROM principal p
     WHERE p.principal_type='MEMBER' AND p.is_deleted=false AND p.status='ACTIVE') AS active_member_count,
    (SELECT COUNT(*)::bigint FROM model WHERE is_deleted=false AND status='ACTIVE') AS model_count,
    (SELECT COUNT(*)::bigint FROM provider WHERE is_deleted=false AND status='ACTIVE') AS provider_count,
    COALESCE((
        SELECT SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0))::bigint
        FROM usage_record u
        WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
          AND u.started_at<sqlc.arg(to_time)::timestamptz
    ), 0)::bigint AS total_tokens;

-- name: UsageTokenRanking :many
SELECT u.principal_id, p.name,
       SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0))::bigint AS tokens
FROM usage_record u
JOIN principal p ON p.id=u.principal_id
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY u.principal_id, p.name
HAVING SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)) > 0
ORDER BY tokens DESC, u.principal_id DESC
LIMIT 10;

-- name: UsageClientModelRanking :many
SELECT m.id AS model_id,
       m.model_code,
       m.display_name AS model_name,
       COUNT(DISTINCT u.request_id)::bigint AS requests,
       COALESCE(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)), 0)::bigint AS tokens
FROM usage_record u
JOIN model m ON m.id=u.model_id
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY m.id, m.model_code, m.display_name
ORDER BY requests DESC, tokens DESC, m.id DESC
LIMIT 10;

-- name: UsageProviderRanking :many
SELECT p.id AS provider_id,
       p.provider_name,
       COUNT(*)::bigint AS calls,
       COALESCE(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)), 0)::bigint AS tokens
FROM usage_record u
JOIN provider p ON p.id=u.provider_id
WHERE u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY p.id, p.provider_name
ORDER BY calls DESC, tokens DESC, p.id DESC
LIMIT 10;
