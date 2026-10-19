package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func (s *ManagementStore) ReadPrice(ctx context.Context, actor admin.Identity, fn func(mgmt.PriceReadSession) error) error {
	return s.run(ctx, actor, false, func(session *managementSession) error { return fn(session) })
}

func (s *ManagementStore) WritePrice(ctx context.Context, actor admin.Identity, fn func(mgmt.PriceWriteSession) error) error {
	return s.run(ctx, actor, true, func(session *managementSession) error { return fn(session) })
}

func priceNumber(value pgtype.Numeric) string {
	text := numericString(value)
	if text == nil {
		return ""
	}
	formatted := *text
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	return formatted
}

// optionalPriceNumeric 将空字符串映射为 NULL，用于可选的缓存写入单价。
func optionalPriceNumeric(value string) pgtype.Numeric {
	if value == "" {
		return pgtype.Numeric{}
	}
	return pgNumeric(&value)
}

func priceTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func (s *managementSession) ModelPrices(ctx context.Context, credentialID int64) ([]mgmt.ModelPrice, error) {
	rows, err := s.q.ManageModelPrices(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	prices := make([]mgmt.ModelPrice, 0, len(rows))
	for _, row := range rows {
		prices = append(prices, mgmt.ModelPrice{
			ID: row.ID, ProviderCredentialID: row.ProviderCredentialID, ProviderModelID: row.ProviderModelID,
			Currency: row.Currency, InputPrice: priceNumber(row.InputPrice),
			OutputPrice: priceNumber(row.OutputPrice), CachedInputPrice: priceNumber(row.CachedInputPrice),
			CacheCreationInputPrice: optionalPriceNumber(row.CacheCreationInputPrice),
			EffectiveAt:             row.EffectiveAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(),
		})
	}
	return prices, nil
}

func (s *managementSession) SubscriptionPrices(ctx context.Context, credentialID int64) ([]mgmt.SubscriptionPrice, error) {
	rows, err := s.q.ManageSubscriptionPrices(ctx, credentialID)
	if err != nil {
		return nil, err
	}
	prices := make([]mgmt.SubscriptionPrice, 0, len(rows))
	for _, row := range rows {
		prices = append(prices, mgmt.SubscriptionPrice{
			ID: row.ID, ProviderCredentialID: row.ProviderCredentialID, Currency: row.Currency,
			PeriodAmount: priceNumber(row.PeriodAmount), BillingPeriod: row.BillingPeriod,
			EffectiveAt: row.EffectiveAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(),
		})
	}
	return prices, nil
}

func (s *managementSession) InsertModelPrice(ctx context.Context, price mgmt.ModelPrice, actor string) error {
	input := pgNumeric(&price.InputPrice)
	output := pgNumeric(&price.OutputPrice)
	cached := pgNumeric(&price.CachedInputPrice)
	return s.q.ManageInsertModelPrice(ctx, dbgen.ManageInsertModelPriceParams{
		ID: price.ID, ProviderCredentialID: price.ProviderCredentialID,
		ProviderModelID: price.ProviderModelID, Currency: price.Currency,
		InputPrice: input, OutputPrice: output, CachedInputPrice: cached,
		CacheCreationInputPrice: pgNumeric(price.CacheCreationInputPrice),
		EffectiveAt:             priceTime(price.EffectiveAt), CreatedBy: actor, CreatedAt: priceTime(price.CreatedAt),
	})
}

func (s *managementSession) UpdateModelPrice(ctx context.Context, price mgmt.ModelPrice) (bool, error) {
	input := pgNumeric(&price.InputPrice)
	output := pgNumeric(&price.OutputPrice)
	cached := pgNumeric(&price.CachedInputPrice)
	count, err := s.q.ManageUpdateModelPrice(ctx, dbgen.ManageUpdateModelPriceParams{
		Currency: price.Currency, InputPrice: input, OutputPrice: output, CachedInputPrice: cached,
		CacheCreationInputPrice: pgNumeric(price.CacheCreationInputPrice), EffectiveAt: priceTime(price.EffectiveAt), ID: price.ID,
		ProviderCredentialID: price.ProviderCredentialID, ProviderModelID: price.ProviderModelID,
	})
	return count == 1, err
}

func (s *managementSession) DeleteModelPrice(ctx context.Context, credentialID, modelID, priceID int64) (bool, error) {
	count, err := s.q.ManageDeleteModelPrice(ctx, dbgen.ManageDeleteModelPriceParams{
		ID: priceID, ProviderCredentialID: credentialID, ProviderModelID: modelID,
	})
	return count == 1, err
}

func (s *managementSession) InsertSubscriptionPrice(ctx context.Context, price mgmt.SubscriptionPrice, actor string) error {
	amount := pgNumeric(&price.PeriodAmount)
	return s.q.ManageInsertSubscriptionPrice(ctx, dbgen.ManageInsertSubscriptionPriceParams{
		ID: price.ID, ProviderCredentialID: price.ProviderCredentialID, Currency: price.Currency,
		PeriodAmount: amount, BillingPeriod: price.BillingPeriod,
		EffectiveAt: priceTime(price.EffectiveAt), CreatedBy: actor, CreatedAt: priceTime(price.CreatedAt),
	})
}

func (s *managementSession) UpdateSubscriptionPrice(ctx context.Context, price mgmt.SubscriptionPrice) (bool, error) {
	amount := pgNumeric(&price.PeriodAmount)
	count, err := s.q.ManageUpdateSubscriptionPrice(ctx, dbgen.ManageUpdateSubscriptionPriceParams{
		Currency: price.Currency, PeriodAmount: amount, BillingPeriod: price.BillingPeriod,
		EffectiveAt: priceTime(price.EffectiveAt), ID: price.ID, ProviderCredentialID: price.ProviderCredentialID,
	})
	return count == 1, err
}

func (s *managementSession) DeleteSubscriptionPrice(ctx context.Context, credentialID, priceID int64) (bool, error) {
	count, err := s.q.ManageDeleteSubscriptionPrice(ctx, dbgen.ManageDeleteSubscriptionPriceParams{
		ID: priceID, ProviderCredentialID: credentialID,
	})
	return count == 1, err
}

func (s *managementSession) SubscriptionPriceReferenced(ctx context.Context, priceID int64) (bool, error) {
	return s.q.ManageSubscriptionPriceReferenced(ctx, priceID)
}

func (s *managementSession) SubscriptionChargedUntil(ctx context.Context, credentialID int64) (*time.Time, error) {
	value, err := s.q.ManageSubscriptionChargedUntil(ctx, credentialID)
	if err != nil || !value.Valid {
		return nil, err
	}
	chargedUntil := value.Time.UTC()
	return &chargedUntil, nil
}

func (s *managementSession) ModelPriceReferenced(ctx context.Context, priceID int64) (bool, error) {
	return s.q.ManageModelPriceReferenced(ctx, priceID)
}

func (s *managementSession) ModelPriceOverridesRatedUsage(ctx context.Context, price mgmt.ModelPrice) (bool, error) {
	return s.q.ManageModelPriceOverridesRatedUsage(ctx, dbgen.ManageModelPriceOverridesRatedUsageParams{
		ProviderCredentialID: price.ProviderCredentialID, ProviderModelID: price.ProviderModelID,
		EffectiveAt: priceTime(price.EffectiveAt), PriceID: price.ID,
	})
}

// priceTriggerConflicts 是价格保护触发器（迁移 00047、00048）抛出的固定报错文本。
// 只有这些报错代表"价格已被账单或核算引用 / 会覆盖已核算历史"的业务冲突；
// 其他 23503、23514 仍按依赖异常处理，避免掩盖真实的数据完整性问题。
var priceTriggerConflicts = map[string]string{
	"referenced subscription price cannot be deleted":        "23503",
	"referenced subscription price structure is immutable":   "23503",
	"referenced model price cannot be deleted":               "23503",
	"referenced model price structure is immutable":          "23503",
	"model price version would override rated usage history": "23514",
}

func isPriceTriggerConflict(code, message string) bool {
	expected, found := priceTriggerConflicts[message]
	return found && expected == code
}
