package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/zentrola/zentrola/internal/application/billing"
	appsec "github.com/zentrola/zentrola/internal/application/security"
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
	return s.createDocument(ctx, document, "", 0, nil)
}

func (s *BillingStore) SubscriptionCorrections(ctx context.Context) ([]app.Correction, error) {
	rows, err := s.q.BillingSubscriptionCorrections(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]app.Correction, 0, len(rows))
	for _, row := range rows {
		if row.SourcePriceID == nil {
			return nil, errors.New("subscription billing source price is missing")
		}
		result = append(result, app.Correction{
			OriginalDocumentID: row.OriginalDocumentID, CredentialID: row.ProviderCredentialID,
			SourcePriceID: *row.SourcePriceID, PeriodStart: row.PeriodStart.Time.UTC(), PeriodEnd: row.PeriodEnd.Time.UTC(),
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
	return s.createDocument(ctx, document, expectedCurrent, 0, nil)
}

func (s *BillingStore) createDocument(ctx context.Context, document app.Document, expectedCurrent string, expectedTokens int64, ratingIDs []int64) (bool, error) {
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
		if document.BillingType == "API_KEY" {
			currentDocument, currentErr := q.BillingAPIKeyCurrentDocument(ctx, dbgen.BillingAPIKeyCurrentDocumentParams{
				ProviderCredentialID: document.CredentialID, PeriodStart: priceTime(document.PeriodStart),
				PeriodEnd: priceTime(document.PeriodEnd), Currency: document.Currency,
			})
			if currentErr != nil {
				return false, currentErr
			}
			if currentDocument.OriginalDocumentID != document.OriginalDocumentID || currentDocument.TotalTokens != expectedTokens {
				return false, nil
			}
		}
	}
	var originalID *int64
	if document.OriginalDocumentID > 0 {
		originalID = &document.OriginalDocumentID
	}
	var sourceID *int64
	if document.SourceID > 0 {
		sourceID = &document.SourceID
	}
	amount := pgNumeric(&document.TotalAmount)
	inserted, err := q.BillingInsertDocument(ctx, dbgen.BillingInsertDocumentParams{
		ID: document.ID, Status: document.Status, BillingType: document.BillingType,
		DocumentType: document.DocumentType, ProviderCredentialID: document.CredentialID,
		SourceType: document.SourceType, SourceID: sourceID, OriginalDocumentID: originalID,
		PeriodStart: priceTime(document.PeriodStart), PeriodEnd: priceTime(document.PeriodEnd),
		TotalTokens: document.TotalTokens, TotalAmount: amount, Currency: document.Currency,
		CreatedAt: priceTime(document.CreatedAt),
	})
	if err != nil {
		return false, err
	}
	if inserted == 0 {
		if document.DocumentType != "CHARGE" {
			return false, errors.New("billing document ID conflict")
		}
		exists, existsErr := q.BillingChargeExists(ctx, dbgen.BillingChargeExistsParams{
			BillingType: document.BillingType, ProviderCredentialID: document.CredentialID,
			PeriodStart: priceTime(document.PeriodStart), PeriodEnd: priceTime(document.PeriodEnd),
			Currency: document.Currency,
		})
		if existsErr != nil {
			return false, existsErr
		}
		if !exists {
			return false, errors.New("billing document ID conflict")
		}
		return false, nil
	}
	for _, item := range document.Items {
		var ratioValue *string
		if item.AllocationRatio != "" {
			ratioValue = &item.AllocationRatio
		}
		ratio := pgNumeric(ratioValue)
		itemAmount := pgNumeric(&item.Amount)
		if err := q.BillingInsertDocumentItem(ctx, dbgen.BillingInsertDocumentItemParams{
			ID: item.ID, BillingDocumentID: document.ID, PrincipalID: item.PrincipalID,
			UsageTokens: item.UsageTokens, AllocationRatio: ratio, Amount: itemAmount,
			CreatedAt: priceTime(document.CreatedAt),
		}); err != nil {
			return false, err
		}
	}
	for _, ratingID := range ratingIDs {
		if err := q.BillingInsertDocumentUsageRating(ctx, dbgen.BillingInsertDocumentUsageRatingParams{
			BillingDocumentID: document.ID, UsageRatingID: ratingID, CreatedAt: priceTime(document.CreatedAt),
		}); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *BillingStore) UsageRatingCandidates(ctx context.Context, after int64, limit int32) ([]app.UsageRatingCandidate, error) {
	rows, err := s.q.BillingUsageRatingCandidates(ctx, dbgen.BillingUsageRatingCandidatesParams{AfterID: after, PageLimit: limit})
	if err != nil {
		return nil, err
	}
	result := make([]app.UsageRatingCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.UsageRatingCandidate{
			UsageRecordID: row.UsageRecordID, PrincipalID: row.PrincipalID,
			CredentialID: row.ProviderCredentialID, ProviderModelID: row.ProviderModelID,
			ModelPriceID: row.ModelPriceID, LatestRatingID: row.LatestRatingID,
			LatestRevision: row.LatestRevision, UsageStartedAt: row.UsageStartedAt.Time.UTC(),
			InputTokens: row.InputTokens, CachedInputTokens: row.CachedInputTokens, OutputTokens: row.OutputTokens,
			InputPrice: priceNumber(row.InputPrice), CachedInputPrice: priceNumber(row.CachedInputPrice),
			OutputPrice: priceNumber(row.OutputPrice), Currency: row.Currency,
		})
	}
	return result, nil
}

func (s *BillingStore) CreateUsageRating(ctx context.Context, rating app.UsageRating) (bool, error) {
	var supersedesID *int64
	if rating.SupersedesRatingID > 0 {
		supersedesID = &rating.SupersedesRatingID
	}
	inputPrice, cachedPrice, outputPrice := pgNumeric(&rating.InputPrice), pgNumeric(&rating.CachedInputPrice), pgNumeric(&rating.OutputPrice)
	inputCost, cachedCost, outputCost := pgNumeric(&rating.InputCost), pgNumeric(&rating.CachedInputCost), pgNumeric(&rating.OutputCost)
	totalCost := pgNumeric(&rating.TotalCost)
	inserted, err := s.q.BillingInsertUsageRating(ctx, dbgen.BillingInsertUsageRatingParams{
		ID: rating.ID, UsageRecordID: rating.UsageRecordID, Revision: rating.Revision,
		SupersedesRatingID: supersedesID, PrincipalID: rating.PrincipalID,
		ProviderCredentialID: rating.CredentialID, ProviderModelID: rating.ProviderModelID,
		ModelPriceID: rating.ModelPriceID, UsageStartedAt: priceTime(rating.UsageStartedAt),
		InputTokens: rating.InputTokens, CachedInputTokens: rating.CachedInputTokens, OutputTokens: rating.OutputTokens,
		InputPrice: inputPrice, CachedInputPrice: cachedPrice, OutputPrice: outputPrice,
		InputCost: inputCost, CachedInputCost: cachedCost, OutputCost: outputCost,
		TotalCost: totalCost, Currency: rating.Currency, CreatedAt: priceTime(rating.CreatedAt),
	})
	return inserted > 0, err
}

func (s *BillingStore) APIKeyPeriods(ctx context.Context, cutoff time.Time) ([]app.APIKeyPeriod, error) {
	rows, err := s.q.BillingAPIKeyPeriods(ctx, priceTime(cutoff))
	if err != nil {
		return nil, err
	}
	result := make([]app.APIKeyPeriod, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.APIKeyPeriod{
			CredentialID: row.ProviderCredentialID, PeriodStart: row.PeriodStart.Time.UTC(),
			PeriodEnd: row.PeriodEnd.Time.UTC(), Currency: row.Currency,
		})
	}
	return result, nil
}

func (s *BillingStore) APIKeyUsage(ctx context.Context, period app.APIKeyPeriod) ([]domain.RatedShare, []int64, error) {
	rows, err := s.q.BillingAPIKeyUsage(ctx, dbgen.BillingAPIKeyUsageParams{
		ProviderCredentialID: period.CredentialID, PeriodStart: priceTime(period.PeriodStart),
		PeriodEnd: priceTime(period.PeriodEnd), Currency: period.Currency,
	})
	if err != nil {
		return nil, nil, err
	}
	shares := make([]domain.RatedShare, 0, len(rows))
	ratingIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		shares = append(shares, domain.RatedShare{PrincipalID: row.PrincipalID, Tokens: row.Tokens, Amount: priceNumber(row.TotalCost)})
		ratingIDs = append(ratingIDs, row.UsageRatingID)
	}
	return shares, ratingIDs, nil
}

func (s *BillingStore) APIKeyCurrent(ctx context.Context, period app.APIKeyPeriod) (app.APIKeyCurrent, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.APIKeyCurrent{}, err
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	params := dbgen.BillingAPIKeyCurrentDocumentParams{
		ProviderCredentialID: period.CredentialID, PeriodStart: priceTime(period.PeriodStart),
		PeriodEnd: priceTime(period.PeriodEnd), Currency: period.Currency,
	}
	document, err := q.BillingAPIKeyCurrentDocument(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.APIKeyCurrent{}, nil
	}
	if err != nil {
		return app.APIKeyCurrent{}, err
	}
	rows, err := q.BillingAPIKeyCurrentItems(ctx, dbgen.BillingAPIKeyCurrentItemsParams(params))
	if err != nil {
		return app.APIKeyCurrent{}, err
	}
	result := app.APIKeyCurrent{
		Found: true, OriginalDocumentID: document.OriginalDocumentID,
		TotalTokens: document.TotalTokens, TotalAmount: priceNumber(document.TotalAmount),
		Items: make([]app.APIKeyCurrentItem, 0, len(rows)),
	}
	for _, row := range rows {
		result.Items = append(result.Items, app.APIKeyCurrentItem{
			PrincipalID: row.PrincipalID, Tokens: row.Tokens, Amount: priceNumber(row.Amount),
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return app.APIKeyCurrent{}, err
	}
	return result, nil
}

func (s *BillingStore) CreateAPIKeyDocument(ctx context.Context, expectedAmount string, expectedTokens int64, document app.Document, ratingIDs []int64) (bool, error) {
	return s.createDocument(ctx, document, expectedAmount, expectedTokens, ratingIDs)
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

func (s *BillingStore) BillingDocuments(ctx context.Context, filter app.DocumentFilter) (app.DocumentPage, error) {
	rows, err := s.q.BillingDocuments(ctx, dbgen.BillingDocumentsParams{
		FromTime: priceTime(filter.From), ToTime: priceTime(filter.To),
		BillingType: filter.BillingType, DocumentType: filter.DocumentType, Currency: filter.Currency,
		AfterID: filter.After, PageLimit: filter.Limit + 1,
	})
	if err != nil {
		return app.DocumentPage{}, err
	}
	result := app.DocumentPage{Items: make([]app.DocumentSummary, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(result.Items, billingDocumentSummary(
			row.ID, row.BillingType, row.DocumentType, row.Status, row.ProviderCredentialID,
			row.CredentialName, row.OriginalDocumentID, row.PeriodStart.Time, row.PeriodEnd.Time,
			row.TotalTokens, priceNumber(row.TotalAmount), row.Currency, row.CreatedAt.Time,
		))
		result.Total = row.TotalCount
	}
	return result, nil
}

func (s *BillingStore) BillingDocument(ctx context.Context, id int64) (app.DocumentDetail, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.DocumentDetail{}, err
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	row, err := q.BillingDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.DocumentDetail{}, appsec.ErrNotFound
	}
	if err != nil {
		return app.DocumentDetail{}, err
	}
	result := app.DocumentDetail{DocumentSummary: billingDocumentSummary(
		row.ID, row.BillingType, row.DocumentType, row.Status, row.ProviderCredentialID,
		row.CredentialName, row.OriginalDocumentID, row.PeriodStart.Time, row.PeriodEnd.Time,
		row.TotalTokens, priceNumber(row.TotalAmount), row.Currency, row.CreatedAt.Time,
	), Items: []app.DocumentItemDetail{}, Adjustments: []app.DocumentSummary{}}
	items, err := q.BillingDocumentItems(ctx, id)
	if err != nil {
		return app.DocumentDetail{}, err
	}
	for _, item := range items {
		result.Items = append(result.Items, app.DocumentItemDetail{
			ID: item.ID, PrincipalID: item.PrincipalID, PrincipalName: item.PrincipalName,
			PrincipalType: item.PrincipalType, UsageTokens: item.UsageTokens,
			AllocationRatio: numericString(item.AllocationRatio), Amount: priceNumber(item.Amount),
		})
	}
	if row.OriginalDocumentID != nil {
		original, originalErr := q.BillingDocument(ctx, *row.OriginalDocumentID)
		if originalErr != nil {
			return app.DocumentDetail{}, originalErr
		}
		summary := billingDocumentSummary(
			original.ID, original.BillingType, original.DocumentType, original.Status,
			original.ProviderCredentialID, original.CredentialName, original.OriginalDocumentID,
			original.PeriodStart.Time, original.PeriodEnd.Time, original.TotalTokens,
			priceNumber(original.TotalAmount), original.Currency, original.CreatedAt.Time,
		)
		result.Original = &summary
	} else {
		adjustments, adjustmentErr := q.BillingDocumentAdjustments(ctx, id)
		if adjustmentErr != nil {
			return app.DocumentDetail{}, adjustmentErr
		}
		for _, adjustment := range adjustments {
			result.Adjustments = append(result.Adjustments, billingDocumentSummary(
				adjustment.ID, adjustment.BillingType, adjustment.DocumentType, adjustment.Status,
				adjustment.ProviderCredentialID, adjustment.CredentialName, adjustment.OriginalDocumentID,
				adjustment.PeriodStart.Time, adjustment.PeriodEnd.Time, adjustment.TotalTokens,
				priceNumber(adjustment.TotalAmount), adjustment.Currency, adjustment.CreatedAt.Time,
			))
		}
	}
	result.RatingCount, err = q.BillingDocumentRatingCount(ctx, id)
	if err != nil {
		return app.DocumentDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return app.DocumentDetail{}, err
	}
	return result, nil
}

func (s *BillingStore) UnratedUsage(ctx context.Context, filter app.UnratedUsageFilter) (app.UnratedUsagePage, error) {
	rows, err := s.q.BillingUnratedUsage(ctx, dbgen.BillingUnratedUsageParams{
		FromTime: priceTime(filter.From), ToTime: priceTime(filter.To), Reason: filter.Reason,
		AfterID: filter.After, PageLimit: filter.Limit + 1,
	})
	if err != nil {
		return app.UnratedUsagePage{}, err
	}
	result := app.UnratedUsagePage{Items: make([]app.UnratedUsage, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(result.Items, app.UnratedUsage{
			UsageRecordID: row.UsageRecordID, PrincipalID: row.PrincipalID,
			PrincipalName: row.PrincipalName, PrincipalType: row.PrincipalType,
			CredentialID: row.ProviderCredentialID, CredentialName: row.CredentialName,
			ProviderModelID: row.ProviderModelID, StartedAt: row.StartedAt.Time.UTC(),
			InputTokens: row.InputTokens, CachedInputTokens: row.CachedInputTokens,
			OutputTokens: row.OutputTokens, Reason: row.Reason, WaitingSince: row.WaitingSince.Time.UTC(),
		})
		result.Total = row.TotalCount
	}
	return result, nil
}

func billingDocumentSummary(id int64, billingType, documentType, status string, credentialID int64,
	credentialName string, originalDocumentID *int64, periodStart, periodEnd time.Time,
	totalTokens int64, totalAmount, currency string, createdAt time.Time,
) app.DocumentSummary {
	return app.DocumentSummary{
		ID: id, BillingType: billingType, DocumentType: documentType, Status: status,
		CredentialID: credentialID, CredentialName: credentialName, OriginalDocumentID: originalDocumentID,
		PeriodStart: periodStart.UTC(), PeriodEnd: periodEnd.UTC(), TotalTokens: totalTokens,
		TotalAmount: totalAmount, Currency: currency, CreatedAt: createdAt.UTC(),
	}
}
