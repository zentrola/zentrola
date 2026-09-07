-- +goose Up
ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_provider_type_check;
ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_provider_type_check
    CHECK (provider_type IN ('OFFICIAL','PLATFORM','PARTNER','CUSTOM'));
ALTER TABLE ai_provider ALTER COLUMN anthropic_base_url DROP NOT NULL;
ALTER TABLE ai_provider ADD COLUMN official_website VARCHAR(2048)
    CHECK (official_website IS NULL OR btrim(official_website) <> '');
ALTER TABLE ai_provider ADD CONSTRAINT ck_provider_endpoint
    CHECK (anthropic_base_url IS NOT NULL OR openai_base_url IS NOT NULL);
COMMENT ON COLUMN ai_provider.official_website IS '服务商官方网站；NULL=未填写';
COMMENT ON COLUMN ai_provider.anthropic_base_url IS 'Anthropic 兼容协议上游基础地址；NULL=未配置';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));

-- +goose Down
-- 自定义服务商或相关审计存在时由约束和数据检查拒绝回滚，避免静默丢失配置语义。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS(SELECT 1 FROM ai_provider WHERE provider_type <> 'OFFICIAL' OR official_website IS NOT NULL OR anthropic_base_url IS NULL) THEN
    RAISE EXCEPTION 'Cannot downgrade while managed provider data exists';
END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));
ALTER TABLE ai_provider DROP CONSTRAINT ck_provider_endpoint;
ALTER TABLE ai_provider DROP COLUMN official_website;
ALTER TABLE ai_provider ALTER COLUMN anthropic_base_url SET NOT NULL;
ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_provider_type_check;
ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_provider_type_check CHECK (provider_type IN ('OFFICIAL'));
