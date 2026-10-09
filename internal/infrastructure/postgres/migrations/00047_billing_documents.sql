-- +goose Up

CREATE TABLE billing_document (
    id BIGINT NOT NULL CONSTRAINT billing_document_id_check CHECK (id > 0),
    status VARCHAR(16) NOT NULL
        CONSTRAINT billing_document_status_check CHECK (status IN ('CONFIRMED', 'VOID')),
    billing_type VARCHAR(32) NOT NULL
        CONSTRAINT billing_document_billing_type_check CHECK (billing_type IN ('SUBSCRIPTION', 'API_KEY')),
    document_type VARCHAR(32) NOT NULL
        CONSTRAINT billing_document_document_type_check CHECK (document_type IN ('CHARGE', 'ADJUSTMENT')),
    provider_credential_id BIGINT NOT NULL REFERENCES provider_credential(id),
    source_type VARCHAR(32) NOT NULL
        CONSTRAINT billing_document_source_type_check CHECK (source_type IN ('SUBSCRIPTION_PRICE', 'API_KEY_USAGE')),
    source_id BIGINT NOT NULL CONSTRAINT billing_document_source_id_check CHECK (source_id > 0),
    original_document_id BIGINT REFERENCES billing_document(id),
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    total_tokens BIGINT NOT NULL CONSTRAINT billing_document_total_tokens_check CHECK (total_tokens >= 0),
    total_amount NUMERIC(20, 8) NOT NULL,
    currency VARCHAR(3) NOT NULL CONSTRAINT billing_document_currency_check CHECK (currency IN ('CNY', 'USD')),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT billing_document_pkey PRIMARY KEY (id),
    CONSTRAINT ck_billing_document_period CHECK (period_end > period_start),
    CONSTRAINT ck_billing_document_source CHECK (
        (billing_type = 'SUBSCRIPTION' AND source_type = 'SUBSCRIPTION_PRICE')
        OR
        (billing_type = 'API_KEY' AND source_type = 'API_KEY_USAGE')
    ),
    CONSTRAINT ck_billing_document_original CHECK (
        (document_type = 'CHARGE' AND original_document_id IS NULL AND total_amount >= 0)
        OR
        (document_type = 'ADJUSTMENT' AND original_document_id IS NOT NULL AND total_amount <> 0)
    )
);

CREATE TABLE billing_document_item (
    id BIGINT NOT NULL CONSTRAINT billing_document_item_id_check CHECK (id > 0),
    billing_document_id BIGINT NOT NULL REFERENCES billing_document(id),
    principal_id BIGINT NOT NULL REFERENCES principal(id),
    usage_tokens BIGINT NOT NULL CONSTRAINT billing_document_item_usage_tokens_check CHECK (usage_tokens >= 0),
    allocation_ratio NUMERIC(20, 16)
        CONSTRAINT billing_document_item_allocation_ratio_check CHECK (allocation_ratio >= 0 AND allocation_ratio <= 1),
    amount NUMERIC(20, 8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT billing_document_item_pkey PRIMARY KEY (id)
);

CREATE UNIQUE INDEX uk_billing_document_charge_period
    ON billing_document (billing_type, provider_credential_id, period_start, period_end)
    WHERE document_type = 'CHARGE' AND status = 'CONFIRMED';

CREATE UNIQUE INDEX uk_billing_document_item_principal
    ON billing_document_item (billing_document_id, principal_id)
    WHERE TRUE;

CREATE INDEX ix_billing_document_statistics
    ON billing_document (period_start, billing_type, status, currency);

CREATE INDEX ix_billing_document_credential_period
    ON billing_document (provider_credential_id, period_start, period_end);

CREATE INDEX ix_billing_document_original
    ON billing_document (original_document_id)
    WHERE original_document_id IS NOT NULL;

CREATE INDEX ix_billing_document_item_principal
    ON billing_document_item (principal_id, billing_document_id);

COMMENT ON TABLE billing_document IS '统一计费单据；按计费类型和 UTC 归属账期保存个人订阅与 API Key 的费用确认及调整结果';
COMMENT ON COLUMN billing_document.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN billing_document.status IS '单据状态：CONFIRMED=已确认并参与统计；VOID=已作废且不参与统计';
COMMENT ON COLUMN billing_document.billing_type IS '计费类型：SUBSCRIPTION=个人订阅；API_KEY=API Key 按量调用';
COMMENT ON COLUMN billing_document.document_type IS '单据类型：CHARGE=账期费用；ADJUSTMENT=对原单据的差额调整';
COMMENT ON COLUMN billing_document.provider_credential_id IS '费用对应的实际供应方凭证 ID';
COMMENT ON COLUMN billing_document.source_type IS '费用来源类型：SUBSCRIPTION_PRICE=订阅价格版本；API_KEY_USAGE=API Key 用量核算汇总';
COMMENT ON COLUMN billing_document.source_id IS '费用来源记录 ID；含义由 source_type 决定';
COMMENT ON COLUMN billing_document.original_document_id IS '调整单对应的原始账期单据 ID；普通账期单据为 NULL';
COMMENT ON COLUMN billing_document.period_start IS '费用归属账期起始时间，UTC，包含该时刻';
COMMENT ON COLUMN billing_document.period_end IS '费用归属账期结束时间，UTC，不包含该时刻';
COMMENT ON COLUMN billing_document.total_tokens IS '原始账期单据用于分摊的输入与输出 Token 总数；调整单固定为 0，避免统计重复累计';
COMMENT ON COLUMN billing_document.total_amount IS '单据总金额；账期费用为非负数，调整金额可正可负';
COMMENT ON COLUMN billing_document.currency IS '单据币种：CNY=人民币，USD=美元；不同币种不自动换算或合计';
COMMENT ON COLUMN billing_document.created_at IS '单据创建时间，UTC';

COMMENT ON TABLE billing_document_item IS '统一计费单据主体明细；保存个人订阅分摊结果或 API Key 按量费用结果，用于主体成本统计';
COMMENT ON COLUMN billing_document_item.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN billing_document_item.billing_document_id IS '所属计费单据 ID';
COMMENT ON COLUMN billing_document_item.principal_id IS '费用归属的用户或应用主体 ID';
COMMENT ON COLUMN billing_document_item.usage_tokens IS '主体在原始账期内的计费 Token 数；调整明细固定为 0，避免统计重复累计';
COMMENT ON COLUMN billing_document_item.allocation_ratio IS '个人订阅费用按 Token 计算的分摊比例；API Key 按量费用为 NULL；调整明细沿用原单据比例';
COMMENT ON COLUMN billing_document_item.amount IS '主体承担的费用金额；个人订阅为分摊金额，API Key 为按量核算金额，调整明细可正可负';
COMMENT ON COLUMN billing_document_item.created_at IS '明细创建时间，UTC';

-- +goose StatementBegin
CREATE FUNCTION validate_billing_document_source() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    source_currency VARCHAR(3);
    source_amount NUMERIC(20, 8);
BEGIN
    IF NEW.billing_type = 'SUBSCRIPTION' THEN
        SELECT currency, period_amount
        INTO source_currency, source_amount
        FROM provider_credential_subscription_price
        WHERE id = NEW.source_id
          AND provider_credential_id = NEW.provider_credential_id
        FOR SHARE;

        IF NOT FOUND THEN
            RAISE EXCEPTION 'subscription billing source does not exist' USING ERRCODE = '23503';
        END IF;
        IF NEW.currency <> source_currency THEN
            RAISE EXCEPTION 'subscription billing currency does not match source' USING ERRCODE = '23514';
        END IF;
        IF NEW.document_type = 'CHARGE' AND NEW.total_amount <> source_amount THEN
            RAISE EXCEPTION 'subscription charge amount does not match source' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION validate_billing_document_source() IS '校验订阅计费单据的价格来源、凭证、币种和账期金额，并通过行锁避免来源并发删除或修改';

CREATE TRIGGER billing_document_source_validation
    BEFORE INSERT ON billing_document
    FOR EACH ROW EXECUTE FUNCTION validate_billing_document_source();

-- +goose StatementBegin
CREATE FUNCTION protect_referenced_subscription_price() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM billing_document
        WHERE source_type = 'SUBSCRIPTION_PRICE'
          AND source_id = OLD.id
    ) THEN
        IF TG_OP = 'DELETE' THEN
            RAISE EXCEPTION 'referenced subscription price cannot be deleted' USING ERRCODE = '23503';
        END IF;
        IF NEW.id <> OLD.id
            OR NEW.provider_credential_id <> OLD.provider_credential_id
            OR NEW.currency <> OLD.currency
            OR NEW.billing_period <> OLD.billing_period
            OR NEW.effective_at <> OLD.effective_at THEN
            RAISE EXCEPTION 'referenced subscription price structure is immutable' USING ERRCODE = '23503';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION protect_referenced_subscription_price() IS '阻止删除账单已引用的订阅价格，或修改其凭证、币种、周期和生效时间；周期金额仍允许纠错';

CREATE TRIGGER subscription_price_reference_protection
    BEFORE UPDATE OR DELETE ON provider_credential_subscription_price
    FOR EACH ROW EXECUTE FUNCTION protect_referenced_subscription_price();

-- +goose StatementBegin
CREATE FUNCTION reject_billing_document_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'billing documents are append only' USING ERRCODE = '42501';
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION reject_billing_document_mutation() IS '阻止普通操作修改、删除或清空计费单据及明细；金额纠错必须新增调整单';

CREATE TRIGGER billing_document_append_only
    BEFORE UPDATE OR DELETE OR TRUNCATE ON billing_document
    FOR EACH STATEMENT EXECUTE FUNCTION reject_billing_document_mutation();

CREATE TRIGGER billing_document_item_append_only
    BEFORE UPDATE OR DELETE OR TRUNCATE ON billing_document_item
    FOR EACH STATEMENT EXECUTE FUNCTION reject_billing_document_mutation();

-- +goose Down

DROP TRIGGER subscription_price_reference_protection ON provider_credential_subscription_price;
DROP TRIGGER billing_document_source_validation ON billing_document;
DROP TRIGGER billing_document_item_append_only ON billing_document_item;
DROP TRIGGER billing_document_append_only ON billing_document;
DROP FUNCTION protect_referenced_subscription_price();
DROP FUNCTION validate_billing_document_source();
DROP FUNCTION reject_billing_document_mutation();
DROP TABLE billing_document_item;
DROP TABLE billing_document;
