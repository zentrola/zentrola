-- +goose Up
ALTER TABLE operation_log DROP CONSTRAINT operation_log_module_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_module_check
    CHECK (module IN ('MEMBER','ACCESS_KEY','GROUP','MODEL','PROVIDER','RESOURCE','AUTH'));
ALTER TABLE operation_log DROP CONSTRAINT operation_log_target_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_target_type_check
    CHECK (target_type IN ('PRINCIPAL','ACCESS_KEY','GROUP','MODEL','PROVIDER','RESOURCE','ADMIN_USER'));
COMMENT ON COLUMN operation_log.module IS '业务模块：MEMBER=成员；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；PROVIDER=服务商；RESOURCE=服务商凭证；AUTH=认证';
COMMENT ON COLUMN operation_log.target_type IS '目标类型：PRINCIPAL=治理主体；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；PROVIDER=服务商；RESOURCE=服务商凭证；ADMIN_USER=管理员';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
IF EXISTS(SELECT 1 FROM operation_log WHERE module='PROVIDER' OR target_type='PROVIDER') THEN
    RAISE EXCEPTION 'Cannot downgrade while provider operation logs exist';
END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE operation_log DROP CONSTRAINT operation_log_module_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_module_check
    CHECK (module IN ('MEMBER','ACCESS_KEY','GROUP','MODEL','RESOURCE','AUTH'));
ALTER TABLE operation_log DROP CONSTRAINT operation_log_target_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_target_type_check
    CHECK (target_type IN ('PRINCIPAL','ACCESS_KEY','GROUP','MODEL','RESOURCE','ADMIN_USER'));
COMMENT ON COLUMN operation_log.module IS '业务模块：MEMBER=成员；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；RESOURCE=资源；AUTH=认证';
COMMENT ON COLUMN operation_log.target_type IS '目标类型：PRINCIPAL=治理主体；ACCESS_KEY=调用凭证；GROUP=分组；MODEL=模型；RESOURCE=资源；ADMIN_USER=管理员';
