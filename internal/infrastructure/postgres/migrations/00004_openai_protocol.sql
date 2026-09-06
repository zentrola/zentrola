-- +goose Up
ALTER TABLE ai_provider ADD COLUMN openai_base_url VARCHAR(2048) CHECK (openai_base_url IS NULL OR btrim(openai_base_url) <> '');
COMMENT ON COLUMN ai_provider.openai_base_url IS 'OpenAI 兼容协议上游基础地址；NULL=未配置';
ALTER TABLE provider_model DROP CONSTRAINT provider_model_protocol_type_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_protocol_type_check CHECK (protocol_type IN ('ANTHROPIC','OPENAI'));
COMMENT ON COLUMN provider_model.protocol_type IS '上游协议：ANTHROPIC=Messages；OPENAI=Chat Completions';
DROP INDEX uk_provider_model_pair;
DROP INDEX uk_provider_model_active;
CREATE UNIQUE INDEX uk_provider_model_pair ON provider_model(provider_id,model_id,protocol_type) WHERE NOT is_deleted;
CREATE UNIQUE INDEX uk_provider_model_active ON provider_model(model_id,protocol_type) WHERE NOT is_deleted AND status='ACTIVE';
ALTER TABLE ai_request DROP CONSTRAINT ai_request_client_protocol_check;
ALTER TABLE ai_request ADD CONSTRAINT ai_request_client_protocol_check CHECK (client_protocol IN ('ANTHROPIC','OPENAI'));
COMMENT ON COLUMN ai_request.client_protocol IS '客户端协议：ANTHROPIC=Messages；OPENAI=Chat Completions';

-- +goose Down
-- 不删除历史 Usage 来强行回滚协议；已有新协议数据时显式拒绝。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS(SELECT 1 FROM provider_model WHERE protocol_type='OPENAI') OR EXISTS(SELECT 1 FROM ai_request WHERE client_protocol='OPENAI') THEN
    RAISE EXCEPTION 'Cannot downgrade while OpenAI mappings or request history exist';
END IF;
END $$;
-- +goose StatementEnd
DROP INDEX uk_provider_model_pair;
DROP INDEX uk_provider_model_active;
CREATE UNIQUE INDEX uk_provider_model_pair ON provider_model(provider_id,model_id) WHERE NOT is_deleted;
CREATE UNIQUE INDEX uk_provider_model_active ON provider_model(model_id) WHERE NOT is_deleted AND status='ACTIVE';
ALTER TABLE provider_model DROP CONSTRAINT provider_model_protocol_type_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_protocol_type_check CHECK (protocol_type IN ('ANTHROPIC'));
COMMENT ON COLUMN provider_model.protocol_type IS '协议类型：ANTHROPIC=Anthropic 原生协议';
ALTER TABLE ai_request DROP CONSTRAINT ai_request_client_protocol_check;
ALTER TABLE ai_request ADD CONSTRAINT ai_request_client_protocol_check CHECK (client_protocol IN ('ANTHROPIC'));
COMMENT ON COLUMN ai_request.client_protocol IS '客户端协议：ANTHROPIC=Anthropic 原生协议';
ALTER TABLE ai_provider DROP COLUMN openai_base_url;
