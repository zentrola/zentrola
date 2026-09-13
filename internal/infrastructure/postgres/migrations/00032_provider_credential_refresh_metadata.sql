-- +goose Up
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

-- +goose Down
DROP INDEX ix_provider_credential_subscription_refresh;
ALTER TABLE provider_credential
    DROP COLUMN credential_expires_at,
    DROP COLUMN credential_refreshed_at;
