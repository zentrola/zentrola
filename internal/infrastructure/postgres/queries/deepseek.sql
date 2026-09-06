-- name: DeepSeekProviderHistory :many
-- 包括软删除历史，防止 setup 命令恢复或覆盖管理员的配置。
SELECT id, provider_type, anthropic_base_url,openai_base_url FROM ai_provider WHERE provider_code='deepseek-official';

-- name: CatalogModelHistory :many
SELECT id FROM ai_model WHERE model_code=$1;

-- name: CatalogMappingHistory :many
SELECT provider_id, upstream_model_code, protocol_type,status,is_deleted FROM provider_model WHERE model_id=$1 AND protocol_type=$2;

-- name: SetupDeepSeekOpenAIURL :exec
UPDATE ai_provider SET openai_base_url='https://api.deepseek.com' WHERE id=$1 AND openai_base_url IS NULL;

-- name: CreateDeepSeekOpenAIModel :exec
INSERT INTO provider_model(id,provider_id,model_id,upstream_model_code,protocol_type,status,created_by,updated_by,created_at,updated_at)
VALUES($1,$2,$3,$4,'OPENAI',$5,'system','system',$6,$6);
