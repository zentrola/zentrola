-- +goose Up
CREATE TABLE provider_endpoint (
    provider_id BIGINT NOT NULL CHECK (provider_id > 0),
    protocol_type VARCHAR(48) NOT NULL CHECK (protocol_type IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'ANTHROPIC_MESSAGES')),
    base_url VARCHAR(2048) NOT NULL CHECK (btrim(base_url) <> ''),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (provider_id, protocol_type)
);
COMMENT ON TABLE provider_endpoint IS '服务商支持的协议及对应上游基础地址';
COMMENT ON COLUMN provider_endpoint.provider_id IS '所属服务商 ID';
COMMENT ON COLUMN provider_endpoint.protocol_type IS '协议类型：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';
COMMENT ON COLUMN provider_endpoint.base_url IS '该协议对应的上游基础地址';
COMMENT ON COLUMN provider_endpoint.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_endpoint.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_endpoint.created_at IS '创建时间，UTC';
COMMENT ON COLUMN provider_endpoint.updated_at IS '更新时间，UTC';

INSERT INTO provider_endpoint(provider_id, protocol_type, base_url, created_by, updated_by, created_at, updated_at)
SELECT id, 'ANTHROPIC_MESSAGES', anthropic_base_url, created_by, updated_by, created_at, updated_at
FROM ai_provider WHERE anthropic_base_url IS NOT NULL
UNION ALL
SELECT id, 'OPENAI_CHAT', openai_base_url, created_by, updated_by, created_at, updated_at
FROM ai_provider WHERE openai_base_url IS NOT NULL;

-- 旧的停用映射在新语义中等价于取消勾选。
UPDATE provider_model
SET is_deleted = TRUE, updated_by = 'system', updated_at = now()
WHERE NOT is_deleted AND status = 'DISABLED';

-- 同一服务商模型的协议编码必须一致，否则无法无损合并。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS (
    SELECT 1 FROM provider_model
    WHERE NOT is_deleted
    GROUP BY provider_id, model_id
    HAVING count(DISTINCT upstream_model_code) > 1
) THEN
    RAISE EXCEPTION 'Cannot merge provider model mappings with different upstream model codes';
END IF;
END $$;
-- +goose StatementEnd

DROP INDEX uk_provider_model_pair;
DROP INDEX uk_provider_model_active;

-- 每组保留一条映射，其他协议副本转为不可恢复的历史记录。
WITH ranked AS (
    SELECT id, row_number() OVER (
        PARTITION BY provider_id, model_id
        ORDER BY id
    ) AS position
    FROM provider_model
    WHERE NOT is_deleted
)
UPDATE provider_model pm
SET is_deleted = TRUE, updated_by = 'system', updated_at = now()
FROM ranked r
WHERE pm.id = r.id AND r.position > 1;

ALTER TABLE provider_model DROP CONSTRAINT provider_model_protocol_type_check;
ALTER TABLE provider_model DROP COLUMN protocol_type;
ALTER TABLE provider_model DROP CONSTRAINT provider_model_status_check;
ALTER TABLE provider_model DROP COLUMN status;
ALTER TABLE provider_model ADD COLUMN priority INTEGER NOT NULL DEFAULT 100
    CHECK (priority BETWEEN 1 AND 10000);
COMMENT ON COLUMN provider_model.priority IS '路由优先级，数值越小越优先';
CREATE UNIQUE INDEX uk_provider_model_pair ON provider_model(provider_id, model_id) WHERE NOT is_deleted;
CREATE INDEX ix_provider_model_route ON provider_model(model_id, priority, id) WHERE NOT is_deleted;

ALTER TABLE ai_request DROP CONSTRAINT ai_request_client_protocol_check;
UPDATE ai_request SET client_protocol = CASE client_protocol
    WHEN 'ANTHROPIC' THEN 'ANTHROPIC_MESSAGES'
    WHEN 'OPENAI' THEN 'OPENAI_CHAT'
END;
ALTER TABLE ai_request ADD CONSTRAINT ai_request_client_protocol_check
    CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'ANTHROPIC_MESSAGES'));
COMMENT ON COLUMN ai_request.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';

ALTER TABLE ai_provider DROP CONSTRAINT ck_provider_endpoint;
ALTER TABLE ai_provider DROP COLUMN anthropic_base_url;
ALTER TABLE ai_provider DROP COLUMN openai_base_url;

-- +goose Down
ALTER TABLE ai_provider ADD COLUMN anthropic_base_url VARCHAR(2048)
    CHECK (anthropic_base_url IS NULL OR btrim(anthropic_base_url) <> '');
ALTER TABLE ai_provider ADD COLUMN openai_base_url VARCHAR(2048)
    CHECK (openai_base_url IS NULL OR btrim(openai_base_url) <> '');
UPDATE ai_provider p SET
    anthropic_base_url = (
        SELECT base_url FROM provider_endpoint
        WHERE provider_id = p.id AND protocol_type = 'ANTHROPIC_MESSAGES'
    ),
    openai_base_url = (
        SELECT base_url FROM provider_endpoint
        WHERE provider_id = p.id AND protocol_type = 'OPENAI_CHAT'
    );
ALTER TABLE ai_provider ADD CONSTRAINT ck_provider_endpoint
    CHECK (anthropic_base_url IS NOT NULL OR openai_base_url IS NOT NULL);
COMMENT ON COLUMN ai_provider.anthropic_base_url IS 'Anthropic 兼容协议上游基础地址；NULL=未配置';
COMMENT ON COLUMN ai_provider.openai_base_url IS 'OpenAI 兼容协议上游基础地址；NULL=未配置';

DROP INDEX uk_provider_model_pair;
DROP INDEX ix_provider_model_route;
ALTER TABLE provider_model DROP COLUMN priority;
ALTER TABLE provider_model ADD COLUMN protocol_type VARCHAR(48);
ALTER TABLE provider_model ADD COLUMN status VARCHAR(48);
UPDATE provider_model pm SET protocol_type = CASE
    WHEN EXISTS (
        SELECT 1 FROM provider_endpoint pe
        WHERE pe.provider_id = pm.provider_id AND pe.protocol_type = 'ANTHROPIC_MESSAGES'
    ) THEN 'ANTHROPIC'
    ELSE 'OPENAI'
END,
status = CASE WHEN is_deleted THEN 'DISABLED' ELSE 'ACTIVE' END;

-- 同时支持两种协议的服务商恢复为两条协议映射。
WITH duplicated AS (
    SELECT pm.*, row_number() OVER (ORDER BY pm.id) AS position,
           coalesce((SELECT max(id) FROM provider_model), 0) AS max_id
    FROM provider_model pm
    WHERE NOT pm.is_deleted
      AND EXISTS (
          SELECT 1 FROM provider_endpoint pe
          WHERE pe.provider_id = pm.provider_id AND pe.protocol_type = 'ANTHROPIC_MESSAGES'
      )
      AND EXISTS (
          SELECT 1 FROM provider_endpoint pe
          WHERE pe.provider_id = pm.provider_id AND pe.protocol_type = 'OPENAI_CHAT'
      )
)
INSERT INTO provider_model(
    id, is_deleted, provider_id, model_id, upstream_model_code, protocol_type,
    status, created_by, updated_by, created_at, updated_at
)
SELECT max_id + position, FALSE, provider_id, model_id, upstream_model_code, 'OPENAI',
       status, created_by, updated_by, created_at, updated_at
FROM duplicated;

ALTER TABLE provider_model ALTER COLUMN protocol_type SET NOT NULL;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_protocol_type_check
    CHECK (protocol_type IN ('ANTHROPIC', 'OPENAI'));
ALTER TABLE provider_model ALTER COLUMN status SET NOT NULL;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_status_check
    CHECK (status IN ('ACTIVE', 'DISABLED'));
COMMENT ON COLUMN provider_model.protocol_type IS '上游协议：ANTHROPIC=Messages；OPENAI=Chat Completions';
COMMENT ON COLUMN provider_model.status IS '状态：ACTIVE=启用；DISABLED=停用';
CREATE UNIQUE INDEX uk_provider_model_pair ON provider_model(provider_id, model_id, protocol_type) WHERE NOT is_deleted;
CREATE UNIQUE INDEX uk_provider_model_active ON provider_model(model_id, protocol_type) WHERE NOT is_deleted AND status = 'ACTIVE';

-- Responses 历史无法降级到旧协议定义，存在时明确拒绝回滚。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS (SELECT 1 FROM ai_request WHERE client_protocol='OPENAI_RESPONSES') THEN
    RAISE EXCEPTION 'Cannot downgrade while OpenAI Responses request history exists';
END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE ai_request DROP CONSTRAINT ai_request_client_protocol_check;
UPDATE ai_request SET client_protocol = CASE client_protocol
    WHEN 'ANTHROPIC_MESSAGES' THEN 'ANTHROPIC'
    WHEN 'OPENAI_CHAT' THEN 'OPENAI'
END;
ALTER TABLE ai_request ADD CONSTRAINT ai_request_client_protocol_check
    CHECK (client_protocol IN ('ANTHROPIC', 'OPENAI'));
COMMENT ON COLUMN ai_request.client_protocol IS '客户端协议：ANTHROPIC=Messages；OPENAI=Chat Completions';

DROP TABLE provider_endpoint;
