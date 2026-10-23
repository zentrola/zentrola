-- +goose NO TRANSACTION

-- +goose Up
DROP INDEX CONCURRENTLY IF EXISTS ix_usage_record_key_model_recent;
CREATE INDEX CONCURRENTLY ix_usage_record_key_model_recent
    ON usage_record (access_key_id, model_id, completed_at DESC, id DESC)
    WHERE access_key_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS ix_usage_record_key_model_recent;
