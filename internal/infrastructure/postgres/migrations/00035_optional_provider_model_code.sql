-- +goose Up
ALTER TABLE provider_model DROP CONSTRAINT provider_model_upstream_model_code_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_upstream_model_code_check
    CHECK (upstream_model_code = btrim(upstream_model_code));
COMMENT ON COLUMN provider_model.upstream_model_code IS '服务商模型编码覆盖；空字符串表示调用时使用系统模型编码';

-- +goose Down
UPDATE provider_model pm
SET upstream_model_code = m.model_code
FROM model m
WHERE pm.model_id = m.id AND pm.upstream_model_code = '';
ALTER TABLE provider_model DROP CONSTRAINT provider_model_upstream_model_code_check;
ALTER TABLE provider_model ADD CONSTRAINT provider_model_upstream_model_code_check
    CHECK (btrim(upstream_model_code) <> '');
COMMENT ON COLUMN provider_model.upstream_model_code IS '实际发送到上游的模型编码';
