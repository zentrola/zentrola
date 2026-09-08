-- +goose Up
ALTER TABLE access_key DROP CONSTRAINT access_key_key_prefix_check;
ALTER TABLE access_key RENAME COLUMN key_prefix TO masked_key;
ALTER TABLE access_key ALTER COLUMN masked_key TYPE VARCHAR(64);
UPDATE access_key SET masked_key = masked_key || '********';
ALTER TABLE access_key ADD CONSTRAINT access_key_masked_key_check
    CHECK (btrim(masked_key) <> '' AND position('*' IN masked_key) > 0);
COMMENT ON COLUMN access_key.masked_key IS '脱敏后的 Access Key，仅用于识别和展示，不能用于认证';

-- +goose Down
ALTER TABLE access_key DROP CONSTRAINT access_key_masked_key_check;
UPDATE access_key SET masked_key = split_part(masked_key, '*', 1);
ALTER TABLE access_key RENAME COLUMN masked_key TO key_prefix;
ALTER TABLE access_key ALTER COLUMN key_prefix TYPE VARCHAR(32);
ALTER TABLE access_key ADD CONSTRAINT access_key_key_prefix_check CHECK (btrim(key_prefix) <> '');
COMMENT ON COLUMN access_key.key_prefix IS '仅用于识别的 Key 前缀，不能用于认证';
