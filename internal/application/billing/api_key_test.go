package billing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

func TestRatePendingAPIKeyUsageCreatesNextRevision(t *testing.T) {
	startedAt := time.Date(2026, 9, 15, 1, 2, 3, 0, time.UTC)
	store := &billingStoreStub{ratingCandidates: []UsageRatingCandidate{{
		UsageRecordID: 10, PrincipalID: 20, CredentialID: 30, ProviderModelID: 40,
		ModelPriceID: 50, LatestRatingID: 60, LatestRevision: 1, UsageStartedAt: startedAt,
		InputTokens: 5, InputPrice: "5", CachedInputPrice: "1", OutputPrice: "10", Currency: "CNY",
	}}}
	service := New(store, &billingIDs{next: 100})
	service.now = func() time.Time { return startedAt.Add(time.Minute) }
	summary, err := service.RatePendingAPIKeyUsage(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 1 || len(store.ratings) != 1 {
		t.Fatalf("summary=%+v ratings=%+v", summary, store.ratings)
	}
	rating := store.ratings[0]
	if rating.Revision != 2 || rating.SupersedesRatingID != 60 || rating.TotalCost != "0.000025" || rating.ID != 101 {
		t.Fatalf("unexpected rating: %+v", rating)
	}
}

func TestRatePendingAPIKeyUsageSnapshotsCacheCreation(t *testing.T) {
	startedAt := time.Date(2026, 9, 15, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name, cacheCreationPrice, cacheCreationCost, totalCost string
	}{
		{name: "configured price", cacheCreationPrice: "3.75", cacheCreationCost: "0.00015", totalCost: "0.000456"},
		{name: "fallback to input price", cacheCreationPrice: "", cacheCreationCost: "0.00012", totalCost: "0.000426"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &billingStoreStub{ratingCandidates: []UsageRatingCandidate{{
				UsageRecordID: 10, PrincipalID: 20, CredentialID: 30, ProviderModelID: 40,
				ModelPriceID: 50, UsageStartedAt: startedAt,
				InputTokens: 110, CachedInputTokens: 20, CacheCreationInputTokens: 40, OutputTokens: 10,
				InputPrice: "3", CachedInputPrice: "0.3", CacheCreationInputPrice: test.cacheCreationPrice,
				OutputPrice: "15", Currency: "USD",
			}}}
			service := New(store, &billingIDs{next: 100})
			service.now = func() time.Time { return startedAt.Add(time.Minute) }
			if _, err := service.RatePendingAPIKeyUsage(context.Background(), 100); err != nil {
				t.Fatal(err)
			}
			if len(store.ratings) != 1 {
				t.Fatalf("ratings=%+v", store.ratings)
			}
			rating := store.ratings[0]
			if rating.CacheCreationInputTokens != 40 || rating.CacheCreationInputPrice != test.cacheCreationPrice ||
				rating.InputCost != "0.00015" || rating.CacheCreationInputCost != test.cacheCreationCost ||
				rating.TotalCost != test.totalCost {
				t.Fatalf("unexpected rating: %+v", rating)
			}
		})
	}
}

func TestSettleDueAPIKeyCreatesMonthlyCharge(t *testing.T) {
	period := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	store := &billingStoreStub{
		apiKeyPeriods: []APIKeyPeriod{period},
		apiKeyUsage: map[APIKeyPeriod][]domain.RatedShare{period: {
			{PrincipalID: 2, Tokens: 5, Amount: "0.000025"},
			{PrincipalID: 1, Tokens: 10, Amount: "0.00005"},
		}},
		apiKeyRatingIDs: map[APIKeyPeriod][]int64{period: {101, 102}},
		apiKeyCurrent:   map[APIKeyPeriod]APIKeyCurrent{},
	}
	service := New(store, &billingIDs{})
	service.now = func() time.Time { return period.PeriodEnd.Add(time.Hour) }
	summary, err := service.SettleDueAPIKeys(context.Background(), period.PeriodEnd)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 1 || len(store.apiKeyDocuments) != 1 {
		t.Fatalf("summary=%+v documents=%+v", summary, store.apiKeyDocuments)
	}
	document := store.apiKeyDocuments[0]
	if document.BillingType != "API_KEY" || document.SourceID != 0 || document.TotalTokens != 15 ||
		document.TotalAmount != "0.000075" || len(document.Items) != 2 || document.Items[0].PrincipalID != 1 ||
		document.Items[0].AllocationRatio != "" {
		t.Fatalf("unexpected document: %+v", document)
	}
}

func TestSettleDueAPIKeyCreatesPriceCorrectionAdjustmentWithoutTokens(t *testing.T) {
	period := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	store := &billingStoreStub{
		apiKeyPeriods: []APIKeyPeriod{period},
		apiKeyUsage: map[APIKeyPeriod][]domain.RatedShare{period: {
			{PrincipalID: 1, Tokens: 100, Amount: "0.0008"},
		}},
		apiKeyCurrent: map[APIKeyPeriod]APIKeyCurrent{period: {
			Found: true, OriginalDocumentID: 99, TotalTokens: 100, TotalAmount: "0.001",
			Items: []APIKeyCurrentItem{{PrincipalID: 1, Tokens: 100, Amount: "0.001"}},
		}},
	}
	service := New(store, &billingIDs{})
	summary, err := service.SettleDueAPIKeys(context.Background(), period.PeriodEnd)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Adjusted != 1 || len(store.apiKeyDocuments) != 1 {
		t.Fatalf("summary=%+v documents=%+v", summary, store.apiKeyDocuments)
	}
	document := store.apiKeyDocuments[0]
	if document.DocumentType != "ADJUSTMENT" || document.OriginalDocumentID != 99 ||
		document.TotalTokens != 0 || document.TotalAmount != "-0.0002" ||
		len(document.Items) != 1 || document.Items[0].UsageTokens != 0 || document.Items[0].Amount != "-0.0002" {
		t.Fatalf("unexpected adjustment: %+v", document)
	}
}

func TestSettleDueAPIKeyAddsLateUsageTokens(t *testing.T) {
	period := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	store := &billingStoreStub{
		apiKeyPeriods: []APIKeyPeriod{period},
		apiKeyUsage: map[APIKeyPeriod][]domain.RatedShare{period: {
			{PrincipalID: 1, Tokens: 120, Amount: "1.2"},
		}},
		apiKeyCurrent: map[APIKeyPeriod]APIKeyCurrent{period: {
			Found: true, OriginalDocumentID: 99, TotalTokens: 100, TotalAmount: "1",
			Items: []APIKeyCurrentItem{{PrincipalID: 1, Tokens: 100, Amount: "1"}},
		}},
	}
	service := New(store, &billingIDs{})
	_, err := service.SettleDueAPIKeys(context.Background(), period.PeriodEnd)
	if err != nil {
		t.Fatal(err)
	}
	document := store.apiKeyDocuments[0]
	if document.TotalTokens != 20 || document.TotalAmount != "0.2" || document.Items[0].UsageTokens != 20 {
		t.Fatalf("unexpected late usage adjustment: %+v", document)
	}
}

func TestSettleDueAPIKeyAddsLatePrincipal(t *testing.T) {
	period := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	store := &billingStoreStub{
		apiKeyPeriods: []APIKeyPeriod{period},
		apiKeyUsage: map[APIKeyPeriod][]domain.RatedShare{period: {
			{PrincipalID: 1, Tokens: 100, Amount: "1"},
			{PrincipalID: 2, Tokens: 20, Amount: "0.2"},
		}},
		apiKeyCurrent: map[APIKeyPeriod]APIKeyCurrent{period: {
			Found: true, OriginalDocumentID: 99, TotalTokens: 100, TotalAmount: "1",
			Items: []APIKeyCurrentItem{{PrincipalID: 1, Tokens: 100, Amount: "1"}},
		}},
	}
	service := New(store, &billingIDs{})
	if _, err := service.SettleDueAPIKeys(context.Background(), period.PeriodEnd); err != nil {
		t.Fatal(err)
	}
	document := store.apiKeyDocuments[0]
	if document.TotalTokens != 20 || document.TotalAmount != "0.2" || len(document.Items) != 1 ||
		document.Items[0].PrincipalID != 2 || document.Items[0].UsageTokens != 20 || document.Items[0].Amount != "0.2" {
		t.Fatalf("unexpected late principal adjustment: %+v", document)
	}
}

func TestRatePendingAPIKeyUsageScansUnratedThenRepricedAndIsolatesFailures(t *testing.T) {
	startedAt := time.Date(2026, 9, 15, 1, 2, 3, 0, time.UTC)
	candidate := func(id int64, revision int32) UsageRatingCandidate {
		latest := int64(0)
		if revision > 0 {
			latest = id + 1000
		}
		return UsageRatingCandidate{
			UsageRecordID: id, PrincipalID: 20, CredentialID: 30, ProviderModelID: 40, ModelPriceID: 50,
			LatestRatingID: latest, LatestRevision: revision, UsageStartedAt: startedAt,
			InputTokens: 5, InputPrice: "5", CachedInputPrice: "1", OutputPrice: "10", Currency: "CNY",
		}
	}
	store := &billingStoreStub{
		ratingCandidates: []UsageRatingCandidate{candidate(10, 0), candidate(11, 0), candidate(12, 0)},
		repricedRatings:  []UsageRatingCandidate{candidate(5, 1), candidate(13, 2)},
		ratingErrors: map[int64]error{11: &StoreError{
			Operation: "create_usage_rating", SQLState: "23514", Message: "usage rating price snapshot is not current",
		}},
	}
	service := New(store, &billingIDs{next: 100})
	summary, err := service.RatePendingAPIKeyUsage(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 4 || summary.Failed != 1 || len(store.ratings) != 4 {
		t.Fatalf("summary=%+v ratings=%+v", summary, store.ratings)
	}
	gotIDs := make([]int64, 0, len(store.ratings))
	for _, rating := range store.ratings {
		gotIDs = append(gotIDs, rating.UsageRecordID)
	}
	if want := []int64{10, 12, 5, 13}; !equalInt64s(gotIDs, want) {
		t.Fatalf("rated usage=%v; want %v", gotIDs, want)
	}
	if store.ratings[2].Revision != 2 || store.ratings[2].SupersedesRatingID != 1005 || store.ratings[3].Revision != 3 {
		t.Fatalf("repriced revisions=%+v", store.ratings[2:])
	}
	if len(summary.FailureSamples) != 1 || !strings.Contains(summary.FailureSamples[0], "usage=11") ||
		!strings.Contains(summary.FailureSamples[0], "sqlstate=23514") {
		t.Fatalf("failure samples=%v", summary.FailureSamples)
	}
	if store.ratingScans[0] != RatingScanUnrated || store.ratingScans[len(store.ratingScans)-1] != RatingScanRepriced {
		t.Fatalf("scan order=%v", store.ratingScans)
	}
}

func TestSettleDueAPIKeyContinuesAfterPeriodFailure(t *testing.T) {
	failed := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
	settled := APIKeyPeriod{
		CredentialID: 30, Currency: "CNY",
		PeriodStart: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	store := &billingStoreStub{
		apiKeyPeriods: []APIKeyPeriod{failed, settled},
		apiKeyUsage:   map[APIKeyPeriod][]domain.RatedShare{settled: {{PrincipalID: 1, Tokens: 10, Amount: "1"}}},
		apiKeyCurrent: map[APIKeyPeriod]APIKeyCurrent{},
		apiKeyErrors:  map[APIKeyPeriod]error{failed: errors.New("api key settled token total exceeds rated usage")},
	}
	service := New(store, &billingIDs{})
	summary, err := service.SettleDueAPIKeys(context.Background(), settled.PeriodEnd)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Created != 1 || summary.Failed != 1 || len(store.apiKeyDocuments) != 1 ||
		!store.apiKeyDocuments[0].PeriodStart.Equal(settled.PeriodStart) {
		t.Fatalf("summary=%+v documents=%+v", summary, store.apiKeyDocuments)
	}
	if len(summary.FailureSamples) != 1 || !strings.Contains(summary.FailureSamples[0], "period=2026-08-01T00:00:00Z") {
		t.Fatalf("failure samples=%v", summary.FailureSamples)
	}
}

func equalInt64s(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
