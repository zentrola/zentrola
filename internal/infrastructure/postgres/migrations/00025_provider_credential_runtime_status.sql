-- +goose Up
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

-- +goose Down
ALTER TABLE provider_credential
    DROP CONSTRAINT ck_provider_credential_blocked_reason,
    DROP CONSTRAINT ck_provider_credential_runtime_block,
    DROP COLUMN last_error_code,
    DROP COLUMN last_http_status,
    DROP COLUMN last_error_at,
    DROP COLUMN blocked_at,
    DROP COLUMN blocked_reason,
    DROP COLUMN runtime_status,
    ADD COLUMN last_active_at TIMESTAMPTZ;

COMMENT ON COLUMN provider_credential.last_active_at IS '最后调用时间；NULL=从未使用';
