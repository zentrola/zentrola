-- +goose Up
-- 仅补齐历史撤销记录中原本为空的到期时间，不覆盖原有到期信息。
UPDATE principal_access_key
SET expires_at = revoked_at
WHERE status = 'REVOKED' AND revoked_at IS NOT NULL AND expires_at IS NULL;

-- +goose Down
-- 无法区分历史空值与原有到期时间，保留已补齐的失效时刻。
SELECT 1;
