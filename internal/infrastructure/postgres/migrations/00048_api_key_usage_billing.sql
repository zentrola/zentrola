-- +goose Up
ALTER TABLE billing_document
    ALTER COLUMN source_id DROP NOT NULL,
    DROP CONSTRAINT billing_document_source_id_check,
    ADD CONSTRAINT billing_document_source_id_check CHECK (source_id IS NULL OR source_id > 0),
    DROP CONSTRAINT ck_billing_document_source,
    ADD CONSTRAINT ck_billing_document_source CHECK (
        (billing_type = 'SUBSCRIPTION' AND source_type = 'SUBSCRIPTION_PRICE' AND source_id IS NOT NULL)
        OR
        (billing_type = 'API_KEY' AND source_type = 'API_KEY_USAGE' AND source_id IS NULL)
    ),
    DROP CONSTRAINT ck_billing_document_original,
    ADD CONSTRAINT ck_billing_document_original CHECK (
        (document_type = 'CHARGE' AND original_document_id IS NULL AND total_amount >= 0)
        OR
        (document_type = 'ADJUSTMENT' AND original_document_id IS NOT NULL)
    );

DROP INDEX uk_billing_document_charge_period;
CREATE UNIQUE INDEX uk_billing_document_charge_period
    ON billing_document (provider_credential_id, period_start, period_end)
    WHERE billing_type = 'SUBSCRIPTION' AND document_type = 'CHARGE' AND status = 'CONFIRMED';

CREATE UNIQUE INDEX uk_billing_document_api_key_charge_period
    ON billing_document (provider_credential_id, period_start, period_end, currency)
    WHERE billing_type = 'API_KEY' AND document_type = 'CHARGE' AND status = 'CONFIRMED';

CREATE TABLE usage_rating (
    id BIGINT NOT NULL CONSTRAINT usage_rating_id_check CHECK (id > 0),
    usage_record_id BIGINT NOT NULL REFERENCES usage_record(id),
    revision INTEGER NOT NULL CONSTRAINT usage_rating_revision_check CHECK (revision > 0),
    supersedes_rating_id BIGINT REFERENCES usage_rating(id),
    principal_id BIGINT NOT NULL REFERENCES principal(id),
    provider_credential_id BIGINT NOT NULL REFERENCES provider_credential(id),
    provider_model_id BIGINT NOT NULL REFERENCES provider_model(id),
    model_price_id BIGINT NOT NULL REFERENCES provider_credential_model_price(id),
    usage_started_at TIMESTAMPTZ NOT NULL,
    input_tokens BIGINT NOT NULL CONSTRAINT usage_rating_input_tokens_check CHECK (input_tokens >= 0),
    cached_input_tokens BIGINT NOT NULL CONSTRAINT usage_rating_cached_input_tokens_check
        CHECK (cached_input_tokens >= 0 AND cached_input_tokens <= input_tokens),
    output_tokens BIGINT NOT NULL CONSTRAINT usage_rating_output_tokens_check CHECK (output_tokens >= 0),
    input_price NUMERIC(20, 8) NOT NULL CONSTRAINT usage_rating_input_price_check CHECK (input_price >= 0),
    cached_input_price NUMERIC(20, 8) NOT NULL CONSTRAINT usage_rating_cached_input_price_check CHECK (cached_input_price >= 0),
    output_price NUMERIC(20, 8) NOT NULL CONSTRAINT usage_rating_output_price_check CHECK (output_price >= 0),
    input_cost NUMERIC(30, 14) NOT NULL CONSTRAINT usage_rating_input_cost_check CHECK (input_cost >= 0),
    cached_input_cost NUMERIC(30, 14) NOT NULL CONSTRAINT usage_rating_cached_input_cost_check CHECK (cached_input_cost >= 0),
    output_cost NUMERIC(30, 14) NOT NULL CONSTRAINT usage_rating_output_cost_check CHECK (output_cost >= 0),
    total_cost NUMERIC(30, 14) NOT NULL CONSTRAINT usage_rating_total_cost_check CHECK (total_cost >= 0),
    currency VARCHAR(3) NOT NULL CONSTRAINT usage_rating_currency_check CHECK (currency IN ('CNY', 'USD')),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT usage_rating_pkey PRIMARY KEY (id),
    CONSTRAINT uk_usage_rating_revision UNIQUE (usage_record_id, revision),
    CONSTRAINT uk_usage_rating_supersedes UNIQUE (supersedes_rating_id),
    CONSTRAINT ck_usage_rating_revision_chain CHECK (
        (revision = 1 AND supersedes_rating_id IS NULL)
        OR
        (revision > 1 AND supersedes_rating_id IS NOT NULL)
    )
);

CREATE INDEX ix_usage_rating_current
    ON usage_rating (usage_record_id, revision DESC, id DESC);

CREATE INDEX ix_usage_rating_settlement
    ON usage_rating (provider_credential_id, usage_started_at, currency, principal_id);

CREATE TABLE billing_document_usage_rating (
    billing_document_id BIGINT NOT NULL REFERENCES billing_document(id),
    usage_rating_id BIGINT NOT NULL REFERENCES usage_rating(id),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT billing_document_usage_rating_pkey PRIMARY KEY (billing_document_id, usage_rating_id),
    CONSTRAINT uk_billing_document_usage_rating UNIQUE (usage_rating_id)
);

CREATE INDEX ix_billing_document_usage_rating_document
    ON billing_document_usage_rating (billing_document_id, usage_rating_id);

COMMENT ON TABLE usage_rating IS 'API Key 单次调用的计价快照；价格纠错时追加 revision，原核算记录永久保留';
COMMENT ON COLUMN usage_rating.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN usage_rating.usage_record_id IS '被核算的原始上游调用用量记录 ID';
COMMENT ON COLUMN usage_rating.revision IS '同一用量记录的核算版本，从 1 开始连续递增';
COMMENT ON COLUMN usage_rating.supersedes_rating_id IS '当前 revision 直接替代的上一核算记录 ID；首版为 NULL';
COMMENT ON COLUMN usage_rating.principal_id IS '费用直接归属的用户或应用主体 ID';
COMMENT ON COLUMN usage_rating.provider_credential_id IS '实际调用的 API Key 凭证 ID';
COMMENT ON COLUMN usage_rating.provider_model_id IS '实际调用的供应方模型映射 ID';
COMMENT ON COLUMN usage_rating.model_price_id IS '按调用发生时间命中的模型价格版本 ID';
COMMENT ON COLUMN usage_rating.usage_started_at IS '原始调用开始时间快照，UTC；用于确定价格版本和自然月账期';
COMMENT ON COLUMN usage_rating.input_tokens IS '输入 Token 总数快照，包含缓存命中输入';
COMMENT ON COLUMN usage_rating.cached_input_tokens IS '缓存命中输入 Token 数快照';
COMMENT ON COLUMN usage_rating.output_tokens IS '输出 Token 数快照';
COMMENT ON COLUMN usage_rating.input_price IS '普通输入每百万 Token 价格快照，保留 8 位小数';
COMMENT ON COLUMN usage_rating.cached_input_price IS '缓存命中输入每百万 Token 价格快照，保留 8 位小数';
COMMENT ON COLUMN usage_rating.output_price IS '输出每百万 Token 价格快照，保留 8 位小数';
COMMENT ON COLUMN usage_rating.input_cost IS '普通输入成本，保留 14 位小数';
COMMENT ON COLUMN usage_rating.cached_input_cost IS '缓存命中输入成本，保留 14 位小数';
COMMENT ON COLUMN usage_rating.output_cost IS '输出成本，保留 14 位小数';
COMMENT ON COLUMN usage_rating.total_cost IS '单次调用总成本，保留 14 位小数';
COMMENT ON COLUMN usage_rating.currency IS '成本币种：CNY=人民币，USD=美元；不同币种分别结算';
COMMENT ON COLUMN usage_rating.created_at IS '核算记录创建时间，UTC';

COMMENT ON TABLE billing_document_usage_rating IS 'API Key 计费单据与单次调用核算 revision 的追加式审计关联';
COMMENT ON COLUMN billing_document_usage_rating.billing_document_id IS '引用的 API Key 计费单据 ID';
COMMENT ON COLUMN billing_document_usage_rating.usage_rating_id IS '被该单据首次确认的单次调用核算 revision ID';
COMMENT ON COLUMN billing_document_usage_rating.created_at IS '审计关联创建时间，UTC';

COMMENT ON COLUMN billing_document.source_id IS '费用来源记录 ID；订阅单据为价格版本 ID，API Key 汇总单据为 NULL';
COMMENT ON COLUMN billing_document.total_tokens IS '单据新增确认的输入与输出 Token 数；价格纠错调整单为 0，晚到用量调整单可大于 0';
COMMENT ON COLUMN billing_document_item.usage_tokens IS '主体在该单据中新增确认的输入与输出 Token 数；价格纠错调整明细为 0';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_billing_document_source() RETURNS trigger LANGUAGE plpgsql AS $$
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
    ELSIF NEW.source_id IS NOT NULL THEN
        RAISE EXCEPTION 'api key billing source id must be null' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION validate_usage_rating() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    usage_row usage_record%ROWTYPE;
    price_row provider_credential_model_price%ROWTYPE;
    previous_row usage_rating%ROWTYPE;
BEGIN
    SELECT * INTO usage_row FROM usage_record WHERE id = NEW.usage_record_id FOR SHARE;
    IF NOT FOUND OR usage_row.status <> 'SUCCESS'
        OR usage_row.input_tokens IS NULL OR usage_row.output_tokens IS NULL
        OR usage_row.cached_input_tokens IS NULL THEN
        RAISE EXCEPTION 'usage rating requires complete successful usage' USING ERRCODE = '23514';
    END IF;

    IF NEW.principal_id <> usage_row.principal_id
        OR NEW.provider_credential_id <> usage_row.provider_credential_id
        OR NEW.provider_model_id <> usage_row.provider_model_id
        OR NEW.usage_started_at <> usage_row.started_at
        OR NEW.input_tokens <> usage_row.input_tokens
        OR NEW.cached_input_tokens <> usage_row.cached_input_tokens
        OR NEW.output_tokens <> usage_row.output_tokens THEN
        RAISE EXCEPTION 'usage rating fact snapshot does not match usage record' USING ERRCODE = '23514';
    END IF;

    SELECT p.* INTO price_row
    FROM provider_credential_model_price p
    JOIN provider_credential c ON c.id = p.provider_credential_id AND c.auth_type = 'API_KEY'
    WHERE p.provider_credential_id = usage_row.provider_credential_id
      AND p.provider_model_id = usage_row.provider_model_id
      AND p.effective_at <= usage_row.started_at
    ORDER BY p.effective_at DESC, p.id DESC
    LIMIT 1
    FOR SHARE OF p;

    IF NOT FOUND OR NEW.model_price_id <> price_row.id
        OR NEW.currency <> price_row.currency
        OR NEW.input_price <> price_row.input_price
        OR NEW.cached_input_price <> price_row.cached_input_price
        OR NEW.output_price <> price_row.output_price THEN
        RAISE EXCEPTION 'usage rating price snapshot is not current' USING ERRCODE = '23514';
    END IF;

    SELECT * INTO previous_row
    FROM usage_rating
    WHERE usage_record_id = NEW.usage_record_id
    ORDER BY revision DESC, id DESC
    LIMIT 1
    FOR UPDATE;

    IF NOT FOUND THEN
        IF NEW.revision <> 1 OR NEW.supersedes_rating_id IS NOT NULL THEN
            RAISE EXCEPTION 'usage rating root revision is invalid' USING ERRCODE = '23514';
        END IF;
    ELSIF NEW.revision = previous_row.revision THEN
        -- 并发任务可能基于同一上一版计算出相同 revision；后到者直接幂等跳过。
        RETURN NULL;
    ELSIF NEW.revision <> previous_row.revision + 1
        OR NEW.supersedes_rating_id <> previous_row.id THEN
        RAISE EXCEPTION 'usage rating revision chain is invalid' USING ERRCODE = '23514';
    END IF;

    IF NEW.input_cost <> ROUND(((NEW.input_tokens - NEW.cached_input_tokens)::numeric * NEW.input_price) / 1000000, 14)
        OR NEW.cached_input_cost <> ROUND((NEW.cached_input_tokens::numeric * NEW.cached_input_price) / 1000000, 14)
        OR NEW.output_cost <> ROUND((NEW.output_tokens::numeric * NEW.output_price) / 1000000, 14)
        OR NEW.total_cost <> NEW.input_cost + NEW.cached_input_cost + NEW.output_cost THEN
        RAISE EXCEPTION 'usage rating cost calculation is invalid' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION validate_usage_rating() IS '校验用量事实、当时有效价格、连续 revision 链和 14 位定点成本公式';

CREATE TRIGGER usage_rating_validation
    BEFORE INSERT ON usage_rating
    FOR EACH ROW EXECUTE FUNCTION validate_usage_rating();

-- +goose StatementBegin
CREATE FUNCTION protect_referenced_model_price() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM usage_rating WHERE model_price_id = OLD.id) THEN
        IF TG_OP = 'DELETE' THEN
            RAISE EXCEPTION 'referenced model price cannot be deleted' USING ERRCODE = '23503';
        END IF;
        IF NEW.id <> OLD.id
            OR NEW.provider_credential_id <> OLD.provider_credential_id
            OR NEW.provider_model_id <> OLD.provider_model_id
            OR NEW.currency <> OLD.currency
            OR NEW.effective_at <> OLD.effective_at THEN
            RAISE EXCEPTION 'referenced model price structure is immutable' USING ERRCODE = '23503';
        END IF;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION protect_referenced_model_price() IS '阻止删除核算已引用的模型价格，或修改其凭证、模型、币种和生效时间；价格金额仍允许纠错';

CREATE TRIGGER model_price_reference_protection
    BEFORE UPDATE OR DELETE ON provider_credential_model_price
    FOR EACH ROW EXECUTE FUNCTION protect_referenced_model_price();

-- +goose StatementBegin
CREATE FUNCTION reject_model_price_history_override() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM usage_record usage
        JOIN LATERAL (
            SELECT rating.model_price_id
            FROM usage_rating rating
            WHERE rating.usage_record_id = usage.id
            ORDER BY rating.revision DESC, rating.id DESC
            LIMIT 1
        ) latest ON TRUE
        WHERE usage.provider_credential_id = NEW.provider_credential_id
          AND usage.provider_model_id = NEW.provider_model_id
          AND usage.started_at >= NEW.effective_at
          AND latest.model_price_id <> NEW.id
          AND NOT EXISTS (
              SELECT 1
              FROM provider_credential_model_price later
              WHERE later.id <> CASE WHEN TG_OP = 'UPDATE' THEN OLD.id ELSE NEW.id END
                AND later.provider_credential_id = NEW.provider_credential_id
                AND later.provider_model_id = NEW.provider_model_id
                AND later.effective_at <= usage.started_at
                AND (later.effective_at > NEW.effective_at
                    OR (later.effective_at = NEW.effective_at AND later.id > NEW.id))
          )
    ) THEN
        RAISE EXCEPTION 'model price version would override rated usage history' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION reject_model_price_history_override() IS '禁止新增或移动价格版本去覆盖已核算调用；正常调价必须面向尚未核算的未来调用';

CREATE TRIGGER model_price_history_override_rejection
    BEFORE INSERT OR UPDATE ON provider_credential_model_price
    FOR EACH ROW EXECUTE FUNCTION reject_model_price_history_override();

CREATE TRIGGER usage_rating_append_only
    BEFORE UPDATE OR DELETE OR TRUNCATE ON usage_rating
    FOR EACH STATEMENT EXECUTE FUNCTION reject_billing_document_mutation();

CREATE TRIGGER billing_document_usage_rating_append_only
    BEFORE UPDATE OR DELETE OR TRUNCATE ON billing_document_usage_rating
    FOR EACH STATEMENT EXECUTE FUNCTION reject_billing_document_mutation();

-- +goose Down
DROP TRIGGER billing_document_usage_rating_append_only ON billing_document_usage_rating;
DROP TRIGGER usage_rating_append_only ON usage_rating;
DROP TRIGGER model_price_history_override_rejection ON provider_credential_model_price;
DROP TRIGGER model_price_reference_protection ON provider_credential_model_price;
DROP TRIGGER usage_rating_validation ON usage_rating;
DROP FUNCTION protect_referenced_model_price();
DROP FUNCTION reject_model_price_history_override();
DROP FUNCTION validate_usage_rating();
DROP TABLE billing_document_usage_rating;
DROP TABLE usage_rating;

DROP INDEX uk_billing_document_api_key_charge_period;
DROP INDEX uk_billing_document_charge_period;
CREATE UNIQUE INDEX uk_billing_document_charge_period
    ON billing_document (billing_type, provider_credential_id, period_start, period_end)
    WHERE document_type = 'CHARGE' AND status = 'CONFIRMED';

ALTER TABLE billing_document
    DROP CONSTRAINT billing_document_source_id_check,
    ADD CONSTRAINT billing_document_source_id_check CHECK (source_id > 0),
    ALTER COLUMN source_id SET NOT NULL,
    DROP CONSTRAINT ck_billing_document_source,
    ADD CONSTRAINT ck_billing_document_source CHECK (
        (billing_type = 'SUBSCRIPTION' AND source_type = 'SUBSCRIPTION_PRICE')
        OR
        (billing_type = 'API_KEY' AND source_type = 'API_KEY_USAGE')
    ),
    DROP CONSTRAINT ck_billing_document_original,
    ADD CONSTRAINT ck_billing_document_original CHECK (
        (document_type = 'CHARGE' AND original_document_id IS NULL AND total_amount >= 0)
        OR
        (document_type = 'ADJUSTMENT' AND original_document_id IS NOT NULL AND total_amount <> 0)
    );

COMMENT ON COLUMN billing_document.source_id IS '费用来源记录 ID；含义由 source_type 决定';
COMMENT ON COLUMN billing_document.total_tokens IS '原始账期单据用于分摊的输入与输出 Token 总数；调整单固定为 0，避免统计重复累计';
COMMENT ON COLUMN billing_document_item.usage_tokens IS '主体在原始账期内的计费 Token 数；调整明细固定为 0，避免统计重复累计';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_billing_document_source() RETURNS trigger LANGUAGE plpgsql AS $$
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
