-- +goose Up
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

-- +goose Down
ALTER TABLE principal_access_key RENAME CONSTRAINT ck_principal_access_key_revoked TO ck_access_key_revoked;

-- 先恢复自动约束名，避免表名恢复后留下新前缀。
-- +goose StatementBegin
DO $$
DECLARE
    item RECORD;
BEGIN
    FOR item IN
        SELECT c.conname, mapping.current_table, mapping.current_prefix, mapping.old_prefix
        FROM (VALUES
            (1, 'principal_group', 'principal_group_', 'ai_group_'),
            (2, 'principal_group_membership', 'principal_group_membership_', 'principal_group_'),
            (3, 'principal_access_key', 'principal_access_key_', 'access_key_'),
            (4, 'provider', 'provider_', 'ai_provider_'),
            (5, 'model', 'model_', 'ai_model_'),
            (6, 'provider_credential', 'provider_credential_', 'ai_resource_'),
            (7, 'principal_group_model_permission', 'principal_group_model_permission_', 'group_model_permission_')
        ) AS mapping(position, current_table, current_prefix, old_prefix)
        JOIN pg_constraint c ON c.conrelid = mapping.current_table::regclass
        WHERE c.conname LIKE mapping.current_prefix || '%'
        ORDER BY mapping.position
    LOOP
        EXECUTE format(
            'ALTER TABLE %I RENAME CONSTRAINT %I TO %I',
            item.current_table,
            item.conname,
            item.old_prefix || substr(item.conname, length(item.current_prefix) + 1)
        );
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER INDEX uk_principal_access_key_hash RENAME TO uk_access_key_hash;
ALTER INDEX ix_principal_access_key_principal RENAME TO ix_access_key_principal;
ALTER INDEX uk_principal_group_code RENAME TO uk_ai_group_code;
ALTER INDEX uk_principal_group_membership RENAME TO uk_principal_group;
ALTER INDEX ix_principal_group_membership_group RENAME TO ix_principal_group_group;
ALTER INDEX uk_provider_credential_active RENAME TO uk_resource_active;
ALTER INDEX ix_provider_credential_provider RENAME TO ix_resource_provider;
ALTER INDEX uk_principal_group_model_permission RENAME TO uk_group_model_permission;
ALTER INDEX ix_principal_group_model_permission_model RENAME TO ix_group_model_permission_model;
ALTER INDEX ix_usage_provider_credential_time RENAME TO ix_usage_resource_time;

ALTER TABLE usage_record RENAME COLUMN provider_credential_id TO resource_id;
ALTER TABLE principal_group_model_permission RENAME TO group_model_permission;
ALTER TABLE provider_credential RENAME TO ai_resource;
ALTER TABLE model RENAME TO ai_model;
ALTER TABLE provider RENAME TO ai_provider;
ALTER TABLE principal_access_key RENAME TO access_key;
ALTER TABLE principal_group RENAME TO ai_group;
ALTER TABLE principal_group_membership RENAME TO principal_group;
