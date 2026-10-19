-- +goose Up

-- 核算扫描所需的 usage_record / usage_rating 索引由 00058 在事务外以 CONCURRENTLY 方式创建，
-- 避免建索引期间阻塞用量与核算写入。

-- +goose StatementBegin
CREATE FUNCTION billing_rating_matches_price(
    rating_price_id BIGINT,
    rating_currency TEXT,
    rating_input_price NUMERIC,
    rating_cached_input_price NUMERIC,
    rating_output_price NUMERIC,
    price_id BIGINT,
    price_currency TEXT,
    price_input_price NUMERIC,
    price_cached_input_price NUMERIC,
    price_output_price NUMERIC
) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT COALESCE(
        rating_price_id = price_id
        AND rating_currency = price_currency
        AND rating_input_price = price_input_price
        AND rating_cached_input_price = price_cached_input_price
        AND rating_output_price = price_output_price,
        FALSE
    )
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION billing_rating_matches_price(BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC) IS
    '判断最新核算的价格快照是否与调用时有效价格一致；任一侧缺失时返回 FALSE。单表达式 SQL 函数会被规划器内联，不影响查询计划';

-- +goose StatementBegin
CREATE FUNCTION reject_overlapping_subscription_charge() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.billing_type = 'SUBSCRIPTION' AND NEW.document_type = 'CHARGE' AND NEW.status = 'CONFIRMED' THEN
        -- 同一凭据的订阅账期写入串行化，避免并发插入同时通过重叠检查。
        PERFORM pg_advisory_xact_lock(hashtextextended('billing_subscription_charge:' || NEW.provider_credential_id::text, 0));
        IF EXISTS (
            SELECT 1
            FROM billing_document existing
            WHERE existing.provider_credential_id = NEW.provider_credential_id
              AND existing.billing_type = 'SUBSCRIPTION'
              AND existing.document_type = 'CHARGE'
              AND existing.status = 'CONFIRMED'
              AND existing.period_start < NEW.period_end
              AND existing.period_end > NEW.period_start
              AND NOT (existing.period_start = NEW.period_start AND existing.period_end = NEW.period_end)
        ) THEN
            RAISE EXCEPTION 'subscription charge period overlaps an existing charge' USING ERRCODE = '23P01';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION reject_overlapping_subscription_charge() IS
    '拒绝与同一凭据已确认订阅账期部分重叠的新账期单据；完全相同的账期由唯一索引幂等处理。历史已重叠的数据不受影响';

CREATE TRIGGER billing_document_subscription_overlap_rejection
    BEFORE INSERT ON billing_document
    FOR EACH ROW EXECUTE FUNCTION reject_overlapping_subscription_charge();

-- +goose Down
DROP TRIGGER billing_document_subscription_overlap_rejection ON billing_document;
DROP FUNCTION reject_overlapping_subscription_charge();
DROP FUNCTION billing_rating_matches_price(BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC);
