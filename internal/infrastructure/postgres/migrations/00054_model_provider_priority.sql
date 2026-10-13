-- +goose Up
-- 历史值超出新范围时停止迁移，由管理员先核对并显式调整，避免静默改变路由顺序。
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM provider_model WHERE priority > 100) THEN
        RAISE EXCEPTION 'provider_model has priority above 100; review and update these mappings before migration 00054';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE provider_model DROP CONSTRAINT provider_model_priority_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_priority_check CHECK (priority BETWEEN 1 AND 100);

-- +goose Down
ALTER TABLE provider_model DROP CONSTRAINT provider_model_priority_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_priority_check CHECK (priority BETWEEN 1 AND 10000);
