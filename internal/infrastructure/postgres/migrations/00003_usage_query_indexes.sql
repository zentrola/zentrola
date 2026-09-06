-- +goose Up
-- 阶段 5：请求事实列表支持组织时间范围和逻辑模型筛选；保留拒绝请求。
CREATE INDEX ix_ai_request_time ON ai_request (organization_id, request_at DESC);
CREATE INDEX ix_ai_request_model_time ON ai_request (organization_id, model_id, request_at DESC);

-- +goose Down
DROP INDEX ix_ai_request_model_time;
DROP INDEX ix_ai_request_time;
