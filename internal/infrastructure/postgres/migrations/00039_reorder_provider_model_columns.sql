-- +goose Up
-- 将逻辑删除标识紧跟主键，并将路由优先级放到原 is_deleted 的位置。
-- PostgreSQL 不支持直接移动列，因此在事务内重建表并保留现有数据、约束、索引和注释。
ALTER TABLE provider_model RENAME TO provider_model_reordered;
ALTER INDEX provider_model_pkey RENAME TO provider_model_reordered_pkey;
ALTER INDEX uk_provider_model_pair RENAME TO uk_provider_model_pair_reordered;
ALTER INDEX ix_provider_model_route RENAME TO ix_provider_model_route_reordered;

CREATE TABLE provider_model (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    provider_id BIGINT NOT NULL CHECK (provider_id > 0),
    model_id BIGINT NOT NULL CHECK (model_id > 0),
    upstream_model_code VARCHAR(128) NOT NULL CONSTRAINT provider_model_upstream_model_code_check CHECK (upstream_model_code = btrim(upstream_model_code)),
    priority INTEGER NOT NULL DEFAULT 100 CONSTRAINT provider_model_priority_check CHECK (priority BETWEEN 1 AND 10000),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

INSERT INTO provider_model (
    id, is_deleted, provider_id, model_id, upstream_model_code, priority,
    created_by, updated_by, created_at, updated_at
)
SELECT
    id, is_deleted, provider_id, model_id, upstream_model_code, priority,
    created_by, updated_by, created_at, updated_at
FROM provider_model_reordered;

COMMENT ON TABLE provider_model IS '供应方与逻辑模型的多对多映射';
COMMENT ON COLUMN provider_model.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider_model.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN provider_model.provider_id IS '供应方 ID';
COMMENT ON COLUMN provider_model.model_id IS '逻辑模型 ID';
COMMENT ON COLUMN provider_model.upstream_model_code IS '服务商模型编码覆盖；空字符串表示调用时使用系统模型编码';
COMMENT ON COLUMN provider_model.priority IS '路由优先级，数值越小越优先';
COMMENT ON COLUMN provider_model.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_model.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_model.created_at IS '创建时间，UTC';
COMMENT ON COLUMN provider_model.updated_at IS '更新时间，UTC';

CREATE UNIQUE INDEX uk_provider_model_pair ON provider_model (provider_id, model_id) WHERE NOT is_deleted;
CREATE INDEX ix_provider_model_route ON provider_model (model_id, priority, id) WHERE NOT is_deleted;

DROP TABLE provider_model_reordered;

-- +goose Down
-- 列顺序调整不影响业务语义，保留前向迁移结果。
SELECT 1;
