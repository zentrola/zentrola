package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	app "github.com/zentrola/zentrola/internal/application/billing"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func usageCostSummaryParams(filter app.UsageCostFilter) dbgen.BillingUsageCostSummaryParams {
	return dbgen.BillingUsageCostSummaryParams{
		FromTime: priceTime(filter.From), ToTime: priceTime(filter.To),
		PrincipalID: filter.PrincipalID, PrincipalType: filter.PrincipalType, GroupID: filter.GroupID,
		ModelID: filter.ModelID, ProviderID: filter.ProviderID, ResourceID: filter.ResourceID,
		ClientProtocol: filter.ClientProtocol, UsageStatus: filter.Status,
		RatingStatus: filter.RatingStatus, BillingType: filter.BillingType, Currency: filter.Currency,
	}
}

func (s *BillingStore) UsageCostSummary(ctx context.Context, actor admin.Identity, filter app.UsageCostFilter) (app.UsageCostSummary, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.UsageCostSummary{}, err
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		return app.UsageCostSummary{}, managementError(err)
	}
	params := usageCostSummaryParams(filter)
	row, err := q.BillingUsageCostSummary(ctx, params)
	if err != nil {
		return app.UsageCostSummary{}, err
	}
	totals, err := q.BillingUsageCostTotals(ctx, dbgen.BillingUsageCostTotalsParams(params))
	if err != nil {
		return app.UsageCostSummary{}, err
	}
	result := app.UsageCostSummary{
		From: filter.From.UTC(), To: filter.To.UTC(), Requests: row.Requests, Attempts: row.Attempts,
		Successful: row.Successful, InputTokens: row.InputTokens, CachedInputTokens: row.CachedInputTokens,
		OutputTokens: row.OutputTokens, Rated: row.Rated, Shared: row.Shared, Unrated: row.Unrated,
		Totals: make([]app.UsageCostTotal, 0, len(totals)),
	}
	for _, total := range totals {
		result.Totals = append(result.Totals, app.UsageCostTotal{
			Currency: total.Currency, Amount: priceNumber(total.Amount), Rated: total.Rated,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return app.UsageCostSummary{}, err
	}
	return result, nil
}

func (s *BillingStore) UsageCosts(ctx context.Context, actor admin.Identity, filter app.UsageCostFilter) (app.UsageCostPage, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.UsageCostPage{}, err
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		return app.UsageCostPage{}, managementError(err)
	}
	base := usageCostSummaryParams(filter)
	rows, err := q.BillingUsageCosts(ctx, dbgen.BillingUsageCostsParams{
		SortBy: filter.Sort, SortOrder: filter.Order, PageOffset: filter.After, PageLimit: filter.Limit,
		FromTime: base.FromTime, ToTime: base.ToTime, PrincipalID: base.PrincipalID,
		PrincipalType: base.PrincipalType, GroupID: base.GroupID, ModelID: base.ModelID,
		ProviderID: base.ProviderID, ResourceID: base.ResourceID, ClientProtocol: base.ClientProtocol,
		UsageStatus: base.UsageStatus, RatingStatus: base.RatingStatus,
		BillingType: base.BillingType, Currency: base.Currency,
	})
	if err != nil {
		return app.UsageCostPage{}, err
	}
	result := app.UsageCostPage{Items: make([]app.UsageCostRow, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(result.Items, usageCostRow(
			row.ID, row.RequestID, row.AttemptNo, row.PrincipalID, row.PrincipalName, row.PrincipalType,
			row.ModelID, row.ModelName, row.ProviderID, row.ProviderName, row.ProviderModelID,
			row.ResourceID, row.ResourceName, row.ClientProtocol, row.Status, row.ErrorType,
			row.StartedAt.Time, row.CompletedAt.Time, row.LatencyMs, row.InputTokens,
			row.CachedInputTokens, row.OutputTokens, row.BillingType, row.RatingStatus,
			row.RatingID, row.RatingRevision, row.Currency, priceNumber(row.TotalCost), row.RatedAt.Time,
		))
		result.Total = row.TotalCount
	}
	if err := tx.Commit(ctx); err != nil {
		return app.UsageCostPage{}, err
	}
	return result, nil
}

func (s *BillingStore) UsageCost(ctx context.Context, actor admin.Identity, id int64) (app.UsageCostDetail, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.UsageCostDetail{}, err
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		return app.UsageCostDetail{}, managementError(err)
	}
	row, err := q.BillingUsageCost(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.UsageCostDetail{}, appsec.ErrNotFound
	}
	if err != nil {
		return app.UsageCostDetail{}, err
	}
	result := app.UsageCostDetail{
		UsageCostRow: usageCostRow(
			row.ID, row.RequestID, row.AttemptNo, row.PrincipalID, row.PrincipalName, row.PrincipalType,
			row.ModelID, row.ModelName, row.ProviderID, row.ProviderName, row.ProviderModelID,
			row.ResourceID, row.ResourceName, row.ClientProtocol, row.Status, row.ErrorType,
			row.StartedAt.Time, row.CompletedAt.Time, row.LatencyMs, row.InputTokens,
			row.CachedInputTokens, row.OutputTokens, row.BillingType, row.RatingStatus,
			row.RatingID, row.RatingRevision, row.Currency, priceNumber(row.TotalCost), row.RatedAt.Time,
		),
		Ratings: []app.UsageCostRating{},
	}
	ratings, err := q.BillingUsageCostRatings(ctx, id)
	if err != nil {
		return app.UsageCostDetail{}, err
	}
	for _, rating := range ratings {
		result.Ratings = append(result.Ratings, app.UsageCostRating{
			ID: rating.ID, Revision: rating.Revision, ModelPriceID: rating.ModelPriceID,
			PriceEffectiveAt: rating.PriceEffectiveAt.Time.UTC(), InputPrice: priceNumber(rating.InputPrice),
			CachedInputPrice: priceNumber(rating.CachedInputPrice), OutputPrice: priceNumber(rating.OutputPrice),
			InputCost: priceNumber(rating.InputCost), CachedInputCost: priceNumber(rating.CachedInputCost),
			OutputCost: priceNumber(rating.OutputCost), TotalCost: priceNumber(rating.TotalCost),
			Currency: rating.Currency, CreatedAt: rating.CreatedAt.Time.UTC(),
		})
	}
	if row.BillingType == "SUBSCRIPTION" {
		allocation, allocationErr := q.BillingUsageSubscriptionAllocation(ctx, id)
		if allocationErr != nil && !errors.Is(allocationErr, pgx.ErrNoRows) {
			return app.UsageCostDetail{}, allocationErr
		}
		if allocationErr == nil {
			result.Subscription = &app.SubscriptionAllocation{
				PriceID: allocation.PriceID, Currency: allocation.Currency,
				PeriodAmount: priceNumber(allocation.PeriodAmount), BillingPeriod: allocation.BillingPeriod,
				EffectiveAt: allocation.EffectiveAt.Time.UTC(), DocumentID: allocation.DocumentID,
				PeriodStart: validTime(allocation.PeriodStart), PeriodEnd: validTime(allocation.PeriodEnd),
				PrincipalAmount: optionalPriceNumber(allocation.PrincipalAmount),
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return app.UsageCostDetail{}, err
	}
	return result, nil
}

func (s *BillingStore) PrincipalIdentities(ctx context.Context, ids []int64) ([]app.PrincipalIdentity, error) {
	rows, err := s.q.BillingPrincipalIdentities(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]app.PrincipalIdentity, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.PrincipalIdentity{ID: row.ID, Name: row.Name, Type: row.PrincipalType})
	}
	return result, nil
}

func (s *BillingStore) CurrentAPIKeyAttribution(
	ctx context.Context,
	from, to time.Time,
) ([]app.PrincipalTotal, error) {
	rows, err := s.q.BillingCurrentAPIKeyAttribution(ctx, dbgen.BillingCurrentAPIKeyAttributionParams{
		FromTime: priceTime(from), ToTime: priceTime(to),
	})
	if err != nil {
		return nil, err
	}
	result := make([]app.PrincipalTotal, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.PrincipalTotal{
			PrincipalID: row.PrincipalID, PrincipalName: row.PrincipalName, PrincipalType: row.PrincipalType,
			BillingType: "API_KEY", Currency: row.Currency, Amount: priceNumber(row.Amount), Tokens: row.Tokens,
		})
	}
	return result, nil
}

func usageCostRow(id int64, requestID string, attemptNo, principalID int64, principalName, principalType string,
	modelID int64, modelName string, providerID int64, providerName string, providerModelID, resourceID int64,
	resourceName, clientProtocol, status string, errorType *string, startedAt, completedAt time.Time,
	latencyMS int64, inputTokens, cachedInputTokens, outputTokens *int64, billingType, ratingStatus string,
	ratingID int64, ratingRevision int32, currency, totalCost string, ratedAt time.Time,
) app.UsageCostRow {
	result := app.UsageCostRow{
		ID: id, RequestID: requestID, AttemptNo: attemptNo, PrincipalID: principalID,
		PrincipalName: principalName, PrincipalType: principalType, ModelID: modelID, ModelName: modelName,
		ProviderID: providerID, ProviderName: providerName, ProviderModelID: providerModelID,
		ResourceID: resourceID, ResourceName: resourceName, ClientProtocol: clientProtocol,
		Status: status, ErrorType: errorType, StartedAt: startedAt.UTC(), CompletedAt: completedAt.UTC(),
		LatencyMS: latencyMS, InputTokens: inputTokens, CachedInputTokens: cachedInputTokens,
		OutputTokens: outputTokens, BillingType: billingType, RatingStatus: ratingStatus,
	}
	if currency != "" {
		result.Currency = &currency
	}
	if ratingID > 0 {
		result.RatingID, result.RatingRevision = &ratingID, &ratingRevision
		value := ratedAt.UTC()
		result.RatedAt = &value
	}
	if ratingStatus == "RATED" {
		result.TotalCost = &totalCost
	}
	return result
}

func validTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func optionalPriceNumber(value pgtype.Numeric) *string {
	if !value.Valid {
		return nil
	}
	result := priceNumber(value)
	return &result
}
