-- +goose Up
-- PostgreSQL 不支持原地调整列顺序。使用同结构新表在事务内换表，保留全部数据，
-- 并统一为 id、is_deleted、status 在前，其余业务字段居中，审计字段在后。
LOCK TABLE organization, admin_user, principal, principal_access_key, principal_group,
    provider, model, provider_credential, usage_record IN ACCESS EXCLUSIVE MODE;

CREATE TABLE organization_reordered (
    id BIGINT NOT NULL CONSTRAINT organization_id_check CHECK (id > 0),
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
INSERT INTO organization_reordered (
    id, is_deleted, status, organization_code, organization_name, remark,
    created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, organization_code, organization_name, remark,
       created_by, updated_by, created_at, updated_at
FROM organization;
DROP TABLE organization;
ALTER TABLE organization_reordered RENAME TO organization;
ALTER TABLE organization ADD CONSTRAINT organization_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_organization_code ON organization (organization_code) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_organization_singleton ON organization ((true)) WHERE is_deleted = false;
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

CREATE TABLE admin_user_reordered (
    id BIGINT NOT NULL CONSTRAINT admin_user_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT admin_user_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    organization_id BIGINT NOT NULL CONSTRAINT admin_user_organization_id_check CHECK (organization_id > 0),
    username VARCHAR(64) NOT NULL CONSTRAINT admin_user_username_check CHECK (btrim(username) <> ''),
    password_hash VARCHAR(255) NOT NULL CONSTRAINT admin_user_password_hash_check CHECK (btrim(password_hash) <> ''),
    display_name VARCHAR(128) NOT NULL CONSTRAINT admin_user_display_name_check CHECK (btrim(display_name) <> ''),
    failed_login_count BIGINT NOT NULL DEFAULT 0 CONSTRAINT admin_user_failed_login_count_check CHECK (failed_login_count >= 0),
    locked_until TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    credential_version BIGINT NOT NULL DEFAULT 0 CONSTRAINT admin_user_credential_version_check CHECK (credential_version >= 0),
    created_by VARCHAR(64) NOT NULL CONSTRAINT admin_user_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT admin_user_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO admin_user_reordered (
    id, is_deleted, status, organization_id, username, password_hash, display_name,
    failed_login_count, locked_until, last_login_at, credential_version,
    created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, organization_id, username, password_hash, display_name,
       failed_login_count, locked_until, last_login_at, credential_version,
       created_by, updated_by, created_at, updated_at
FROM admin_user;
DROP TABLE admin_user;
ALTER TABLE admin_user_reordered RENAME TO admin_user;
ALTER TABLE admin_user ADD CONSTRAINT admin_user_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_admin_user_username ON admin_user (organization_id, username) WHERE is_deleted = false;
COMMENT ON TABLE admin_user IS '管理面管理员；与调用面 Principal 分离';
COMMENT ON COLUMN admin_user.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN admin_user.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN admin_user.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN admin_user.organization_id IS '所属组织 ID';
COMMENT ON COLUMN admin_user.username IS '管理员用户名';
COMMENT ON COLUMN admin_user.password_hash IS '密码慢 Hash；不保存明文';
COMMENT ON COLUMN admin_user.display_name IS '管理员显示名称';
COMMENT ON COLUMN admin_user.failed_login_count IS '连续登录失败次数';
COMMENT ON COLUMN admin_user.locked_until IS '锁定截止时间；NULL=未锁定';
COMMENT ON COLUMN admin_user.last_login_at IS '最后登录时间；NULL=从未登录';
COMMENT ON COLUMN admin_user.credential_version IS '凭证版本；重置密码递增，使旧管理员 JWT 失效';
COMMENT ON COLUMN admin_user.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN admin_user.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN admin_user.created_at IS '创建时间，UTC';
COMMENT ON COLUMN admin_user.updated_at IS '更新时间，UTC';

CREATE TABLE principal_reordered (
    id BIGINT NOT NULL CONSTRAINT principal_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT principal_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    organization_id BIGINT NOT NULL CONSTRAINT principal_organization_id_check CHECK (organization_id > 0),
    principal_type VARCHAR(48) NOT NULL CONSTRAINT principal_principal_type_check CHECK (principal_type IN ('MEMBER', 'APPLICATION')),
    name VARCHAR(128) NOT NULL CONSTRAINT principal_name_check CHECK (btrim(name) <> ''),
    remark VARCHAR(2000) CONSTRAINT principal_remark_check CHECK (btrim(remark) <> ''),
    created_by VARCHAR(64) NOT NULL CONSTRAINT principal_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT principal_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO principal_reordered (
    id, is_deleted, status, organization_id, principal_type, name, remark,
    created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, organization_id, principal_type, name, remark,
       created_by, updated_by, created_at, updated_at
FROM principal;
DROP TABLE principal;
ALTER TABLE principal_reordered RENAME TO principal;
ALTER TABLE principal ADD CONSTRAINT principal_pkey PRIMARY KEY (id);
CREATE INDEX ix_principal_organization ON principal (organization_id, principal_type, id) WHERE is_deleted = false;
COMMENT ON TABLE principal IS '统一治理主体；MVP 仅建设 MEMBER 流程';
COMMENT ON COLUMN principal.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN principal.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN principal.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN principal.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal.principal_type IS '主体类型：MEMBER=成员；APPLICATION=应用，仅预留类型';
COMMENT ON COLUMN principal.name IS '主体名称';
COMMENT ON COLUMN principal.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN principal.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal.created_at IS '创建时间，UTC';
COMMENT ON COLUMN principal.updated_at IS '更新时间，UTC';

CREATE TABLE principal_access_key_reordered (
    id BIGINT NOT NULL CONSTRAINT principal_access_key_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT principal_access_key_status_check CHECK (status IN ('ACTIVE', 'DISABLED', 'REVOKED')),
    organization_id BIGINT NOT NULL CONSTRAINT principal_access_key_organization_id_check CHECK (organization_id > 0),
    principal_id BIGINT NOT NULL CONSTRAINT principal_access_key_principal_id_check CHECK (principal_id > 0),
    key_hash BYTEA NOT NULL CONSTRAINT principal_access_key_key_hash_check CHECK (octet_length(key_hash) = 32),
    masked_key VARCHAR(64) NOT NULL CONSTRAINT principal_access_key_masked_key_check CHECK (btrim(masked_key) <> '' AND position('*' IN masked_key) > 0),
    name VARCHAR(128) NOT NULL CONSTRAINT principal_access_key_name_check CHECK (btrim(name) <> ''),
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by VARCHAR(64) NOT NULL CONSTRAINT principal_access_key_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT principal_access_key_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_principal_access_key_revoked CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL))
);
INSERT INTO principal_access_key_reordered (
    id, is_deleted, status, organization_id, principal_id, key_hash, masked_key, name,
    expires_at, last_used_at, revoked_at, created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, organization_id, principal_id, key_hash, masked_key, name,
       expires_at, last_used_at, revoked_at, created_by, updated_by, created_at, updated_at
FROM principal_access_key;
DROP TABLE principal_access_key;
ALTER TABLE principal_access_key_reordered RENAME TO principal_access_key;
ALTER TABLE principal_access_key ADD CONSTRAINT principal_access_key_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_principal_access_key_hash ON principal_access_key (key_hash) WHERE is_deleted = false;
CREATE INDEX ix_principal_access_key_principal ON principal_access_key (organization_id, principal_id) WHERE is_deleted = false;
COMMENT ON TABLE principal_access_key IS '调用主体访问凭证；仅识别主体，不存储权限或完整 Key';
COMMENT ON COLUMN principal_access_key.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN principal_access_key.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN principal_access_key.status IS '凭证状态：ACTIVE=启用；DISABLED=停用；REVOKED=已撤销';
COMMENT ON COLUMN principal_access_key.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_access_key.principal_id IS '所属治理主体 ID';
COMMENT ON COLUMN principal_access_key.key_hash IS '完整 Access Key 的 SHA-256 摘要，32 bytes';
COMMENT ON COLUMN principal_access_key.masked_key IS '脱敏后的 Access Key，仅用于识别和展示，不能用于认证';
COMMENT ON COLUMN principal_access_key.name IS '凭证名称';
COMMENT ON COLUMN principal_access_key.expires_at IS '到期时间；NULL=永不过期';
COMMENT ON COLUMN principal_access_key.last_used_at IS '最后使用时间；NULL=从未使用';
COMMENT ON COLUMN principal_access_key.revoked_at IS '撤销时间；NULL=未撤销';
COMMENT ON COLUMN principal_access_key.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal_access_key.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal_access_key.created_at IS '创建时间，UTC';
COMMENT ON COLUMN principal_access_key.updated_at IS '更新时间，UTC';

CREATE TABLE principal_group_reordered (
    id BIGINT NOT NULL CONSTRAINT principal_group_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT principal_group_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    organization_id BIGINT NOT NULL CONSTRAINT principal_group_organization_id_check CHECK (organization_id > 0),
    group_code VARCHAR(64) NOT NULL CONSTRAINT principal_group_group_code_check CHECK (btrim(group_code) <> ''),
    group_name VARCHAR(128) NOT NULL CONSTRAINT principal_group_group_name_check CHECK (btrim(group_name) <> ''),
    remark VARCHAR(2000) CONSTRAINT principal_group_remark_check CHECK (btrim(remark) <> ''),
    created_by VARCHAR(64) NOT NULL CONSTRAINT principal_group_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT principal_group_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO principal_group_reordered (
    id, is_deleted, status, organization_id, group_code, group_name, remark,
    created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, organization_id, group_code, group_name, remark,
       created_by, updated_by, created_at, updated_at
FROM principal_group;
DROP TABLE principal_group;
ALTER TABLE principal_group_reordered RENAME TO principal_group;
ALTER TABLE principal_group ADD CONSTRAINT principal_group_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_principal_group_code ON principal_group (organization_id, group_code) WHERE is_deleted = false;
COMMENT ON TABLE principal_group IS '调用主体的 AI 使用治理分组';
COMMENT ON COLUMN principal_group.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN principal_group.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN principal_group.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN principal_group.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_group.group_code IS '分组业务编码';
COMMENT ON COLUMN principal_group.group_name IS '分组名称';
COMMENT ON COLUMN principal_group.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN principal_group.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal_group.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal_group.created_at IS '创建时间，UTC';
COMMENT ON COLUMN principal_group.updated_at IS '更新时间，UTC';

CREATE TABLE provider_reordered (
    id BIGINT NOT NULL CONSTRAINT provider_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT provider_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    provider_code VARCHAR(64) NOT NULL CONSTRAINT provider_provider_code_check CHECK (btrim(provider_code) <> ''),
    provider_name VARCHAR(128) NOT NULL CONSTRAINT provider_provider_name_check CHECK (btrim(provider_name) <> ''),
    provider_type VARCHAR(48) NOT NULL CONSTRAINT provider_provider_type_check CHECK (provider_type IN ('OFFICIAL', 'PLATFORM', 'PARTNER', 'CUSTOM')),
    official_website VARCHAR(2048) CONSTRAINT provider_official_website_check CHECK (official_website IS NULL OR btrim(official_website) <> ''),
    proxy_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    proxy_url_display VARCHAR(2048),
    proxy_url_ciphertext BYTEA,
    proxy_url_nonce BYTEA,
    proxy_url_key_version INTEGER,
    proxy_header_names JSONB NOT NULL DEFAULT '[]'::jsonb,
    proxy_headers_ciphertext BYTEA,
    proxy_headers_nonce BYTEA,
    proxy_headers_key_version INTEGER,
    created_by VARCHAR(64) NOT NULL CONSTRAINT provider_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT provider_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_provider_proxy_header_names CHECK (jsonb_typeof(proxy_header_names) = 'array'),
    CONSTRAINT ck_provider_proxy CHECK (
        (NOT proxy_enabled
            AND proxy_url_display IS NULL
            AND proxy_url_ciphertext IS NULL
            AND proxy_url_nonce IS NULL
            AND proxy_url_key_version IS NULL
            AND proxy_header_names = '[]'::jsonb
            AND proxy_headers_ciphertext IS NULL
            AND proxy_headers_nonce IS NULL
            AND proxy_headers_key_version IS NULL)
        OR
        (proxy_enabled
            AND proxy_url_display IS NOT NULL
            AND btrim(proxy_url_display) <> ''
            AND octet_length(proxy_url_ciphertext) > 16
            AND octet_length(proxy_url_nonce) = 12
            AND proxy_url_key_version > 0
            AND (
                (proxy_header_names = '[]'::jsonb
                    AND proxy_headers_ciphertext IS NULL
                    AND proxy_headers_nonce IS NULL
                    AND proxy_headers_key_version IS NULL)
                OR
                (proxy_header_names <> '[]'::jsonb
                    AND octet_length(proxy_headers_ciphertext) > 16
                    AND octet_length(proxy_headers_nonce) = 12
                    AND proxy_headers_key_version > 0)
            ))
    )
);
INSERT INTO provider_reordered (
    id, is_deleted, status, provider_code, provider_name, provider_type, official_website,
    proxy_enabled, proxy_url_display, proxy_url_ciphertext, proxy_url_nonce, proxy_url_key_version,
    proxy_header_names, proxy_headers_ciphertext, proxy_headers_nonce, proxy_headers_key_version,
    created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, provider_code, provider_name, provider_type, official_website,
       proxy_enabled, proxy_url_display, proxy_url_ciphertext, proxy_url_nonce, proxy_url_key_version,
       proxy_header_names, proxy_headers_ciphertext, proxy_headers_nonce, proxy_headers_key_version,
       created_by, updated_by, created_at, updated_at
FROM provider;
DROP TABLE provider;
ALTER TABLE provider_reordered RENAME TO provider;
ALTER TABLE provider ADD CONSTRAINT provider_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_provider_code ON provider (provider_code) WHERE is_deleted = false;
COMMENT ON TABLE provider IS '模型服务供应方；不保存组织调用凭证';
COMMENT ON COLUMN provider.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN provider.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN provider.provider_code IS '供应方业务编码';
COMMENT ON COLUMN provider.provider_name IS '供应方名称';
COMMENT ON COLUMN provider.provider_type IS '供应方类型：OFFICIAL=官方供应方';
COMMENT ON COLUMN provider.official_website IS '服务商官方网站；NULL=未填写';
COMMENT ON COLUMN provider.proxy_enabled IS '是否通过服务商专属出站代理访问上游';
COMMENT ON COLUMN provider.proxy_url_display IS '脱敏后的代理 URL，仅用于管理端展示';
COMMENT ON COLUMN provider.proxy_url_ciphertext IS '完整代理 URL 的 AES-256-GCM 密文，可包含用户名和密码';
COMMENT ON COLUMN provider.proxy_url_nonce IS '代理 URL 密文的 AES-GCM 随机 Nonce';
COMMENT ON COLUMN provider.proxy_url_key_version IS '代理 URL 密文的根密钥版本';
COMMENT ON COLUMN provider.proxy_header_names IS '代理 Header 名称列表，不含 Header Value';
COMMENT ON COLUMN provider.proxy_headers_ciphertext IS '代理 Header Key/Value 对象的 AES-256-GCM 密文';
COMMENT ON COLUMN provider.proxy_headers_nonce IS '代理 Header 密文的 AES-GCM 随机 Nonce';
COMMENT ON COLUMN provider.proxy_headers_key_version IS '代理 Header 密文的根密钥版本';
COMMENT ON COLUMN provider.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider.created_at IS '创建时间，UTC';
COMMENT ON COLUMN provider.updated_at IS '更新时间，UTC';

CREATE TABLE model_reordered (
    id BIGINT NOT NULL CONSTRAINT model_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL DEFAULT 'DISABLED' CONSTRAINT model_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    model_code VARCHAR(128) NOT NULL CONSTRAINT model_model_code_check CHECK (btrim(model_code) <> ''),
    display_name VARCHAR(128) NOT NULL CONSTRAINT model_display_name_check CHECK (btrim(display_name) <> ''),
    input_modalities JSONB NOT NULL CONSTRAINT ck_model_input_modalities CHECK (valid_model_modalities(input_modalities)),
    output_modalities JSONB NOT NULL CONSTRAINT ck_model_output_modalities CHECK (valid_model_modalities(output_modalities)),
    remark VARCHAR(2000) NOT NULL DEFAULT '' CONSTRAINT ck_model_remark CHECK (remark = btrim(remark) AND octet_length(remark) <= 2000),
    created_by VARCHAR(64) NOT NULL CONSTRAINT model_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT model_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO model_reordered (
    id, is_deleted, status, model_code, display_name, input_modalities, output_modalities,
    remark, created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, model_code, display_name, input_modalities, output_modalities,
       remark, created_by, updated_by, created_at, updated_at
FROM model;
DROP TABLE model;
ALTER TABLE model_reordered RENAME TO model;
ALTER TABLE model ADD CONSTRAINT model_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_model_code ON model (model_code) WHERE is_deleted = false;
COMMENT ON TABLE model IS '平台稳定逻辑模型；与供应方解耦';
COMMENT ON COLUMN model.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN model.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN model.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN model.model_code IS '官方标准模型编码；已有客户端编码仅由管理员显式修改';
COMMENT ON COLUMN model.display_name IS '官方模型名称';
COMMENT ON COLUMN model.input_modalities IS '输入类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN model.output_modalities IS '输出类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN model.remark IS '模型用途、限制等备注，空字符串表示未填写';
COMMENT ON COLUMN model.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN model.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN model.created_at IS '创建时间，UTC';
COMMENT ON COLUMN model.updated_at IS '更新时间，UTC';

CREATE TABLE provider_credential_reordered (
    id BIGINT NOT NULL CONSTRAINT provider_credential_id_check CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(48) NOT NULL CONSTRAINT provider_credential_status_check CHECK (status IN ('ACTIVE', 'DISABLED')),
    organization_id BIGINT NOT NULL CONSTRAINT provider_credential_organization_id_check CHECK (organization_id > 0),
    provider_id BIGINT NOT NULL CONSTRAINT provider_credential_provider_id_check CHECK (provider_id > 0),
    resource_name VARCHAR(128) NOT NULL CONSTRAINT provider_credential_resource_name_check CHECK (btrim(resource_name) <> ''),
    credential_ciphertext BYTEA NOT NULL CONSTRAINT provider_credential_credential_ciphertext_check CHECK (octet_length(credential_ciphertext) > 16),
    credential_nonce BYTEA NOT NULL CONSTRAINT provider_credential_credential_nonce_check CHECK (octet_length(credential_nonce) = 12),
    key_version INTEGER NOT NULL CONSTRAINT provider_credential_key_version_check CHECK (key_version > 0),
    last_active_at TIMESTAMPTZ,
    created_by VARCHAR(64) NOT NULL CONSTRAINT provider_credential_created_by_check CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CONSTRAINT provider_credential_updated_by_check CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
INSERT INTO provider_credential_reordered (
    id, is_deleted, status, organization_id, provider_id, resource_name,
    credential_ciphertext, credential_nonce, key_version, last_active_at,
    created_by, updated_by, created_at, updated_at
)
SELECT id, is_deleted, status, organization_id, provider_id, resource_name,
       credential_ciphertext, credential_nonce, key_version, last_active_at,
       created_by, updated_by, created_at, updated_at
FROM provider_credential;
DROP TABLE provider_credential;
ALTER TABLE provider_credential_reordered RENAME TO provider_credential;
ALTER TABLE provider_credential ADD CONSTRAINT provider_credential_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_provider_credential_active ON provider_credential (organization_id, provider_id) WHERE is_deleted = false AND status = 'ACTIVE';
CREATE INDEX ix_provider_credential_provider ON provider_credential (provider_id) WHERE is_deleted = false;
COMMENT ON TABLE provider_credential IS '组织持有的供应方调用凭证；归属于 Provider';
COMMENT ON COLUMN provider_credential.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider_credential.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN provider_credential.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN provider_credential.organization_id IS '所属组织 ID';
COMMENT ON COLUMN provider_credential.provider_id IS '资源所属供应方 ID，不归属于 Provider Model';
COMMENT ON COLUMN provider_credential.resource_name IS '资源名称';
COMMENT ON COLUMN provider_credential.credential_ciphertext IS 'AES-256-GCM 密文，包含 16-byte 认证标签；禁止明文';
COMMENT ON COLUMN provider_credential.credential_nonce IS 'AES-GCM 随机 Nonce，12 bytes；每次加密重新生成';
COMMENT ON COLUMN provider_credential.key_version IS '加密根密钥版本，不含 Master Key 本身';
COMMENT ON COLUMN provider_credential.last_active_at IS '最后调用时间；NULL=从未使用';
COMMENT ON COLUMN provider_credential.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_credential.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_credential.created_at IS '创建时间，UTC';
COMMENT ON COLUMN provider_credential.updated_at IS '更新时间，UTC';

CREATE TABLE usage_record_reordered (
    id BIGINT NOT NULL CONSTRAINT usage_record_id_check CHECK (id > 0),
    status VARCHAR(48) NOT NULL CONSTRAINT usage_record_status_check CHECK (status IN ('SUCCESS', 'FAILED', 'CANCELLED')),
    organization_id BIGINT NOT NULL CONSTRAINT usage_record_organization_id_check CHECK (organization_id > 0),
    request_id VARCHAR(64) NOT NULL CONSTRAINT usage_record_request_id_check CHECK (btrim(request_id) <> ''),
    attempt_no BIGINT NOT NULL CONSTRAINT usage_record_attempt_no_check CHECK (attempt_no = 1),
    principal_id BIGINT NOT NULL CONSTRAINT usage_record_principal_id_check CHECK (principal_id > 0),
    provider_id BIGINT NOT NULL CONSTRAINT usage_record_provider_id_check CHECK (provider_id > 0),
    provider_model_id BIGINT NOT NULL CONSTRAINT usage_record_provider_model_id_check CHECK (provider_model_id > 0),
    provider_credential_id BIGINT NOT NULL CONSTRAINT usage_record_provider_credential_id_check CHECK (provider_credential_id > 0),
    model_id BIGINT NOT NULL CONSTRAINT usage_record_model_id_check CHECK (model_id > 0),
    usage_scene VARCHAR(48) NOT NULL CONSTRAINT usage_record_usage_scene_check CHECK (usage_scene IN ('MODEL_GATEWAY')),
    client_protocol VARCHAR(48) NOT NULL CONSTRAINT usage_record_client_protocol_check CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'ANTHROPIC_MESSAGES')),
    input_tokens BIGINT CONSTRAINT usage_record_input_tokens_check CHECK (input_tokens >= 0),
    output_tokens BIGINT CONSTRAINT usage_record_output_tokens_check CHECK (output_tokens >= 0),
    cached_input_tokens BIGINT CONSTRAINT usage_record_cached_input_tokens_check CHECK (cached_input_tokens >= 0),
    billing_unit VARCHAR(48) NOT NULL CONSTRAINT usage_record_billing_unit_check CHECK (billing_unit IN ('TOKEN', 'CALL', 'IMAGE', 'SECOND')),
    billing_quantity NUMERIC(20,8) CONSTRAINT usage_record_billing_quantity_check CHECK (billing_quantity >= 0),
    cost_amount NUMERIC(20,8) CONSTRAINT usage_record_cost_amount_check CHECK (cost_amount >= 0),
    cost_currency VARCHAR(3) CONSTRAINT usage_record_cost_currency_check CHECK (cost_currency ~ '^[A-Z]{3}$'),
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    latency_ms BIGINT NOT NULL CONSTRAINT usage_record_latency_ms_check CHECK (latency_ms >= 0),
    error_type VARCHAR(64) CONSTRAINT usage_record_error_type_check CHECK (error_type ~ '^[A-Z][A-Z0-9_]*$'),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_usage_record_time CHECK (completed_at >= started_at),
    CONSTRAINT ck_usage_record_cost CHECK ((cost_amount IS NULL) = (cost_currency IS NULL)),
    CONSTRAINT ck_usage_record_error CHECK ((status = 'SUCCESS') = (error_type IS NULL))
);
INSERT INTO usage_record_reordered (
    id, status, organization_id, request_id, attempt_no, principal_id, provider_id,
    provider_model_id, provider_credential_id, model_id, usage_scene, client_protocol,
    input_tokens, output_tokens, cached_input_tokens, billing_unit, billing_quantity,
    cost_amount, cost_currency, started_at, completed_at, latency_ms, error_type, created_at
)
SELECT id, status, organization_id, request_id, attempt_no, principal_id, provider_id,
       provider_model_id, provider_credential_id, model_id, usage_scene, client_protocol,
       input_tokens, output_tokens, cached_input_tokens, billing_unit, billing_quantity,
       cost_amount, cost_currency, started_at, completed_at, latency_ms, error_type, created_at
FROM usage_record;
DROP TABLE usage_record;
ALTER TABLE usage_record_reordered RENAME TO usage_record;
ALTER TABLE usage_record ADD CONSTRAINT usage_record_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX uk_usage_request_attempt ON usage_record (request_id, attempt_no);
CREATE INDEX ix_usage_principal_time ON usage_record (organization_id, principal_id, started_at DESC);
CREATE INDEX ix_usage_model_time ON usage_record (organization_id, model_id, started_at DESC);
CREATE INDEX ix_usage_provider_credential_time ON usage_record (organization_id, provider_credential_id, started_at DESC);
CREATE INDEX ix_usage_time ON usage_record (organization_id, started_at DESC);
COMMENT ON TABLE usage_record IS '一次真实上游调用 Attempt 的用量事实；无逻辑删除';
COMMENT ON COLUMN usage_record.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN usage_record.status IS '调用最终状态：SUCCESS=成功；FAILED=失败；CANCELLED=客户端取消';
COMMENT ON COLUMN usage_record.organization_id IS '所属组织 ID';
COMMENT ON COLUMN usage_record.request_id IS '所属请求标识';
COMMENT ON COLUMN usage_record.attempt_no IS '上游调用序号；MVP 固定为 1，不重试或 Failover';
COMMENT ON COLUMN usage_record.principal_id IS '治理主体 ID';
COMMENT ON COLUMN usage_record.provider_id IS '实际供应方 ID';
COMMENT ON COLUMN usage_record.provider_model_id IS '实际供应方模型映射 ID';
COMMENT ON COLUMN usage_record.provider_credential_id IS '实际调用的供应方凭证 ID';
COMMENT ON COLUMN usage_record.model_id IS '逻辑模型 ID';
COMMENT ON COLUMN usage_record.usage_scene IS '使用场景：MODEL_GATEWAY=模型网关调用';
COMMENT ON COLUMN usage_record.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；ANTHROPIC_MESSAGES=Messages';
COMMENT ON COLUMN usage_record.input_tokens IS '输入 Token 数；NULL=上游未返回可靠值，0=已确认零用量';
COMMENT ON COLUMN usage_record.output_tokens IS '输出 Token 数；NULL=上游未返回可靠值，0=已确认零用量';
COMMENT ON COLUMN usage_record.cached_input_tokens IS '缓存读取 Token 数；NULL=上游未返回可靠值，0=已确认零用量';
COMMENT ON COLUMN usage_record.billing_unit IS '计量单位：TOKEN=Token；CALL=调用次数；IMAGE=图像数量；SECOND=秒数';
COMMENT ON COLUMN usage_record.billing_quantity IS '计量数量；NULL=无法可靠确认；非客户销售金额';
COMMENT ON COLUMN usage_record.cost_amount IS '上游成本快照；NULL=未计算，MVP 不实现成本计算';
COMMENT ON COLUMN usage_record.cost_currency IS '成本币种；NULL=未计算成本';
COMMENT ON COLUMN usage_record.started_at IS '真实上游调用开始时间，UTC';
COMMENT ON COLUMN usage_record.completed_at IS '上游调用结束时间，UTC';
COMMENT ON COLUMN usage_record.latency_ms IS '上游耗时，毫秒';
COMMENT ON COLUMN usage_record.error_type IS '稳定大写错误码；NULL=无错误';
COMMENT ON COLUMN usage_record.created_at IS '事实记录创建时间，UTC';

-- +goose Down
-- 列顺序不改变数据语义；回退版本时保留规范化后的物理顺序，重新 Up 仍可安全换表。
SELECT 1;
