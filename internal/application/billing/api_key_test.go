package billing

import (
	"context"
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
