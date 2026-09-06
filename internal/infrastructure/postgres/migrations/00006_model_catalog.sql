-- +goose Up
-- JSONB 保存集合；同时拒绝空集合、重复值、null、对象和未知类型。
-- +goose StatementBegin
CREATE FUNCTION valid_model_modalities(value JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT CASE WHEN jsonb_typeof(value) = 'array' THEN
        jsonb_array_length(value) BETWEEN 1 AND 4
        AND value <@ '["TEXT","IMAGE","AUDIO","VIDEO"]'::jsonb
        AND (SELECT count(*) = count(DISTINCT item) FROM jsonb_array_elements(value) AS t(item))
    ELSE false END;
$$;
-- +goose StatementEnd
COMMENT ON FUNCTION valid_model_modalities(JSONB) IS '校验非空、无重复的模型输入输出类型 JSON 数组';

ALTER TABLE ai_model
    ADD COLUMN publisher_code VARCHAR(64),
    ADD COLUMN input_modalities JSONB,
    ADD COLUMN output_modalities JSONB,
    ADD COLUMN remark VARCHAR(2000) NOT NULL DEFAULT '';

-- 只回填已有安装命令的已知模型；不根据 CHAT 或未知编码猜测能力。
-- 依据 https://platform.claude.com/docs/en/about-claude/models/overview
-- 及 https://api-docs.deepseek.com/ 。保留已有编码，避免中断客户端调用。
UPDATE ai_model SET publisher_code='anthropic', input_modalities='["TEXT","IMAGE"]', output_modalities='["TEXT"]'
WHERE model_code IN ('claude-sonnet','claude-opus');
UPDATE ai_model SET publisher_code='deepseek', input_modalities='["TEXT"]', output_modalities='["TEXT"]'
WHERE model_code IN ('deepseek-v4-flash','deepseek-v4-pro');

-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM ai_model WHERE publisher_code IS NULL) THEN
        RAISE EXCEPTION 'Unknown legacy models require explicit publisher and modality backfill in migration 00006';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE ai_model
    ALTER COLUMN publisher_code SET NOT NULL,
    ALTER COLUMN input_modalities SET NOT NULL,
    ALTER COLUMN output_modalities SET NOT NULL,
    ALTER COLUMN status SET DEFAULT 'DISABLED',
    ADD CONSTRAINT ck_model_publisher CHECK (publisher_code ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
    ADD CONSTRAINT ck_model_input_modalities CHECK (valid_model_modalities(input_modalities)),
    ADD CONSTRAINT ck_model_output_modalities CHECK (valid_model_modalities(output_modalities)),
    ADD CONSTRAINT ck_model_remark CHECK (remark = btrim(remark) AND octet_length(remark) <= 2000),
    DROP COLUMN model_type;
COMMENT ON TABLE ai_model IS '官方模型目录；已有逻辑编码兼容保留，管理员可显式修正';
COMMENT ON COLUMN ai_model.model_code IS '官方标准模型编码；已有客户端编码仅由管理员显式修改';
COMMENT ON COLUMN ai_model.display_name IS '官方模型名称';
COMMENT ON COLUMN ai_model.publisher_code IS '模型官方发布方编码，与调用服务商无关';
COMMENT ON COLUMN ai_model.input_modalities IS '输入类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN ai_model.output_modalities IS '输出类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN ai_model.remark IS '模型用途、限制等备注，空字符串表示未填写';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','MEMBER_CREATE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));

-- +goose Down
-- 有新审计记录时由约束拒绝回滚，保留历史。
ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','MEMBER_CREATE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));
ALTER TABLE ai_model ADD COLUMN model_type VARCHAR(48) NOT NULL DEFAULT 'CHAT' CHECK (model_type IN ('CHAT'));
COMMENT ON COLUMN ai_model.model_type IS '模型类型：CHAT=对话模型';
ALTER TABLE ai_model ALTER COLUMN model_type DROP DEFAULT, ALTER COLUMN status DROP DEFAULT,
    DROP COLUMN publisher_code, DROP COLUMN input_modalities, DROP COLUMN output_modalities, DROP COLUMN remark;
DROP FUNCTION valid_model_modalities(JSONB);
