-- +goose Up
-- Usage 只记录真实上游调用。客户端协议下沉到上游调用事实后，移除独立请求事实表。
ALTER TABLE usage_record ADD COLUMN client_protocol VARCHAR(48);

UPDATE usage_record u
SET client_protocol = r.client_protocol
FROM ai_request r
WHERE r.organization_id = u.organization_id
  AND r.request_id = u.request_id;

-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM usage_record WHERE client_protocol IS NULL) THEN
        RAISE EXCEPTION 'Cannot remove ai_request while orphan usage records exist';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE usage_record ALTER COLUMN client_protocol SET NOT NULL;
ALTER TABLE usage_record ADD CONSTRAINT usage_record_client_protocol_check
    CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'ANTHROPIC_MESSAGES'));
COMMENT ON COLUMN usage_record.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';

DROP TABLE ai_request;

-- +goose Down
CREATE TABLE ai_request (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    request_id VARCHAR(64) NOT NULL CHECK (btrim(request_id) <> ''),
    principal_id BIGINT NOT NULL CHECK (principal_id > 0),
    model_id BIGINT CHECK (model_id > 0),
    usage_scene VARCHAR(48) NOT NULL CHECK (usage_scene IN ('MODEL_GATEWAY')),
    client_protocol VARCHAR(48) NOT NULL
        CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'ANTHROPIC_MESSAGES')),
    request_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    latency_ms BIGINT NOT NULL CHECK (latency_ms >= 0),
    status VARCHAR(48) NOT NULL CHECK (status IN ('SUCCESS', 'FAILED', 'CANCELLED')),
    error_type VARCHAR(64) CHECK (error_type ~ '^[A-Z][A-Z0-9_]*$'),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_ai_request_time CHECK (completed_at >= request_at),
    CONSTRAINT ck_ai_request_error CHECK ((status = 'SUCCESS') = (error_type IS NULL))
);

COMMENT ON TABLE ai_request IS '主体视角的一次 AI 请求事实；无逻辑删除';
COMMENT ON COLUMN ai_request.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN ai_request.organization_id IS '所属组织 ID';
COMMENT ON COLUMN ai_request.request_id IS '全链路请求标识';
COMMENT ON COLUMN ai_request.principal_id IS '已识别的治理主体 ID';
COMMENT ON COLUMN ai_request.model_id IS '已识别的逻辑模型 ID；NULL=模型未识别';
COMMENT ON COLUMN ai_request.usage_scene IS '使用场景：MODEL_GATEWAY=模型网关调用';
COMMENT ON COLUMN ai_request.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';
COMMENT ON COLUMN ai_request.request_at IS '请求开始时间，UTC';
COMMENT ON COLUMN ai_request.completed_at IS '请求结束时间，UTC';
COMMENT ON COLUMN ai_request.latency_ms IS '请求耗时，毫秒';
COMMENT ON COLUMN ai_request.status IS '调用最终状态：SUCCESS=成功；FAILED=失败；CANCELLED=客户端取消';
COMMENT ON COLUMN ai_request.error_type IS '稳定大写错误码；NULL=无错误';
COMMENT ON COLUMN ai_request.created_at IS '事实记录创建时间，UTC';

CREATE UNIQUE INDEX uk_ai_request_request_id ON ai_request (request_id);
CREATE INDEX ix_ai_request_principal_time ON ai_request (organization_id, principal_id, request_at DESC);
CREATE INDEX ix_ai_request_time ON ai_request (organization_id, request_at DESC);
CREATE INDEX ix_ai_request_model_time ON ai_request (organization_id, model_id, request_at DESC);

INSERT INTO ai_request (
    id, organization_id, request_id, principal_id, model_id, usage_scene, client_protocol,
    request_at, completed_at, latency_ms, status, error_type, created_at
)
SELECT DISTINCT ON (request_id)
    id, organization_id, request_id, principal_id, model_id, usage_scene, client_protocol,
    started_at, completed_at, latency_ms, status, error_type, created_at
FROM usage_record
ORDER BY request_id, attempt_no DESC;

ALTER TABLE usage_record DROP CONSTRAINT usage_record_client_protocol_check;
ALTER TABLE usage_record DROP COLUMN client_protocol;
