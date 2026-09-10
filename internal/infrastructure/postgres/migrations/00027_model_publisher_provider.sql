-- +goose Up
ALTER TABLE model
    ADD COLUMN publisher_provider_id BIGINT
    CONSTRAINT model_publisher_provider_id_check CHECK (publisher_provider_id > 0);
COMMENT ON COLUMN model.publisher_provider_id IS '模型发布厂商对应的服务商 ID；手工创建且尚未由官方目录同步时为空';
COMMENT ON TABLE model IS '平台稳定逻辑模型；发布厂商仅用于归属展示，与调用供应方映射解耦';

-- +goose Down
ALTER TABLE model DROP COLUMN publisher_provider_id;
COMMENT ON TABLE model IS '平台稳定逻辑模型；与供应方解耦';
