-- +goose Up
ALTER TABLE provider_credential
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1
        CONSTRAINT provider_credential_version_check CHECK (version > 0);

COMMENT ON COLUMN provider_credential.version IS '乐观锁版本；每次资源状态、凭据或配置更新后递增';

-- +goose Down
ALTER TABLE provider_credential DROP COLUMN version;
