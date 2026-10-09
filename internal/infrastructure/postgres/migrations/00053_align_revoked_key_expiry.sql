-- +goose Up
-- 已撤销密钥的失效时间统一以撤销时间为准，包括原本设置过到期时间的历史记录。
UPDATE principal_access_key
SET expires_at = revoked_at
WHERE status = 'REVOKED' AND revoked_at IS NOT NULL AND expires_at IS DISTINCT FROM revoked_at;

-- +goose Down
-- 原到期时间无法恢复，保留已统一的失效时刻。
SELECT 1;
