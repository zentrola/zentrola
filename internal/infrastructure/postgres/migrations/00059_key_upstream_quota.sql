-- +goose Up
ALTER TABLE principal_access_key ADD COLUMN can_view_upstream_quota BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN principal_access_key.can_view_upstream_quota IS '允许该成员 Key 查看自己最近实际使用的订阅号额度快照';
ALTER TABLE usage_record ADD COLUMN access_key_id BIGINT;
COMMENT ON COLUMN usage_record.access_key_id IS '发起调用的 Access Key ID；历史记录为 NULL';

-- +goose Down
ALTER TABLE usage_record DROP COLUMN access_key_id;
ALTER TABLE principal_access_key DROP COLUMN can_view_upstream_quota;
