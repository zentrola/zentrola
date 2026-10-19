package billing

import (
	"context"
	"errors"
	"fmt"
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

// RatingScan 区分核算候选的两类来源，分别由针对性的索引查询定位。
type RatingScan string

const (
	// RatingScanUnrated 查找从未核算、且调用时已有有效价格的可计费调用。
	RatingScanUnrated RatingScan = "UNRATED"
	// RatingScanRepriced 查找最新核算的价格快照与所引用价格当前金额不一致、需要追加 revision 的调用。
	RatingScanRepriced RatingScan = "REPRICED"
)

// RatingSummary 汇总一次核算扫描；单条记录失败计入 Failed 并在本轮内跳过，下一轮会重新尝试。
type RatingSummary struct {
	Created, Skipped, Failed int
	FailureSamples           []string
}

func (s *RatingSummary) recordFailure(format string, args ...any) {
	s.Failed++
	if len(s.FailureSamples) < maxFailureSamples {
		s.FailureSamples = append(s.FailureSamples, fmt.Sprintf(format, args...))
	}
}

// RatePendingAPIKeyUsage 先核算从未核算的调用，再为价格金额纠错后的调用追加 revision。
// 两类扫描各自按用量 ID 递增翻页并共享 maxRatingsPerRun 上限；单条记录核算失败只计入失败并继续。
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
	processed := 0
	for _, scan := range []RatingScan{RatingScanUnrated, RatingScanRepriced} {
		if err := s.rateScan(ctx, scan, pageSize, &processed, &summary); err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func (s *Service) rateScan(ctx context.Context, scan RatingScan, pageSize int32, processed *int, summary *RatingSummary) error {
	var after int64
	for {
		if *processed >= maxRatingsPerRun {
			return errors.New("usage rating run exceeded record limit")
		}
		candidates, err := s.store.UsageRatingCandidates(ctx, scan, after, pageSize)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if candidate.UsageRecordID <= after {
				return errors.New("usage rating candidates are not ordered")
			}
			after = candidate.UsageRecordID
			created, err := s.rateCandidate(ctx, candidate)
			*processed++
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if errors.Is(err, errBillingID) {
					return err
				}
				summary.recordFailure("usage rating scan=%s usage=%d: %s", scan, candidate.UsageRecordID, SafeErrorText(err))
				continue
			}
			if created {
				summary.Created++
			} else {
				summary.Skipped++
			}
		}
		if len(candidates) < int(pageSize) {
			return nil
		}
	}
}

func (s *Service) rateCandidate(ctx context.Context, candidate UsageRatingCandidate) (bool, error) {
	cost, err := domain.RateAPIKeyUsage(
		candidate.InputTokens, candidate.CachedInputTokens, candidate.OutputTokens,
		candidate.InputPrice, candidate.CachedInputPrice, candidate.OutputPrice,
	)
	if err != nil {
		return false, err
	}
	id, err := s.ids.NextID(ctx)
	if err != nil || id <= 0 {
		return false, errBillingID
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
	return s.store.CreateUsageRating(ctx, rating)
}

// SettleDueAPIKeys 结算仍有未确认核算的到期 API Key 自然月账期；单个账期失败计入 Failed 后继续。
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
		outcome, err := s.settleAPIKeyPeriod(ctx, period)
		if err != nil {
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			if errors.Is(err, errBillingID) {
				return summary, err
			}
			summary.recordFailure("api key credential=%d period=%s/%s currency=%s: %s", period.CredentialID,
				formatPeriodTime(period.PeriodStart), formatPeriodTime(period.PeriodEnd), period.Currency, SafeErrorText(err))
			continue
		}
		switch outcome {
		case apiKeyCharged:
			summary.Created++
		case apiKeyAdjusted:
			summary.Adjusted++
		default:
			summary.Skipped++
		}
	}
	return summary, nil
}

type apiKeyOutcome int

const (
	apiKeySkipped apiKeyOutcome = iota
	apiKeyCharged
	apiKeyAdjusted
)

func (s *Service) settleAPIKeyPeriod(ctx context.Context, period APIKeyPeriod) (apiKeyOutcome, error) {
	shares, ratingIDs, err := s.store.APIKeyUsage(ctx, period)
	if err != nil {
		return apiKeySkipped, err
	}
	targetTokens, targetAmount, allocations, err := domain.RoundRatedCosts(shares)
	if err != nil {
		return apiKeySkipped, err
	}
	current, err := s.store.APIKeyCurrent(ctx, period)
	if err != nil {
		return apiKeySkipped, err
	}
	if !current.Found {
		document, err := s.newAPIKeyDocument(ctx, "CHARGE", period, 0, targetTokens, targetAmount, allocations)
		if err != nil {
			return apiKeySkipped, err
		}
		created, err := s.store.CreateAPIKeyDocument(ctx, "", 0, document, ratingIDs)
		if err != nil || !created {
			return apiKeySkipped, err
		}
		return apiKeyCharged, nil
	}
	deltaAmount, err := domain.Subtract(targetAmount, current.TotalAmount)
	if err != nil {
		return apiKeySkipped, err
	}
	deltaTokens := targetTokens - current.TotalTokens
	if deltaTokens < 0 {
		return apiKeySkipped, errors.New("api key settled token total exceeds rated usage")
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
			return apiKeySkipped, err
		}
		itemTokens := allocation.Tokens - currentItem.Tokens
		if itemTokens < 0 {
			return apiKeySkipped, errors.New("api key principal settled token total exceeds rated usage")
		}
		if itemDelta != "0" || itemTokens != 0 {
			deltaAllocations = append(deltaAllocations, domain.Allocation{
				PrincipalID: allocation.PrincipalID, Tokens: itemTokens, Amount: itemDelta,
			})
		}
		delete(currentItems, allocation.PrincipalID)
	}
	if len(currentItems) != 0 {
		return apiKeySkipped, errors.New("api key settled principal is absent from rated usage")
	}
	if deltaAmount == "0" && deltaTokens == 0 && len(deltaAllocations) == 0 {
		return apiKeySkipped, nil
	}
	document, err := s.newAPIKeyDocument(ctx, "ADJUSTMENT", period, current.OriginalDocumentID,
		deltaTokens, deltaAmount, deltaAllocations)
	if err != nil {
		return apiKeySkipped, err
	}
	created, err := s.store.CreateAPIKeyDocument(ctx, current.TotalAmount, current.TotalTokens, document, ratingIDs)
	if err != nil || !created {
		return apiKeySkipped, err
	}
	return apiKeyAdjusted, nil
}

func (s *Service) newAPIKeyDocument(ctx context.Context, documentType string, period APIKeyPeriod,
	originalID, tokens int64, amount string, allocations []domain.Allocation,
) (Document, error) {
	documentID, err := s.ids.NextID(ctx)
	if err != nil || documentID <= 0 {
		return Document{}, errBillingID
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
			return Document{}, errBillingID
		}
		document.Items = append(document.Items, DocumentItem{
			ID: itemID, PrincipalID: allocation.PrincipalID, UsageTokens: allocation.Tokens,
			Amount: allocation.Amount,
		})
	}
	return document, nil
}
