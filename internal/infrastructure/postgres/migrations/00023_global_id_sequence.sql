-- +goose Up
-- 所有业务表共用一个 BIGINT sequence，多实例由 PostgreSQL 保证 ID 唯一。
CREATE SEQUENCE zentrola_global_id_seq
    AS BIGINT
    INCREMENT BY 1
    MINVALUE 1
    NO MAXVALUE
    NO CYCLE
    OWNED BY NONE;

-- 升级已有数据库时，从当前 schema 所有 BIGINT id 的最大值之后继续。
-- +goose StatementBegin
DO $$
DECLARE
    candidate RECORD;
    table_max BIGINT;
    existing_max BIGINT := 0;
BEGIN
    FOR candidate IN
        SELECT columns.table_schema, columns.table_name
        FROM information_schema.columns AS columns
        JOIN information_schema.tables AS tables
          ON tables.table_schema = columns.table_schema
         AND tables.table_name = columns.table_name
         AND tables.table_type = 'BASE TABLE'
        WHERE columns.table_schema = current_schema()
          AND columns.column_name = 'id'
          AND columns.data_type = 'bigint'
    LOOP
        EXECUTE format('SELECT max(id) FROM %I.%I', candidate.table_schema, candidate.table_name)
           INTO table_max;
        existing_max := GREATEST(existing_max, COALESCE(table_max, 0));
    END LOOP;

    IF existing_max = 0 THEN
        PERFORM setval('zentrola_global_id_seq', 1, false);
    ELSE
        PERFORM setval('zentrola_global_id_seq', existing_max, true);
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP SEQUENCE zentrola_global_id_seq;
