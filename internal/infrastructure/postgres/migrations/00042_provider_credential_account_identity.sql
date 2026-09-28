-- +goose Up
-- 凭证名称仅用于展示，不参与业务唯一性判断。
-- OpenAI 等能够解析稳定账号标识的个人订阅，按服务商、认证适配器和上游账号去重。
DROP INDEX IF EXISTS uk_provider_credential_provider_name;

CREATE UNIQUE INDEX IF NOT EXISTS uk_provider_credential_subscription_account
    ON provider_credential (provider_id, auth_adapter, external_account_ref)
    WHERE is_deleted = false
      AND auth_type = 'SUBSCRIPTION'
      AND external_account_ref IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS uk_provider_credential_subscription_account;

-- 恢复旧约束前明确拒绝新结构允许的同名数据，避免静默丢失凭证。
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM provider_credential
        WHERE is_deleted = false
        GROUP BY provider_id, resource_name
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'Cannot restore provider credential name uniqueness while duplicate names exist';
    END IF;
END $$;
-- +goose StatementEnd

CREATE UNIQUE INDEX uk_provider_credential_provider_name
    ON provider_credential (provider_id, resource_name)
    WHERE is_deleted = false;
