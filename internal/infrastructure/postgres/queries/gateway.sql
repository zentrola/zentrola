-- name: GatewayIdentityActive :one
SELECT EXISTS (SELECT 1 FROM principal_access_key k
JOIN principal p ON p.id=k.principal_id
WHERE k.id=sqlc.arg(access_key_id) AND k.principal_id=sqlc.arg(principal_id)
AND NOT k.is_deleted AND k.status='ACTIVE' AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())
AND NOT p.is_deleted AND p.status='ACTIVE' AND p.principal_type='MEMBER');

-- name: GatewayModel :one
SELECT id,model_code,display_name,status FROM model WHERE model_code=$1 AND NOT is_deleted;

-- name: GatewayCandidates :many
SELECT pm.id AS provider_model_id,pm.provider_id,p.provider_name,pm.upstream_model_code,pe.base_url,pe.protocol_type,pe.network_scope,
       p.proxy_enabled,p.proxy_url_ciphertext,p.proxy_url_nonce,p.proxy_url_key_version,
       p.proxy_headers_ciphertext,p.proxy_headers_nonce,p.proxy_headers_key_version,
       r.id AS resource_id,r.auth_type,r.auth_adapter,r.subscription_type,r.priority AS resource_priority,r.expires_at,
       r.quota_status,r.quota_checked_at,r.quota_resets_at,r.credential_refreshed_at,r.credential_expires_at,
       r.credential_ciphertext,r.credential_nonce,r.key_version
FROM provider_model pm
JOIN provider p ON p.id=pm.provider_id
JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type IN ('OPENAI','ANTHROPIC')
JOIN provider_credential r ON r.provider_id=p.id
    AND NOT r.is_deleted AND r.runtime_status='HEALTHY'
    AND (r.effective_at IS NULL OR r.effective_at<=now())
    AND (r.expires_at IS NULL OR r.expires_at>now())
    AND (r.auth_type='API_KEY' OR r.quota_status IN ('AVAILABLE','NEAR_LIMIT')
         OR (r.quota_status='EXHAUSTED' AND r.quota_resets_at IS NOT NULL AND r.quota_resets_at<=now()))
WHERE pm.model_id=sqlc.arg(model_id) AND NOT pm.is_deleted
  AND NOT p.is_deleted AND p.status='ACTIVE'
ORDER BY pm.priority,
    CASE WHEN pe.protocol_type=sqlc.arg(preferred_protocol) THEN 0 ELSE 1 END,
    CASE WHEN r.auth_type='SUBSCRIPTION' THEN 0 ELSE 1 END,
    r.priority,pm.id,r.id;

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
    last_error_code=sqlc.arg(error_code),updated_by='system',updated_at=sqlc.arg(blocked_at),version=version+1
WHERE id=sqlc.arg(resource_id)
  AND NOT is_deleted AND runtime_status='HEALTHY';

-- name: UpdateGatewayResourceCredential :execrows
UPDATE provider_credential
SET credential_ciphertext=sqlc.arg(credential_ciphertext),credential_nonce=sqlc.arg(credential_nonce),
    key_version=sqlc.arg(key_version),credential_refreshed_at=sqlc.narg(credential_refreshed_at),
    credential_expires_at=sqlc.narg(credential_expires_at),updated_by='system',updated_at=sqlc.arg(updated_at),version=version+1
WHERE id=sqlc.arg(resource_id) AND provider_id=sqlc.arg(provider_id) AND NOT is_deleted;

-- name: UpdateGatewayResourceCredentialRefreshMetadata :execrows
UPDATE provider_credential
SET credential_refreshed_at=sqlc.narg(credential_refreshed_at),
    credential_expires_at=sqlc.narg(credential_expires_at),updated_by='system',updated_at=sqlc.arg(updated_at),version=version+1
WHERE id=sqlc.arg(resource_id) AND provider_id=sqlc.arg(provider_id) AND NOT is_deleted
  AND credential_expires_at IS NULL;

-- name: GatewayResourceCredential :one
SELECT credential_ciphertext,credential_nonce,key_version
FROM provider_credential
WHERE id=sqlc.arg(resource_id) AND provider_id=sqlc.arg(provider_id)
  AND NOT is_deleted AND auth_type='SUBSCRIPTION';

-- name: LockGatewaySubscriptionRefresh :one
SELECT pg_advisory_lock(-(sqlc.arg(resource_id)::bigint));

-- name: UnlockGatewaySubscriptionRefresh :one
SELECT pg_advisory_unlock(-(sqlc.arg(resource_id)::bigint));

-- name: ListGatewaySubscriptionCredentials :many
SELECT r.id AS resource_id,r.provider_id,r.auth_adapter,r.credential_refreshed_at,r.credential_expires_at,
       p.proxy_enabled,p.proxy_url_ciphertext,p.proxy_url_nonce,p.proxy_url_key_version,
       p.proxy_headers_ciphertext,p.proxy_headers_nonce,p.proxy_headers_key_version,
       r.credential_ciphertext,r.credential_nonce,r.key_version
FROM provider_credential r
JOIN provider p ON p.id=r.provider_id
WHERE r.id>sqlc.arg(after_id)
  AND NOT r.is_deleted AND r.auth_type='SUBSCRIPTION' AND r.subscription_type='PERSONAL'
  AND r.runtime_status='HEALTHY'
  AND (r.effective_at IS NULL OR r.effective_at<=now())
  AND (r.expires_at IS NULL OR r.expires_at>now())
  AND (r.credential_expires_at IS NULL OR r.credential_expires_at<=sqlc.arg(refresh_before)::timestamptz)
  AND NOT p.is_deleted AND p.status='ACTIVE'
ORDER BY r.id
LIMIT sqlc.arg(page_limit);

-- name: OpenAIModels :many
SELECT DISTINCT ON (m.model_code) m.model_code,m.created_at,p.provider_code FROM model m
JOIN provider_model pm ON pm.model_id=m.id AND NOT pm.is_deleted
JOIN provider p ON p.id=pm.provider_id AND NOT p.is_deleted AND p.status='ACTIVE'
JOIN provider_endpoint pe ON pe.provider_id=p.id AND pe.protocol_type IN ('OPENAI','ANTHROPIC')
WHERE NOT m.is_deleted AND m.status='ACTIVE'
AND EXISTS(SELECT 1 FROM provider_credential r WHERE r.provider_id=p.id AND NOT r.is_deleted AND r.runtime_status='HEALTHY'
    AND (r.effective_at IS NULL OR r.effective_at<=now()) AND (r.expires_at IS NULL OR r.expires_at>now())
    AND (r.auth_type='API_KEY' OR r.quota_status IN ('AVAILABLE','NEAR_LIMIT')
         OR (r.quota_status='EXHAUSTED' AND r.quota_resets_at IS NOT NULL AND r.quota_resets_at<=now())))
AND EXISTS(SELECT 1 FROM principal_group_membership pg JOIN principal_group g ON g.id=pg.group_id
JOIN principal_group_model_permission gp ON gp.group_id=g.id
WHERE pg.principal_id=sqlc.arg(principal_id)
AND NOT pg.is_deleted AND NOT g.is_deleted AND g.status='ACTIVE' AND NOT gp.is_deleted AND gp.model_id=m.id)
ORDER BY m.model_code,CASE pe.protocol_type WHEN 'OPENAI' THEN 0 ELSE 1 END,pm.priority,pm.id;
