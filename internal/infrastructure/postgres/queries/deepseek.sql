-- name: DeepSeekProviderHistory :many
-- 包括软删除历史，防止 setup 命令恢复或覆盖管理员的配置。
SELECT p.id,p.provider_type,
       coalesce((SELECT base_url FROM provider_endpoint WHERE provider_id=p.id AND protocol_type='ANTHROPIC_MESSAGES'), ''::varchar(2048))::text AS anthropic_base_url,
       coalesce((SELECT base_url FROM provider_endpoint WHERE provider_id=p.id AND protocol_type='OPENAI_CHAT'), ''::varchar(2048))::text AS openai_base_url
FROM ai_provider p WHERE p.provider_code='deepseek-official';

-- name: CatalogModelHistory :many
SELECT id FROM ai_model WHERE model_code=$1;

-- name: CatalogMappingHistory :many
SELECT provider_id,upstream_model_code,is_deleted FROM provider_model WHERE model_id=$1;

-- name: SetupDeepSeekOpenAIEndpoint :exec
INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at)
VALUES($1,'OPENAI_CHAT','https://api.deepseek.com','system','system',$2,$2)
ON CONFLICT(provider_id,protocol_type) DO NOTHING;
