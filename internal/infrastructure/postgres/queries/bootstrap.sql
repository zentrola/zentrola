-- name: HasOrganizationHistory :one
SELECT EXISTS (SELECT 1 FROM organization);

-- name: CreateBootstrapOrganization :exec
INSERT INTO organization (id, organization_code, organization_name, status, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, 'ACTIVE', 'system', 'system', $4, $4);

-- name: CreateBootstrapProvider :exec
INSERT INTO ai_provider (id, provider_code, provider_name, provider_type, anthropic_base_url, status, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, 'OFFICIAL', $4, 'ACTIVE', 'system', 'system', $5, $5);

-- name: CreateBootstrapModel :exec
INSERT INTO ai_model (id, model_code, display_name, input_modalities, output_modalities, status, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, '["TEXT"]', 'ACTIVE', 'system', 'system', $5, $5);

-- name: CreateBootstrapProviderModel :exec
INSERT INTO provider_model (id, provider_id, model_id, upstream_model_code, protocol_type, status, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, 'ANTHROPIC', 'ACTIVE', 'system', 'system', $5, $5);

-- name: BootstrapInitialized :one
-- 只检查首次初始化的事实，不要求管理员未曾修改/停用模型或 Provider。
SELECT EXISTS (SELECT 1 FROM organization WHERE is_deleted = false);
