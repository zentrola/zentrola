-- +goose Up
-- 首发版本基线：由 00001–00041 的最终结构合并生成。
-- 文件名使用版本 41，使 Goose 将全新数据库直接标记为当前版本。
-- 阶段 0 空迁移，仅验证 Migration 执行及版本记录，不创建业务表。
SELECT 1;



-- P0-MVP 阶段 1。全部业务 ID 由应用生成，无外键，无级联删除。

CREATE TABLE organization (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_code VARCHAR(64) NOT NULL CHECK (btrim(organization_code) <> ''),
    organization_name VARCHAR(128) NOT NULL CHECK (btrim(organization_name) <> ''),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    remark VARCHAR(2000) CHECK (btrim(remark) <> ''),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE organization IS '组织；P0-MVP 仅支持单组织';
COMMENT ON COLUMN organization.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN organization.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN organization.organization_code IS '组织业务编码';
COMMENT ON COLUMN organization.organization_name IS '组织名称';
COMMENT ON COLUMN organization.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN organization.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN organization.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN organization.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN organization.created_at IS '创建时间，UTC';
COMMENT ON COLUMN organization.updated_at IS '更新时间，UTC';

CREATE TABLE admin_user (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    username VARCHAR(64) NOT NULL CHECK (btrim(username) <> ''),
    password_hash VARCHAR(255) NOT NULL CHECK (btrim(password_hash) <> ''),
    display_name VARCHAR(128) NOT NULL CHECK (btrim(display_name) <> ''),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    failed_login_count BIGINT NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
    locked_until TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE admin_user IS '管理面管理员；与调用面 Principal 分离';
COMMENT ON COLUMN admin_user.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN admin_user.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN admin_user.organization_id IS '所属组织 ID';
COMMENT ON COLUMN admin_user.username IS '管理员用户名';
COMMENT ON COLUMN admin_user.password_hash IS '密码慢 Hash；不保存明文';
COMMENT ON COLUMN admin_user.display_name IS '管理员显示名称';
COMMENT ON COLUMN admin_user.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN admin_user.failed_login_count IS '连续登录失败次数';
COMMENT ON COLUMN admin_user.locked_until IS '锁定截止时间；NULL=未锁定';
COMMENT ON COLUMN admin_user.last_login_at IS '最后登录时间；NULL=从未登录';
COMMENT ON COLUMN admin_user.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN admin_user.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN admin_user.created_at IS '创建时间，UTC';
COMMENT ON COLUMN admin_user.updated_at IS '更新时间，UTC';

CREATE TABLE principal (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    principal_type VARCHAR(48) NOT NULL CHECK (principal_type IN ('MEMBER', 'APPLICATION')),
    name VARCHAR(128) NOT NULL CHECK (btrim(name) <> ''),
    remark VARCHAR(2000) CHECK (btrim(remark) <> ''),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE principal IS '统一治理主体；MVP 仅建设 MEMBER 流程';
COMMENT ON COLUMN principal.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN principal.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN principal.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal.principal_type IS '主体类型：MEMBER=成员；APPLICATION=应用，仅预留类型';
COMMENT ON COLUMN principal.name IS '主体名称';
COMMENT ON COLUMN principal.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN principal.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN principal.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal.created_at IS '创建时间，UTC';
COMMENT ON COLUMN principal.updated_at IS '更新时间，UTC';

CREATE TABLE access_key (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    principal_id BIGINT NOT NULL CHECK (principal_id > 0),
    key_hash BYTEA NOT NULL CHECK (octet_length(key_hash) = 32),
    key_prefix VARCHAR(32) NOT NULL CHECK (btrim(key_prefix) <> ''),
    name VARCHAR(128) NOT NULL CHECK (btrim(name) <> ''),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED', 'REVOKED')),
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_access_key_revoked CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL))
);
COMMENT ON TABLE access_key IS '调用凭证；仅识别主体，不存储权限或完整 Key';
COMMENT ON COLUMN access_key.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN access_key.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN access_key.organization_id IS '所属组织 ID';
COMMENT ON COLUMN access_key.principal_id IS '所属治理主体 ID';
COMMENT ON COLUMN access_key.key_hash IS '完整 Access Key 的 SHA-256 摘要，32 bytes';
COMMENT ON COLUMN access_key.key_prefix IS '仅用于识别的 Key 前缀，不能用于认证';
COMMENT ON COLUMN access_key.name IS '凭证名称';
COMMENT ON COLUMN access_key.status IS '凭证状态：ACTIVE=启用；DISABLED=停用；REVOKED=已撤销';
COMMENT ON COLUMN access_key.expires_at IS '到期时间；NULL=永不过期';
COMMENT ON COLUMN access_key.last_used_at IS '最后使用时间；NULL=从未使用';
COMMENT ON COLUMN access_key.revoked_at IS '撤销时间；NULL=未撤销';
COMMENT ON COLUMN access_key.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN access_key.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN access_key.created_at IS '创建时间，UTC';
COMMENT ON COLUMN access_key.updated_at IS '更新时间，UTC';

CREATE TABLE ai_group (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    group_code VARCHAR(64) NOT NULL CHECK (btrim(group_code) <> ''),
    group_name VARCHAR(128) NOT NULL CHECK (btrim(group_name) <> ''),
    remark VARCHAR(2000) CHECK (btrim(remark) <> ''),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE ai_group IS 'AI 使用治理分组';
COMMENT ON COLUMN ai_group.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN ai_group.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN ai_group.organization_id IS '所属组织 ID';
COMMENT ON COLUMN ai_group.group_code IS '分组业务编码';
COMMENT ON COLUMN ai_group.group_name IS '分组名称';
COMMENT ON COLUMN ai_group.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN ai_group.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN ai_group.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_group.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_group.created_at IS '创建时间，UTC';
COMMENT ON COLUMN ai_group.updated_at IS '更新时间，UTC';

CREATE TABLE principal_group (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    principal_id BIGINT NOT NULL CHECK (principal_id > 0),
    group_id BIGINT NOT NULL CHECK (group_id > 0),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE principal_group IS '主体与分组的多对多关系';
COMMENT ON COLUMN principal_group.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN principal_group.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN principal_group.organization_id IS '所属组织 ID';
COMMENT ON COLUMN principal_group.principal_id IS '治理主体 ID';
COMMENT ON COLUMN principal_group.group_id IS '分组 ID';
COMMENT ON COLUMN principal_group.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal_group.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN principal_group.created_at IS '创建时间，UTC';
COMMENT ON COLUMN principal_group.updated_at IS '更新时间，UTC';

CREATE TABLE ai_provider (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    provider_code VARCHAR(64) NOT NULL CHECK (btrim(provider_code) <> ''),
    provider_name VARCHAR(128) NOT NULL CHECK (btrim(provider_name) <> ''),
    provider_type VARCHAR(48) NOT NULL CHECK (provider_type IN ('OFFICIAL')),
    anthropic_base_url VARCHAR(2048) NOT NULL CHECK (btrim(anthropic_base_url) <> ''),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE ai_provider IS '模型服务供应方；不保存调用凭证';
COMMENT ON COLUMN ai_provider.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN ai_provider.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN ai_provider.provider_code IS '供应方业务编码';
COMMENT ON COLUMN ai_provider.provider_name IS '供应方名称';
COMMENT ON COLUMN ai_provider.provider_type IS '供应方类型：OFFICIAL=官方供应方';
COMMENT ON COLUMN ai_provider.anthropic_base_url IS 'Anthropic 协议上游基础地址';
COMMENT ON COLUMN ai_provider.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN ai_provider.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_provider.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_provider.created_at IS '创建时间，UTC';
COMMENT ON COLUMN ai_provider.updated_at IS '更新时间，UTC';

CREATE TABLE ai_model (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    model_code VARCHAR(128) NOT NULL CHECK (btrim(model_code) <> ''),
    display_name VARCHAR(128) NOT NULL CHECK (btrim(display_name) <> ''),
    model_type VARCHAR(48) NOT NULL CHECK (model_type IN ('CHAT')),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE ai_model IS '平台稳定逻辑模型；与供应方解耦';
COMMENT ON COLUMN ai_model.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN ai_model.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN ai_model.model_code IS '平台稳定逻辑模型编码；不要求等于上游模型编码';
COMMENT ON COLUMN ai_model.display_name IS '模型显示名称';
COMMENT ON COLUMN ai_model.model_type IS '模型类型：CHAT=对话模型';
COMMENT ON COLUMN ai_model.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN ai_model.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_model.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_model.created_at IS '创建时间，UTC';
COMMENT ON COLUMN ai_model.updated_at IS '更新时间，UTC';

CREATE TABLE provider_model (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    provider_id BIGINT NOT NULL CHECK (provider_id > 0),
    model_id BIGINT NOT NULL CHECK (model_id > 0),
    upstream_model_code VARCHAR(128) NOT NULL CHECK (btrim(upstream_model_code) <> ''),
    protocol_type VARCHAR(48) NOT NULL CHECK (protocol_type IN ('ANTHROPIC')),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE provider_model IS '供应方与逻辑模型的多对多映射';
COMMENT ON COLUMN provider_model.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider_model.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN provider_model.provider_id IS '供应方 ID';
COMMENT ON COLUMN provider_model.model_id IS '逻辑模型 ID';
COMMENT ON COLUMN provider_model.upstream_model_code IS '实际发送到上游的模型编码';
COMMENT ON COLUMN provider_model.protocol_type IS '协议类型：ANTHROPIC=Anthropic 原生协议';
COMMENT ON COLUMN provider_model.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN provider_model.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_model.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_model.created_at IS '创建时间，UTC';
COMMENT ON COLUMN provider_model.updated_at IS '更新时间，UTC';

CREATE TABLE ai_resource (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    provider_id BIGINT NOT NULL CHECK (provider_id > 0),
    resource_name VARCHAR(128) NOT NULL CHECK (btrim(resource_name) <> ''),
    credential_ciphertext BYTEA NOT NULL CHECK (octet_length(credential_ciphertext) > 16),
    credential_nonce BYTEA NOT NULL CHECK (octet_length(credential_nonce) = 12),
    key_version INTEGER NOT NULL CHECK (key_version > 0),
    status VARCHAR(48) NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    last_active_at TIMESTAMPTZ,
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE ai_resource IS '组织持有的供应方调用资源；归属于 Provider';
COMMENT ON COLUMN ai_resource.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN ai_resource.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN ai_resource.organization_id IS '所属组织 ID';
COMMENT ON COLUMN ai_resource.provider_id IS '资源所属供应方 ID，不归属于 Provider Model';
COMMENT ON COLUMN ai_resource.resource_name IS '资源名称';
COMMENT ON COLUMN ai_resource.credential_ciphertext IS 'AES-256-GCM 密文，包含 16-byte 认证标签；禁止明文';
COMMENT ON COLUMN ai_resource.credential_nonce IS 'AES-GCM 随机 Nonce，12 bytes；每次加密重新生成';
COMMENT ON COLUMN ai_resource.key_version IS '加密根密钥版本，不含 Master Key 本身';
COMMENT ON COLUMN ai_resource.status IS '状态：ACTIVE=启用；DISABLED=停用';
COMMENT ON COLUMN ai_resource.last_active_at IS '最后调用时间；NULL=从未使用';
COMMENT ON COLUMN ai_resource.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_resource.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN ai_resource.created_at IS '创建时间，UTC';
COMMENT ON COLUMN ai_resource.updated_at IS '更新时间，UTC';

CREATE TABLE group_model_permission (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    group_id BIGINT NOT NULL CHECK (group_id > 0),
    model_id BIGINT NOT NULL CHECK (model_id > 0),
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    updated_by VARCHAR(64) NOT NULL CHECK (btrim(updated_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
COMMENT ON TABLE group_model_permission IS '分组模型显式授权；默认拒绝，多组授权取并集';
COMMENT ON COLUMN group_model_permission.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN group_model_permission.is_deleted IS '逻辑删除标识；删除后不可恢复';
COMMENT ON COLUMN group_model_permission.organization_id IS '所属组织 ID';
COMMENT ON COLUMN group_model_permission.group_id IS '被授权分组 ID';
COMMENT ON COLUMN group_model_permission.model_id IS '被授权逻辑模型 ID';
COMMENT ON COLUMN group_model_permission.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN group_model_permission.updated_by IS '更新者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN group_model_permission.created_at IS '创建时间，UTC';
COMMENT ON COLUMN group_model_permission.updated_at IS '更新时间，UTC';

CREATE TABLE ai_request (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    request_id VARCHAR(64) NOT NULL CHECK (btrim(request_id) <> ''),
    principal_id BIGINT NOT NULL CHECK (principal_id > 0),
    model_id BIGINT CHECK (model_id > 0),
    usage_scene VARCHAR(48) NOT NULL CHECK (usage_scene IN ('MODEL_GATEWAY')),
    client_protocol VARCHAR(48) NOT NULL CHECK (client_protocol IN ('ANTHROPIC')),
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
COMMENT ON COLUMN ai_request.client_protocol IS '客户端协议：ANTHROPIC=Anthropic 原生协议';
COMMENT ON COLUMN ai_request.request_at IS '请求开始时间，UTC';
COMMENT ON COLUMN ai_request.completed_at IS '请求结束时间，UTC';
COMMENT ON COLUMN ai_request.latency_ms IS '请求耗时，毫秒';
COMMENT ON COLUMN ai_request.status IS '调用最终状态：SUCCESS=成功；FAILED=失败；CANCELLED=客户端取消';
COMMENT ON COLUMN ai_request.error_type IS '稳定大写错误码；NULL=无错误';
COMMENT ON COLUMN ai_request.created_at IS '事实记录创建时间，UTC';

CREATE TABLE usage_record (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    request_id VARCHAR(64) NOT NULL CHECK (btrim(request_id) <> ''),
    attempt_no BIGINT NOT NULL CHECK (attempt_no = 1),
    principal_id BIGINT NOT NULL CHECK (principal_id > 0),
    provider_id BIGINT NOT NULL CHECK (provider_id > 0),
    provider_model_id BIGINT NOT NULL CHECK (provider_model_id > 0),
    resource_id BIGINT NOT NULL CHECK (resource_id > 0),
    model_id BIGINT NOT NULL CHECK (model_id > 0),
    usage_scene VARCHAR(48) NOT NULL CHECK (usage_scene IN ('MODEL_GATEWAY')),
    input_tokens BIGINT CHECK (input_tokens >= 0),
    output_tokens BIGINT CHECK (output_tokens >= 0),
    cached_input_tokens BIGINT CHECK (cached_input_tokens >= 0),
    billing_unit VARCHAR(48) NOT NULL CHECK (billing_unit IN ('TOKEN', 'CALL', 'IMAGE', 'SECOND')),
    billing_quantity NUMERIC(20,8) CHECK (billing_quantity >= 0),
    cost_amount NUMERIC(20,8) CHECK (cost_amount >= 0),
    cost_currency VARCHAR(3) CHECK (cost_currency ~ '^[A-Z]{3}$'),
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL,
    latency_ms BIGINT NOT NULL CHECK (latency_ms >= 0),
    status VARCHAR(48) NOT NULL CHECK (status IN ('SUCCESS', 'FAILED', 'CANCELLED')),
    error_type VARCHAR(64) CHECK (error_type ~ '^[A-Z][A-Z0-9_]*$'),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_usage_record_time CHECK (completed_at >= started_at),
    CONSTRAINT ck_usage_record_cost CHECK ((cost_amount IS NULL) = (cost_currency IS NULL)),
    CONSTRAINT ck_usage_record_error CHECK ((status = 'SUCCESS') = (error_type IS NULL))
);
COMMENT ON TABLE usage_record IS '一次真实上游调用 Attempt 的用量事实；无逻辑删除';
COMMENT ON COLUMN usage_record.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN usage_record.organization_id IS '所属组织 ID';
COMMENT ON COLUMN usage_record.request_id IS '所属请求标识';
COMMENT ON COLUMN usage_record.attempt_no IS '上游调用序号；MVP 固定为 1，不重试或 Failover';
COMMENT ON COLUMN usage_record.principal_id IS '治理主体 ID';
COMMENT ON COLUMN usage_record.provider_id IS '实际供应方 ID';
COMMENT ON COLUMN usage_record.provider_model_id IS '实际供应方模型映射 ID';
COMMENT ON COLUMN usage_record.resource_id IS '实际调用资源 ID';
COMMENT ON COLUMN usage_record.model_id IS '逻辑模型 ID';
COMMENT ON COLUMN usage_record.usage_scene IS '使用场景：MODEL_GATEWAY=模型网关调用';
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
COMMENT ON COLUMN usage_record.status IS '调用最终状态：SUCCESS=成功；FAILED=失败；CANCELLED=客户端取消';
COMMENT ON COLUMN usage_record.error_type IS '稳定大写错误码；NULL=无错误';
COMMENT ON COLUMN usage_record.created_at IS '事实记录创建时间，UTC';

CREATE TABLE operation_log (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    organization_id BIGINT NOT NULL CHECK (organization_id > 0),
    operator_type VARCHAR(48) NOT NULL CHECK (operator_type IN ('ADMIN', 'SYSTEM')),
    operator_id BIGINT CHECK (operator_id > 0),
    operator_name VARCHAR(128) NOT NULL CHECK (btrim(operator_name) <> ''),
    module VARCHAR(48) NOT NULL CHECK (module IN ('MEMBER', 'ACCESS_KEY', 'GROUP', 'MODEL', 'RESOURCE', 'AUTH')),
    operation_type VARCHAR(48) NOT NULL CHECK (operation_type IN ('MEMBER_CREATE', 'MEMBER_STATUS_CHANGE', 'ACCESS_KEY_CREATE', 'ACCESS_KEY_REVOKE', 'GROUP_CREATE', 'GROUP_MEMBER_ADD', 'GROUP_MEMBER_REMOVE', 'GROUP_MODEL_GRANT', 'GROUP_MODEL_REVOKE', 'MODEL_STATUS_CHANGE', 'RESOURCE_CREATE', 'RESOURCE_CREDENTIAL_UPDATE', 'RESOURCE_STATUS_CHANGE', 'RESOURCE_CONNECTION_TEST', 'LOGIN_SUCCESS', 'LOGIN_FAILED', 'LOGIN_LOCKED')),
    target_type VARCHAR(48) NOT NULL CHECK (target_type IN ('PRINCIPAL', 'ACCESS_KEY', 'GROUP', 'MODEL', 'RESOURCE', 'ADMIN_USER')),
    target_id BIGINT CHECK (target_id > 0),
    target_name VARCHAR(128) CHECK (btrim(target_name) <> ''),
    request_id VARCHAR(64) CHECK (btrim(request_id) <> ''),
    request_method VARCHAR(16) CHECK (btrim(request_method) <> ''),
    request_path VARCHAR(2048) CHECK (btrim(request_path) <> ''),
    ip_address INET,
    user_agent VARCHAR(1024) CHECK (btrim(user_agent) <> ''),
    result VARCHAR(48) NOT NULL CHECK (result IN ('SUCCESS', 'FAILED')),
    error_code VARCHAR(64) CHECK (error_code ~ '^[A-Z][A-Z0-9_]*$'),
    before_data JSONB CHECK (jsonb_typeof(before_data) = 'object'),
    after_data JSONB CHECK (jsonb_typeof(after_data) = 'object'),
    remark VARCHAR(2000) CHECK (btrim(remark) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ck_operation_log_operator CHECK ((operator_type = 'ADMIN') = (operator_id IS NOT NULL)),
    CONSTRAINT ck_operation_log_error CHECK ((result = 'SUCCESS') = (error_code IS NULL))
);
COMMENT ON TABLE operation_log IS '管理员与系统操作日志；Append Only，普通操作禁止更新和删除';
COMMENT ON COLUMN operation_log.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN operation_log.organization_id IS '所属组织 ID';
COMMENT ON COLUMN operation_log.operator_type IS '操作人类型：ADMIN=管理员；SYSTEM=系统';
COMMENT ON COLUMN operation_log.operator_id IS '管理员 ID；NULL=系统操作';
COMMENT ON COLUMN operation_log.operator_name IS '操作人名称快照';
COMMENT ON COLUMN operation_log.module IS '业务模块：MEMBER=成员；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；RESOURCE=资源；AUTH=认证';
COMMENT ON COLUMN operation_log.operation_type IS '操作类型：MEMBER_CREATE=创建成员；MEMBER_STATUS_CHANGE=成员状态变更；ACCESS_KEY_CREATE=创建调用凭证；ACCESS_KEY_REVOKE=撤销调用凭证；GROUP_CREATE=创建分组；GROUP_MEMBER_ADD=成员加入分组；GROUP_MEMBER_REMOVE=成员移出分组；GROUP_MODEL_GRANT=分组模型授权；GROUP_MODEL_REVOKE=撤销分组模型授权；MODEL_STATUS_CHANGE=模型状态变更；RESOURCE_CREATE=创建资源；RESOURCE_CREDENTIAL_UPDATE=更新资源凭证；RESOURCE_STATUS_CHANGE=资源状态变更；RESOURCE_CONNECTION_TEST=测试资源连接；LOGIN_SUCCESS=登录成功；LOGIN_FAILED=登录失败；LOGIN_LOCKED=登录锁定';
COMMENT ON COLUMN operation_log.target_type IS '目标类型：PRINCIPAL=治理主体；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；RESOURCE=资源；ADMIN_USER=管理员';
COMMENT ON COLUMN operation_log.target_id IS '目标 ID；NULL=失败操作尚未创建对象';
COMMENT ON COLUMN operation_log.target_name IS '目标名称快照；NULL=未设置';
COMMENT ON COLUMN operation_log.request_id IS '关联 HTTP 请求标识；系统操作无请求时为空；NULL=未设置';
COMMENT ON COLUMN operation_log.request_method IS 'HTTP 请求方法；系统操作无请求时为空；NULL=未设置';
COMMENT ON COLUMN operation_log.request_path IS 'HTTP 路径，不包含 Query String；系统操作时可为空；NULL=未设置';
COMMENT ON COLUMN operation_log.ip_address IS '调用者 IP；NULL=无 HTTP 来源';
COMMENT ON COLUMN operation_log.user_agent IS '客户端 User Agent；NULL=无此信息；NULL=未设置';
COMMENT ON COLUMN operation_log.result IS '操作结果：SUCCESS=成功；FAILED=失败';
COMMENT ON COLUMN operation_log.error_code IS '稳定大写错误码；NULL=无错误';
COMMENT ON COLUMN operation_log.before_data IS '变更前安全快照；NULL=不适用；禁止密码、Hash、Credential、JWT、完整 Key';
COMMENT ON COLUMN operation_log.after_data IS '变更后安全快照；NULL=不适用；禁止密码、Hash、Credential、JWT、完整 Key';
COMMENT ON COLUMN operation_log.remark IS '备注；NULL=未设置';
COMMENT ON COLUMN operation_log.created_at IS '事实记录创建时间，UTC';

CREATE UNIQUE INDEX uk_organization_code ON organization (organization_code) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_organization_singleton ON organization ((true)) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_admin_user_username ON admin_user (organization_id, username) WHERE is_deleted = false;
CREATE INDEX ix_principal_organization ON principal (organization_id, principal_type, id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_access_key_hash ON access_key (key_hash) WHERE is_deleted = false;
CREATE INDEX ix_access_key_principal ON access_key (organization_id, principal_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_ai_group_code ON ai_group (organization_id, group_code) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_principal_group ON principal_group (principal_id, group_id) WHERE is_deleted = false;
CREATE INDEX ix_principal_group_group ON principal_group (organization_id, group_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_provider_code ON ai_provider (provider_code) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_model_code ON ai_model (model_code) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_provider_model_pair ON provider_model (provider_id, model_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_provider_model_active ON provider_model (model_id) WHERE is_deleted = false AND status = 'ACTIVE';
CREATE UNIQUE INDEX uk_resource_active ON ai_resource (organization_id, provider_id) WHERE is_deleted = false AND status = 'ACTIVE';
CREATE INDEX ix_resource_provider ON ai_resource (provider_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_group_model_permission ON group_model_permission (group_id, model_id) WHERE is_deleted = false;
CREATE INDEX ix_group_model_permission_model ON group_model_permission (organization_id, model_id) WHERE is_deleted = false;
CREATE UNIQUE INDEX uk_ai_request_request_id ON ai_request (request_id);
CREATE INDEX ix_ai_request_principal_time ON ai_request (organization_id, principal_id, request_at DESC);
CREATE UNIQUE INDEX uk_usage_request_attempt ON usage_record (request_id, attempt_no);
CREATE INDEX ix_usage_principal_time ON usage_record (organization_id, principal_id, started_at DESC);
CREATE INDEX ix_usage_model_time ON usage_record (organization_id, model_id, started_at DESC);
CREATE INDEX ix_usage_resource_time ON usage_record (organization_id, resource_id, started_at DESC);
CREATE INDEX ix_usage_time ON usage_record (organization_id, started_at DESC);
CREATE INDEX ix_operation_log_time ON operation_log (organization_id, created_at DESC);
CREATE INDEX ix_operation_log_request ON operation_log (request_id) WHERE request_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION reject_operation_log_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'operation_log is append only' USING ERRCODE = '42501';
END;
$$;
-- +goose StatementEnd
COMMENT ON FUNCTION reject_operation_log_mutation() IS '阻止普通操作修改、删除或清空操作日志；未来保留期清理需专用维护流程';
CREATE TRIGGER operation_log_append_only BEFORE UPDATE OR DELETE OR TRUNCATE ON operation_log
FOR EACH STATEMENT EXECUTE FUNCTION reject_operation_log_mutation();



-- 阶段 5：请求事实列表支持组织时间范围和逻辑模型筛选；保留拒绝请求。
CREATE INDEX ix_ai_request_time ON ai_request (organization_id, request_at DESC);
CREATE INDEX ix_ai_request_model_time ON ai_request (organization_id, model_id, request_at DESC);



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



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE', 'MEMBER_CREATE', 'MEMBER_STATUS_CHANGE', 'ACCESS_KEY_CREATE', 'ACCESS_KEY_REVOKE', 'GROUP_CREATE', 'GROUP_MEMBER_ADD', 'GROUP_MEMBER_REMOVE', 'GROUP_MODEL_GRANT', 'GROUP_MODEL_REVOKE', 'MODEL_STATUS_CHANGE', 'RESOURCE_CREATE', 'RESOURCE_CREDENTIAL_UPDATE', 'RESOURCE_STATUS_CHANGE', 'RESOURCE_CONNECTION_TEST', 'LOGIN_SUCCESS', 'LOGIN_FAILED', 'LOGIN_LOCKED'));
COMMENT ON COLUMN operation_log.operation_type IS 'ADMIN_INITIALIZE=初始化首位管理员；操作类型：MEMBER_CREATE=创建成员；MEMBER_STATUS_CHANGE=成员状态变更；ACCESS_KEY_CREATE=创建调用凭证；ACCESS_KEY_REVOKE=撤销调用凭证；GROUP_CREATE=创建分组；GROUP_MEMBER_ADD=成员加入分组；GROUP_MEMBER_REMOVE=成员移出分组；GROUP_MODEL_GRANT=分组模型授权；GROUP_MODEL_REVOKE=撤销分组模型授权；MODEL_STATUS_CHANGE=模型状态变更；RESOURCE_CREATE=创建资源；RESOURCE_CREDENTIAL_UPDATE=更新资源凭证；RESOURCE_STATUS_CHANGE=资源状态变更；RESOURCE_CONNECTION_TEST=测试资源连接；LOGIN_SUCCESS=登录成功；LOGIN_FAILED=登录失败；LOGIN_LOCKED=登录锁定';
CREATE INDEX idx_operation_admin_initialization_history ON operation_log (operation_type) WHERE operation_type IN ('ADMIN_INITIALIZE','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED');



-- JSONB 保存集合；同时拒绝空集合、重复值、null、对象和未知类型。
-- +goose StatementBegin
CREATE FUNCTION valid_model_modalities(value JSONB) RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT CASE WHEN jsonb_typeof(value) = 'array' THEN
        jsonb_array_length(value) BETWEEN 1 AND 4
        AND value <@ '["TEXT","IMAGE","AUDIO","VIDEO"]'::jsonb
        AND (SELECT count(*) = count(DISTINCT item) FROM jsonb_array_elements(value) AS t(item))
    ELSE false END;
$$;
-- +goose StatementEnd
COMMENT ON FUNCTION valid_model_modalities(JSONB) IS '校验非空、无重复的模型输入输出类型 JSON 数组';

ALTER TABLE ai_model
    ADD COLUMN publisher_code VARCHAR(64),
    ADD COLUMN input_modalities JSONB,
    ADD COLUMN output_modalities JSONB,
    ADD COLUMN remark VARCHAR(2000) NOT NULL DEFAULT '';

-- 只回填已有安装命令的已知模型；不根据 CHAT 或未知编码猜测能力。
-- 依据 https://platform.claude.com/docs/en/about-claude/models/overview
-- 及 https://api-docs.deepseek.com/ 。保留已有编码，避免中断客户端调用。
UPDATE ai_model SET publisher_code='anthropic', input_modalities='["TEXT","IMAGE"]', output_modalities='["TEXT"]'
WHERE model_code IN ('claude-sonnet','claude-opus');
UPDATE ai_model SET publisher_code='deepseek', input_modalities='["TEXT"]', output_modalities='["TEXT"]'
WHERE model_code IN ('deepseek-v4-flash','deepseek-v4-pro');

-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM ai_model WHERE publisher_code IS NULL) THEN
        RAISE EXCEPTION 'Unknown legacy models require explicit publisher and modality backfill in migration 00006';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE ai_model
    ALTER COLUMN publisher_code SET NOT NULL,
    ALTER COLUMN input_modalities SET NOT NULL,
    ALTER COLUMN output_modalities SET NOT NULL,
    ALTER COLUMN status SET DEFAULT 'DISABLED',
    ADD CONSTRAINT ck_model_publisher CHECK (publisher_code ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
    ADD CONSTRAINT ck_model_input_modalities CHECK (valid_model_modalities(input_modalities)),
    ADD CONSTRAINT ck_model_output_modalities CHECK (valid_model_modalities(output_modalities)),
    ADD CONSTRAINT ck_model_remark CHECK (remark = btrim(remark) AND octet_length(remark) <= 2000),
    DROP COLUMN model_type;
COMMENT ON TABLE ai_model IS '官方模型目录；已有逻辑编码兼容保留，管理员可显式修正';
COMMENT ON COLUMN ai_model.model_code IS '官方标准模型编码；已有客户端编码仅由管理员显式修改';
COMMENT ON COLUMN ai_model.display_name IS '官方模型名称';
COMMENT ON COLUMN ai_model.publisher_code IS '模型官方发布方编码，与调用服务商无关';
COMMENT ON COLUMN ai_model.input_modalities IS '输入类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN ai_model.output_modalities IS '输出类型 JSON 数组：TEXT、IMAGE、AUDIO、VIDEO，非空且无重复';
COMMENT ON COLUMN ai_model.remark IS '模型用途、限制等备注，空字符串表示未填写';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','MEMBER_CREATE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE ai_model DROP COLUMN publisher_code;



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','MEMBER_CREATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE admin_user ADD COLUMN credential_version BIGINT NOT NULL DEFAULT 0 CHECK (credential_version >= 0);
COMMENT ON COLUMN admin_user.credential_version IS '凭证版本；重置密码递增，使旧管理员 JWT 失效';
ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_provider_type_check;
ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_provider_type_check
    CHECK (provider_type IN ('OFFICIAL','PLATFORM','PARTNER','CUSTOM'));
ALTER TABLE ai_provider ALTER COLUMN anthropic_base_url DROP NOT NULL;
ALTER TABLE ai_provider ADD COLUMN official_website VARCHAR(2048)
    CHECK (official_website IS NULL OR btrim(official_website) <> '');
ALTER TABLE ai_provider ADD CONSTRAINT ck_provider_endpoint
    CHECK (anthropic_base_url IS NOT NULL OR openai_base_url IS NOT NULL);
COMMENT ON COLUMN ai_provider.official_website IS '服务商官方网站；NULL=未填写';
COMMENT ON COLUMN ai_provider.anthropic_base_url IS 'Anthropic 兼容协议上游基础地址；NULL=未配置';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE operation_log DROP CONSTRAINT operation_log_module_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_module_check
    CHECK (module IN ('MEMBER','ACCESS_KEY','GROUP','MODEL','PROVIDER','RESOURCE','AUTH'));
ALTER TABLE operation_log DROP CONSTRAINT operation_log_target_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_target_type_check
    CHECK (target_type IN ('PRINCIPAL','ACCESS_KEY','GROUP','MODEL','PROVIDER','RESOURCE','ADMIN_USER'));
COMMENT ON COLUMN operation_log.module IS '业务模块：MEMBER=成员；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；PROVIDER=服务商；RESOURCE=服务商凭证；AUTH=认证';
COMMENT ON COLUMN operation_log.target_type IS '目标类型：PRINCIPAL=治理主体；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；PROVIDER=服务商；RESOURCE=服务商凭证；ADMIN_USER=管理员';



ALTER TABLE ai_provider
    ADD COLUMN proxy_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN proxy_url_display VARCHAR(2048),
    ADD COLUMN proxy_url_ciphertext BYTEA,
    ADD COLUMN proxy_url_nonce BYTEA,
    ADD COLUMN proxy_url_key_version INTEGER,
    ADD COLUMN proxy_header_names JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN proxy_headers_ciphertext BYTEA,
    ADD COLUMN proxy_headers_nonce BYTEA,
    ADD COLUMN proxy_headers_key_version INTEGER;

ALTER TABLE ai_provider ADD CONSTRAINT ck_provider_proxy_header_names
    CHECK (jsonb_typeof(proxy_header_names) = 'array');
ALTER TABLE ai_provider ADD CONSTRAINT ck_provider_proxy
    CHECK (
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
    );

COMMENT ON COLUMN ai_provider.proxy_enabled IS '是否通过服务商专属出站代理访问上游';
COMMENT ON COLUMN ai_provider.proxy_url_display IS '脱敏后的代理 URL，仅用于管理端展示';
COMMENT ON COLUMN ai_provider.proxy_url_ciphertext IS '完整代理 URL 的 AES-256-GCM 密文，可包含用户名和密码';
COMMENT ON COLUMN ai_provider.proxy_url_nonce IS '代理 URL 密文的 AES-GCM 随机 Nonce';
COMMENT ON COLUMN ai_provider.proxy_url_key_version IS '代理 URL 密文的根密钥版本';
COMMENT ON COLUMN ai_provider.proxy_header_names IS '代理 Header 名称列表，不含 Header Value';
COMMENT ON COLUMN ai_provider.proxy_headers_ciphertext IS '代理 Header Key/Value 对象的 AES-256-GCM 密文';
COMMENT ON COLUMN ai_provider.proxy_headers_nonce IS '代理 Header 密文的 AES-GCM 随机 Nonce';
COMMENT ON COLUMN ai_provider.proxy_headers_key_version IS '代理 Header 密文的根密钥版本';



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE access_key DROP CONSTRAINT access_key_key_prefix_check;
ALTER TABLE access_key RENAME COLUMN key_prefix TO masked_key;
ALTER TABLE access_key ALTER COLUMN masked_key TYPE VARCHAR(64);
UPDATE access_key SET masked_key = masked_key || '********';
ALTER TABLE access_key ADD CONSTRAINT access_key_masked_key_check
    CHECK (btrim(masked_key) <> '' AND position('*' IN masked_key) > 0);
COMMENT ON COLUMN access_key.masked_key IS '脱敏后的 Access Key，仅用于识别和展示，不能用于认证';



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



-- 按业务归属统一表名；仅调整物理命名，不改变数据范围与业务行为。
ALTER TABLE principal_group RENAME TO principal_group_membership;
ALTER TABLE ai_group RENAME TO principal_group;
ALTER TABLE access_key RENAME TO principal_access_key;
ALTER TABLE ai_provider RENAME TO provider;
ALTER TABLE ai_model RENAME TO model;
ALTER TABLE ai_resource RENAME TO provider_credential;
ALTER TABLE group_model_permission RENAME TO principal_group_model_permission;
ALTER TABLE usage_record RENAME COLUMN resource_id TO provider_credential_id;

ALTER TABLE principal_access_key RENAME CONSTRAINT ck_access_key_revoked TO ck_principal_access_key_revoked;

ALTER INDEX uk_access_key_hash RENAME TO uk_principal_access_key_hash;
ALTER INDEX ix_access_key_principal RENAME TO ix_principal_access_key_principal;
ALTER INDEX uk_ai_group_code RENAME TO uk_principal_group_code;
ALTER INDEX uk_principal_group RENAME TO uk_principal_group_membership;
ALTER INDEX ix_principal_group_group RENAME TO ix_principal_group_membership_group;
ALTER INDEX uk_resource_active RENAME TO uk_provider_credential_active;
ALTER INDEX ix_resource_provider RENAME TO ix_provider_credential_provider;
ALTER INDEX uk_group_model_permission RENAME TO uk_principal_group_model_permission;
ALTER INDEX ix_group_model_permission_model RENAME TO ix_principal_group_model_permission_model;
ALTER INDEX ix_usage_resource_time RENAME TO ix_usage_provider_credential_time;

-- PostgreSQL 重命名表时不会同步自动生成的约束名，这里一并消除旧前缀。
-- +goose StatementBegin
DO $$
DECLARE
    item RECORD;
BEGIN
    FOR item IN
        SELECT c.conname, mapping.new_table, mapping.old_prefix, mapping.new_prefix
        FROM (VALUES
            (1, 'principal_group_membership', 'principal_group_', 'principal_group_membership_'),
            (2, 'principal_group', 'ai_group_', 'principal_group_'),
            (3, 'principal_access_key', 'access_key_', 'principal_access_key_'),
            (4, 'provider', 'ai_provider_', 'provider_'),
            (5, 'model', 'ai_model_', 'model_'),
            (6, 'provider_credential', 'ai_resource_', 'provider_credential_'),
            (7, 'principal_group_model_permission', 'group_model_permission_', 'principal_group_model_permission_')
        ) AS mapping(position, new_table, old_prefix, new_prefix)
        JOIN pg_constraint c ON c.conrelid = mapping.new_table::regclass
        WHERE c.conname LIKE mapping.old_prefix || '%'
        ORDER BY mapping.position
    LOOP
        EXECUTE format(
            'ALTER TABLE %I RENAME CONSTRAINT %I TO %I',
            item.new_table,
            item.conname,
            item.new_prefix || substr(item.conname, length(item.old_prefix) + 1)
        );
    END LOOP;
END $$;
-- +goose StatementEnd

COMMENT ON TABLE principal_access_key IS '调用主体访问凭证；仅识别主体，不存储权限或完整 Key';
COMMENT ON TABLE principal_group IS '调用主体的 AI 使用治理分组';
COMMENT ON TABLE principal_group_membership IS '调用主体与治理分组的多对多关系';
COMMENT ON TABLE provider IS '模型服务供应方；不保存组织调用凭证';
COMMENT ON TABLE model IS '平台稳定逻辑模型；与供应方解耦';
COMMENT ON TABLE provider_credential IS '组织持有的供应方调用凭证；归属于 Provider';
COMMENT ON TABLE principal_group_model_permission IS '主体分组的模型显式授权；默认拒绝，多组授权取并集';
COMMENT ON COLUMN usage_record.provider_credential_id IS '实际调用的供应方凭证 ID';



-- Provider 端点按上游协议族保存；OpenAI Chat 与 Responses 共用同一个 Base URL。
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS (
    SELECT provider_id
    FROM provider_endpoint
    WHERE protocol_type IN ('OPENAI_CHAT', 'OPENAI_RESPONSES')
    GROUP BY provider_id
    HAVING count(DISTINCT base_url) > 1
) THEN
    RAISE EXCEPTION 'Cannot merge OpenAI provider endpoints with different base URLs';
END IF;
END $$;
-- +goose StatementEnd
DELETE FROM provider_endpoint responses
USING provider_endpoint chat
WHERE responses.provider_id=chat.provider_id
  AND responses.protocol_type='OPENAI_RESPONSES'
  AND chat.protocol_type='OPENAI_CHAT';
ALTER TABLE provider_endpoint DROP CONSTRAINT provider_endpoint_protocol_type_check;
UPDATE provider_endpoint SET protocol_type=CASE protocol_type
    WHEN 'OPENAI_CHAT' THEN 'OPENAI'
    WHEN 'OPENAI_RESPONSES' THEN 'OPENAI'
    WHEN 'ANTHROPIC_MESSAGES' THEN 'ANTHROPIC'
END;
ALTER TABLE provider_endpoint ADD CONSTRAINT provider_endpoint_protocol_type_check
    CHECK (protocol_type IN ('OPENAI','ANTHROPIC'));
COMMENT ON COLUMN provider_endpoint.protocol_type IS '上游协议族：OPENAI=Chat Completions/Responses；ANTHROPIC=Messages';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



-- 所有业务表共用一个 BIGINT sequence，多实例由 PostgreSQL 保证 ID 唯一。
CREATE SEQUENCE zentrola_global_id_seq
    AS BIGINT
    INCREMENT BY 1
    MINVALUE 1
    NO MAXVALUE
    NO CYCLE
    OWNED BY NONE;

-- 升级已有数据库时，从当前 schema 所有 BIGINT id 的最大值之后继续。
-- +goose StatementBegin
DO $$
DECLARE
    candidate RECORD;
    table_max BIGINT;
    existing_max BIGINT := 0;
BEGIN
    FOR candidate IN
        SELECT columns.table_schema, columns.table_name
        FROM information_schema.columns AS columns
        JOIN information_schema.tables AS tables
          ON tables.table_schema = columns.table_schema
         AND tables.table_name = columns.table_name
         AND tables.table_type = 'BASE TABLE'
        WHERE columns.table_schema = current_schema()
          AND columns.column_name = 'id'
          AND columns.data_type = 'bigint'
    LOOP
        EXECUTE format('SELECT max(id) FROM %I.%I', candidate.table_schema, candidate.table_name)
           INTO table_max;
        existing_max := GREATEST(existing_max, COALESCE(table_max, 0));
    END LOOP;

    IF existing_max = 0 THEN
        PERFORM setval('zentrola_global_id_seq', 1, false);
    ELSE
        PERFORM setval('zentrola_global_id_seq', existing_max, true);
    END IF;
END
$$;
-- +goose StatementEnd



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



ALTER TABLE provider_credential
    DROP COLUMN last_active_at,
    ADD COLUMN runtime_status VARCHAR(32) NOT NULL DEFAULT 'HEALTHY'
        CONSTRAINT provider_credential_runtime_status_check
        CHECK (runtime_status IN ('HEALTHY', 'BLOCKED')),
    ADD COLUMN blocked_reason VARCHAR(64),
    ADD COLUMN blocked_at TIMESTAMPTZ,
    ADD COLUMN last_error_at TIMESTAMPTZ,
    ADD COLUMN last_http_status INTEGER
        CONSTRAINT provider_credential_last_http_status_check
        CHECK (last_http_status BETWEEN 100 AND 599),
    ADD COLUMN last_error_code VARCHAR(128),
    ADD CONSTRAINT ck_provider_credential_runtime_block
        CHECK (
            (runtime_status = 'HEALTHY' AND blocked_reason IS NULL AND blocked_at IS NULL)
            OR
            (runtime_status = 'BLOCKED' AND blocked_reason IS NOT NULL AND blocked_at IS NOT NULL)
        ),
    ADD CONSTRAINT ck_provider_credential_blocked_reason
        CHECK (blocked_reason IS NULL OR blocked_reason IN (
            'BILLING', 'AUTHENTICATION', 'ACCOUNT_SUSPENDED',
            'CREDENTIAL_REVOKED', 'CREDENTIAL_UNRECOVERABLE', 'UNKNOWN_PERMANENT'
        ));

COMMENT ON COLUMN provider_credential.runtime_status IS '系统检测的运行状态：HEALTHY=可参与路由；BLOCKED=长期故障，等待管理员恢复';
COMMENT ON COLUMN provider_credential.blocked_reason IS '长期阻断原因；NULL=未阻断';
COMMENT ON COLUMN provider_credential.blocked_at IS '系统检测到长期阻断的时间；NULL=未阻断';
COMMENT ON COLUMN provider_credential.last_error_at IS '最近一次导致长期阻断的上游错误时间';
COMMENT ON COLUMN provider_credential.last_http_status IS '最近一次导致长期阻断的上游 HTTP 状态；网络或本地错误为 NULL';
COMMENT ON COLUMN provider_credential.last_error_code IS '最近一次导致长期阻断的规范化错误编码，不保存上游响应正文';



-- Failover 会为同一客户端请求记录多次上游调用，attempt_no 从 1 开始递增。
ALTER TABLE usage_record
    DROP CONSTRAINT usage_record_attempt_no_check,
    ADD CONSTRAINT usage_record_attempt_no_check CHECK (attempt_no > 0);

COMMENT ON COLUMN usage_record.attempt_no IS '同一客户端请求的上游调用序号，从 1 开始；Failover 时逐次递增';



ALTER TABLE model
    ADD COLUMN publisher_provider_id BIGINT
    CONSTRAINT model_publisher_provider_id_check CHECK (publisher_provider_id > 0);
COMMENT ON COLUMN model.publisher_provider_id IS '模型发布厂商对应的服务商 ID；手工创建且尚未由官方目录同步时为空';
COMMENT ON TABLE model IS '平台稳定逻辑模型；发布厂商仅用于归属展示，与调用供应方映射解耦';



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



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_DELETE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_STATUS_CHANGE','RESOURCE_DELETE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_DELETE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_CREDENTIAL_EXPORT','RESOURCE_STATUS_CHANGE','RESOURCE_DELETE','RESOURCE_CONNECTION_TEST','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE provider_credential
    ADD COLUMN credential_refreshed_at TIMESTAMPTZ,
    ADD COLUMN credential_expires_at TIMESTAMPTZ;

CREATE INDEX ix_provider_credential_subscription_refresh
    ON provider_credential(credential_expires_at, id)
    WHERE is_deleted = false
      AND auth_type = 'SUBSCRIPTION'
      AND subscription_type = 'PERSONAL'
      AND runtime_status = 'HEALTHY';

COMMENT ON COLUMN provider_credential.credential_refreshed_at IS '订阅认证文件的最近刷新时间；OpenAI Codex 取自 auth.json.last_refresh，缺失时取 access token iat';
COMMENT ON COLUMN provider_credential.credential_expires_at IS '订阅短期访问凭据到期时间；OpenAI Codex 取自 access token exp，用于 SQL 筛选待刷新凭据';



-- usage_record 只保存上游调用产生的原始用量事实；价格与成本由独立的核算模型处理。
ALTER TABLE usage_record
    DROP CONSTRAINT ck_usage_record_cost,
    DROP COLUMN billing_unit,
    DROP COLUMN billing_quantity,
    DROP COLUMN cost_amount,
    DROP COLUMN cost_currency;

COMMENT ON TABLE usage_record IS '一次真实上游调用 Attempt 的原始用量事实；不保存价格与成本，无逻辑删除';



ALTER TABLE usage_record DROP CONSTRAINT usage_record_client_protocol_check;
ALTER TABLE usage_record ADD CONSTRAINT usage_record_client_protocol_check
    CHECK (client_protocol IN ('OPENAI_CHAT', 'OPENAI_RESPONSES', 'OPENAI_IMAGES', 'ANTHROPIC_MESSAGES'));
COMMENT ON COLUMN usage_record.client_protocol IS '客户端协议：OPENAI_CHAT=Chat Completions；OPENAI_RESPONSES=Responses；OPENAI_IMAGES=Images；ANTHROPIC_MESSAGES=Messages';



ALTER TABLE provider_model DROP CONSTRAINT provider_model_upstream_model_code_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_upstream_model_code_check
    CHECK (upstream_model_code = btrim(upstream_model_code));
COMMENT ON COLUMN provider_model.upstream_model_code IS '服务商模型编码覆盖；空字符串表示调用时使用系统模型编码';



ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN ('ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET','MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE','ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE','GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE','GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE','MODEL_CREATE','MODEL_UPDATE','MODEL_DELETE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC','PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE','RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_CREDENTIAL_EXPORT','RESOURCE_STATUS_CHANGE','RESOURCE_DELETE','RESOURCE_CONNECTION_TEST','RESOURCE_RATE_LIMIT_RESET','LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'));



ALTER TABLE provider_endpoint
ADD COLUMN network_scope VARCHAR(16) NOT NULL DEFAULT 'PUBLIC'
    CHECK (network_scope IN ('PUBLIC', 'PRIVATE'));

COMMENT ON COLUMN provider_endpoint.network_scope IS '上游网络范围：PUBLIC=仅公网地址；PRIVATE=管理员显式允许私网地址';



-- 旧版本通过 net/url 生成脱敏代理地址，星号会被编码为 %2A。
-- 这里只修正不含真实凭据的展示列，代理密文保持不变。
UPDATE provider
SET proxy_url_display = replace(proxy_url_display, '%2A', '*')
WHERE strpos(proxy_url_display, '%2A') > 0;




-- 首发版本同时固化 provider_model 的最终列顺序。
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
INSERT INTO provider_model (id, is_deleted, provider_id, model_id, upstream_model_code, priority, created_by, updated_by, created_at, updated_at)
SELECT id, is_deleted, provider_id, model_id, upstream_model_code, priority, created_by, updated_by, created_at, updated_at
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

-- 00041 的 Up 内容：调整 model 列顺序。
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


-- 00040 的 Up 内容：调整 provider_credential 列顺序。
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

CREATE UNIQUE INDEX uk_provider_credential_subscription_account
    ON provider_credential (provider_id, auth_adapter, external_account_ref)
    WHERE is_deleted = false
      AND auth_type = 'SUBSCRIPTION'
      AND external_account_ref IS NOT NULL;
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
-- 首发基线不提供回滚；后续版本请新增 00042 及更高版本的前向迁移。
SELECT 1;

