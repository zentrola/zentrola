-- name: HasAnyAdmin :one
SELECT EXISTS (SELECT 1 FROM admin_user);

-- name: HasActiveAdmin :one
SELECT EXISTS (SELECT 1 FROM admin_user a JOIN organization o ON o.id=a.organization_id
WHERE a.is_deleted=false AND a.status='ACTIVE' AND o.is_deleted=false AND o.status='ACTIVE');

-- name: GetActiveOrganization :one
SELECT * FROM organization WHERE is_deleted=false AND status='ACTIVE';

-- name: CreateInitialAdmin :exec
INSERT INTO admin_user (id,organization_id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,$4,$3,'ACTIVE','system','system',$5,$5);

-- name: GetAdminForLogin :one
SELECT a.*, (o.status='ACTIVE') AS organization_active
FROM admin_user a JOIN organization o ON o.id=a.organization_id
WHERE a.username=$1 AND a.is_deleted=false AND o.is_deleted=false
FOR UPDATE OF a;

-- name: SaveAdminLoginState :exec
UPDATE admin_user SET failed_login_count=$3,locked_until=$4,last_login_at=$5,updated_by=$6,updated_at=$7
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;

-- name: GetActiveAdmin :one
SELECT a.id,a.organization_id,a.username,a.display_name,a.credential_version FROM admin_user a JOIN organization o ON o.id=a.organization_id
WHERE a.organization_id=$1 AND a.id=$2 AND a.is_deleted=false AND a.status='ACTIVE'
AND o.is_deleted=false AND o.status='ACTIVE';

-- name: ResetAdminPassword :execrows
UPDATE admin_user SET password_hash=$3,failed_login_count=0,locked_until=NULL,
credential_version=credential_version+1,updated_by=$5,updated_at=$4
WHERE organization_id=$1 AND id=$2 AND is_deleted=false AND status='ACTIVE';

-- name: GetMemberForKey :one
SELECT p.* FROM principal p JOIN organization o ON o.id=p.organization_id
WHERE p.organization_id=$1 AND p.id=$2 AND p.principal_type='MEMBER' AND p.is_deleted=false
AND o.status='ACTIVE' AND o.is_deleted=false FOR UPDATE OF p;

-- name: CreateAccessKey :exec
INSERT INTO principal_access_key (id,organization_id,principal_id,key_hash,masked_key,name,status,expires_at,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE',$7,$8,$8,$9,$9);

-- name: GetKeyForRevoke :one
SELECT * FROM principal_access_key WHERE organization_id=$1 AND id=$2 AND is_deleted=false FOR UPDATE;

-- name: RevokeAccessKey :exec
UPDATE principal_access_key SET status='REVOKED',revoked_at=$3,updated_by=$4,updated_at=$3
WHERE organization_id=$1 AND id=$2 AND is_deleted=false;

-- name: AuthenticateAccessKey :one
SELECT k.id,k.organization_id,k.principal_id FROM principal_access_key k
JOIN principal p ON p.id=k.principal_id AND p.organization_id=k.organization_id
JOIN organization o ON o.id=k.organization_id
WHERE k.key_hash=$1 AND k.is_deleted=false AND k.status='ACTIVE' AND k.revoked_at IS NULL
AND (k.expires_at IS NULL OR k.expires_at > sqlc.arg(now)::timestamptz)
AND p.is_deleted=false AND p.status='ACTIVE' AND p.principal_type='MEMBER'
AND o.is_deleted=false AND o.status='ACTIVE';

-- name: AppendSecurityOperation :exec
INSERT INTO operation_log (id,organization_id,operator_type,operator_id,operator_name,module,operation_type,target_type,target_id,target_name,
request_id,request_method,request_path,ip_address,user_agent,result,error_code,before_data,after_data,remark,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21);

-- name: ListResourcesForCredentialCheck :many
SELECT * FROM provider_credential WHERE is_deleted=false AND status='ACTIVE' ORDER BY id FOR UPDATE;

-- name: DisableUnrecoverableResource :exec
UPDATE provider_credential SET status='DISABLED',updated_by='system',updated_at=$2 WHERE id=$1 AND is_deleted=false;
