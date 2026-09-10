-- name: GatewayIdentityActive :one
SELECT EXISTS (SELECT 1 FROM principal_access_key k
JOIN principal p ON p.id=k.principal_id
WHERE k.id=sqlc.arg(access_key_id) AND k.principal_id=sqlc.arg(principal_id)
AND NOT k.is_deleted AND k.status='ACTIVE' AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())
AND NOT p.is_deleted AND p.status='ACTIVE' AND p.principal_type='MEMBER');

-- name: GatewayModel :one
SELECT id,status FROM model WHERE model_code=$1 AND NOT is_deleted;

-- name: GatewayCandidates :many
SELECT pm.id AS provider_model_id,pm.provider_id,pm.upstream_model_code,pe.base_url,pe.protocol_type,
       p.proxy_enabled,p.proxy_url_ciphertext,p.proxy_url_nonce,p.proxy_url_key_version,
       p.proxy_headers_ciphertext,p.proxy_headers_nonce,p.proxy_headers_key_version,
       r.id AS resource_id,r.credential_ciphertext,r.credential_nonce,r.key_version
FROM provider_model pm
JOIN provider p ON p.id=pm.provider_id
JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type IN ('OPENAI','ANTHROPIC')
JOIN provider_credential r ON r.provider_id=p.id
    AND NOT r.is_deleted AND r.status='ACTIVE' AND r.runtime_status='HEALTHY'
WHERE pm.model_id=sqlc.arg(model_id) AND NOT pm.is_deleted
  AND NOT p.is_deleted AND p.status='ACTIVE'
ORDER BY pm.priority,
    CASE WHEN pe.protocol_type=sqlc.arg(preferred_protocol) THEN 0 ELSE 1 END,
    pm.id,r.id;

-- name: GatewayRouteExists :one
SELECT EXISTS(
    SELECT 1
    FROM provider_model pm
    JOIN provider p ON p.id=pm.provider_id
    JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type IN ('OPENAI','ANTHROPIC')
    WHERE pm.model_id=sqlc.arg(model_id) AND NOT pm.is_deleted
      AND NOT p.is_deleted AND p.status='ACTIVE'
);

-- name: BlockGatewayResource :execrows
UPDATE provider_credential
SET runtime_status='BLOCKED',blocked_reason=sqlc.arg(blocked_reason),blocked_at=sqlc.arg(blocked_at),
    last_error_at=sqlc.arg(blocked_at),last_http_status=sqlc.narg(http_status),
    last_error_code=sqlc.arg(error_code),updated_by='system',updated_at=sqlc.arg(blocked_at)
WHERE id=sqlc.arg(resource_id)
  AND NOT is_deleted AND status='ACTIVE' AND runtime_status='HEALTHY';

-- name: OpenAIModels :many
SELECT DISTINCT ON (m.model_code) m.model_code,m.created_at,p.provider_code FROM model m
JOIN provider_model pm ON pm.model_id=m.id AND NOT pm.is_deleted
JOIN provider p ON p.id=pm.provider_id AND NOT p.is_deleted AND p.status='ACTIVE'
JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type IN ('OPENAI','ANTHROPIC')
WHERE NOT m.is_deleted AND m.status='ACTIVE'
AND EXISTS(SELECT 1 FROM provider_credential r WHERE r.provider_id=p.id AND NOT r.is_deleted AND r.status='ACTIVE' AND r.runtime_status='HEALTHY')
AND EXISTS(SELECT 1 FROM principal_group_membership pg JOIN principal_group g ON g.id=pg.group_id
JOIN principal_group_model_permission gp ON gp.group_id=g.id
WHERE pg.principal_id=sqlc.arg(principal_id)
AND NOT pg.is_deleted AND NOT g.is_deleted AND g.status='ACTIVE' AND NOT gp.is_deleted AND gp.model_id=m.id)
ORDER BY m.model_code,CASE pe.protocol_type WHEN 'OPENAI' THEN 0 ELSE 1 END,pm.priority,pm.id;
