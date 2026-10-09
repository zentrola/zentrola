-- +goose Up
-- Group 归属在调用发生时写入，后续成员关系和模型授权变更不改写历史用量。
ALTER TABLE usage_record
    ADD COLUMN quota_group_ids BIGINT[] NOT NULL DEFAULT '{}'::bigint[];

CREATE TABLE monthly_token_usage (
    scope_type VARCHAR(16) NOT NULL CHECK (scope_type IN ('PRINCIPAL', 'GROUP')),
    scope_id BIGINT NOT NULL CHECK (scope_id > 0),
    period_start TIMESTAMPTZ NOT NULL,
    used_tokens BIGINT NOT NULL CHECK (used_tokens >= 0),
    PRIMARY KEY (scope_type, scope_id, period_start)
);

COMMENT ON COLUMN usage_record.quota_group_ids IS '调用发生时具有该模型权限的有效 Group ID 快照，不受后续成员和授权变更影响';
COMMENT ON TABLE monthly_token_usage IS '与 Usage 写入事务同步维护的主体及 Group 每月 Token 累计量';
COMMENT ON COLUMN monthly_token_usage.scope_type IS '配额范围类型：PRINCIPAL 或 GROUP';
COMMENT ON COLUMN monthly_token_usage.scope_id IS '主体或 Group ID';
COMMENT ON COLUMN monthly_token_usage.period_start IS 'UTC 自然月起始时刻';
COMMENT ON COLUMN monthly_token_usage.used_tokens IS '该范围该月已写入 Usage 的输入与输出 Token 总量';

-- 仅补齐当前 UTC 月；更早月份不会参与当前月配额准入。
-- 升级前的记录没有调用时 Group 快照，只能用升级时的有效归属补齐。
UPDATE usage_record AS usage
SET quota_group_ids = COALESCE((
    SELECT array_agg(DISTINCT membership.group_id ORDER BY membership.group_id)
    FROM principal_group_membership membership
    JOIN principal_group g ON g.id = membership.group_id
    JOIN principal_group_model_permission permission
      ON permission.group_id = g.id AND permission.model_id = usage.model_id
    WHERE membership.principal_id = usage.principal_id
      AND NOT membership.is_deleted
      AND NOT g.is_deleted AND g.status = 'ACTIVE'
      AND NOT permission.is_deleted
), '{}'::bigint[])
WHERE usage.started_at >= date_trunc('month', now(), 'UTC')::timestamptz
  AND usage.started_at < (date_trunc('month', now(), 'UTC') + interval '1 month')::timestamptz;

WITH scoped AS (
    SELECT 'PRINCIPAL'::varchar(16) AS scope_type, principal_id AS scope_id,
           date_trunc('month', started_at, 'UTC')::timestamptz AS period_start,
           COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) AS tokens
    FROM usage_record
    WHERE started_at >= date_trunc('month', now(), 'UTC')::timestamptz
      AND started_at < (date_trunc('month', now(), 'UTC') + interval '1 month')::timestamptz
    UNION ALL
    SELECT 'GROUP'::varchar(16), group_id,
           date_trunc('month', usage.started_at, 'UTC')::timestamptz,
           COALESCE(usage.input_tokens, 0) + COALESCE(usage.output_tokens, 0)
    FROM usage_record usage
    CROSS JOIN LATERAL unnest(usage.quota_group_ids) AS group_id
    WHERE usage.started_at >= date_trunc('month', now(), 'UTC')::timestamptz
      AND usage.started_at < (date_trunc('month', now(), 'UTC') + interval '1 month')::timestamptz
)
INSERT INTO monthly_token_usage (scope_type, scope_id, period_start, used_tokens)
SELECT scope_type, scope_id, period_start, SUM(tokens)::bigint
FROM scoped
GROUP BY scope_type, scope_id, period_start;

-- +goose StatementBegin
CREATE FUNCTION tally_monthly_token_usage() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    WITH scoped AS (
        SELECT 'PRINCIPAL'::varchar(16) AS scope_type, principal_id AS scope_id,
               date_trunc('month', started_at, 'UTC')::timestamptz AS period_start,
               COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) AS tokens
        FROM new_usage
        UNION ALL
        SELECT 'GROUP'::varchar(16), group_id,
               date_trunc('month', usage.started_at, 'UTC')::timestamptz,
               COALESCE(usage.input_tokens, 0) + COALESCE(usage.output_tokens, 0)
        FROM new_usage usage
        CROSS JOIN LATERAL unnest(usage.quota_group_ids) AS group_id
    ), totals AS (
        SELECT scope_type, scope_id, period_start, SUM(tokens)::bigint AS tokens
        FROM scoped
        GROUP BY scope_type, scope_id, period_start
    )
    INSERT INTO monthly_token_usage (scope_type, scope_id, period_start, used_tokens)
    SELECT scope_type, scope_id, period_start, tokens FROM totals
    ON CONFLICT (scope_type, scope_id, period_start)
    DO UPDATE SET used_tokens = monthly_token_usage.used_tokens + EXCLUDED.used_tokens;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER usage_monthly_token_tally
    AFTER INSERT ON usage_record
    REFERENCING NEW TABLE AS new_usage
    FOR EACH STATEMENT EXECUTE FUNCTION tally_monthly_token_usage();

-- +goose Down
DROP TRIGGER usage_monthly_token_tally ON usage_record;
DROP FUNCTION tally_monthly_token_usage();
DROP TABLE monthly_token_usage;
ALTER TABLE usage_record DROP COLUMN quota_group_ids;
