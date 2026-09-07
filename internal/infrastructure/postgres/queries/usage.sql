-- name: InsertUsageRequests :exec
INSERT INTO ai_request(id,organization_id,request_id,principal_id,model_id,usage_scene,client_protocol,request_at,completed_at,latency_ms,status,error_type,created_at)
SELECT id,organization_id,request_id,principal_id,model_id,'MODEL_GATEWAY',client_protocol,request_at,completed_at,latency_ms,status,error_type,completed_at
FROM jsonb_to_recordset(sqlc.arg(payload)::jsonb) AS x(id bigint,organization_id bigint,request_id text,principal_id bigint,model_id bigint,client_protocol text,request_at timestamptz,completed_at timestamptz,latency_ms bigint,status text,error_type text)
ON CONFLICT(request_id) DO NOTHING;

-- name: InsertUsageAttempts :exec
INSERT INTO usage_record(id,organization_id,request_id,attempt_no,principal_id,provider_id,provider_model_id,resource_id,model_id,usage_scene,input_tokens,output_tokens,cached_input_tokens,billing_unit,started_at,completed_at,latency_ms,status,error_type,created_at)
SELECT id,organization_id,request_id,1,principal_id,provider_id,provider_model_id,resource_id,model_id,'MODEL_GATEWAY',input_tokens,output_tokens,cached_input_tokens,'TOKEN',started_at,completed_at,latency_ms,status,error_type,completed_at
FROM jsonb_to_recordset(sqlc.arg(payload)::jsonb) AS x(id bigint,organization_id bigint,request_id text,principal_id bigint,provider_id bigint,provider_model_id bigint,resource_id bigint,model_id bigint,input_tokens bigint,output_tokens bigint,cached_input_tokens bigint,started_at timestamptz,completed_at timestamptz,latency_ms bigint,status text,error_type text)
ON CONFLICT(request_id,attempt_no) DO NOTHING;

-- name: QueryUsage :many
SELECT r.id,r.request_id,r.client_protocol,r.principal_id,r.model_id,r.request_at,r.completed_at,r.latency_ms,r.status,r.error_type,
u.id AS usage_id,u.attempt_no,u.provider_id,u.provider_model_id,u.resource_id,u.input_tokens,u.output_tokens,u.cached_input_tokens,u.status AS attempt_status,u.error_type AS attempt_error_type
FROM ai_request r LEFT JOIN usage_record u ON u.request_id=r.request_id AND u.organization_id=r.organization_id
WHERE r.organization_id=sqlc.arg(organization_id)
AND (r.id<sqlc.arg(after_id)::bigint OR sqlc.arg(after_id)::bigint=0)
AND r.request_at>=sqlc.arg(from_time)::timestamptz AND r.request_at<sqlc.arg(to_time)::timestamptz
AND (sqlc.narg(principal_id)::bigint IS NULL OR r.principal_id=sqlc.narg(principal_id))
AND (sqlc.narg(model_id)::bigint IS NULL OR r.model_id=sqlc.narg(model_id))
AND (sqlc.narg(resource_id)::bigint IS NULL OR u.resource_id=sqlc.narg(resource_id))
ORDER BY r.id DESC LIMIT sqlc.arg(page_limit)::int;
