-- +goose Up
-- 将认证、运行状态、额度和刷新元数据移动到 key_version 之前，
-- 保留 id、is_deleted、provider_id、resource_name、密文材料的主体顺序，
-- 并让审计字段（created_by 至 updated_at）保持在表尾。
-- PostgreSQL 不支持直接移动列，因此在事务内重建表并保留现有数据、约束、索引、注释和额度表外键。
ALTER TABLE provider_credential_quota
    DROP CONSTRAINT IF EXISTS provider_credential_quota_provider_credential_id_fkey;
ALTER TABLE provider_credential RENAME TO provider_credential_reordered;
ALTER INDEX provider_credential_pkey RENAME TO provider_credential_reordered_pkey;
ALTER INDEX uk_provider_credential_provider_name RENAME TO uk_provider_credential_provider_name_reordered;
ALTER INDEX ix_provider_credential_route RENAME TO ix_provider_credential_route_reordered;
ALTER INDEX ix_provider_credential_subscription_refresh RENAME TO ix_provider_credential_subscription_refresh_reordered;

CREATE TABLE provider_credential (
    id BIGINT NOT NULL CONSTRAINT provider_credential_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    provider_id BIGINT NOT NULL CONSTRAINT provider_credential_provider_id_check CHECK (provider_id > 0),
    resource_name VARCHAR(128) NOT NULL CONSTRAINT provider_credential_resource_name_check CHECK (btrim(resource_name) <> ''),
    credential_ciphertext BYTEA NOT NULL CONSTRAINT provider_credential_credential_ciphertext_check CHECK (octet_length(credential_ciphertext) > 16),
    credential_nonce BYTEA NOT NULL CONSTRAINT provider_credential_credential_nonce_check CHECK (octet_length(credential_nonce) = 12),
    runtime_status VARCHAR(32) NOT NULL DEFAULT 'HEALTHY'
        CONSTRAINT provider_credential_runtime_status_check CHECK (runtime_status IN ('HEALTHY', 'BLOCKED')),
    blocked_reason VARCHAR(64),
    blocked_at TIMESTAMPTZ,
    last_error_at TIMESTAMPTZ,
    last_http_status INTEGER
        CONSTRAINT provider_credential_last_http_status_check CHECK (last_http_status BETWEEN 100 AND 599),
    last_error_code VARCHAR(128),
    auth_type VARCHAR(32) NOT NULL DEFAULT 'API_KEY'
        CONSTRAINT provider_credential_auth_type_check CHECK (auth_type IN ('API_KEY', 'SUBSCRIPTION')),
    auth_adapter VARCHAR(64) NOT NULL DEFAULT 'API_KEY'
        CONSTRAINT provider_credential_auth_adapter_check CHECK (btrim(auth_adapter) <> ''),
    subscription_type VARCHAR(32)
        CONSTRAINT provider_credential_subscription_type_check CHECK (subscription_type IN ('PERSONAL', 'SEAT')),
    plan_code VARCHAR(64)
        CONSTRAINT provider_credential_plan_code_check CHECK (plan_code IS NULL OR btrim(plan_code) <> ''),
    external_account_ref VARCHAR(255)
        CONSTRAINT provider_credential_external_account_ref_check CHECK (external_account_ref IS NULL OR btrim(external_account_ref) <> ''),
    priority INTEGER NOT NULL DEFAULT 100
        CONSTRAINT provider_credential_priority_check CHECK (priority >= 0),
    effective_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    quota_status VARCHAR(32) NOT NULL DEFAULT 'UNKNOWN'
        CONSTRAINT provider_credential_quota_status_check CHECK (quota_status IN ('AVAILABLE', 'NEAR_LIMIT', 'EXHAUSTED', 'UNKNOWN')),
    quota_checked_at TIMESTAMPTZ,
    quota_resets_at TIMESTAMPTZ,
    credential_refreshed_at TIMESTAMPTZ,
    credential_expires_at TIMESTAMPTZ,
    key_version INTEGER NOT NULL CONSTRAINT provider_credential_key_version_check CHECK (key_version > 0),
    created_by VARCHAR(64) NOT NULL CONSTRAINT provider_credential_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT provider_credential_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_provider_credential_runtime_block CHECK (
        (runtime_status = 'HEALTHY' AND blocked_reason IS NULL AND blocked_at IS NULL)
        OR
        (runtime_status = 'BLOCKED' AND blocked_reason IS NOT NULL AND blocked_at IS NOT NULL)
    ),
    CONSTRAINT ck_provider_credential_blocked_reason CHECK (blocked_reason IS NULL OR blocked_reason IN (
        'BILLING', 'AUTHENTICATION', 'ACCOUNT_SUSPENDED',
        'CREDENTIAL_REVOKED', 'CREDENTIAL_UNRECOVERABLE', 'UNKNOWN_PERMANENT'
    )),
    CONSTRAINT ck_provider_credential_subscription_fields CHECK (
        (auth_type = 'API_KEY' AND subscription_type IS NULL AND plan_code IS NULL)
        OR
        (auth_type = 'SUBSCRIPTION' AND subscription_type IS NOT NULL)
    ),
    CONSTRAINT ck_provider_credential_effective_period CHECK (
        effective_at IS NULL OR expires_at IS NULL OR expires_at > effective_at
    ),
    PRIMARY KEY (id)
);

INSERT INTO provider_credential (
    id, is_deleted, provider_id, resource_name, credential_ciphertext, credential_nonce,
    runtime_status, blocked_reason, blocked_at, last_error_at, last_http_status, last_error_code,
    auth_type, auth_adapter, subscription_type, plan_code, external_account_ref, priority,
    effective_at, expires_at, quota_status, quota_checked_at, quota_resets_at,
    credential_refreshed_at, credential_expires_at, key_version,
    created_by, updated_by, created_at, updated_at
)
SELECT
    id, is_deleted, provider_id, resource_name, credential_ciphertext, credential_nonce,
    runtime_status, blocked_reason, blocked_at, last_error_at, last_http_status, last_error_code,
    auth_type, auth_adapter, subscription_type, plan_code, external_account_ref, priority,
    effective_at, expires_at, quota_status, quota_checked_at, quota_resets_at,
    credential_refreshed_at, credential_expires_at, key_version,
    created_by, updated_by, created_at, updated_at
FROM provider_credential_reordered;

COMMENT ON TABLE provider_credential IS 'Provider 认证资源；可使用 API Key 或订阅认证参与上游调用';
COMMENT ON COLUMN provider_credential.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider_credential.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN provider_credential.provider_id IS '资源所属供应方 ID，不归属于 Provider Model';
COMMENT ON COLUMN provider_credential.resource_name IS '资源名称';
COMMENT ON COLUMN provider_credential.credential_ciphertext IS 'AES-256-GCM 认证材料密文，包含 16-byte 认证标签；内容格式由 auth_adapter 解释，禁止明文';
COMMENT ON COLUMN provider_credential.credential_nonce IS '认证材料 AES-GCM 随机 Nonce，12 bytes；每次加密重新生成';
COMMENT ON COLUMN provider_credential.runtime_status IS '系统检测的运行状态：HEALTHY=可参与路由；BLOCKED=长期故障，等待管理员恢复';
COMMENT ON COLUMN provider_credential.blocked_reason IS '长期阻断原因；NULL=未阻断';
COMMENT ON COLUMN provider_credential.blocked_at IS '系统检测到长期阻断的时间；NULL=未阻断';
COMMENT ON COLUMN provider_credential.last_error_at IS '最近一次导致长期阻断的上游错误时间';
COMMENT ON COLUMN provider_credential.last_http_status IS '最近一次导致长期阻断的上游 HTTP 状态；网络或本地错误为 NULL';
COMMENT ON COLUMN provider_credential.last_error_code IS '最近一次导致长期阻断的规范化错误编码，不保存上游响应正文';
COMMENT ON COLUMN provider_credential.auth_type IS '认证资源类型：API_KEY=按量 API；SUBSCRIPTION=订阅';
COMMENT ON COLUMN provider_credential.auth_adapter IS '认证与调用适配器编码；由应用注册表解释，不绑定具体 Provider 枚举';
COMMENT ON COLUMN provider_credential.subscription_type IS '订阅形态：PERSONAL=个人订阅；SEAT=席位订阅；API Key 为 NULL';
COMMENT ON COLUMN provider_credential.plan_code IS '服务商套餐编码；仅订阅使用，具体值由对应适配器解释';
COMMENT ON COLUMN provider_credential.external_account_ref IS '服务商侧账号或订阅引用；不得保存密码、Token 等秘密';
COMMENT ON COLUMN provider_credential.priority IS '同类型认证资源的路由优先级；数值越小优先级越高';
COMMENT ON COLUMN provider_credential.effective_at IS '认证资源生效时间；NULL=立即生效';
COMMENT ON COLUMN provider_credential.expires_at IS '认证资源到期时间；NULL=未知或不限制';
COMMENT ON COLUMN provider_credential.quota_status IS '最近观测的额度状态；适用于订阅，也预留给存在额度限制的按量 API';
COMMENT ON COLUMN provider_credential.quota_checked_at IS '最近一次额度检查时间；NULL=尚未检查';
COMMENT ON COLUMN provider_credential.quota_resets_at IS '额度预计重置时间；NULL=上游未提供或不适用';
COMMENT ON COLUMN provider_credential.credential_refreshed_at IS '订阅认证文件的最近刷新时间；OpenAI Codex 取自 auth.json.last_refresh，缺失时取 access token iat';
COMMENT ON COLUMN provider_credential.credential_expires_at IS '订阅短期访问凭据到期时间；OpenAI Codex 取自 access token exp，用于 SQL 筛选待刷新凭据';
COMMENT ON COLUMN provider_credential.key_version IS '认证材料密文格式版本，不含 Master Key 本身';
COMMENT ON COLUMN provider_credential.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_credential.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_credential.created_at IS '创建时间，UTC';
COMMENT ON COLUMN provider_credential.updated_at IS '更新时间，UTC';

CREATE UNIQUE INDEX uk_provider_credential_provider_name
    ON provider_credential (provider_id, resource_name)
    WHERE is_deleted = false;
CREATE INDEX ix_provider_credential_route
    ON provider_credential (provider_id, auth_type, priority, id)
    WHERE is_deleted = false;
CREATE INDEX ix_provider_credential_subscription_refresh
    ON provider_credential (credential_expires_at, id)
    WHERE is_deleted = false
      AND auth_type = 'SUBSCRIPTION'
      AND subscription_type = 'PERSONAL'
      AND runtime_status = 'HEALTHY';

DROP TABLE provider_credential_reordered;
ALTER TABLE provider_credential_quota
    ADD CONSTRAINT provider_credential_quota_provider_credential_id_fkey
    FOREIGN KEY (provider_credential_id) REFERENCES provider_credential(id);

-- +goose Down
-- 列顺序调整不影响业务语义，保留前向迁移结果。
SELECT 1;
