-- +goose Up
ALTER TABLE ai_model DROP COLUMN publisher_code;

-- +goose Down
-- 删除的历史发布方不推断恢复；回退后 unknown 表示待维护。
ALTER TABLE ai_model ADD COLUMN publisher_code VARCHAR(64) NOT NULL DEFAULT 'unknown';
ALTER TABLE ai_model ADD CONSTRAINT ck_model_publisher CHECK (publisher_code ~ '^[a-z0-9][a-z0-9._-]{0,63}$');
ALTER TABLE ai_model ALTER COLUMN publisher_code DROP DEFAULT;
COMMENT ON COLUMN ai_model.publisher_code IS '模型官方发布方编码；回退恢复为 unknown，不代表已知发布方';
