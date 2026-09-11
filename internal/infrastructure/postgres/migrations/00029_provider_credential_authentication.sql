-- +goose Up
-- provider_credential 继续作为 Usage 和 Gateway 引用的 Provider 认证资源。
-- 现有数据均为 API Key；新增订阅认证后，两类资源可以同时参与路由。
ALTER TABLE provider_credential
    ADD COLUMN auth_type VARCHAR(32) NOT NULL DEFAULT 'API_KEY'
        CONSTRAINT provider_credential_auth_type_check
        CHECK (auth_type IN ('API_KEY', 'SUBSCRIPTION')),
    ADD COLUMN auth_adapter VARCHAR(64) NOT NULL DEFAULT 'API_KEY'
        CONSTRAINT provider_credential_auth_adapter_check
        CHECK (btrim(auth_adapter) <> ''),
    ADD COLUMN subscription_type VARCHAR(32)
        CONSTRAINT provider_credential_subscription_type_check
        CHECK (subscription_type IN ('PERSONAL', 'SEAT')),
    ADD COLUMN plan_code VARCHAR(64)
        CONSTRAINT provider_credential_plan_code_check
        CHECK (plan_code IS NULL OR btrim(plan_code) <> ''),
    ADD COLUMN external_account_ref VARCHAR(255)
        CONSTRAINT provider_credential_external_account_ref_check
        CHECK (external_account_ref IS NULL OR btrim(external_account_ref) <> ''),
    ADD COLUMN priority INTEGER NOT NULL DEFAULT 100
        CONSTRAINT provider_credential_priority_check
        CHECK (priority >= 0),
    ADD COLUMN effective_at TIMESTAMPTZ,
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD COLUMN quota_status VARCHAR(32) NOT NULL DEFAULT 'UNKNOWN'
        CONSTRAINT provider_credential_quota_status_check
        CHECK (quota_status IN ('AVAILABLE', 'NEAR_LIMIT', 'EXHAUSTED', 'UNKNOWN')),
    ADD COLUMN quota_checked_at TIMESTAMPTZ,
    ADD COLUMN quota_resets_at TIMESTAMPTZ,
    ADD CONSTRAINT ck_provider_credential_subscription_fields
        CHECK (
            (auth_type = 'API_KEY' AND subscription_type IS NULL AND plan_code IS NULL)
            OR
            (auth_type = 'SUBSCRIPTION' AND subscription_type IS NOT NULL)
        ),
    ADD CONSTRAINT ck_provider_credential_effective_period
        CHECK (effective_at IS NULL OR expires_at IS NULL OR expires_at > effective_at);

DROP INDEX uk_provider_credential_active;

-- 旧版已停用凭据不能在移除 status 后意外重新参与路由，按新语义转为逻辑删除。
UPDATE provider_credential
SET is_deleted = true, updated_by = 'system', updated_at = now()
WHERE status = 'DISABLED' AND is_deleted = false;

ALTER TABLE provider_credential
    DROP COLUMN status;

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_DELETE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));

CREATE UNIQUE INDEX uk_provider_credential_provider_name
    ON provider_credential(provider_id, resource_name)
    WHERE is_deleted = false;

CREATE INDEX ix_provider_credential_route
    ON provider_credential(provider_id, auth_type, priority, id)
    WHERE is_deleted = false;

COMMENT ON TABLE provider_credential IS 'Provider 认证资源；可使用 API Key 或订阅认证参与上游调用';
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
COMMENT ON COLUMN provider_credential.credential_ciphertext IS 'AES-256-GCM 认证材料密文，包含 16-byte 认证标签；内容格式由 auth_adapter 解释，禁止明文';
COMMENT ON COLUMN provider_credential.credential_nonce IS '认证材料 AES-GCM 随机 Nonce，12 bytes；每次加密重新生成';
COMMENT ON COLUMN provider_credential.key_version IS '认证材料密文格式版本，不含 Master Key 本身';

CREATE TABLE provider_credential_quota (
    provider_credential_id BIGINT NOT NULL
        REFERENCES provider_credential(id)
        CONSTRAINT provider_credential_quota_provider_credential_id_check
        CHECK (provider_credential_id > 0),
    quota_code VARCHAR(64) NOT NULL
        CONSTRAINT provider_credential_quota_quota_code_check
        CHECK (btrim(quota_code) <> ''),
    quota_name VARCHAR(128)
        CONSTRAINT provider_credential_quota_quota_name_check
        CHECK (quota_name IS NULL OR btrim(quota_name) <> ''),
    quota_status VARCHAR(32) NOT NULL
        CONSTRAINT provider_credential_quota_status_check
        CHECK (quota_status IN ('AVAILABLE', 'NEAR_LIMIT', 'EXHAUSTED', 'UNKNOWN')),
    quota_unit VARCHAR(32)
        CONSTRAINT provider_credential_quota_quota_unit_check
        CHECK (quota_unit IS NULL OR btrim(quota_unit) <> ''),
    limit_value NUMERIC(20,8)
        CONSTRAINT provider_credential_quota_limit_value_check
        CHECK (limit_value IS NULL OR limit_value >= 0),
    used_value NUMERIC(20,8)
        CONSTRAINT provider_credential_quota_used_value_check
        CHECK (used_value IS NULL OR used_value >= 0),
    remaining_value NUMERIC(20,8)
        CONSTRAINT provider_credential_quota_remaining_value_check
        CHECK (remaining_value IS NULL OR remaining_value >= 0),
    used_percent NUMERIC(7,4)
        CONSTRAINT provider_credential_quota_used_percent_check
        CHECK (used_percent IS NULL OR used_percent BETWEEN 0 AND 100),
    window_duration_seconds BIGINT
        CONSTRAINT provider_credential_quota_window_duration_check
        CHECK (window_duration_seconds IS NULL OR window_duration_seconds > 0),
    resets_at TIMESTAMPTZ,
    reached_type VARCHAR(64)
        CONSTRAINT provider_credential_quota_reached_type_check
        CHECK (reached_type IS NULL OR btrim(reached_type) <> ''),
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (provider_credential_id, quota_code)
);

COMMENT ON TABLE provider_credential_quota IS 'Provider 认证资源最近观测到的额度窗口；只保存当前状态，不作为 Usage 事实';
COMMENT ON COLUMN provider_credential_quota.provider_credential_id IS '所属 Provider 认证资源 ID';
COMMENT ON COLUMN provider_credential_quota.quota_code IS '适配器提供的稳定额度窗口编码';
COMMENT ON COLUMN provider_credential_quota.quota_name IS '服务商返回的额度窗口显示名称';
COMMENT ON COLUMN provider_credential_quota.quota_status IS '额度窗口状态：AVAILABLE、NEAR_LIMIT、EXHAUSTED 或 UNKNOWN';
COMMENT ON COLUMN provider_credential_quota.quota_unit IS '额度单位，如 TOKEN、REQUEST、CREDIT、COST 或 CONCURRENCY；允许适配器扩展';
COMMENT ON COLUMN provider_credential_quota.limit_value IS '额度上限；NULL=上游未提供绝对值';
COMMENT ON COLUMN provider_credential_quota.used_value IS '已使用额度；NULL=上游未提供绝对值';
COMMENT ON COLUMN provider_credential_quota.remaining_value IS '剩余额度；NULL=上游未提供绝对值';
COMMENT ON COLUMN provider_credential_quota.used_percent IS '已使用百分比，范围 0～100；NULL=上游未提供';
COMMENT ON COLUMN provider_credential_quota.window_duration_seconds IS '额度窗口长度，单位秒；NULL=上游未提供或非周期额度';
COMMENT ON COLUMN provider_credential_quota.resets_at IS '额度窗口预计重置时间；NULL=上游未提供或不重置';
COMMENT ON COLUMN provider_credential_quota.reached_type IS '服务商返回或适配器归一化的额度耗尽类型';
COMMENT ON COLUMN provider_credential_quota.observed_at IS '本额度状态从服务商观测到的时间';
COMMENT ON COLUMN provider_credential_quota.created_at IS '额度窗口首次写入时间，UTC';
COMMENT ON COLUMN provider_credential_quota.updated_at IS '额度窗口最近更新时间，UTC';

-- +goose Down
-- 一旦同一 Provider 存在多条未删除认证资源，就不能恢复旧的单活约束。
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM provider_credential
        WHERE is_deleted = false
        GROUP BY provider_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'Cannot restore the single active provider credential constraint while multiple authentication resources are active';
    END IF;
END $$;
-- +goose StatementEnd

DROP TABLE provider_credential_quota;
DROP INDEX ix_provider_credential_route;
DROP INDEX uk_provider_credential_provider_name;

-- 已产生删除审计时不能恢复不认识 RESOURCE_DELETE 的旧约束。
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM operation_log WHERE operation_type = 'RESOURCE_DELETE') THEN
        RAISE EXCEPTION 'Cannot remove RESOURCE_DELETE while resource deletion audit records exist';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));

ALTER TABLE provider_credential
    ADD COLUMN status VARCHAR(48) NOT NULL DEFAULT 'ACTIVE'
        CONSTRAINT provider_credential_status_check
        CHECK (status IN ('ACTIVE', 'DISABLED')),
    DROP CONSTRAINT ck_provider_credential_effective_period,
    DROP CONSTRAINT ck_provider_credential_subscription_fields,
    DROP COLUMN quota_resets_at,
    DROP COLUMN quota_checked_at,
    DROP COLUMN quota_status,
    DROP COLUMN expires_at,
    DROP COLUMN effective_at,
    DROP COLUMN priority,
    DROP COLUMN external_account_ref,
    DROP COLUMN plan_code,
    DROP COLUMN subscription_type,
    DROP COLUMN auth_adapter,
    DROP COLUMN auth_type;

ALTER TABLE provider_credential
    ALTER COLUMN status DROP DEFAULT;

CREATE UNIQUE INDEX uk_provider_credential_active
    ON provider_credential(provider_id)
    WHERE is_deleted = false AND status = 'ACTIVE';

COMMENT ON TABLE provider_credential IS '企业持有的供应方调用凭证；归属于 Provider';
COMMENT ON COLUMN provider_credential.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN provider_credential.credential_ciphertext IS 'AES-256-GCM 密文，包含 16-byte 认证标签；禁止明文';
COMMENT ON COLUMN provider_credential.credential_nonce IS 'AES-GCM 随机 Nonce，12 bytes；每次加密重新生成';
COMMENT ON COLUMN provider_credential.key_version IS '资源凭证密文格式版本：1=含旧 AAD 上下文；2=无组织或实例依赖';
