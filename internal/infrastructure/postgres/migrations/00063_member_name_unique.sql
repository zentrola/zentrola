-- +goose Up
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM principal WHERE principal_type='MEMBER' AND is_deleted=false
        GROUP BY btrim(name) HAVING count(*)>1
    ) THEN
        RAISE EXCEPTION 'Duplicate member names exist; rename them before upgrading. Find them with: SELECT btrim(name), array_agg(id) FROM principal WHERE principal_type=''MEMBER'' AND NOT is_deleted GROUP BY btrim(name) HAVING count(*)>1';
    END IF;
END $$;
-- +goose StatementEnd
CREATE UNIQUE INDEX uk_principal_member_name ON principal (btrim(name))
    WHERE principal_type='MEMBER' AND is_deleted=false;
COMMENT ON INDEX uk_principal_member_name IS '未删除成员名称唯一（忽略首尾空白）';

-- +goose Down
DROP INDEX IF EXISTS uk_principal_member_name;
