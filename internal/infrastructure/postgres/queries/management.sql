-- name: LockManagementOrganization :one
SELECT o.id FROM organization o JOIN admin_user a ON a.organization_id=o.id
WHERE o.id=$1 AND a.id=$2 AND o.is_deleted=false AND o.status='ACTIVE'
AND a.is_deleted=false AND a.status='ACTIVE' FOR UPDATE OF o FOR SHARE OF a;

-- name: ManageMembers :many
SELECT * FROM principal WHERE organization_id=$1 AND is_deleted=false AND principal_type='MEMBER' AND id>$2 ORDER BY id LIMIT $3;
-- name: ManageMember :one
SELECT * FROM principal WHERE organization_id=$1 AND id=$2 AND is_deleted=false AND principal_type='MEMBER';
-- name: ManageCreateMember :exec
INSERT INTO principal(id,organization_id,principal_type,name,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,'MEMBER',$3,$4,'ACTIVE',$5,$5,$6,$6);
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
SELECT * FROM ai_group WHERE organization_id=$1 AND is_deleted=false AND id>$2 ORDER BY id LIMIT $3;
-- name: ManageGroup :one
SELECT * FROM ai_group WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageCreateGroup :exec
INSERT INTO ai_group(id,organization_id,group_code,group_name,remark,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,'ACTIVE',$6,$6,$7,$7);
-- name: ManageGroupMembers :many
SELECT p.* FROM principal p JOIN principal_group g ON g.principal_id=p.id AND g.organization_id=p.organization_id
WHERE p.organization_id=$1 AND g.group_id=$2 AND g.is_deleted=false AND p.is_deleted=false AND p.principal_type='MEMBER' AND p.id>$3 ORDER BY p.id LIMIT $4;
-- name: ManageGroupModels :many
SELECT m.* FROM ai_model m JOIN group_model_permission g ON g.model_id=m.id
WHERE g.organization_id=$1 AND g.group_id=$2 AND g.is_deleted=false AND m.is_deleted=false AND m.id>$3 ORDER BY m.id LIMIT $4;
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
SELECT * FROM ai_model WHERE is_deleted=false AND id>$1 ORDER BY id LIMIT $2;
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
SELECT * FROM ai_provider WHERE is_deleted=false AND provider_code IN ('anthropic-official','deepseek-official') AND provider_type='OFFICIAL' AND id>$1 ORDER BY id LIMIT $2;
-- name: ManageProvider :one
SELECT * FROM ai_provider WHERE id=$1 AND is_deleted=false AND provider_code IN ('anthropic-official','deepseek-official') AND provider_type='OFFICIAL';

-- name: ManageResources :many
SELECT id,provider_id,resource_name,status,created_at,updated_at FROM ai_resource
WHERE organization_id=$1 AND is_deleted=false AND id>$2 ORDER BY id LIMIT $3;
-- name: ManageResource :one
SELECT * FROM ai_resource WHERE organization_id=$1 AND id=$2 AND is_deleted=false;
-- name: ManageCreateResource :exec
INSERT INTO ai_resource(id,organization_id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$10,$10);
-- name: ManageUpdateResource :exec
UPDATE ai_resource SET resource_name=$3,credential_ciphertext=$4,credential_nonce=$5,key_version=$6,status=$7,updated_by=$8,updated_at=$9
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;

-- name: ManageKeys :many
SELECT id,name,key_prefix,status,expires_at,revoked_at,created_at FROM access_key
WHERE organization_id=$1 AND principal_id=$2 AND is_deleted=false AND id>$3 ORDER BY id LIMIT $4;
-- name: ManageOperations :many
SELECT id,operator_name,operation_type,target_type,target_id,request_id,result,error_code,before_data,after_data,created_at
FROM operation_log WHERE organization_id=$1 AND id>$2 ORDER BY id LIMIT $3;
