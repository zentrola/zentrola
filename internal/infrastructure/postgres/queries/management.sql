-- name: LockManagementOrganization :one
SELECT o.id FROM organization o JOIN admin_user a ON a.organization_id=o.id
WHERE o.id=$1 AND a.id=$2 AND o.is_deleted=false AND o.status='ACTIVE'
AND a.is_deleted=false AND a.status='ACTIVE' FOR UPDATE OF o FOR SHARE OF a;

-- name: ManageMembers :many
SELECT * FROM principal WHERE organization_id=$1 AND is_deleted=false AND principal_type='MEMBER' AND (id<$2 OR $2=0) ORDER BY id DESC LIMIT $3;
-- name: ManageMember :one
SELECT * FROM principal WHERE organization_id=$1 AND id=$2 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageCreateMember :exec
INSERT INTO principal(id,organization_id,principal_type,name,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,'MEMBER',$3,$4,'DISABLED',$5,$5,$6,$6);
-- name: ManageUpdateMember :exec
UPDATE principal SET name=$3,remark=$4,updated_by=$5,updated_at=$6
WHERE organization_id=$1 AND id=$2 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageMemberStatus :exec
UPDATE principal SET status=$3,updated_by=$4,updated_at=$5 WHERE organization_id=$1 AND id=$2 AND is_deleted=false AND principal_type='MEMBER';

-- name: ManageDeleteMember :exec
UPDATE principal SET is_deleted=true,status='DISABLED',updated_by=$3,updated_at=$4
WHERE organization_id=$1 AND id=$2 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageDeleteMemberGroups :exec
UPDATE principal_group SET is_deleted=true,updated_by=$3,updated_at=$4
WHERE organization_id=$1 AND principal_id=$2 AND is_deleted=false;
-- name: ManageRevokeMemberKeys :exec
UPDATE access_key SET status='REVOKED',revoked_at=$4,updated_by=$3,updated_at=$4
WHERE organization_id=$1 AND principal_id=$2 AND is_deleted=false AND status<>'REVOKED';

-- name: ManageGroups :many
SELECT * FROM ai_group
WHERE organization_id=sqlc.arg(organization_id)
  AND is_deleted=false
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
  AND (id<sqlc.arg(after_id) OR sqlc.arg(after_id)=0)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit);
-- name: ManageGroup :one
SELECT * FROM ai_group WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageCreateGroup :exec
INSERT INTO ai_group(id,organization_id,group_code,group_name,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,'ACTIVE',$6,$6,$7,$7);
-- name: ManageUpdateGroup :exec
UPDATE ai_group SET group_name=$3,remark=$4,updated_by=$5,updated_at=$6
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageGroupStatus :exec
UPDATE ai_group SET status=$3,updated_by=$4,updated_at=$5
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageDeleteGroup :exec
UPDATE ai_group SET is_deleted=true,status='DISABLED',updated_by=$3,updated_at=$4
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageDeleteGroupMembers :exec
UPDATE principal_group SET is_deleted=true,updated_by=$3,updated_at=$4
WHERE organization_id=$1 AND group_id=$2 AND is_deleted=false;
-- name: ManageDeleteGroupModels :exec
UPDATE group_model_permission SET is_deleted=true,updated_by=$3,updated_at=$4
WHERE organization_id=$1 AND group_id=$2 AND is_deleted=false;
-- name: ManageGroupMembers :many
SELECT p.* FROM principal p JOIN principal_group g ON g.principal_id=p.id AND g.organization_id=p.organization_id
WHERE p.organization_id=$1 AND g.group_id=$2 AND g.is_deleted=false AND p.is_deleted=false AND p.principal_type='MEMBER' AND (p.id<$3 OR $3=0) ORDER BY p.id DESC LIMIT $4;
-- name: ManageMemberGroups :many
SELECT g.* FROM ai_group g JOIN principal_group pg ON pg.group_id=g.id AND pg.organization_id=g.organization_id
WHERE g.organization_id=$1 AND pg.principal_id=$2 AND pg.is_deleted=false AND g.is_deleted=false AND (g.id<$3 OR $3=0) ORDER BY g.id DESC LIMIT $4;
-- name: ManageGroupModels :many
SELECT m.* FROM ai_model m JOIN group_model_permission g ON g.model_id=m.id
WHERE g.organization_id=$1 AND g.group_id=$2 AND g.is_deleted=false AND m.is_deleted=false AND (m.id<$3 OR $3=0) ORDER BY m.id DESC LIMIT $4;
-- name: ManageMembershipExists :one
SELECT EXISTS(SELECT 1 FROM principal_group WHERE organization_id=$1 AND group_id=$2 AND principal_id=$3 AND is_deleted=false);
-- name: ManageAddMember :exec
INSERT INTO principal_group(id,organization_id,group_id,principal_id,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$5,$6,$6);
-- name: ManageRemoveMember :exec
UPDATE principal_group SET is_deleted=true,updated_by=$4,updated_at=$5 WHERE organization_id=$1 AND group_id=$2 AND principal_id=$3 AND is_deleted=false;
-- name: ManagePermissionExists :one
SELECT EXISTS(SELECT 1 FROM group_model_permission WHERE organization_id=$1 AND group_id=$2 AND model_id=$3 AND is_deleted=false);
-- name: ManageGrantModel :exec
INSERT INTO group_model_permission(id,organization_id,group_id,model_id,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$5,$6,$6);
-- name: ManageRevokeModel :exec
UPDATE group_model_permission SET is_deleted=true,updated_by=$4,updated_at=$5 WHERE organization_id=$1 AND group_id=$2 AND model_id=$3 AND is_deleted=false;

-- name: ManageModels :many
SELECT * FROM ai_model
WHERE is_deleted=false
  AND (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
  AND (id<sqlc.arg(after_id) OR sqlc.arg(after_id)=0)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit);
-- name: ManageModel :one
SELECT * FROM ai_model WHERE id=$1 AND is_deleted=false;
-- name: ManageModelStatus :exec
UPDATE ai_model SET status=$2,updated_by=$3,updated_at=$4 WHERE id=$1 AND is_deleted=false;
-- name: ManageCreateModel :exec
INSERT INTO ai_model(id,model_code,display_name,input_modalities,output_modalities,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,'DISABLED',$7,$7,$8,$8);
-- name: ManageUpdateModel :exec
UPDATE ai_model SET model_code=$2,display_name=$3,input_modalities=$4,output_modalities=$5,remark=$6,updated_by=$7,updated_at=$8
WHERE id=$1 AND is_deleted=false;
-- name: ManageProviders :many
SELECT * FROM ai_provider WHERE is_deleted=false AND (id<$1 OR $1=0) ORDER BY id DESC LIMIT $2;
-- name: ManageProvider :one
SELECT * FROM ai_provider WHERE id=$1 AND is_deleted=false;
-- name: ManageProviderEndpoints :many
SELECT * FROM provider_endpoint WHERE provider_id=$1 ORDER BY protocol_type;
-- name: ManageCreateProvider :exec
INSERT INTO ai_provider(id,provider_code,provider_name,provider_type,official_website,proxy_enabled,proxy_url_display,proxy_url_ciphertext,proxy_url_nonce,proxy_url_key_version,proxy_header_names,proxy_headers_ciphertext,proxy_headers_nonce,proxy_headers_key_version,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16,$17,$17);
-- name: ManageUpdateProvider :exec
UPDATE ai_provider SET provider_name=$2,official_website=$3,proxy_enabled=$4,proxy_url_display=$5,proxy_url_ciphertext=$6,proxy_url_nonce=$7,proxy_url_key_version=$8,proxy_header_names=$9,proxy_headers_ciphertext=$10,proxy_headers_nonce=$11,proxy_headers_key_version=$12,updated_by=$13,updated_at=$14
WHERE id=$1 AND is_deleted=false;
-- name: ManageUpsertProviderEndpoint :exec
INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$4,$5,$5)
ON CONFLICT(provider_id,protocol_type) DO UPDATE
SET base_url=excluded.base_url,updated_by=excluded.updated_by,updated_at=excluded.updated_at;
-- name: ManageDeleteProviderEndpoint :exec
DELETE FROM provider_endpoint WHERE provider_id=$1 AND protocol_type=$2;
-- name: ManageDeleteProvider :exec
UPDATE ai_provider
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
UPDATE ai_resource
SET status='DISABLED',is_deleted=true,updated_by=$2,updated_at=$3
WHERE provider_id=$1 AND is_deleted=false;
-- name: ManageProviderStatus :exec
UPDATE ai_provider SET status=$2,updated_by=$3,updated_at=$4 WHERE id=$1 AND is_deleted=false;

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
SELECT id,provider_id,resource_name,status,created_at,updated_at FROM ai_resource
WHERE organization_id=$1 AND is_deleted=false AND (id<$2 OR $2=0) ORDER BY id DESC LIMIT $3;
-- name: ManageResource :one
SELECT * FROM ai_resource WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageCreateResource :exec
INSERT INTO ai_resource(id,organization_id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10,$10);
-- name: ManageUpdateResource :exec
UPDATE ai_resource SET resource_name=$3,credential_ciphertext=$4,credential_nonce=$5,key_version=$6,status=$7,updated_by=$8,updated_at=$9
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;

-- name: ManageKeys :many
SELECT id,name,masked_key,status,expires_at,revoked_at,created_at FROM access_key
WHERE organization_id=$1 AND principal_id=$2 AND is_deleted=false AND (id<$3 OR $3=0) ORDER BY id DESC LIMIT $4;
-- name: ManageOperations :many
SELECT id,operator_name,operation_type,target_type,target_id,request_id,result,error_code,before_data,after_data,created_at
FROM operation_log WHERE organization_id=$1 AND (id<$2 OR $2=0) ORDER BY id DESC LIMIT $3;
