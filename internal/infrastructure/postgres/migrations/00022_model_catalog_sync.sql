-- +goose Up
-- Provider 端点按上游协议族保存；OpenAI Chat 与 Responses 共用同一个 Base URL。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS (
    SELECT provider_id
    FROM provider_endpoint
    WHERE protocol_type IN ('OPENAI_CHAT', 'OPENAI_RESPONSES')
    GROUP BY provider_id
    HAVING count(DISTINCT base_url) > 1
) THEN
    RAISE EXCEPTION 'Cannot merge OpenAI provider endpoints with different base URLs';
END IF;
END $$;
-- +goose StatementEnd
DELETE FROM provider_endpoint responses
USING provider_endpoint chat
WHERE responses.provider_id=chat.provider_id
  AND responses.protocol_type='OPENAI_RESPONSES'
  AND chat.protocol_type='OPENAI_CHAT';
ALTER TABLE provider_endpoint DROP CONSTRAINT provider_endpoint_protocol_type_check;
UPDATE provider_endpoint SET protocol_type=CASE protocol_type
    WHEN 'OPENAI_CHAT' THEN 'OPENAI'
    WHEN 'OPENAI_RESPONSES' THEN 'OPENAI'
    WHEN 'ANTHROPIC_MESSAGES' THEN 'ANTHROPIC'
END;
ALTER TABLE provider_endpoint ADD CONSTRAINT provider_endpoint_protocol_type_check
    CHECK (protocol_type IN ('OPENAI','ANTHROPIC'));
COMMENT ON COLUMN provider_endpoint.protocol_type IS '上游协议族：OPENAI=Chat Completions/Responses；ANTHROPIC=Messages';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));

-- +goose Down
-- 已存在目录同步审计时明确拒绝回滚，避免丢失操作历史。
ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));
ALTER TABLE provider_endpoint DROP CONSTRAINT provider_endpoint_protocol_type_check;
UPDATE provider_endpoint SET protocol_type=CASE protocol_type
    WHEN 'OPENAI' THEN 'OPENAI_CHAT'
    WHEN 'ANTHROPIC' THEN 'ANTHROPIC_MESSAGES'
END;
ALTER TABLE provider_endpoint ADD CONSTRAINT provider_endpoint_protocol_type_check
    CHECK (protocol_type IN ('OPENAI_CHAT','OPENAI_RESPONSES','ANTHROPIC_MESSAGES'));
COMMENT ON COLUMN provider_endpoint.protocol_type IS '协议类型：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';
