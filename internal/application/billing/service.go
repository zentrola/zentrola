// Package billing 编排个人订阅与 API Key 的用量核算、账期结算、差额调整和费用统计。
package billing

import (
	"context"
	"errors"
	"sort"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

const maxPeriodsPerScan = 10000

type SubscriptionPrice struct {
	ID, CredentialID                      int64
	Currency, PeriodAmount, BillingPeriod string
	EffectiveAt, CredentialEffectiveAt    time.Time
	CredentialEndAt                       *time.Time
}

type ChargePeriod struct {
	CredentialID           int64
	PeriodStart, PeriodEnd time.Time
}

type Correction struct {
	OriginalDocumentID          int64
	CredentialID, SourcePriceID int64
	PeriodStart, PeriodEnd      time.Time
	Currency, TargetCurrency    string
	CurrentAmount, TargetAmount string
}

type DocumentItem struct {
	ID, PrincipalID int64
	UsageTokens     int64
	AllocationRatio string
	Amount          string
}

type Document struct {
	ID, CredentialID, SourceID, OriginalDocumentID int64
	BillingType, DocumentType, Status, SourceType  string
	PeriodStart, PeriodEnd, CreatedAt              time.Time
	TotalTokens                                    int64
	TotalAmount, Currency                          string
	Items                                          []DocumentItem
}

type StatisticsFilter struct {
	BillingType string
	From, To    time.Time
	After       int64
	Limit       int32
}

type CurrencyTotal struct {
	BillingType       string `json:"billingType"`
	Currency          string `json:"currency"`
	TotalAmount       string `json:"totalAmount"`
	AllocatedAmount   string `json:"allocatedAmount"`
	UnallocatedAmount string `json:"unallocatedAmount"`
	TotalTokens       int64  `json:"totalTokens"`
}

type PrincipalTotal struct {
	PrincipalID   int64  `json:"principalId,string"`
	PrincipalName string `json:"principalName"`
	PrincipalType string `json:"principalType"`
	BillingType   string `json:"billingType"`
	Currency      string `json:"currency"`
	Amount        string `json:"amount"`
	Tokens        int64  `json:"tokens"`
}

type Statistics struct {
	From   time.Time        `json:"from"`
	To     time.Time        `json:"to"`
	Totals []CurrencyTotal  `json:"totals"`
	Items  []PrincipalTotal `json:"items"`
	Total  int64            `json:"total"`
}

type Summary struct {
	Created, Adjusted, Skipped, Failed int
}

type Store interface {
	SubscriptionPrices(context.Context) ([]SubscriptionPrice, error)
	ChargePeriods(context.Context) ([]ChargePeriod, error)
	SubscriptionUsage(context.Context, int64, time.Time, time.Time) ([]domain.UsageShare, error)
	CreateCharge(context.Context, Document) (bool, error)
	SubscriptionCorrections(context.Context) ([]Correction, error)
	OriginalUsage(context.Context, int64) ([]domain.UsageShare, error)
	CreateAdjustment(context.Context, string, Document) (bool, error)
	UsageRatingCandidates(context.Context, int64, int32) ([]UsageRatingCandidate, error)
	CreateUsageRating(context.Context, UsageRating) (bool, error)
	APIKeyPeriods(context.Context, time.Time) ([]APIKeyPeriod, error)
	APIKeyUsage(context.Context, APIKeyPeriod) ([]domain.RatedShare, []int64, error)
	APIKeyCurrent(context.Context, APIKeyPeriod) (APIKeyCurrent, error)
	CreateAPIKeyDocument(context.Context, string, int64, Document, []int64) (bool, error)
	Statistics(context.Context, admin.Identity, StatisticsFilter) (Statistics, error)
}

type Service struct {
	store Store
	ids   shared.IDGenerator
	now   func() time.Time
}

func New(store Store, ids shared.IDGenerator) *Service {
	return &Service{store: store, ids: ids, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SettleDueSubscriptions(ctx context.Context, cutoff time.Time) (Summary, error) {
	var summary Summary
	if s == nil || s.store == nil || s.ids == nil || cutoff.IsZero() {
		return summary, appsec.ErrUnavailable
	}
	cutoff = cutoff.UTC()
	prices, err := s.store.SubscriptionPrices(ctx)
	if err != nil {
		return summary, err
	}
	periods, err := s.store.ChargePeriods(ctx)
	if err != nil {
		return summary, err
	}
	existing := make(map[periodKey]struct{}, len(periods))
	for _, period := range periods {
		existing[newPeriodKey(period.CredentialID, period.PeriodStart, period.PeriodEnd)] = struct{}{}
	}

	groups := groupSubscriptionPrices(prices)
	periodCount := 0
	for _, group := range groups {
		periodStart := group[0].EffectiveAt.UTC()
		if group[0].CredentialEffectiveAt.After(periodStart) {
			periodStart = group[0].CredentialEffectiveAt.UTC()
		}
		anchorDay := periodStart.Day()
		for !periodStart.After(cutoff) {
			if group[0].CredentialEndAt != nil && !periodStart.Before(group[0].CredentialEndAt.UTC()) {
				break
			}
			price := priceAt(group, periodStart)
			if price == nil {
				break
			}
			periodEnd, addErr := addBillingPeriod(periodStart, price.BillingPeriod, anchorDay)
			if addErr != nil {
				return summary, addErr
			}
			if periodEnd.After(cutoff) {
				break
			}
			key := newPeriodKey(price.CredentialID, periodStart, periodEnd)
			if _, found := existing[key]; found {
				summary.Skipped++
				periodStart = periodEnd
				continue
			}
			periodCount++
			if periodCount > maxPeriodsPerScan {
				return summary, errors.New("billing settlement scan exceeded period limit")
			}
			usage, usageErr := s.store.SubscriptionUsage(ctx, price.CredentialID, periodStart, periodEnd)
			if usageErr != nil {
				return summary, usageErr
			}
			totalTokens, allocations, allocationErr := domain.Allocate(price.PeriodAmount, usage)
			if allocationErr != nil {
				return summary, allocationErr
			}
			document, documentErr := s.newDocument(ctx, "CHARGE", price.CredentialID, price.ID, 0,
				periodStart, periodEnd, totalTokens, price.PeriodAmount, price.Currency, allocations, true)
			if documentErr != nil {
				return summary, documentErr
			}
			created, createErr := s.store.CreateCharge(ctx, document)
			if createErr != nil {
				return summary, createErr
			}
			if created {
				summary.Created++
			} else {
				summary.Skipped++
			}
			existing[key] = struct{}{}
			periodStart = periodEnd
		}
	}

	corrections, err := s.store.SubscriptionCorrections(ctx)
	if err != nil {
		return summary, err
	}
	for _, correction := range corrections {
		if correction.Currency != correction.TargetCurrency {
			summary.Failed++
			continue
		}
		delta, deltaErr := domain.Subtract(correction.TargetAmount, correction.CurrentAmount)
		if deltaErr != nil {
			return summary, deltaErr
		}
		if delta == "0" {
			continue
		}
		usage, usageErr := s.store.OriginalUsage(ctx, correction.OriginalDocumentID)
		if usageErr != nil {
			return summary, usageErr
		}
		_, allocations, allocationErr := domain.Allocate(delta, usage)
		if allocationErr != nil {
			return summary, allocationErr
		}
		document, documentErr := s.newDocument(ctx, "ADJUSTMENT", correction.CredentialID,
			correction.SourcePriceID, correction.OriginalDocumentID, correction.PeriodStart,
			correction.PeriodEnd, 0, delta, correction.Currency, allocations, false)
		if documentErr != nil {
			return summary, documentErr
		}
		created, createErr := s.store.CreateAdjustment(ctx, correction.CurrentAmount, document)
		if createErr != nil {
			return summary, createErr
		}
		if created {
			summary.Adjusted++
		} else {
			summary.Skipped++
		}
	}
	return summary, nil
}

func (s *Service) Statistics(ctx context.Context, actor admin.Identity, filter StatisticsFilter) (Statistics, error) {
	if actor.ID <= 0 {
		return Statistics{}, appsec.ErrUnauthenticated
	}
	if s == nil || s.store == nil {
		return Statistics{}, appsec.ErrUnavailable
	}
	if filter.From.IsZero() || !filter.To.After(filter.From) || filter.To.Sub(filter.From) > 366*24*time.Hour ||
		filter.After < 0 || filter.Limit < 1 || filter.Limit > 100 ||
		filter.BillingType != "" && filter.BillingType != "SUBSCRIPTION" && filter.BillingType != "API_KEY" {
		return Statistics{}, appsec.ErrInvalidArgument
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	return s.store.Statistics(ctx, actor, filter)
}

func (s *Service) newDocument(ctx context.Context, documentType string, credentialID, sourceID, originalID int64,
	periodStart, periodEnd time.Time, totalTokens int64, amount, currency string,
	allocations []domain.Allocation, keepUsage bool,
) (Document, error) {
	documentID, err := s.ids.NextID(ctx)
	if err != nil || documentID <= 0 {
		return Document{}, errors.New("cannot generate billing document ID")
	}
	document := Document{
		ID: documentID, CredentialID: credentialID, SourceID: sourceID, OriginalDocumentID: originalID,
		BillingType: "SUBSCRIPTION", DocumentType: documentType, Status: "CONFIRMED", SourceType: "SUBSCRIPTION_PRICE",
		PeriodStart: periodStart.UTC(), PeriodEnd: periodEnd.UTC(), CreatedAt: s.now().UTC(),
		TotalTokens: totalTokens, TotalAmount: amount, Currency: currency, Items: make([]DocumentItem, 0, len(allocations)),
	}
	for _, allocation := range allocations {
		itemID, idErr := s.ids.NextID(ctx)
		if idErr != nil || itemID <= 0 {
			return Document{}, errors.New("cannot generate billing document item ID")
		}
		usageTokens := int64(0)
		if keepUsage {
			usageTokens = allocation.Tokens
		}
		document.Items = append(document.Items, DocumentItem{
			ID: itemID, PrincipalID: allocation.PrincipalID, UsageTokens: usageTokens,
			AllocationRatio: allocation.Ratio, Amount: allocation.Amount,
		})
	}
	return document, nil
}

type periodKey struct {
	credentialID int64
	start, end   int64
}

func newPeriodKey(credentialID int64, start, end time.Time) periodKey {
	return periodKey{credentialID: credentialID, start: start.UTC().UnixNano(), end: end.UTC().UnixNano()}
}

func groupSubscriptionPrices(prices []SubscriptionPrice) [][]SubscriptionPrice {
	prices = append([]SubscriptionPrice(nil), prices...)
	sort.Slice(prices, func(i, j int) bool {
		if prices[i].CredentialID != prices[j].CredentialID {
			return prices[i].CredentialID < prices[j].CredentialID
		}
		if !prices[i].EffectiveAt.Equal(prices[j].EffectiveAt) {
			return prices[i].EffectiveAt.Before(prices[j].EffectiveAt)
		}
		return prices[i].ID < prices[j].ID
	})
	groups := make([][]SubscriptionPrice, 0)
	for _, price := range prices {
		if len(groups) == 0 || groups[len(groups)-1][0].CredentialID != price.CredentialID {
			groups = append(groups, []SubscriptionPrice{price})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], price)
		}
	}
	return groups
}

func priceAt(prices []SubscriptionPrice, at time.Time) *SubscriptionPrice {
	var result *SubscriptionPrice
	for index := range prices {
		if prices[index].EffectiveAt.After(at) {
			break
		}
		result = &prices[index]
	}
	return result
}

func addBillingPeriod(start time.Time, period string, anchorDay int) (time.Time, error) {
	start = start.UTC()
	year, month, day := start.Date()
	switch period {
	case "MONTH":
		month++
		if month > 12 {
			year++
			month = 1
		}
	case "YEAR":
		year++
	default:
		return time.Time{}, errors.New("invalid subscription billing period")
	}
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day = anchorDay
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), time.UTC), nil
}
