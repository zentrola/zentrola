-- +goose Up
-- Failover 会为同一客户端请求记录多次上游调用，attempt_no 从 1 开始递增。
ALTER TABLE usage_record
    DROP CONSTRAINT usage_record_attempt_no_check,
    ADD CONSTRAINT usage_record_attempt_no_check CHECK (attempt_no > 0);

COMMENT ON COLUMN usage_record.attempt_no IS '同一客户端请求的上游调用序号，从 1 开始；Failover 时逐次递增';

-- +goose Down
-- 保留已产生的 Failover 审计记录；NOT VALID 只恢复旧版本对后续写入的限制。
ALTER TABLE usage_record
    DROP CONSTRAINT usage_record_attempt_no_check,
    ADD CONSTRAINT usage_record_attempt_no_check CHECK (attempt_no = 1) NOT VALID;

COMMENT ON COLUMN usage_record.attempt_no IS '上游调用序号；MVP 固定为 1，不重试或 Failover';
