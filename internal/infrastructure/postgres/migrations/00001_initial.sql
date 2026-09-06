-- +goose Up
-- 阶段 0 空迁移，仅验证 Migration 执行及版本记录，不创建业务表。
SELECT 1;

-- +goose Down
SELECT 1;
