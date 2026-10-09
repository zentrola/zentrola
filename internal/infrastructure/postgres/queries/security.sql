-- name: HasAnyAdmin :one
SELECT EXISTS (SELECT 1 FROM admin_user);

-- name: HasActiveAdmin :one
SELECT EXISTS (SELECT 1 FROM admin_user WHERE is_deleted=false AND status='ACTIVE');

-- name: CreateInitialAdmin :exec
INSERT INTO admin_user (id,username,password_hash,display_name,status,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,$2,'ACTIVE','system','system',$4,$4);

-- name: GetAdminForLogin :one
SELECT * FROM admin_user
WHERE username=$1 AND is_deleted=false
FOR UPDATE;

-- name: SaveAdminLoginState :exec
UPDATE admin_user SET failed_login_count=$2,locked_until=$3,last_login_at=$4,updated_by=$5,updated_at=$6
WHERE id=$1 AND is_deleted=false;

-- name: GetActiveAdmin :one
SELECT id,username,display_name,credential_version FROM admin_user
WHERE id=$1 AND is_deleted=false AND status='ACTIVE';

-- name: ResetAdminPassword :execrows
UPDATE admin_user SET password_hash=$2,failed_login_count=0,locked_until=NULL,
credential_version=credential_version+1,updated_by=$4,updated_at=$3
WHERE id=$1 AND is_deleted=false AND status='ACTIVE';

-- name: GetPrincipalForKey :one
SELECT * FROM principal
WHERE id=sqlc.arg(principal_id) AND principal_type=sqlc.arg(principal_type) AND is_deleted=false FOR UPDATE;

-- name: CreateAccessKey :exec
INSERT INTO principal_access_key (id,principal_id,key_hash,masked_key,name,status,expires_at,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,'ACTIVE',$6,$7,$7,$8,$8);

-- name: GetKeyForRevoke :one
SELECT * FROM principal_access_key WHERE id=$1 AND is_deleted=false FOR UPDATE;

-- name: RevokeAccessKey :exec
UPDATE principal_access_key SET status='REVOKED',expires_at=sqlc.arg(revoked_at),revoked_at=sqlc.arg(revoked_at),
updated_by=sqlc.arg(updated_by),updated_at=sqlc.arg(revoked_at)
WHERE id=sqlc.arg(id) AND is_deleted=false;

-- name: AuthenticateAccessKey :one
SELECT k.id,k.principal_id,p.principal_type,k.expires_at FROM principal_access_key k
JOIN principal p ON p.id=k.principal_id
WHERE k.key_hash=$1 AND k.is_deleted=false AND k.status='ACTIVE' AND k.revoked_at IS NULL
AND (k.expires_at IS NULL OR k.expires_at > sqlc.arg(now)::timestamptz)
AND p.is_deleted=false AND p.status='ACTIVE' AND p.principal_type IN ('MEMBER','APPLICATION');

-- name: AppendSecurityOperation :exec
INSERT INTO operation_log (id,operator_type,operator_id,operator_name,module,operation_type,target_type,target_id,target_name,
request_id,request_method,request_path,ip_address,user_agent,result,error_code,before_data,after_data,remark,created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20);

-- name: ListResourcesForCredentialCheck :many
SELECT * FROM provider_credential WHERE is_deleted=false ORDER BY id FOR UPDATE;

-- name: DeleteUnrecoverableResource :exec
UPDATE provider_credential SET is_deleted=true,updated_by='system',updated_at=$2 WHERE id=$1 AND is_deleted=false;
