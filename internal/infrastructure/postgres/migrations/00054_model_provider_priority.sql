-- +goose Up
UPDATE provider_model
SET priority = 100, updated_by = 'system', updated_at = now()
WHERE priority > 100;
ALTER TABLE provider_model DROP CONSTRAINT provider_model_priority_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_priority_check CHECK (priority BETWEEN 1 AND 100);

-- +goose Down
ALTER TABLE provider_model DROP CONSTRAINT provider_model_priority_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_priority_check CHECK (priority BETWEEN 1 AND 10000);
