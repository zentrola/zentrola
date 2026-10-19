package billing

import (
	"context"
	"errors"
	"strings"
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
	prices              []SubscriptionPrice
	periods             []ChargePeriod
	usage               map[int64][]domain.UsageShare
	charges             []Document
	chargePrices        [][]SubscriptionPrice
	chargeAttempts      map[int64]int
	corrections         []Correction
	original            map[int64][]domain.UsageShare
	adjustments         []Document
	statistics          Statistics
	ratingCandidates    []UsageRatingCandidate
	repricedRatings     []UsageRatingCandidate
	ratingScans         []RatingScan
	ratingErrors        map[int64]error
	chargeErrors        map[int64]error
	apiKeyErrors        map[APIKeyPeriod]error
	ratings             []UsageRating
	apiKeyPeriods       []APIKeyPeriod
	apiKeyUsage         map[APIKeyPeriod][]domain.RatedShare
	apiKeyRatingIDs     map[APIKeyPeriod][]int64
	apiKeyCurrent       map[APIKeyPeriod]APIKeyCurrent
	apiKeyDocuments     []Document
	apiKeyConfirmations []APIKeyConfirmation
	documents           DocumentPage
	document            DocumentDetail
	unrated             UnratedUsagePage
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
func (s *billingStoreStub) CreateCharge(_ context.Context, prices []SubscriptionPrice, document Document) (bool, error) {
	if s.chargeAttempts == nil {
		s.chargeAttempts = make(map[int64]int)
	}
	s.chargeAttempts[document.CredentialID]++
	if err := s.chargeErrors[document.CredentialID]; err != nil {
		return false, err
	}
	s.charges = append(s.charges, document)
	s.chargePrices = append(s.chargePrices, prices)
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
func (s *billingStoreStub) BillingDocuments(context.Context, DocumentFilter) (DocumentPage, error) {
	return s.documents, nil
}
func (s *billingStoreStub) BillingDocument(context.Context, int64) (DocumentDetail, error) {
	return s.document, nil
}
func (s *billingStoreStub) UnratedUsage(context.Context, UnratedUsageFilter) (UnratedUsagePage, error) {
	return s.unrated, nil
}
func (s *billingStoreStub) UsageRatingCandidates(_ context.Context, scan RatingScan, after int64, limit int32) ([]UsageRatingCandidate, error) {
	s.ratingScans = append(s.ratingScans, scan)
	source := s.ratingCandidates
	if scan == RatingScanRepriced {
		source = s.repricedRatings
	}
	result := make([]UsageRatingCandidate, 0, limit)
	for _, candidate := range source {
		if candidate.UsageRecordID > after && len(result) < int(limit) {
			result = append(result, candidate)
		}
	}
	return result, nil
}
func (s *billingStoreStub) CreateUsageRating(_ context.Context, rating UsageRating) (bool, error) {
	if err := s.ratingErrors[rating.UsageRecordID]; err != nil {
		return false, err
	}
	s.ratings = append(s.ratings, rating)
	return true, nil
}
func (s *billingStoreStub) APIKeyPeriods(context.Context, time.Time) ([]APIKeyPeriod, error) {
	return s.apiKeyPeriods, nil
}
func (s *billingStoreStub) APIKeyUsage(_ context.Context, period APIKeyPeriod) ([]domain.RatedShare, []int64, error) {
	if err := s.apiKeyErrors[period]; err != nil {
		return nil, nil, err
	}
	return s.apiKeyUsage[period], s.apiKeyRatingIDs[period], nil
}
func (s *billingStoreStub) APIKeyCurrent(_ context.Context, period APIKeyPeriod) (APIKeyCurrent, error) {
	return s.apiKeyCurrent[period], nil
}
func (s *billingStoreStub) CreateAPIKeyDocument(_ context.Context, _ string, _ int64, document Document, _ []int64) (bool, error) {
	s.apiKeyDocuments = append(s.apiKeyDocuments, document)
	return true, nil
}
func (s *billingStoreStub) ConfirmAPIKeyRatings(_ context.Context, confirmation APIKeyConfirmation) (bool, error) {
	s.apiKeyConfirmations = append(s.apiKeyConfirmations, confirmation)
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
	// 交给存储层校验的价格历史是账期起点及之前生效的全部价格，最后一条为来源价格。
	if len(store.chargePrices) != 2 || len(store.chargePrices[0]) != 1 || store.chargePrices[0][0].ID != 1 ||
		len(store.chargePrices[1]) != 2 || store.chargePrices[1][1].ID != 2 || store.charges[1].SourceID != 2 {
		t.Fatalf("charge price history=%+v", store.chargePrices)
	}
}

func TestSettleSkipsCredentialWhenPricesChangedAfterRead(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store := &billingStoreStub{
		prices: []SubscriptionPrice{
			{ID: 1, CredentialID: 8, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: start, CredentialEffectiveAt: start},
			{ID: 2, CredentialID: 9, Currency: "CNY", PeriodAmount: "200", BillingPeriod: "MONTH", EffectiveAt: start, CredentialEffectiveAt: start},
		},
		chargeErrors: map[int64]error{8: ErrSubscriptionPricesChanged},
	}
	service := New(store, &billingIDs{})
	summary, err := service.SettleDueSubscriptions(context.Background(), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	// 价格已变化的凭据本轮只尝试一次即放弃后续账期，不计入失败；其他凭据照常出账。
	if summary.Created != 2 || summary.Skipped != 1 || summary.Failed != 0 ||
		store.chargeAttempts[8] != 1 || store.chargeAttempts[9] != 2 {
		t.Fatalf("summary=%+v attempts=%v", summary, store.chargeAttempts)
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

func TestSettleSkipsPeriodOverlappingChargedHistory(t *testing.T) {
	backfilled := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	original := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	store := &billingStoreStub{
		prices: []SubscriptionPrice{
			{ID: 1, CredentialID: 9, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: backfilled, CredentialEffectiveAt: backfilled},
			{ID: 2, CredentialID: 9, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: original, CredentialEffectiveAt: backfilled},
		},
		// 补录更早价格前，原锚点已出账 9/15–10/15。
		periods: []ChargePeriod{{CredentialID: 9, PeriodStart: original, PeriodEnd: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}},
	}
	service := New(store, &billingIDs{})
	summary, err := service.SettleDueSubscriptions(context.Background(), time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 1 || summary.Failed != 1 || len(store.charges) != 1 ||
		!store.charges[0].PeriodEnd.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("summary=%+v charges=%+v", summary, store.charges)
	}
	if len(summary.FailureSamples) != 1 || !strings.Contains(summary.FailureSamples[0], "overlaps charged period") {
		t.Fatalf("failure samples=%v", summary.FailureSamples)
	}
}

func TestSettleContinuesAfterSinglePeriodFailure(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store := &billingStoreStub{
		prices: []SubscriptionPrice{
			{ID: 1, CredentialID: 8, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: start, CredentialEffectiveAt: start},
			{ID: 2, CredentialID: 9, Currency: "CNY", PeriodAmount: "200", BillingPeriod: "MONTH", EffectiveAt: start, CredentialEffectiveAt: start},
		},
		chargeErrors: map[int64]error{8: &StoreError{
			Operation: "create_document", SQLState: "23514", Message: "subscription charge amount does not match source",
			Err: errors.New("raw driver error with secret"),
		}},
		corrections: []Correction{{OriginalDocumentID: 70, Currency: "CNY", TargetCurrency: "USD"}},
	}
	service := New(store, &billingIDs{})
	summary, err := service.SettleDueSubscriptions(context.Background(), end)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 1 || summary.Failed != 2 || len(store.charges) != 1 || store.charges[0].CredentialID != 9 {
		t.Fatalf("summary=%+v charges=%+v", summary, store.charges)
	}
	joined := strings.Join(summary.FailureSamples, "\n")
	if !strings.Contains(joined, "sqlstate=23514") || !strings.Contains(joined, "does not match source") ||
		strings.Contains(joined, "secret") || !strings.Contains(joined, "currency changed") {
		t.Fatalf("failure samples=%v", summary.FailureSamples)
	}
}

func TestSettleStopsOnCanceledContext(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &billingStoreStub{
		prices: []SubscriptionPrice{
			{ID: 1, CredentialID: 8, Currency: "CNY", PeriodAmount: "100", BillingPeriod: "MONTH", EffectiveAt: start, CredentialEffectiveAt: start},
		},
		chargeErrors: map[int64]error{8: context.Canceled},
	}
	service := New(store, &billingIDs{})
	if _, err := service.SettleDueSubscriptions(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v; want context canceled", err)
	}
}

type sqlStateError struct{ code string }

func (e sqlStateError) Error() string    { return "password=secret " + e.code }
func (e sqlStateError) SQLState() string { return e.code }

func TestSafeErrorTextOmitsUnsanitizedDetails(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		want    string
		exclude string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "store error with database fields", err: &StoreError{Operation: "create_usage_rating", SQLState: "23514",
			Constraint: "usage_rating_total_cost_check", Message: "usage rating price snapshot is not current", Err: sqlStateError{"23514"}},
			want: `billing store create_usage_rating failed: sqlstate=23514 constraint=usage_rating_total_cost_check message="usage rating price snapshot is not current"`},
		{name: "store error without database fields", err: &StoreError{Operation: "api_key_usage", Err: errors.New("dial tcp: password=secret")},
			want: "billing store api_key_usage failed: cause_type=*errors.errorString", exclude: "secret"},
		{name: "store error deadline", err: &StoreError{Operation: "charge_periods", Err: context.DeadlineExceeded},
			want: "billing store charge_periods failed: deadline exceeded"},
		{name: "raw database error", err: sqlStateError{"40001"}, want: "database error: sqlstate=40001", exclude: "secret"},
		{name: "domain error", err: errors.New("invalid billing amount"), want: "invalid billing amount"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeErrorText(tt.err)
			if got != tt.want || tt.exclude != "" && strings.Contains(got, tt.exclude) {
				t.Fatalf("SafeErrorText()=%q; want %q", got, tt.want)
			}
		})
	}
}
