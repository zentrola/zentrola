package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/zentrola/zentrola/internal/application/billing"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type BillingStore struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
}

func NewBillingStore(pool *pgxpool.Pool) *BillingStore {
	return &BillingStore{pool: pool, q: dbgen.New(pool)}
}

func (s *BillingStore) SubscriptionPrices(ctx context.Context) ([]app.SubscriptionPrice, error) {
	rows, err := s.q.BillingSubscriptionPrices(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]app.SubscriptionPrice, 0, len(rows))
	for _, row := range rows {
		price := app.SubscriptionPrice{
			ID: row.ID, CredentialID: row.ProviderCredentialID, Currency: row.Currency,
			PeriodAmount: priceNumber(row.PeriodAmount), BillingPeriod: row.BillingPeriod,
			EffectiveAt: row.EffectiveAt.Time.UTC(), CredentialEffectiveAt: row.CredentialEffectiveAt.Time.UTC(),
		}
		if row.CredentialEndAt.Valid {
			end := row.CredentialEndAt.Time.UTC()
			price.CredentialEndAt = &end
		}
		result = append(result, price)
	}
	return result, nil
}

func (s *BillingStore) ChargePeriods(ctx context.Context) ([]app.ChargePeriod, error) {
	rows, err := s.q.BillingChargePeriods(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]app.ChargePeriod, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.ChargePeriod{
			CredentialID: row.ProviderCredentialID,
			PeriodStart:  row.PeriodStart.Time.UTC(), PeriodEnd: row.PeriodEnd.Time.UTC(),
		})
	}
	return result, nil
}

func (s *BillingStore) SubscriptionUsage(ctx context.Context, credentialID int64, start, end time.Time) ([]domain.UsageShare, error) {
	rows, err := s.q.BillingSubscriptionUsage(ctx, dbgen.BillingSubscriptionUsageParams{
		ProviderCredentialID: credentialID, PeriodStart: priceTime(start), PeriodEnd: priceTime(end),
	})
	if err != nil {
		return nil, err
	}
	result := make([]domain.UsageShare, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.UsageShare{PrincipalID: row.PrincipalID, Tokens: row.Tokens})
	}
	return result, nil
}

func (s *BillingStore) CreateCharge(ctx context.Context, document app.Document) (bool, error) {
	return s.createDocument(ctx, document, "")
}

func (s *BillingStore) SubscriptionCorrections(ctx context.Context) ([]app.Correction, error) {
	rows, err := s.q.BillingSubscriptionCorrections(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]app.Correction, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.Correction{
			OriginalDocumentID: row.OriginalDocumentID, CredentialID: row.ProviderCredentialID,
			SourcePriceID: row.SourcePriceID, PeriodStart: row.PeriodStart.Time.UTC(), PeriodEnd: row.PeriodEnd.Time.UTC(),
			Currency: row.Currency, TargetCurrency: row.TargetCurrency,
			CurrentAmount: priceNumber(row.CurrentAmount), TargetAmount: priceNumber(row.TargetAmount),
		})
	}
	return result, nil
}

func (s *BillingStore) OriginalUsage(ctx context.Context, documentID int64) ([]domain.UsageShare, error) {
	rows, err := s.q.BillingOriginalUsage(ctx, documentID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.UsageShare, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.UsageShare{PrincipalID: row.PrincipalID, Tokens: row.Tokens})
	}
	return result, nil
}

func (s *BillingStore) CreateAdjustment(ctx context.Context, expectedCurrent string, document app.Document) (bool, error) {
	return s.createDocument(ctx, document, expectedCurrent)
}

func (s *BillingStore) createDocument(ctx context.Context, document app.Document, expectedCurrent string) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if document.DocumentType == "ADJUSTMENT" {
		if _, err := q.BillingLockDocument(ctx, document.OriginalDocumentID); err != nil {
			return false, err
		}
		current, err := q.BillingDocumentNetAmount(ctx, document.OriginalDocumentID)
		if err != nil {
			return false, err
		}
		if priceNumber(current) != expectedCurrent {
			return false, nil
		}
	}
	var originalID *int64
	if document.OriginalDocumentID > 0 {
		originalID = &document.OriginalDocumentID
	}
	amount := pgNumeric(&document.TotalAmount)
	inserted, err := q.BillingInsertDocument(ctx, dbgen.BillingInsertDocumentParams{
		ID: document.ID, Status: document.Status, BillingType: document.BillingType,
		DocumentType: document.DocumentType, ProviderCredentialID: document.CredentialID,
		SourceType: document.SourceType, SourceID: document.SourceID, OriginalDocumentID: originalID,
		PeriodStart: priceTime(document.PeriodStart), PeriodEnd: priceTime(document.PeriodEnd),
		TotalTokens: document.TotalTokens, TotalAmount: amount, Currency: document.Currency,
		CreatedAt: priceTime(document.CreatedAt),
	})
	if err != nil || inserted == 0 {
		return false, err
	}
	for _, item := range document.Items {
		ratio := pgNumeric(&item.AllocationRatio)
		itemAmount := pgNumeric(&item.Amount)
		if err := q.BillingInsertDocumentItem(ctx, dbgen.BillingInsertDocumentItemParams{
			ID: item.ID, BillingDocumentID: document.ID, PrincipalID: item.PrincipalID,
			UsageTokens: item.UsageTokens, AllocationRatio: ratio, Amount: itemAmount,
			CreatedAt: priceTime(document.CreatedAt),
		}); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *BillingStore) Statistics(ctx context.Context, _ admin.Identity, filter app.StatisticsFilter) (app.Statistics, error) {
	totals, err := s.q.BillingStatisticsTotals(ctx, dbgen.BillingStatisticsTotalsParams{
		FromTime: priceTime(filter.From), ToTime: priceTime(filter.To), BillingType: filter.BillingType,
	})
	if err != nil {
		return app.Statistics{}, err
	}
	rows, err := s.q.BillingStatisticsPrincipals(ctx, dbgen.BillingStatisticsPrincipalsParams{
		FromTime: priceTime(filter.From), ToTime: priceTime(filter.To), BillingType: filter.BillingType,
		PageOffset: filter.After, PageLimit: filter.Limit,
	})
	if err != nil {
		return app.Statistics{}, err
	}
	result := app.Statistics{
		From: filter.From.UTC(), To: filter.To.UTC(),
		Totals: make([]app.CurrencyTotal, 0, len(totals)), Items: make([]app.PrincipalTotal, 0, len(rows)),
	}
	for _, row := range totals {
		result.Totals = append(result.Totals, app.CurrencyTotal{
			BillingType: row.BillingType, Currency: row.Currency, TotalAmount: priceNumber(row.TotalAmount),
			AllocatedAmount: priceNumber(row.AllocatedAmount), UnallocatedAmount: priceNumber(row.UnallocatedAmount),
			TotalTokens: row.TotalTokens,
		})
	}
	for _, row := range rows {
		result.Items = append(result.Items, app.PrincipalTotal{
			PrincipalID: row.PrincipalID, PrincipalName: row.PrincipalName, PrincipalType: row.PrincipalType,
			BillingType: row.BillingType, Currency: row.Currency, Amount: priceNumber(row.Amount), Tokens: row.Tokens,
		})
		result.Total = row.TotalCount
	}
	return result, nil
}
