-- name: GatewayIdentityActive :one
SELECT EXISTS (SELECT 1 FROM access_key k
JOIN principal p ON p.id=k.principal_id AND p.organization_id=k.organization_id
JOIN organization o ON o.id=k.organization_id
WHERE k.id=sqlc.arg(access_key_id) AND k.organization_id=sqlc.arg(organization_id) AND k.principal_id=sqlc.arg(principal_id)
AND NOT k.is_deleted AND k.status='ACTIVE' AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())
AND NOT p.is_deleted AND p.status='ACTIVE' AND p.principal_type='MEMBER' AND NOT o.is_deleted AND o.status='ACTIVE');

-- name: GatewayModel :one
SELECT id,status FROM ai_model WHERE model_code=$1 AND NOT is_deleted;

-- name: GatewayMappings :many
SELECT pm.id,pm.provider_id,pm.upstream_model_code,pe.base_url,
       p.proxy_enabled,p.proxy_url_ciphertext,p.proxy_url_nonce,p.proxy_url_key_version,
       p.proxy_headers_ciphertext,p.proxy_headers_nonce,p.proxy_headers_key_version
FROM provider_model pm
JOIN ai_provider p ON p.id=pm.provider_id
JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type=$2
WHERE pm.model_id=$1 AND NOT pm.is_deleted
  AND NOT p.is_deleted AND p.status='ACTIVE'
ORDER BY pm.priority,pm.id LIMIT 1;

-- name: GatewayResources :many
SELECT * FROM ai_resource WHERE organization_id=$1 AND provider_id=$2 AND NOT is_deleted AND status='ACTIVE' ORDER BY id LIMIT 2;

-- name: OpenAIModels :many
SELECT DISTINCT ON (m.model_code) m.model_code,m.created_at,p.provider_code FROM ai_model m
JOIN provider_model pm ON pm.model_id=m.id AND NOT pm.is_deleted
JOIN ai_provider p ON p.id=pm.provider_id AND NOT p.is_deleted AND p.status='ACTIVE'
JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type IN ('OPENAI_CHAT','OPENAI_RESPONSES')
WHERE NOT m.is_deleted AND m.status='ACTIVE'
AND EXISTS(SELECT 1 FROM ai_resource r WHERE r.provider_id=p.id AND r.organization_id=sqlc.arg(organization_id) AND NOT r.is_deleted AND r.status='ACTIVE')
AND EXISTS(SELECT 1 FROM principal_group pg JOIN ai_group g ON g.id=pg.group_id AND g.organization_id=pg.organization_id
JOIN group_model_permission gp ON gp.group_id=g.id AND gp.organization_id=g.organization_id
WHERE pg.principal_id=sqlc.arg(principal_id) AND pg.organization_id=sqlc.arg(organization_id)
AND NOT pg.is_deleted AND NOT g.is_deleted AND g.status='ACTIVE' AND NOT gp.is_deleted AND gp.model_id=m.id)
ORDER BY m.model_code,pm.priority,pm.id;
