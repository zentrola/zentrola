-- +goose Up

ALTER TABLE provider_credential_subscription_price
    DROP COLUMN IF EXISTS billing_anchor_at;

-- +goose Down

ALTER TABLE provider_credential_subscription_price
    ADD COLUMN billing_anchor_at TIMESTAMPTZ;

UPDATE provider_credential_subscription_price
SET billing_anchor_at = effective_at;

ALTER TABLE provider_credential_subscription_price
    ALTER COLUMN billing_anchor_at SET NOT NULL;

COMMENT ON COLUMN provider_credential_subscription_price.billing_anchor_at IS
    '计费周期起点，后续周期以此为锚点，UTC';
