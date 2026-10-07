package billing

import (
	"context"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

type billingIDs struct{ next int64 }

func (g *billingIDs) NextID(context.Context) (int64, error) {
	g.next++
	return g.next, nil
}

type billingStoreStub struct {
	prices           []SubscriptionPrice
	periods          []ChargePeriod
	usage            map[int64][]domain.UsageShare
	charges          []Document
	corrections      []Correction
	original         map[int64][]domain.UsageShare
	adjustments      []Document
	statistics       Statistics
	ratingCandidates []UsageRatingCandidate
	ratings          []UsageRating
	apiKeyPeriods    []APIKeyPeriod
	apiKeyUsage      map[APIKeyPeriod][]domain.RatedShare
	apiKeyRatingIDs  map[APIKeyPeriod][]int64
	apiKeyCurrent    map[APIKeyPeriod]APIKeyCurrent
	apiKeyDocuments  []Document
}

func (s *billingStoreStub) SubscriptionPrices(context.Context) ([]SubscriptionPrice, error) {
	return s.prices, nil
}
func (s *billingStoreStub) ChargePeriods(context.Context) ([]ChargePeriod, error) {
	return s.periods, nil
}
func (s *billingStoreStub) SubscriptionUsage(_ context.Context, credentialID int64, _, _ time.Time) ([]domain.UsageShare, error) {
	return s.usage[credentialID], nil
}
func (s *billingStoreStub) CreateCharge(_ context.Context, document Document) (bool, error) {
	s.charges = append(s.charges, document)
	return true, nil
}
func (s *billingStoreStub) SubscriptionCorrections(context.Context) ([]Correction, error) {
	return s.corrections, nil
}
func (s *billingStoreStub) OriginalUsage(_ context.Context, documentID int64) ([]domain.UsageShare, error) {
	return s.original[documentID], nil
}
func (s *billingStoreStub) CreateAdjustment(_ context.Context, _ string, document Document) (bool, error) {
	s.adjustments = append(s.adjustments, document)
	return true, nil
}
func (s *billingStoreStub) Statistics(context.Context, admin.Identity, StatisticsFilter) (Statistics, error) {
	return s.statistics, nil
}
func (s *billingStoreStub) UsageRatingCandidates(_ context.Context, after int64, limit int32) ([]UsageRatingCandidate, error) {
	result := make([]UsageRatingCandidate, 0, limit)
	for _, candidate := range s.ratingCandidates {
		if candidate.UsageRecordID > after && len(result) < int(limit) {
			result = append(result, candidate)
		}
	}
	return result, nil
}
func (s *billingStoreStub) CreateUsageRating(_ context.Context, rating UsageRating) (bool, error) {
	s.ratings = append(s.ratings, rating)
	return true, nil
}
func (s *billingStoreStub) APIKeyPeriods(context.Context, time.Time) ([]APIKeyPeriod, error) {
	return s.apiKeyPeriods, nil
}
func (s *billingStoreStub) APIKeyUsage(_ context.Context, period APIKeyPeriod) ([]domain.RatedShare, []int64, error) {
	return s.apiKeyUsage[period], s.apiKeyRatingIDs[period], nil
}
func (s *billingStoreStub) APIKeyCurrent(_ context.Context, period APIKeyPeriod) (APIKeyCurrent, error) {
	return s.apiKeyCurrent[period], nil
}
func (s *billingStoreStub) CreateAPIKeyDocument(_ context.Context, _ string, _ int64, document Document, _ []int64) (bool, error) {
	s.apiKeyDocuments = append(s.apiKeyDocuments, document)
	return true, nil
}

func TestSettleDueMonthlySubscriptionAndSkipExistingPeriod(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store := &billingStoreStub{
		prices: []SubscriptionPrice{{
			ID: 11, CredentialID: 21, Currency: "CNY", PeriodAmount: "710", BillingPeriod: "MONTH",
			EffectiveAt: start, CredentialEffectiveAt: start,
		}},
		periods: []ChargePeriod{{CredentialID: 21, PeriodStart: start, PeriodEnd: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}},
		usage: map[int64][]domain.UsageShare{21: {
			{PrincipalID: 1, Tokens: 70},
			{PrincipalID: 2, Tokens: 30},
		}},
	}
	service := New(store, &billingIDs{})
	service.now = func() time.Time { return time.Date(2026, 10, 1, 0, 10, 0, 0, time.UTC) }
	summary, err := service.SettleDueSubscriptions(context.Background(), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 1 || summary.Skipped != 1 || len(store.charges) != 1 {
		t.Fatalf("summary=%+v charges=%+v", summary, store.charges)
	}
	document := store.charges[0]
	if !document.PeriodStart.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) ||
		!document.PeriodEnd.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
		document.TotalTokens != 100 || document.TotalAmount != "710" || len(document.Items) != 2 ||
		document.Items[0].Amount != "497" || document.Items[1].Amount != "213" {
		t.Fatalf("unexpected document: %+v", document)
	}
}

func TestSettleUsesNewPriceAtNextBillingBoundary(t *testing.T) {
	start := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	store := &billingStoreStub{
		prices: []SubscriptionPrice{
			{ID: 1, CredentialID: 9, Currency: "CNY", PeriodAmount: "1000", BillingPeriod: "MONTH", EffectiveAt: start, CredentialEffectiveAt: start},
			{ID: 2, CredentialID: 9, Currency: "CNY", PeriodAmount: "800", BillingPeriod: "MONTH", EffectiveAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), CredentialEffectiveAt: start},
		},
		usage: map[int64][]domain.UsageShare{9: {{PrincipalID: 1, Tokens: 1}}},
	}
	service := New(store, &billingIDs{})
	service.now = func() time.Time { return time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC) }
	_, err := service.SettleDueSubscriptions(context.Background(), time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.charges) != 2 || store.charges[0].TotalAmount != "1000" || store.charges[1].TotalAmount != "800" ||
		!store.charges[0].PeriodEnd.Equal(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)) ||
		!store.charges[1].PeriodEnd.Equal(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected month-end schedule: %+v", store.charges)
	}
}

func TestSettleCreatesAdjustmentWithoutRepeatingTokens(t *testing.T) {
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store := &billingStoreStub{
		corrections: []Correction{{
			OriginalDocumentID: 100, CredentialID: 9, SourcePriceID: 8,
			PeriodStart: periodStart, PeriodEnd: periodEnd,
			Currency: "CNY", TargetCurrency: "CNY", CurrentAmount: "1000", TargetAmount: "800",
		}},
		original: map[int64][]domain.UsageShare{100: {
			{PrincipalID: 1, Tokens: 70},
			{PrincipalID: 2, Tokens: 30},
		}},
	}
	service := New(store, &billingIDs{})
	service.now = func() time.Time { return periodEnd.Add(time.Hour) }
	summary, err := service.SettleDueSubscriptions(context.Background(), periodEnd)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Adjusted != 1 || len(store.adjustments) != 1 {
		t.Fatalf("summary=%+v adjustments=%+v", summary, store.adjustments)
	}
	document := store.adjustments[0]
	if document.TotalAmount != "-200" || document.TotalTokens != 0 || len(document.Items) != 2 ||
		document.Items[0].UsageTokens != 0 || document.Items[0].Amount != "-140" ||
		document.Items[1].UsageTokens != 0 || document.Items[1].Amount != "-60" {
		t.Fatalf("unexpected adjustment: %+v", document)
	}
}

func TestSettlePeriodLimitIgnoresAlreadySettledHistory(t *testing.T) {
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	periods := make([]ChargePeriod, 0, maxPeriodsPerScan+1)
	periodStart := start
	for range maxPeriodsPerScan + 1 {
		periodEnd, err := addBillingPeriod(periodStart, "MONTH", 1)
		if err != nil {
			t.Fatal(err)
		}
		periods = append(periods, ChargePeriod{CredentialID: 9, PeriodStart: periodStart, PeriodEnd: periodEnd})
		periodStart = periodEnd
	}
	store := &billingStoreStub{
		prices: []SubscriptionPrice{{
			ID: 1, CredentialID: 9, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH",
			EffectiveAt: start, CredentialEffectiveAt: start,
		}},
		periods: periods,
	}
	service := New(store, &billingIDs{})
	summary, err := service.SettleDueSubscriptions(context.Background(), periodStart)
	if err != nil || summary.Skipped != maxPeriodsPerScan+1 || len(store.charges) != 0 {
		t.Fatalf("summary=%+v charges=%d err=%v", summary, len(store.charges), err)
	}
}

func TestStatisticsValidatesUTCWindowAndType(t *testing.T) {
	service := New(&billingStoreStub{}, &billingIDs{})
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.Statistics(context.Background(), admin.Identity{ID: 1}, StatisticsFilter{
		BillingType: "SUBSCRIPTION", From: from, To: from.AddDate(0, 1, 0), Limit: 50,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Statistics(context.Background(), admin.Identity{ID: 1}, StatisticsFilter{
		BillingType: "UNKNOWN", From: from, To: from.AddDate(0, 1, 0), Limit: 50,
	}); err == nil {
		t.Fatal("invalid billing type accepted")
	}
}
