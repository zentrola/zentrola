-- +goose Up
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

-- +goose Down
-- 仅用于未承载真实数据的迁移验证；线上通过前向 Migration 修正。
DROP TABLE operation_log;
DROP TABLE usage_record;
DROP TABLE ai_request;
DROP TABLE group_model_permission;
DROP TABLE ai_resource;
DROP TABLE provider_model;
DROP TABLE ai_model;
DROP TABLE ai_provider;
DROP TABLE principal_group;
DROP TABLE ai_group;
DROP TABLE access_key;
DROP TABLE principal;
DROP TABLE admin_user;
DROP TABLE organization;
DROP FUNCTION reject_operation_log_mutation();
