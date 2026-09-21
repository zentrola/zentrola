-- +goose Up
ALTER TABLE provider_endpoint
ADD COLUMN network_scope VARCHAR(16) NOT NULL DEFAULT 'PUBLIC'
    CHECK (network_scope IN ('PUBLIC', 'PRIVATE'));

COMMENT ON COLUMN provider_endpoint.network_scope IS '上游网络范围：PUBLIC=仅公网地址；PRIVATE=管理员显式允许私网地址';

-- +goose Down
ALTER TABLE provider_endpoint DROP COLUMN network_scope;
