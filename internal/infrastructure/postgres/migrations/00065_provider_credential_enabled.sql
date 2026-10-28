-- +goose Up
ALTER TABLE provider_credential
    ADD COLUMN is_enabled BOOLEAN NOT NULL DEFAULT TRUE;
COMMENT ON COLUMN provider_credential.is_enabled IS '管理员调用开关：TRUE=参与调用；FALSE=不参与调用';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM provider_credential WHERE NOT is_deleted AND NOT is_enabled) THEN
        RAISE EXCEPTION 'Cannot remove credential enabled flag while manually disabled credentials exist';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE provider_credential DROP COLUMN is_enabled;
