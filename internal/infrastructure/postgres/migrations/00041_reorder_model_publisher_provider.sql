-- +goose Up
-- 将模型发布厂商字段放到 display_name 后面，便于按模型主信息查看。
-- PostgreSQL 不支持直接移动列，因此在事务内重建表并保留现有数据、约束、索引和注释。
ALTER TABLE model RENAME TO model_reordered;
ALTER INDEX model_pkey RENAME TO model_reordered_pkey;
ALTER INDEX uk_model_code RENAME TO uk_model_code_reordered;

CREATE TABLE model (
    id BIGINT NOT NULL CONSTRAINT model_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL DEFAULT 'DISABLED' CONSTRAINT model_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    model_code VARCHAR(128) NOT NULL CONSTRAINT model_model_code_check CHECK (btrim(model_code) <> ''),
    display_name VARCHAR(128) NOT NULL CONSTRAINT model_display_name_check CHECK (btrim(display_name) <> ''),
    publisher_provider_id BIGINT CONSTRAINT model_publisher_provider_id_check CHECK (publisher_provider_id > 0),
    input_modalities JSONB NOT NULL CONSTRAINT ck_model_input_modalities CHECK (valid_model_modalities(input_modalities)),
    output_modalities JSONB NOT NULL CONSTRAINT ck_model_output_modalities CHECK (valid_model_modalities(output_modalities)),
    remark VARCHAR(2000) NOT NULL DEFAULT '' CONSTRAINT ck_model_remark CHECK (remark = btrim(remark) AND octet_length(remark) <= 2000),
    created_by VARCHAR(64) NOT NULL CONSTRAINT model_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT model_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id)
);

INSERT INTO model (
    id, is_deleted, status, model_code, display_name, publisher_provider_id,
    input_modalities, output_modalities, remark, created_by, updated_by, created_at, updated_at
)
SELECT
    id, is_deleted, status, model_code, display_name, publisher_provider_id,
    input_modalities, output_modalities, remark, created_by, updated_by, created_at, updated_at
FROM model_reordered;

COMMENT ON TABLE model IS '平台稳定逻辑模型；发布厂商仅用于归属展示，与调用供应方映射解耦';
COMMENT ON COLUMN model.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN model.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN model.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN model.model_code IS '官方标准模型编码；已有客户端编码仅由管理员显式修改';
COMMENT ON COLUMN model.display_name IS '官方模型名称';
COMMENT ON COLUMN model.publisher_provider_id IS '模型发布厂商对应的服务商 ID；手工创建且尚未由官方目录同步时为空';
COMMENT ON COLUMN model.input_modalities IS '输入类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN model.output_modalities IS '输出类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN model.remark IS '模型用途、限制等备注，空字符串表示未填写';
COMMENT ON COLUMN model.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN model.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN model.created_at IS '创建时间，UTC';
COMMENT ON COLUMN model.updated_at IS '更新时间，UTC';

CREATE UNIQUE INDEX uk_model_code ON model (model_code) WHERE is_deleted = false;
DROP TABLE model_reordered;

-- +goose Down
-- 列顺序调整不影响业务语义，保留前向迁移结果。
SELECT 1;
