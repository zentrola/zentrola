-- +goose Up
-- 额度查看与成员管理合并为单一“后台管理”授权；只有额度授权的 Key 降为普通 Key，避免静默提权。
ALTER TABLE principal_access_key ADD COLUMN can_manage BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN principal_access_key.can_manage IS '后台管理授权：允许成员 Key 查看最近使用的订阅号额度，并通过受限 MCP 接口管理普通成员';
UPDATE principal_access_key SET can_manage=can_manage_members WHERE can_manage_members;
ALTER TABLE principal_access_key DROP COLUMN can_manage_members;
ALTER TABLE principal_access_key DROP COLUMN can_view_upstream_quota;

-- +goose Down
ALTER TABLE principal_access_key ADD COLUMN can_view_upstream_quota BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN principal_access_key.can_view_upstream_quota IS '允许该成员 Key 查看自己最近实际使用的订阅号额度快照';
ALTER TABLE principal_access_key ADD COLUMN can_manage_members BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN principal_access_key.can_manage_members IS '允许成员 Key 通过受限 MCP 接口创建普通成员和签发普通成员 Key';
UPDATE principal_access_key SET can_view_upstream_quota=can_manage,can_manage_members=can_manage WHERE can_manage;
ALTER TABLE principal_access_key DROP COLUMN can_manage;
