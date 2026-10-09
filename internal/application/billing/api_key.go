package billing

import (
	"context"
	"errors"
	"sort"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

const (
	defaultRatingPageSize = int32(500)
	maxRatingsPerRun      = 100000
)

type UsageRatingCandidate struct {
	UsageRecordID, PrincipalID, CredentialID, ProviderModelID, ModelPriceID int64
	LatestRatingID                                                          int64
	LatestRevision                                                          int32
	UsageStartedAt                                                          time.Time
	InputTokens, CachedInputTokens, OutputTokens                            int64
	InputPrice, CachedInputPrice, OutputPrice, Currency                     string
}

type UsageRating struct {
	ID, UsageRecordID, SupersedesRatingID                       int64
	Revision                                                    int32
	PrincipalID, CredentialID, ProviderModelID, ModelPriceID    int64
	UsageStartedAt, CreatedAt                                   time.Time
	InputTokens, CachedInputTokens, OutputTokens                int64
	InputPrice, CachedInputPrice, OutputPrice                   string
	InputCost, CachedInputCost, OutputCost, TotalCost, Currency string
}

type APIKeyPeriod struct {
	CredentialID           int64
	PeriodStart, PeriodEnd time.Time
	Currency               string
}

type APIKeyCurrentItem struct {
	PrincipalID int64
	Tokens      int64
	Amount      string
}

type APIKeyCurrent struct {
	Found              bool
	OriginalDocumentID int64
	TotalTokens        int64
	TotalAmount        string
	Items              []APIKeyCurrentItem
}

type RatingSummary struct {
	Created, Skipped int
}

func (s *Service) RatePendingAPIKeyUsage(ctx context.Context, pageSize int32) (RatingSummary, error) {
	var summary RatingSummary
	if s == nil || s.store == nil || s.ids == nil {
		return summary, appsec.ErrUnavailable
	}
	if pageSize <= 0 {
		pageSize = defaultRatingPageSize
	}
	if pageSize > 5000 {
		return summary, appsec.ErrInvalidArgument
	}
	var after int64
	for processed := 0; processed < maxRatingsPerRun; {
		candidates, err := s.store.UsageRatingCandidates(ctx, after, pageSize)
		if err != nil {
			return summary, err
		}
		if len(candidates) == 0 {
			return summary, nil
		}
		for _, candidate := range candidates {
			if candidate.UsageRecordID <= after {
				return summary, errors.New("usage rating candidates are not ordered")
			}
			after = candidate.UsageRecordID
			cost, err := domain.RateAPIKeyUsage(
				candidate.InputTokens, candidate.CachedInputTokens, candidate.OutputTokens,
				candidate.InputPrice, candidate.CachedInputPrice, candidate.OutputPrice,
			)
			if err != nil {
				return summary, err
			}
			id, err := s.ids.NextID(ctx)
			if err != nil || id <= 0 {
				return summary, errors.New("cannot generate usage rating ID")
			}
			rating := UsageRating{
				ID: id, UsageRecordID: candidate.UsageRecordID, Revision: candidate.LatestRevision + 1,
				SupersedesRatingID: candidate.LatestRatingID, PrincipalID: candidate.PrincipalID,
				CredentialID: candidate.CredentialID, ProviderModelID: candidate.ProviderModelID,
				ModelPriceID: candidate.ModelPriceID, UsageStartedAt: candidate.UsageStartedAt.UTC(),
				CreatedAt: s.now().UTC(), InputTokens: candidate.InputTokens,
				CachedInputTokens: candidate.CachedInputTokens, OutputTokens: candidate.OutputTokens,
				InputPrice: candidate.InputPrice, CachedInputPrice: candidate.CachedInputPrice,
				OutputPrice: candidate.OutputPrice, InputCost: cost.Input,
				CachedInputCost: cost.CachedInput, OutputCost: cost.Output, TotalCost: cost.Total,
				Currency: candidate.Currency,
			}
			created, err := s.store.CreateUsageRating(ctx, rating)
			if err != nil {
				return summary, err
			}
			if created {
				summary.Created++
			} else {
				summary.Skipped++
			}
			processed++
		}
		if len(candidates) < int(pageSize) {
			return summary, nil
		}
	}
	return summary, errors.New("usage rating run exceeded record limit")
}

func (s *Service) SettleDueAPIKeys(ctx context.Context, cutoff time.Time) (Summary, error) {
	var summary Summary
	if s == nil || s.store == nil || s.ids == nil || cutoff.IsZero() {
		return summary, appsec.ErrUnavailable
	}
	periods, err := s.store.APIKeyPeriods(ctx, cutoff.UTC())
	if err != nil {
		return summary, err
	}
	if len(periods) > maxPeriodsPerScan {
		return summary, errors.New("api key billing settlement scan exceeded period limit")
	}
	for _, period := range periods {
		shares, ratingIDs, err := s.store.APIKeyUsage(ctx, period)
		if err != nil {
			return summary, err
		}
		targetTokens, targetAmount, allocations, err := domain.RoundRatedCosts(shares)
		if err != nil {
			return summary, err
		}
		current, err := s.store.APIKeyCurrent(ctx, period)
		if err != nil {
			return summary, err
		}
		if !current.Found {
			document, err := s.newAPIKeyDocument(ctx, "CHARGE", period, 0, targetTokens, targetAmount, allocations)
			if err != nil {
				return summary, err
			}
			created, err := s.store.CreateAPIKeyDocument(ctx, "", 0, document, ratingIDs)
			if err != nil {
				return summary, err
			}
			if created {
				summary.Created++
			} else {
				summary.Skipped++
			}
			continue
		}
		deltaAmount, err := domain.Subtract(targetAmount, current.TotalAmount)
		if err != nil {
			return summary, err
		}
		deltaTokens := targetTokens - current.TotalTokens
		if deltaTokens < 0 {
			return summary, errors.New("api key settled token total exceeds rated usage")
		}
		currentItems := make(map[int64]APIKeyCurrentItem, len(current.Items))
		for _, item := range current.Items {
			currentItems[item.PrincipalID] = item
		}
		deltaAllocations := make([]domain.Allocation, 0, len(allocations))
		for _, allocation := range allocations {
			currentItem, found := currentItems[allocation.PrincipalID]
			if !found {
				currentItem.Amount = "0"
			}
			itemDelta, err := domain.Subtract(allocation.Amount, currentItem.Amount)
			if err != nil {
				return summary, err
			}
			itemTokens := allocation.Tokens - currentItem.Tokens
			if itemTokens < 0 {
				return summary, errors.New("api key principal settled token total exceeds rated usage")
			}
			if itemDelta != "0" || itemTokens != 0 {
				deltaAllocations = append(deltaAllocations, domain.Allocation{
					PrincipalID: allocation.PrincipalID, Tokens: itemTokens, Amount: itemDelta,
				})
			}
			delete(currentItems, allocation.PrincipalID)
		}
		if len(currentItems) != 0 {
			return summary, errors.New("api key settled principal is absent from rated usage")
		}
		if deltaAmount == "0" && deltaTokens == 0 && len(deltaAllocations) == 0 {
			summary.Skipped++
			continue
		}
		document, err := s.newAPIKeyDocument(ctx, "ADJUSTMENT", period, current.OriginalDocumentID,
			deltaTokens, deltaAmount, deltaAllocations)
		if err != nil {
			return summary, err
		}
		created, err := s.store.CreateAPIKeyDocument(ctx, current.TotalAmount, current.TotalTokens, document, ratingIDs)
		if err != nil {
			return summary, err
		}
		if created {
			summary.Adjusted++
		} else {
			summary.Skipped++
		}
	}
	return summary, nil
}

func (s *Service) newAPIKeyDocument(ctx context.Context, documentType string, period APIKeyPeriod,
	originalID, tokens int64, amount string, allocations []domain.Allocation,
) (Document, error) {
	documentID, err := s.ids.NextID(ctx)
	if err != nil || documentID <= 0 {
		return Document{}, errors.New("cannot generate billing document ID")
	}
	sort.Slice(allocations, func(i, j int) bool { return allocations[i].PrincipalID < allocations[j].PrincipalID })
	document := Document{
		ID: documentID, CredentialID: period.CredentialID, OriginalDocumentID: originalID,
		BillingType: "API_KEY", DocumentType: documentType, Status: "CONFIRMED", SourceType: "API_KEY_USAGE",
		PeriodStart: period.PeriodStart.UTC(), PeriodEnd: period.PeriodEnd.UTC(), CreatedAt: s.now().UTC(),
		TotalTokens: tokens, TotalAmount: amount, Currency: period.Currency,
		Items: make([]DocumentItem, 0, len(allocations)),
	}
	for _, allocation := range allocations {
		itemID, err := s.ids.NextID(ctx)
		if err != nil || itemID <= 0 {
			return Document{}, errors.New("cannot generate billing document item ID")
		}
		document.Items = append(document.Items, DocumentItem{
			ID: itemID, PrincipalID: allocation.PrincipalID, UsageTokens: allocation.Tokens,
			Amount: allocation.Amount,
		})
	}
	return document, nil
}
