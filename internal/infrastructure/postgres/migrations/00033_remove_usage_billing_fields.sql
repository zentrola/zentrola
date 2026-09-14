-- +goose Up
-- usage_record 只保存上游调用产生的原始用量事实；价格与成本由独立的核算模型处理。
ALTER TABLE usage_record
    DROP CONSTRAINT ck_usage_record_cost,
    DROP COLUMN billing_unit,
    DROP COLUMN billing_quantity,
    DROP COLUMN cost_amount,
    DROP COLUMN cost_currency;

COMMENT ON TABLE usage_record IS '一次真实上游调用 Attempt 的原始用量事实；不保存价格与成本，无逻辑删除';

-- +goose Down
ALTER TABLE usage_record
    ADD COLUMN billing_unit VARCHAR(48) NOT NULL DEFAULT 'TOKEN'
        CONSTRAINT usage_record_billing_unit_check
        CHECK (billing_unit IN ('TOKEN', 'CALL', 'IMAGE', 'SECOND')),
    ADD COLUMN billing_quantity NUMERIC(20,8)
        CONSTRAINT usage_record_billing_quantity_check
        CHECK (billing_quantity >= 0),
    ADD COLUMN cost_amount NUMERIC(20,8)
        CONSTRAINT usage_record_cost_amount_check
        CHECK (cost_amount >= 0),
    ADD COLUMN cost_currency VARCHAR(3)
        CONSTRAINT usage_record_cost_currency_check
        CHECK (cost_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT ck_usage_record_cost
        CHECK ((cost_amount IS NULL) = (cost_currency IS NULL));

ALTER TABLE usage_record
    ALTER COLUMN billing_unit DROP DEFAULT;

COMMENT ON COLUMN usage_record.billing_unit IS '计量单位：TOKEN=Token；CALL=调用次数；IMAGE=图像数量；SECOND=秒数';
COMMENT ON COLUMN usage_record.billing_quantity IS '计量数量；NULL=无法可靠确认；非客户销售金额';
COMMENT ON COLUMN usage_record.cost_amount IS '上游成本快照；NULL=未计算成本';
COMMENT ON COLUMN usage_record.cost_currency IS '成本币种；NULL=未计算成本';
COMMENT ON TABLE usage_record IS '一次真实上游调用 Attempt 的用量事实；无逻辑删除';
