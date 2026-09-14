-- +goose Up
ALTER TABLE usage_record DROP CONSTRAINT usage_record_client_protocol_check;
ALTER TABLE usage_record ADD CONSTRAINT usage_record_client_protocol_check
    CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'OPENAI_IMAGES', 'ANTHROPIC_MESSAGES'));
COMMENT ON COLUMN usage_record.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；OPENAI_IMAGES=Images；ANTHROPIC_MESSAGES=Messages';

-- +goose Down
-- 已存在 Images 用量记录时拒绝回滚，避免把合法历史数据变成违反约束的数据。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS (SELECT 1 FROM usage_record WHERE client_protocol='OPENAI_IMAGES') THEN
    RAISE EXCEPTION 'Cannot remove OPENAI_IMAGES while usage records still reference it';
END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE usage_record DROP CONSTRAINT usage_record_client_protocol_check;
ALTER TABLE usage_record ADD CONSTRAINT usage_record_client_protocol_check
    CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'ANTHROPIC_MESSAGES'));
COMMENT ON COLUMN usage_record.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';
