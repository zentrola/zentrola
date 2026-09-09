-- name: InsertUsageAttempts :exec
INSERT INTO usage_record(id,organization_id,request_id,attempt_no,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,usage_scene,client_protocol,input_tokens,output_tokens,cached_input_tokens,billing_unit,started_at,completed_at,latency_ms,status,error_type,created_at)
SELECT id,organization_id,request_id,1,principal_id,provider_id,provider_model_id,provider_credential_id,model_id,'MODEL_GATEWAY',client_protocol,input_tokens,output_tokens,cached_input_tokens,'TOKEN',started_at,completed_at,latency_ms,status,error_type,completed_at
FROM jsonb_to_recordset(sqlc.arg(payload)::jsonb) AS x(id bigint,organization_id bigint,request_id text,client_protocol text,principal_id bigint,provider_id bigint,provider_model_id bigint,provider_credential_id bigint,model_id bigint,input_tokens bigint,output_tokens bigint,cached_input_tokens bigint,started_at timestamptz,completed_at timestamptz,latency_ms bigint,status text,error_type text)
ON CONFLICT(request_id,attempt_no) DO NOTHING;

-- name: QueryUsage :many
SELECT u.id,u.request_id,u.client_protocol,u.principal_id,p.name AS principal_name,u.model_id,u.started_at,u.completed_at,u.latency_ms,u.status,u.error_type,
attempt_no,provider_id,provider_model_id,provider_credential_id AS resource_id,input_tokens,output_tokens,cached_input_tokens
FROM usage_record u
JOIN principal p ON p.id=u.principal_id AND p.organization_id=u.organization_id
WHERE u.organization_id=sqlc.arg(organization_id)
AND (u.id<sqlc.arg(after_id)::bigint OR sqlc.arg(after_id)::bigint=0)
AND u.started_at>=sqlc.arg(from_time)::timestamptz AND u.started_at<sqlc.arg(to_time)::timestamptz
AND (sqlc.narg(principal_id)::bigint IS NULL OR u.principal_id=sqlc.narg(principal_id))
AND (sqlc.narg(model_id)::bigint IS NULL OR u.model_id=sqlc.narg(model_id))
AND (sqlc.narg(resource_id)::bigint IS NULL OR u.provider_credential_id=sqlc.narg(resource_id))
ORDER BY u.id DESC LIMIT sqlc.arg(page_limit)::int;

-- name: CountUsage :one
SELECT COUNT(*)::bigint
FROM usage_record u
WHERE u.organization_id=sqlc.arg(organization_id)
AND u.started_at>=sqlc.arg(from_time)::timestamptz AND u.started_at<sqlc.arg(to_time)::timestamptz
AND (sqlc.narg(principal_id)::bigint IS NULL OR u.principal_id=sqlc.narg(principal_id))
AND (sqlc.narg(model_id)::bigint IS NULL OR u.model_id=sqlc.narg(model_id))
AND (sqlc.narg(resource_id)::bigint IS NULL OR u.provider_credential_id=sqlc.narg(resource_id));

-- name: UsageDashboardCounts :one
SELECT
    (SELECT COUNT(*)::bigint FROM principal p
     WHERE p.organization_id=sqlc.arg(organization_id)
       AND p.principal_type='MEMBER' AND p.is_deleted=false AND p.status='ACTIVE') AS active_member_count,
    (SELECT COUNT(*)::bigint FROM model WHERE is_deleted=false AND status='ACTIVE') AS model_count,
    (SELECT COUNT(*)::bigint FROM provider WHERE is_deleted=false AND status='ACTIVE') AS provider_count,
    COALESCE((
        SELECT SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0))::bigint
        FROM usage_record u
        WHERE u.organization_id=sqlc.arg(organization_id)
          AND u.started_at>=sqlc.arg(from_time)::timestamptz
          AND u.started_at<sqlc.arg(to_time)::timestamptz
    ), 0)::bigint AS total_tokens;

-- name: UsageTokenRanking :many
SELECT u.principal_id, p.name,
       SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0))::bigint AS tokens
FROM usage_record u
JOIN principal p ON p.id=u.principal_id AND p.organization_id=u.organization_id
WHERE u.organization_id=sqlc.arg(organization_id)
  AND u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY u.principal_id, p.name
HAVING SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)) > 0
ORDER BY tokens DESC, u.principal_id DESC
LIMIT 10;

-- name: UsageModelRanking :many
SELECT pm.upstream_model_code,
       COUNT(DISTINCT u.request_id)::bigint AS requests,
       COALESCE(SUM(COALESCE(u.input_tokens, 0) + COALESCE(u.output_tokens, 0)), 0)::bigint AS tokens
FROM usage_record u
JOIN provider_model pm ON pm.id=u.provider_model_id AND pm.provider_id=u.provider_id
WHERE u.organization_id=sqlc.arg(organization_id)
  AND u.started_at>=sqlc.arg(from_time)::timestamptz
  AND u.started_at<sqlc.arg(to_time)::timestamptz
GROUP BY pm.upstream_model_code
ORDER BY requests DESC, tokens DESC, pm.upstream_model_code
LIMIT 10;
