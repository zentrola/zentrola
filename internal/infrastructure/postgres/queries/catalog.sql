-- name: GetOrganization :one
SELECT * FROM organization WHERE id = $1 AND is_deleted = false;

-- name: GetAdminByUsername :one
SELECT * FROM admin_user WHERE organization_id = $1 AND username = $2 AND is_deleted = false;

-- name: GetPrincipal :one
SELECT * FROM principal WHERE organization_id = $1 AND id = $2 AND is_deleted = false;

-- name: ListGroupsForPrincipal :many
SELECT g.* FROM principal_group g
JOIN principal_group_membership pg ON pg.group_id = g.id AND pg.organization_id = g.organization_id
WHERE pg.organization_id = $1 AND pg.principal_id = $2
  AND pg.is_deleted = false AND g.is_deleted = false AND g.status = 'ACTIVE'
ORDER BY g.id;

-- name: GetAccessKeyByHash :one
-- 这里只定位凭证；完整认证和撤销/过期判断在阶段 2/4 实现。
SELECT * FROM principal_access_key WHERE key_hash = $1 AND is_deleted = false;

-- name: ListModels :many
SELECT * FROM model WHERE is_deleted = false ORDER BY id;

-- name: GetProvider :one
SELECT * FROM provider WHERE id = $1 AND is_deleted = false;

-- name: GetProviderModel :one
SELECT * FROM provider_model WHERE id = $1 AND is_deleted = false;

-- name: GetResource :one
-- 包含加密凭证，仅供内部 Repository 使用，禁止直接序列化为 API 响应。
SELECT * FROM provider_credential WHERE organization_id = $1 AND id = $2 AND is_deleted = false;

-- name: HasGroupModelPermission :one
-- Default Deny：没有匹配的显式有效授权即返回 false；多组通过 EXISTS 取并集。
SELECT EXISTS (
  SELECT 1 FROM principal p
  JOIN organization o ON o.id = p.organization_id
  JOIN principal_group_membership pg ON pg.principal_id = p.id AND pg.organization_id = p.organization_id
  JOIN principal_group g ON g.id = pg.group_id AND g.organization_id = p.organization_id
  JOIN principal_group_model_permission permission ON permission.group_id = g.id AND permission.organization_id = p.organization_id
  JOIN model m ON m.id = permission.model_id
  WHERE p.organization_id = sqlc.arg(organization_id) AND p.id = sqlc.arg(principal_id) AND m.id = sqlc.arg(model_id)
    AND p.principal_type = 'MEMBER' AND p.status = 'ACTIVE' AND p.is_deleted = false
    AND o.status = 'ACTIVE' AND o.is_deleted = false
    AND pg.is_deleted = false AND g.status = 'ACTIVE' AND g.is_deleted = false
    AND permission.is_deleted = false AND m.status = 'ACTIVE' AND m.is_deleted = false
);
