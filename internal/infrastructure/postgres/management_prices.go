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
			EffectiveAt: row.EffectiveAt.Time.UTC(), CreatedAt: row.CreatedAt.Time.UTC(),
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
		EffectiveAt: priceTime(price.EffectiveAt), CreatedBy: actor, CreatedAt: priceTime(price.CreatedAt),
	})
}

func (s *managementSession) UpdateModelPrice(ctx context.Context, price mgmt.ModelPrice) (bool, error) {
	input := pgNumeric(&price.InputPrice)
	output := pgNumeric(&price.OutputPrice)
	cached := pgNumeric(&price.CachedInputPrice)
	count, err := s.q.ManageUpdateModelPrice(ctx, dbgen.ManageUpdateModelPriceParams{
		Currency: price.Currency, InputPrice: input, OutputPrice: output, CachedInputPrice: cached,
		EffectiveAt: priceTime(price.EffectiveAt), ID: price.ID,
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
