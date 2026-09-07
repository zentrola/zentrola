-- +goose Up
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

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS(SELECT 1 FROM ai_provider WHERE proxy_enabled) THEN
    RAISE EXCEPTION 'Cannot downgrade while provider proxy configuration exists';
END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE ai_provider DROP CONSTRAINT ck_provider_proxy;
ALTER TABLE ai_provider DROP CONSTRAINT ck_provider_proxy_header_names;
ALTER TABLE ai_provider
    DROP COLUMN proxy_headers_key_version,
    DROP COLUMN proxy_headers_nonce,
    DROP COLUMN proxy_headers_ciphertext,
    DROP COLUMN proxy_header_names,
    DROP COLUMN proxy_url_key_version,
    DROP COLUMN proxy_url_nonce,
    DROP COLUMN proxy_url_ciphertext,
    DROP COLUMN proxy_url_display,
    DROP COLUMN proxy_enabled;
