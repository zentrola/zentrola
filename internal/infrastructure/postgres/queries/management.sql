-- name: ManageMembers :many
SELECT * FROM principal WHERE is_deleted=false AND principal_type='MEMBER' AND (id<$1 OR $1=0) ORDER BY id DESC LIMIT $2;
-- name: ManageMemberSuggestions :many
SELECT * FROM principal
WHERE is_deleted=false
  AND principal_type='MEMBER'
  AND strpos(lower(name), lower(sqlc.arg(member_name)::text)) > 0
  AND (id<sqlc.arg(after_id) OR sqlc.arg(after_id)=0)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit);
-- name: ManageMember :one
SELECT * FROM principal WHERE id=$1 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageCreateMember :exec
INSERT INTO principal(id,principal_type,name,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,'MEMBER',$2,$3,'DISABLED',$4,$4,$5,$5);
-- name: ManageUpdateMember :exec
UPDATE principal SET name=$2,remark=$3,updated_by=$4,updated_at=$5
WHERE id=$1 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageMemberStatus :exec
UPDATE principal SET status=$2,updated_by=$3,updated_at=$4 WHERE id=$1 AND is_deleted=false AND principal_type='MEMBER';

-- name: ManageDeleteMember :exec
UPDATE principal SET is_deleted=true,status='DISABLED',updated_by=$2,updated_at=$3
WHERE id=$1 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageDeleteMemberGroups :exec
UPDATE principal_group_membership SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE principal_id=$1 AND is_deleted=false;
-- name: ManageRevokeMemberKeys :exec
UPDATE principal_access_key SET status='REVOKED',revoked_at=$3,updated_by=$2,updated_at=$3
WHERE principal_id=$1 AND is_deleted=false AND status<>'REVOKED';

-- name: ManageGroups :many
SELECT * FROM principal_group
WHERE is_deleted=false
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
  AND (id<sqlc.arg(after_id) OR sqlc.arg(after_id)=0)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit);
-- name: ManageGroup :one
SELECT * FROM principal_group WHERE id=$1 AND is_deleted=false;
-- name: ManageCreateGroup :exec
INSERT INTO principal_group(id,group_code,group_name,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,'ACTIVE',$5,$5,$6,$6);
-- name: ManageUpdateGroup :exec
UPDATE principal_group SET group_name=$2,remark=$3,updated_by=$4,updated_at=$5
WHERE id=$1 AND is_deleted=false;
-- name: ManageGroupStatus :exec
UPDATE principal_group SET status=$2,updated_by=$3,updated_at=$4
WHERE id=$1 AND is_deleted=false;
-- name: ManageDeleteGroup :exec
UPDATE principal_group SET is_deleted=true,status='DISABLED',updated_by=$2,updated_at=$3
WHERE id=$1 AND is_deleted=false;
-- name: ManageDeleteGroupMembers :exec
UPDATE principal_group_membership SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE group_id=$1 AND is_deleted=false;
-- name: ManageDeleteGroupModels :exec
UPDATE principal_group_model_permission SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE group_id=$1 AND is_deleted=false;
-- name: ManageGroupMembers :many
SELECT p.* FROM principal p JOIN principal_group_membership g ON g.principal_id=p.id
WHERE g.group_id=$1 AND g.is_deleted=false AND p.is_deleted=false AND p.principal_type='MEMBER' AND (p.id<$2 OR $2=0) ORDER BY p.id DESC LIMIT $3;
-- name: ManageMemberGroups :many
SELECT g.* FROM principal_group g JOIN principal_group_membership pg ON pg.group_id=g.id
WHERE pg.principal_id=$1 AND pg.is_deleted=false AND g.is_deleted=false AND (g.id<$2 OR $2=0) ORDER BY g.id DESC LIMIT $3;
-- name: ManageGroupModels :many
SELECT m.*,p.provider_name AS publisher_provider_name
FROM model m
JOIN principal_group_model_permission g ON g.model_id=m.id
LEFT JOIN provider p ON p.id=m.publisher_provider_id
WHERE g.group_id=$1 AND g.is_deleted=false AND m.is_deleted=false AND (m.id<$2 OR $2=0) ORDER BY m.id DESC LIMIT $3;
-- name: ManageMembershipExists :one
SELECT EXISTS(SELECT 1 FROM principal_group_membership WHERE group_id=$1 AND principal_id=$2 AND is_deleted=false);
-- name: ManageAddMember :exec
INSERT INTO principal_group_membership(id,group_id,principal_id,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$4,$5,$5);
-- name: ManageRemoveMember :exec
UPDATE principal_group_membership SET is_deleted=true,updated_by=$3,updated_at=$4 WHERE group_id=$1 AND principal_id=$2 AND is_deleted=false;
-- name: ManagePermissionExists :one
SELECT EXISTS(SELECT 1 FROM principal_group_model_permission WHERE group_id=$1 AND model_id=$2 AND is_deleted=false);
-- name: ManageGrantModel :exec
INSERT INTO principal_group_model_permission(id,group_id,model_id,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$4,$5,$5);
-- name: ManageRevokeModel :exec
UPDATE principal_group_model_permission SET is_deleted=true,updated_by=$3,updated_at=$4 WHERE group_id=$1 AND model_id=$2 AND is_deleted=false;

-- name: ManageModels :many
SELECT m.*,p.provider_name AS publisher_provider_name
FROM model m
LEFT JOIN provider p ON p.id=m.publisher_provider_id
WHERE m.is_deleted=false
  AND (sqlc.arg(status)::text = '' OR m.status = sqlc.arg(status)::text)
  AND (m.id<sqlc.arg(after_id) OR sqlc.arg(after_id)=0)
ORDER BY m.id DESC
LIMIT sqlc.arg(page_limit);
-- name: ManageModel :one
SELECT m.*,p.provider_name AS publisher_provider_name
FROM model m
LEFT JOIN provider p ON p.id=m.publisher_provider_id
WHERE m.id=$1 AND m.is_deleted=false;
-- name: ManageModelStatus :exec
UPDATE model SET status=$2,updated_by=$3,updated_at=$4 WHERE id=$1 AND is_deleted=false;
-- name: ManageCreateModel :exec
INSERT INTO model(id,model_code,display_name,input_modalities,output_modalities,remark,status,publisher_provider_id,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10,$10);
-- name: ManageUpdateModel :exec
UPDATE model SET model_code=$2,display_name=$3,input_modalities=$4,output_modalities=$5,remark=$6,publisher_provider_id=$7,updated_by=$8,updated_at=$9
WHERE id=$1 AND is_deleted=false;
-- name: ManageDeleteModelMappings :exec
UPDATE provider_model SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE model_id=$1 AND is_deleted=false;
-- name: ManageDeleteModelPermissions :exec
UPDATE principal_group_model_permission SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE model_id=$1 AND is_deleted=false;
-- name: ManageDeleteModel :exec
UPDATE model SET is_deleted=true,status='DISABLED',updated_by=$2,updated_at=$3
WHERE id=$1 AND is_deleted=false;
-- name: ManageProviders :many
SELECT * FROM provider
WHERE is_deleted=false
  AND (sqlc.arg(provider_type)::text = '' OR provider_type = sqlc.arg(provider_type)::text)
  AND (id < sqlc.arg(after_id) OR sqlc.arg(after_id) = 0)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit);
-- name: ManageProvider :one
SELECT * FROM provider WHERE id=$1 AND is_deleted=false;
-- name: ManageProviderModelCounts :many
SELECT pm.provider_id, COUNT(*)::bigint AS model_count
FROM provider_model pm
JOIN model m ON m.id=pm.model_id
WHERE pm.provider_id=ANY(sqlc.arg(provider_ids)::bigint[])
  AND pm.is_deleted=false
  AND m.is_deleted=false
  AND m.status='ACTIVE'
GROUP BY pm.provider_id
ORDER BY pm.provider_id;
-- name: ManageProviderCredentialConfigured :one
SELECT EXISTS(
    SELECT 1 FROM provider_credential
    WHERE provider_id=$1 AND is_deleted=false
);
-- name: ManageProviderEndpoints :many
SELECT * FROM provider_endpoint WHERE provider_id=$1 ORDER BY protocol_type;
-- name: ManageProviderEndpointsByProviders :many
SELECT * FROM provider_endpoint
WHERE provider_id=ANY(sqlc.arg(provider_ids)::bigint[])
ORDER BY provider_id,protocol_type;
-- name: ManageCreateProvider :exec
INSERT INTO provider(id,provider_code,provider_name,provider_type,official_website,proxy_enabled,proxy_url_display,proxy_url_ciphertext,proxy_url_nonce,proxy_url_key_version,proxy_header_names,proxy_headers_ciphertext,proxy_headers_nonce,proxy_headers_key_version,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16,$17,$17);
-- name: ManageUpdateProvider :exec
UPDATE provider SET provider_name=$2,official_website=$3,proxy_enabled=$4,proxy_url_display=$5,proxy_url_ciphertext=$6,proxy_url_nonce=$7,proxy_url_key_version=$8,proxy_header_names=$9,proxy_headers_ciphertext=$10,proxy_headers_nonce=$11,proxy_headers_key_version=$12,updated_by=$13,updated_at=$14
WHERE id=$1 AND is_deleted=false;
-- name: ManageUpsertProviderEndpoint :exec
INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,network_scope,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$5,$6,$6)
ON CONFLICT(provider_id,protocol_type) DO UPDATE
SET base_url=excluded.base_url,network_scope=excluded.network_scope,updated_by=excluded.updated_by,updated_at=excluded.updated_at;
-- name: ManageDeleteProviderEndpoint :exec
DELETE FROM provider_endpoint WHERE provider_id=$1 AND protocol_type=$2;
-- name: ManageDeleteProvider :exec
UPDATE provider
SET status='DISABLED',is_deleted=true,proxy_enabled=false,proxy_url_display=NULL,
    proxy_url_ciphertext=NULL,proxy_url_nonce=NULL,proxy_url_key_version=NULL,
    proxy_header_names='[]'::jsonb,proxy_headers_ciphertext=NULL,proxy_headers_nonce=NULL,
    proxy_headers_key_version=NULL,updated_by=$2,updated_at=$3
WHERE id=$1 AND is_deleted=false;
-- name: ManageDeleteProviderMappings :exec
UPDATE provider_model
SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE provider_id=$1 AND is_deleted=false;
-- name: ManageDeleteProviderResources :exec
UPDATE provider_credential
SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE provider_id=$1 AND is_deleted=false;
-- name: ManageProviderStatus :exec
UPDATE provider SET status=$2,updated_by=$3,updated_at=$4 WHERE id=$1 AND is_deleted=false;

-- name: ManageProviderMappings :many
SELECT * FROM provider_model
WHERE provider_id=$1 AND is_deleted=false
ORDER BY priority,id;

-- name: ManageCreateProviderMapping :exec
INSERT INTO provider_model(id,provider_id,model_id,upstream_model_code,priority,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$6,$7,$7);

-- name: ManageUpdateProviderMapping :exec
UPDATE provider_model
SET upstream_model_code=$3,priority=$4,updated_by=$5,updated_at=$6
WHERE id=$1 AND provider_id=$2 AND is_deleted=false;

-- name: ManageDeleteProviderMapping :exec
UPDATE provider_model
SET is_deleted=true,updated_by=$3,updated_at=$4
WHERE id=$1 AND provider_id=$2 AND is_deleted=false;

-- name: ManageResources :many
SELECT id,provider_id,resource_name,auth_type,auth_adapter,subscription_type,plan_code,
       external_account_ref,priority,effective_at,expires_at,quota_status,quota_checked_at,
       quota_resets_at,credential_refreshed_at,credential_expires_at,
       runtime_status,blocked_reason,blocked_at,last_error_at,last_http_status,
       last_error_code,created_at,updated_at FROM provider_credential
WHERE is_deleted=false AND (id<$1 OR $1=0) ORDER BY id DESC LIMIT $2;
-- name: ManageResource :one
SELECT * FROM provider_credential WHERE id=$1 AND is_deleted=false;
-- name: ManageCreateResource :exec
INSERT INTO provider_credential(
    id,provider_id,resource_name,auth_type,auth_adapter,subscription_type,plan_code,
    external_account_ref,priority,effective_at,expires_at,quota_status,quota_checked_at,
    quota_resets_at,credential_refreshed_at,credential_expires_at,
    credential_ciphertext,credential_nonce,key_version,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$20,$21,$21);
-- name: ManageUpdateResource :exec
UPDATE provider_credential SET resource_name=$2,auth_type=$3,auth_adapter=$4,subscription_type=$5,
plan_code=$6,external_account_ref=$7,priority=$8,effective_at=$9,expires_at=$10,
quota_status=$11,quota_checked_at=$12,quota_resets_at=$13,
credential_refreshed_at=$14,credential_expires_at=$15,
credential_ciphertext=$16,credential_nonce=$17,key_version=$18,
runtime_status=CASE WHEN credential_ciphertext IS DISTINCT FROM $16 THEN 'HEALTHY' ELSE runtime_status END,
blocked_reason=CASE WHEN credential_ciphertext IS DISTINCT FROM $16 THEN NULL ELSE blocked_reason END,
blocked_at=CASE WHEN credential_ciphertext IS DISTINCT FROM $16 THEN NULL ELSE blocked_at END,
last_error_at=CASE WHEN credential_ciphertext IS DISTINCT FROM $16 THEN NULL ELSE last_error_at END,
last_http_status=CASE WHEN credential_ciphertext IS DISTINCT FROM $16 THEN NULL ELSE last_http_status END,
last_error_code=CASE WHEN credential_ciphertext IS DISTINCT FROM $16 THEN NULL ELSE last_error_code END,
updated_by=$19,updated_at=$20
WHERE id=$1 AND is_deleted=false;

-- name: ManageDeleteResource :execrows
UPDATE provider_credential
SET is_deleted=true,updated_by=$2,updated_at=$3
WHERE id=$1 AND is_deleted=false;

-- name: ManageDeleteResourceQuotas :exec
DELETE FROM provider_credential_quota WHERE provider_credential_id=$1;

-- name: ManageCreateResourceQuota :exec
INSERT INTO provider_credential_quota(
    provider_credential_id,quota_code,quota_name,quota_status,quota_unit,limit_value,
    used_value,remaining_value,used_percent,window_duration_seconds,resets_at,reached_type,
    observed_at,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13,$13);

-- name: ManageResourceQuotas :many
SELECT * FROM provider_credential_quota
WHERE provider_credential_id=$1 ORDER BY quota_code;

-- name: ManageRestoreResourceRuntime :execrows
UPDATE provider_credential
SET runtime_status='HEALTHY',blocked_reason=NULL,blocked_at=NULL,last_error_at=NULL,
    last_http_status=NULL,last_error_code=NULL,updated_by=$2,updated_at=$3
WHERE id=$1 AND is_deleted=false AND runtime_status='BLOCKED';

-- name: ManageBlockResourceRuntime :exec
UPDATE provider_credential
SET runtime_status='BLOCKED',blocked_reason=$2,blocked_at=$3,last_error_at=$3,
    last_http_status=$4,last_error_code=$5,updated_by=$6,updated_at=$3
WHERE id=$1 AND is_deleted=false;

-- name: ManageKeys :many
SELECT id,name,masked_key,status,expires_at,revoked_at,created_at FROM principal_access_key
WHERE principal_id=$1 AND is_deleted=false AND (id<$2 OR $2=0) ORDER BY id DESC LIMIT $3;
-- name: ManageOperations :many
SELECT id,operator_name,operation_type,target_type,target_id,target_name,request_id,result,error_code,before_data,after_data,created_at
FROM operation_log WHERE (id<$1 OR $1=0) ORDER BY id DESC LIMIT $2;

-- 分页总数不受 after cursor 影响；过滤口径必须与对应列表查询保持一致。
-- name: CountManageMembers :one
SELECT COUNT(*)::bigint FROM principal
WHERE is_deleted=false AND principal_type='MEMBER';

-- name: CountManageMemberSuggestions :one
SELECT COUNT(*)::bigint FROM principal
WHERE is_deleted=false
  AND principal_type='MEMBER'
  AND strpos(lower(name), lower(sqlc.arg(member_name)::text)) > 0;

-- name: CountManageGroups :one
SELECT COUNT(*)::bigint FROM principal_group
WHERE is_deleted=false
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text);

-- name: CountManageModels :one
SELECT COUNT(*)::bigint FROM model
WHERE is_deleted=false
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text);

-- name: CountManageProviders :one
SELECT COUNT(*)::bigint FROM provider
WHERE is_deleted=false
  AND (sqlc.arg(provider_type)::text = '' OR provider_type = sqlc.arg(provider_type)::text);

-- name: CountManageResources :one
SELECT COUNT(*)::bigint FROM provider_credential
WHERE is_deleted=false;

-- name: CountManageKeys :one
SELECT COUNT(*)::bigint FROM principal_access_key
WHERE principal_id=$1 AND is_deleted=false;

-- name: CountManageOperations :one
SELECT COUNT(*)::bigint FROM operation_log;

-- name: CountManageGroupMembers :one
SELECT COUNT(*)::bigint
FROM principal p
JOIN principal_group_membership g
  ON g.principal_id=p.id
WHERE g.group_id=$1 AND g.is_deleted=false
  AND p.is_deleted=false AND p.principal_type='MEMBER';

-- name: CountManageMemberGroups :one
SELECT COUNT(*)::bigint
FROM principal_group g
JOIN principal_group_membership pg
  ON pg.group_id=g.id
WHERE pg.principal_id=$1
  AND pg.is_deleted=false AND g.is_deleted=false;

-- name: CountManageGroupModels :one
SELECT COUNT(*)::bigint
FROM model m
JOIN principal_group_model_permission g ON g.model_id=m.id
WHERE g.group_id=$1
  AND g.is_deleted=false AND m.is_deleted=false;
