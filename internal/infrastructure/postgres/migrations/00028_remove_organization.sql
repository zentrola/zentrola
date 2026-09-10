-- +goose Up
-- 私有化部署本身就是唯一治理边界。旧资源凭据的组织 ID 被编码进 v1 密文头，
-- 仅用于兼容原 AES-GCM AAD；新凭据使用不依赖组织或实例记录的 v2 AAD。
-- +goose StatementBegin
DO $$
DECLARE
    organization_count BIGINT;
    invalid_scope_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO organization_count FROM organization;
    IF organization_count > 1 THEN
        RAISE EXCEPTION 'Cannot remove organization from a database containing multiple organizations';
    END IF;

    SELECT COUNT(*) INTO invalid_scope_count
    FROM (
        SELECT organization_id FROM admin_user
        UNION ALL SELECT organization_id FROM principal
        UNION ALL SELECT organization_id FROM principal_access_key
        UNION ALL SELECT organization_id FROM principal_group
        UNION ALL SELECT organization_id FROM principal_group_membership
        UNION ALL SELECT organization_id FROM principal_group_model_permission
        UNION ALL SELECT organization_id FROM provider_credential
        UNION ALL SELECT organization_id FROM usage_record
        UNION ALL SELECT organization_id FROM operation_log
    ) scoped
    WHERE NOT EXISTS (SELECT 1 FROM organization WHERE id = scoped.organization_id);
    IF invalid_scope_count > 0 THEN
        RAISE EXCEPTION 'Cannot remove organization from data with an invalid organization scope';
    END IF;

    IF EXISTS (SELECT 1 FROM provider_credential WHERE key_version <> 1) THEN
        RAISE EXCEPTION 'Cannot migrate provider credentials with an unsupported key version';
    END IF;
END $$;
-- +goose StatementEnd

-- 八字节大端 ID 是非敏感兼容元数据；GCM 密文和认证标签保持原样。
UPDATE provider_credential
SET credential_ciphertext = decode(lpad(to_hex(organization_id), 16, '0'), 'hex') || credential_ciphertext;

ALTER TABLE admin_user DROP COLUMN organization_id;
ALTER TABLE principal DROP COLUMN organization_id;
ALTER TABLE principal_access_key DROP COLUMN organization_id;
ALTER TABLE principal_group DROP COLUMN organization_id;
ALTER TABLE principal_group_membership DROP COLUMN organization_id;
ALTER TABLE principal_group_model_permission DROP COLUMN organization_id;
ALTER TABLE provider_credential DROP COLUMN organization_id;
ALTER TABLE usage_record DROP COLUMN organization_id;
ALTER TABLE operation_log DROP COLUMN organization_id;
DROP TABLE organization;

CREATE UNIQUE INDEX uk_admin_user_username ON admin_user (username) WHERE is_deleted = false;
CREATE INDEX ix_principal_type ON principal (principal_type, id) WHERE is_deleted = false;
CREATE INDEX ix_principal_access_key_principal ON principal_access_key (principal_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_principal_group_code ON principal_group (group_code) WHERE is_deleted = false;
CREATE INDEX ix_principal_group_membership_group ON principal_group_membership (group_id) WHERE is_deleted = false;
CREATE INDEX ix_principal_group_model_permission_model ON principal_group_model_permission (model_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_provider_credential_active ON provider_credential (provider_id) WHERE is_deleted = false AND status = 'ACTIVE';
CREATE INDEX ix_usage_principal_time ON usage_record (principal_id, started_at DESC);
CREATE INDEX ix_usage_model_time ON usage_record (model_id, started_at DESC);
CREATE INDEX ix_usage_provider_credential_time ON usage_record (provider_credential_id, started_at DESC);
CREATE INDEX ix_usage_time ON usage_record (started_at DESC);
CREATE INDEX ix_operation_log_time ON operation_log (created_at DESC);

COMMENT ON TABLE provider IS '模型服务供应方；不保存企业调用凭证';
COMMENT ON TABLE provider_credential IS '企业持有的供应方调用凭证；归属于 Provider';
COMMENT ON COLUMN provider_credential.key_version IS '资源凭证密文格式版本：1=含旧 AAD 上下文；2=无组织或实例依赖';

-- +goose Down
CREATE TABLE organization (
    id BIGINT PRIMARY KEY CONSTRAINT organization_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT organization_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    organization_code VARCHAR(64) NOT NULL CONSTRAINT organization_organization_code_check CHECK (btrim(organization_code) <> ''),
    organization_name VARCHAR(128) NOT NULL CONSTRAINT organization_organization_name_check CHECK (btrim(organization_name) <> ''),
    remark VARCHAR(2000) CONSTRAINT organization_remark_check CHECK (btrim(remark) <> ''),
    created_by VARCHAR(64) NOT NULL CONSTRAINT organization_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT organization_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE organization IS '组织；P0-MVP 仅支持单组织';
COMMENT ON COLUMN organization.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN organization.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN organization.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN organization.organization_code IS '组织业务编码';
COMMENT ON COLUMN organization.organization_name IS '组织名称';
COMMENT ON COLUMN organization.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN organization.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN organization.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN organization.created_at IS '创建时间，UTC';
COMMENT ON COLUMN organization.updated_at IS '更新时间，UTC';
CREATE UNIQUE INDEX uk_organization_code ON organization (organization_code) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_organization_singleton ON organization ((true)) WHERE is_deleted = false;

ALTER TABLE admin_user ADD COLUMN organization_id BIGINT CONSTRAINT admin_user_organization_id_check CHECK (organization_id > 0);
ALTER TABLE principal ADD COLUMN organization_id BIGINT CONSTRAINT principal_organization_id_check CHECK (organization_id > 0);
ALTER TABLE principal_access_key ADD COLUMN organization_id BIGINT CONSTRAINT principal_access_key_organization_id_check CHECK (organization_id > 0);
ALTER TABLE principal_group ADD COLUMN organization_id BIGINT CONSTRAINT principal_group_organization_id_check CHECK (organization_id > 0);
ALTER TABLE principal_group_membership ADD COLUMN organization_id BIGINT CONSTRAINT principal_group_membership_organization_id_check CHECK (organization_id > 0);
ALTER TABLE principal_group_model_permission ADD COLUMN organization_id BIGINT CONSTRAINT principal_group_model_permission_organization_id_check CHECK (organization_id > 0);
ALTER TABLE provider_credential ADD COLUMN organization_id BIGINT CONSTRAINT provider_credential_organization_id_check CHECK (organization_id > 0);
ALTER TABLE usage_record ADD COLUMN organization_id BIGINT CONSTRAINT usage_record_organization_id_check CHECK (organization_id > 0);
ALTER TABLE operation_log ADD COLUMN organization_id BIGINT CONSTRAINT operation_log_organization_id_check CHECK (organization_id > 0);

-- +goose StatementBegin
DO $$
DECLARE
    restored_organization_id BIGINT;
BEGIN
	IF EXISTS (SELECT 1 FROM provider_credential WHERE key_version = 2) THEN
		RAISE EXCEPTION 'Cannot roll back after v2 provider credentials have been created';
	END IF;

    SELECT ('x' || encode(substring(credential_ciphertext FROM 1 FOR 8), 'hex'))::bit(64)::bigint
    INTO restored_organization_id
    FROM provider_credential
    WHERE key_version = 1
    LIMIT 1;
    IF restored_organization_id IS NULL OR restored_organization_id <= 0 THEN
        restored_organization_id := 1;
    END IF;

    INSERT INTO organization (id, organization_code, organization_name, status, created_by, updated_by, created_at, updated_at)
    VALUES (restored_organization_id, 'default', 'zentrola', 'ACTIVE', 'system', 'system', now(), now());

    UPDATE admin_user SET organization_id = restored_organization_id;
    UPDATE principal SET organization_id = restored_organization_id;
    UPDATE principal_access_key SET organization_id = restored_organization_id;
    UPDATE principal_group SET organization_id = restored_organization_id;
    UPDATE principal_group_membership SET organization_id = restored_organization_id;
    UPDATE principal_group_model_permission SET organization_id = restored_organization_id;
    UPDATE provider_credential SET organization_id = restored_organization_id;
    UPDATE usage_record SET organization_id = restored_organization_id;
    ALTER TABLE operation_log DISABLE TRIGGER operation_log_append_only;
    UPDATE operation_log SET organization_id = restored_organization_id;
    ALTER TABLE operation_log ENABLE TRIGGER operation_log_append_only;

    UPDATE provider_credential
    SET credential_ciphertext = substring(credential_ciphertext FROM 9)
    WHERE key_version = 1;
END $$;
-- +goose StatementEnd

ALTER TABLE admin_user ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE principal ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE principal_access_key ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE principal_group ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE principal_group_membership ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE principal_group_model_permission ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE provider_credential ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE usage_record ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE operation_log ALTER COLUMN organization_id SET NOT NULL;

DROP INDEX uk_admin_user_username;
DROP INDEX ix_principal_type;
DROP INDEX ix_principal_access_key_principal;
DROP INDEX uk_principal_group_code;
DROP INDEX ix_principal_group_membership_group;
DROP INDEX ix_principal_group_model_permission_model;
DROP INDEX uk_provider_credential_active;
DROP INDEX ix_usage_principal_time;
DROP INDEX ix_usage_model_time;
DROP INDEX ix_usage_provider_credential_time;
DROP INDEX ix_usage_time;
DROP INDEX ix_operation_log_time;

CREATE UNIQUE INDEX uk_admin_user_username ON admin_user (organization_id, username) WHERE is_deleted = false;
CREATE INDEX ix_principal_organization ON principal (organization_id, principal_type, id) WHERE is_deleted = false;
CREATE INDEX ix_principal_access_key_principal ON principal_access_key (organization_id, principal_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_principal_group_code ON principal_group (organization_id, group_code) WHERE is_deleted = false;
CREATE INDEX ix_principal_group_membership_group ON principal_group_membership (organization_id, group_id) WHERE is_deleted = false;
CREATE INDEX ix_principal_group_model_permission_model ON principal_group_model_permission (organization_id, model_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_provider_credential_active ON provider_credential (organization_id, provider_id) WHERE is_deleted = false AND status = 'ACTIVE';
CREATE INDEX ix_usage_principal_time ON usage_record (organization_id, principal_id, started_at DESC);
CREATE INDEX ix_usage_model_time ON usage_record (organization_id, model_id, started_at DESC);
CREATE INDEX ix_usage_provider_credential_time ON usage_record (organization_id, provider_credential_id, started_at DESC);
CREATE INDEX ix_usage_time ON usage_record (organization_id, started_at DESC);
CREATE INDEX ix_operation_log_time ON operation_log (organization_id, created_at DESC);

COMMENT ON COLUMN admin_user.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_access_key.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_group.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_group_membership.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_group_model_permission.organization_id IS '所属组织 ID';
COMMENT ON COLUMN provider_credential.organization_id IS '所属组织 ID';
COMMENT ON COLUMN usage_record.organization_id IS '所属组织 ID';
COMMENT ON COLUMN operation_log.organization_id IS '所属组织 ID';
COMMENT ON TABLE provider IS '模型服务供应方；不保存组织调用凭证';
COMMENT ON TABLE provider_credential IS '组织持有的供应方调用凭证；归属于 Provider';
COMMENT ON COLUMN provider_credential.key_version IS '加密根密钥版本，不含 Master Key 本身';
