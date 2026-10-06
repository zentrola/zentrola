-- +goose Up
CREATE TABLE provider_credential_model_price (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    provider_credential_id BIGINT NOT NULL REFERENCES provider_credential(id),
    provider_model_id BIGINT NOT NULL REFERENCES provider_model(id),
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('CNY', 'USD')),
    input_price NUMERIC(20, 8) NOT NULL CHECK (input_price >= 0),
    output_price NUMERIC(20, 8) NOT NULL CHECK (output_price >= 0),
    cached_input_price NUMERIC(20, 8) NOT NULL CHECK (cached_input_price >= 0),
    effective_at TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uk_credential_model_price_effective
        UNIQUE (provider_credential_id, provider_model_id, effective_at)
);

COMMENT ON TABLE provider_credential_model_price IS '按量凭证按生效时间保留的模型计价版本';
COMMENT ON COLUMN provider_credential_model_price.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider_credential_model_price.provider_credential_id IS '计价所属的 API Key 凭证 ID';
COMMENT ON COLUMN provider_credential_model_price.provider_model_id IS '计价对应的供应方模型映射 ID';
COMMENT ON COLUMN provider_credential_model_price.currency IS '计价币种：CNY=人民币，USD=美元；不同币种不自动换算或合计';
COMMENT ON COLUMN provider_credential_model_price.input_price IS '普通输入每百万 Token 的价格';
COMMENT ON COLUMN provider_credential_model_price.output_price IS '输出每百万 Token 的价格';
COMMENT ON COLUMN provider_credential_model_price.cached_input_price IS '缓存命中输入每百万 Token 的价格';
COMMENT ON COLUMN provider_credential_model_price.effective_at IS '此版本价格的生效时间，UTC';
COMMENT ON COLUMN provider_credential_model_price.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_credential_model_price.created_at IS '记录创建时间，UTC';

CREATE TABLE provider_credential_subscription_price (
    id BIGINT PRIMARY KEY CHECK (id > 0),
    provider_credential_id BIGINT NOT NULL REFERENCES provider_credential(id),
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('CNY', 'USD')),
    period_amount NUMERIC(20, 8) NOT NULL CHECK (period_amount >= 0),
    billing_period VARCHAR(16) NOT NULL CHECK (billing_period IN ('MONTH', 'YEAR')),
    effective_at TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(64) NOT NULL CHECK (btrim(created_by) <> ''),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uk_credential_subscription_price_effective
        UNIQUE (provider_credential_id, effective_at)
);

COMMENT ON TABLE provider_credential_subscription_price IS '订阅凭证按生效时间保留的周期费用版本';
COMMENT ON COLUMN provider_credential_subscription_price.id IS '主键，由应用侧生成的正数 64-bit ID';
COMMENT ON COLUMN provider_credential_subscription_price.provider_credential_id IS '费用所属的订阅凭证 ID';
COMMENT ON COLUMN provider_credential_subscription_price.currency IS '费用币种：CNY=人民币，USD=美元；不同币种不自动换算或合计';
COMMENT ON COLUMN provider_credential_subscription_price.period_amount IS '一个完整计费周期的固定费用，非每日分摊金额';
COMMENT ON COLUMN provider_credential_subscription_price.billing_period IS '计费周期：MONTH=月，YEAR=年';
COMMENT ON COLUMN provider_credential_subscription_price.effective_at IS '此版本费用的生效时间，UTC';
COMMENT ON COLUMN provider_credential_subscription_price.created_by IS '创建者引用：system、admin:<id> 或 principal:<id>';
COMMENT ON COLUMN provider_credential_subscription_price.created_at IS '记录创建时间，UTC';

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN (
    'ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET',
    'MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE',
    'APPLICATION_CREATE','APPLICATION_UPDATE','APPLICATION_DELETE','APPLICATION_STATUS_CHANGE',
    'ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE',
    'GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE',
    'GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_APPLICATION_ADD','GROUP_APPLICATION_REMOVE',
    'GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE',
    'MODEL_CREATE','MODEL_UPDATE','MODEL_DELETE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC',
    'PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE',
    'RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_CREDENTIAL_EXPORT','RESOURCE_STATUS_CHANGE',
    'RESOURCE_DELETE','RESOURCE_CONNECTION_TEST','RESOURCE_RATE_LIMIT_RESET','RESOURCE_PRICE_UPDATE',
    'LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'
));

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM operation_log WHERE operation_type='RESOURCE_PRICE_UPDATE') THEN
        RAISE EXCEPTION 'Cannot remove credential price audit type while price operation logs exist';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE operation_log DROP CONSTRAINT operation_log_operation_type_check;
ALTER TABLE operation_log ADD CONSTRAINT operation_log_operation_type_check CHECK (operation_type IN (
    'ADMIN_INITIALIZE','ADMIN_PASSWORD_RESET',
    'MEMBER_CREATE','MEMBER_UPDATE','MEMBER_DELETE','MEMBER_STATUS_CHANGE',
    'APPLICATION_CREATE','APPLICATION_UPDATE','APPLICATION_DELETE','APPLICATION_STATUS_CHANGE',
    'ACCESS_KEY_CREATE','ACCESS_KEY_REVOKE',
    'GROUP_CREATE','GROUP_UPDATE','GROUP_STATUS_CHANGE','GROUP_DELETE',
    'GROUP_MEMBER_ADD','GROUP_MEMBER_REMOVE','GROUP_APPLICATION_ADD','GROUP_APPLICATION_REMOVE',
    'GROUP_MODEL_GRANT','GROUP_MODEL_REVOKE',
    'MODEL_CREATE','MODEL_UPDATE','MODEL_DELETE','MODEL_STATUS_CHANGE','MODEL_CATALOG_SYNC',
    'PROVIDER_CREATE','PROVIDER_UPDATE','PROVIDER_DELETE','PROVIDER_STATUS_CHANGE',
    'RESOURCE_CREATE','RESOURCE_CREDENTIAL_UPDATE','RESOURCE_CREDENTIAL_EXPORT','RESOURCE_STATUS_CHANGE',
    'RESOURCE_DELETE','RESOURCE_CONNECTION_TEST','RESOURCE_RATE_LIMIT_RESET',
    'LOGIN_SUCCESS','LOGIN_FAILED','LOGIN_LOCKED'
));

DROP TABLE provider_credential_subscription_price;
DROP TABLE provider_credential_model_price;
