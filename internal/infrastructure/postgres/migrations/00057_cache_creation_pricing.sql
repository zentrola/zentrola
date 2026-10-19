-- +goose Up

-- Anthropic 的缓存写入（cache_creation_input_tokens）单价通常高于普通输入。用量事实单独记录缓存写入
-- Token，价格与核算快照增加缓存写入单价和成本；未配置缓存写入单价时按普通输入单价计费，保持既有行为。

-- 新列全部为 NULL，合并为一个 NOT VALID 约束只校验新写入的记录，避免迁移时全表扫描用量事实表。
ALTER TABLE usage_record ADD COLUMN cache_creation_input_tokens BIGINT;
ALTER TABLE usage_record ADD CONSTRAINT usage_record_cache_creation_input_tokens_check CHECK (
    cache_creation_input_tokens IS NULL
    OR (
        cache_creation_input_tokens >= 0
        AND (input_tokens IS NULL OR cache_creation_input_tokens + COALESCE(cached_input_tokens, 0) <= input_tokens)
    )
) NOT VALID;

COMMENT ON COLUMN usage_record.input_tokens IS '输入 Token 总数，包含缓存读取与缓存写入；NULL=上游未返回可靠值，0=已确认零用量';
COMMENT ON COLUMN usage_record.cache_creation_input_tokens IS '缓存写入 Token 数，包含在 input_tokens 中；NULL=上游未返回或协议不区分缓存写入，计费按 0 处理';

ALTER TABLE provider_credential_model_price
    ADD COLUMN cache_creation_input_price NUMERIC(20, 8)
        CONSTRAINT provider_credential_model_price_cache_creation_input_price_check
        CHECK (cache_creation_input_price >= 0);

COMMENT ON COLUMN provider_credential_model_price.input_price IS '普通输入每百万 Token 的价格；未配置缓存写入单价时缓存写入也按此价格计费';
COMMENT ON COLUMN provider_credential_model_price.cache_creation_input_price IS '缓存写入输入每百万 Token 的价格；NULL=未配置，按普通输入单价计费';

-- usage_rating 为追加式表，默认值 0 回填历史核算（历史调用的缓存写入按 0 处理），并保持与未写入新列的
-- 既有插入语句兼容；触发器会按用量记录与价格复核缓存写入 Token 和成本，默认值不会造成错误计费。
-- 常量默认值只写入元数据；历史记录取默认值 0/NULL 必然满足新约束，因此约束统一以 NOT VALID 添加，
-- 只校验新写入的记录，避免持有 ACCESS EXCLUSIVE 锁时全表扫描核算表。
ALTER TABLE usage_rating
    ADD COLUMN cache_creation_input_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN cache_creation_input_price NUMERIC(20, 8),
    ADD COLUMN cache_creation_input_cost NUMERIC(30, 14) NOT NULL DEFAULT 0,
    ADD CONSTRAINT usage_rating_cache_creation_input_price_check
        CHECK (cache_creation_input_price >= 0) NOT VALID,
    ADD CONSTRAINT usage_rating_cache_creation_input_cost_check
        CHECK (cache_creation_input_cost >= 0) NOT VALID,
    ADD CONSTRAINT usage_rating_cache_creation_input_tokens_check CHECK (
        cache_creation_input_tokens >= 0
        AND cache_creation_input_tokens <= input_tokens - cached_input_tokens
    ) NOT VALID;
COMMENT ON COLUMN usage_rating.input_tokens IS '输入 Token 总数快照，包含缓存命中与缓存写入输入';
COMMENT ON COLUMN usage_rating.cache_creation_input_tokens IS '缓存写入输入 Token 数快照；用量记录未区分缓存写入时为 0';
COMMENT ON COLUMN usage_rating.cache_creation_input_price IS '缓存写入输入每百万 Token 价格快照，保留 8 位小数；NULL=价格未配置，按普通输入单价计费';
COMMENT ON COLUMN usage_rating.input_cost IS '普通输入成本，不含缓存命中与缓存写入输入，保留 14 位小数';
COMMENT ON COLUMN usage_rating.cache_creation_input_cost IS '缓存写入输入成本，保留 14 位小数';
COMMENT ON COLUMN usage_rating.total_cost IS '单次调用总成本，等于普通输入、缓存命中、缓存写入与输出成本之和，保留 14 位小数';

-- 价格纠错重算扫描使用的 ix_usage_rating_price_snapshot 包含缓存写入单价快照，由 00058 在事务外
-- 以 CONCURRENTLY 方式创建。

DROP FUNCTION billing_rating_matches_price(BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC);

-- +goose StatementBegin
CREATE FUNCTION billing_rating_matches_price(
    rating_price_id BIGINT,
    rating_currency TEXT,
    rating_input_price NUMERIC,
    rating_cached_input_price NUMERIC,
    rating_cache_creation_input_price NUMERIC,
    rating_output_price NUMERIC,
    price_id BIGINT,
    price_currency TEXT,
    price_input_price NUMERIC,
    price_cached_input_price NUMERIC,
    price_cache_creation_input_price NUMERIC,
    price_output_price NUMERIC
) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT COALESCE(
        rating_price_id = price_id
        AND rating_currency = price_currency
        AND rating_input_price = price_input_price
        AND rating_cached_input_price = price_cached_input_price
        AND rating_cache_creation_input_price IS NOT DISTINCT FROM price_cache_creation_input_price
        AND rating_output_price = price_output_price,
        FALSE
    )
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION billing_rating_matches_price(BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, NUMERIC, BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, NUMERIC) IS
    '判断最新核算的价格快照是否与调用时有效价格一致；缓存写入单价两侧均未配置视为一致，价格或核算缺失时返回 FALSE。单表达式 SQL 函数会被规划器内联，不影响查询计划';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_usage_rating() RETURNS trigger LANGUAGE plpgsql AS $$
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
        OR NEW.cache_creation_input_tokens <> COALESCE(usage_row.cache_creation_input_tokens, 0)
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
        OR NEW.cache_creation_input_price IS DISTINCT FROM price_row.cache_creation_input_price
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

    IF NEW.input_cost <> ROUND(((NEW.input_tokens - NEW.cached_input_tokens - NEW.cache_creation_input_tokens)::numeric * NEW.input_price) / 1000000, 14)
        OR NEW.cached_input_cost <> ROUND((NEW.cached_input_tokens::numeric * NEW.cached_input_price) / 1000000, 14)
        OR NEW.cache_creation_input_cost <> ROUND((NEW.cache_creation_input_tokens::numeric * COALESCE(NEW.cache_creation_input_price, NEW.input_price)) / 1000000, 14)
        OR NEW.output_cost <> ROUND((NEW.output_tokens::numeric * NEW.output_price) / 1000000, 14)
        OR NEW.total_cost <> NEW.input_cost + NEW.cached_input_cost + NEW.cache_creation_input_cost + NEW.output_cost THEN
        RAISE EXCEPTION 'usage rating cost calculation is invalid' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION validate_usage_rating() IS '校验用量事实（含缓存写入）、当时有效价格、连续 revision 链和 14 位定点成本公式；未配置缓存写入单价时按普通输入单价核算';

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_usage_rating() RETURNS trigger LANGUAGE plpgsql AS $$
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

DROP FUNCTION billing_rating_matches_price(BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, NUMERIC, BIGINT, TEXT, NUMERIC, NUMERIC, NUMERIC, NUMERIC);

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

-- 回滚会丢失缓存写入维度：按缓存写入单价核算的历史成本仍保留在 total_cost 中，
-- 但旧版核算公式无法再复算这些记录。
ALTER TABLE usage_rating
    DROP CONSTRAINT usage_rating_cache_creation_input_tokens_check,
    DROP CONSTRAINT usage_rating_cache_creation_input_cost_check,
    DROP CONSTRAINT usage_rating_cache_creation_input_price_check,
    DROP COLUMN cache_creation_input_cost,
    DROP COLUMN cache_creation_input_price,
    DROP COLUMN cache_creation_input_tokens;

COMMENT ON COLUMN usage_rating.input_tokens IS '输入 Token 总数快照，包含缓存命中输入';
COMMENT ON COLUMN usage_rating.input_cost IS '普通输入成本，保留 14 位小数';
COMMENT ON COLUMN usage_rating.total_cost IS '单次调用总成本，保留 14 位小数';

ALTER TABLE provider_credential_model_price DROP COLUMN cache_creation_input_price;
COMMENT ON COLUMN provider_credential_model_price.input_price IS '普通输入每百万 Token 的价格';

ALTER TABLE usage_record
    DROP CONSTRAINT usage_record_cache_creation_input_tokens_check,
    DROP COLUMN cache_creation_input_tokens;
COMMENT ON COLUMN usage_record.input_tokens IS '输入 Token 数；NULL=上游未返回可靠值，0=已确认零用量';
