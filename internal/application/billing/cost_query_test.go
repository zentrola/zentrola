package billing

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

type usageCostStoreStub struct {
	filter         UsageCostFilter
	page           UsageCostPage
	prices         []SubscriptionPrice
	usage          map[int64][]domain.UsageShare
	identities     []PrincipalIdentity
	apiAttribution []PrincipalTotal
}

func (s *usageCostStoreStub) UsageCostSummary(_ context.Context, _ admin.Identity, filter UsageCostFilter) (UsageCostSummary, error) {
	s.filter = filter
	return UsageCostSummary{}, nil
}

func (s *usageCostStoreStub) UsageCosts(_ context.Context, _ admin.Identity, filter UsageCostFilter) (UsageCostPage, error) {
	s.filter = filter
	return s.page, nil
}

func (*usageCostStoreStub) UsageCost(context.Context, admin.Identity, int64) (UsageCostDetail, error) {
	return UsageCostDetail{}, nil
}

func (s *usageCostStoreStub) SubscriptionPrices(context.Context) ([]SubscriptionPrice, error) {
	return s.prices, nil
}

func (s *usageCostStoreStub) SubscriptionUsage(
	_ context.Context,
	credentialID int64,
	_, _ time.Time,
) ([]domain.UsageShare, error) {
	return s.usage[credentialID], nil
}

func (s *usageCostStoreStub) PrincipalIdentities(context.Context, []int64) ([]PrincipalIdentity, error) {
	return s.identities, nil
}

func (s *usageCostStoreStub) CurrentAPIKeyAttribution(context.Context, time.Time, time.Time) ([]PrincipalTotal, error) {
	return s.apiAttribution, nil
}

func validUsageCostFilter() UsageCostFilter {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return UsageCostFilter{From: from, To: from.AddDate(0, 1, 0), Limit: 50}
}

func TestUsageCostQueryValidatesAndNormalizesFilter(t *testing.T) {
	store := &usageCostStoreStub{}
	service := NewUsageCostQuery(store)
	filter := validUsageCostFilter()
	if _, err := service.Costs(context.Background(), admin.Identity{}, filter); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
	for _, mutate := range []func(*UsageCostFilter){
		func(value *UsageCostFilter) { value.RatingStatus = "UNKNOWN" },
		func(value *UsageCostFilter) { value.Currency = "EUR" },
		func(value *UsageCostFilter) { value.ClientProtocol = "OPENAI" },
		func(value *UsageCostFilter) { value.Sort = "amount" },
		func(value *UsageCostFilter) { value.Order = "sideways" },
		func(value *UsageCostFilter) { value.To = value.From.Add(367 * 24 * time.Hour) },
	} {
		invalid := filter
		mutate(&invalid)
		if _, err := service.Costs(context.Background(), admin.Identity{ID: 1}, invalid); !errors.Is(err, appsec.ErrInvalidArgument) {
			t.Fatalf("expected invalid argument for %+v, got %v", invalid, err)
		}
	}
	if _, err := service.Costs(context.Background(), admin.Identity{ID: 1}, filter); err != nil {
		t.Fatal(err)
	}
	if store.filter.Sort != "startedAt" || store.filter.Order != "desc" {
		t.Fatalf("defaults were not applied: %+v", store.filter)
	}
}

func TestUsageCostExportRejectsMoreThanLimit(t *testing.T) {
	store := &usageCostStoreStub{page: UsageCostPage{Items: make([]UsageCostRow, maxUsageCostExportRows+1)}}
	service := NewUsageCostQuery(store)
	if _, err := service.Export(context.Background(), admin.Identity{ID: 1}, validUsageCostFilter()); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("expected export limit error, got %v", err)
	}
	if store.filter.After != 0 || store.filter.Limit != maxUsageCostExportRows+1 {
		t.Fatalf("unexpected export filter: %+v", store.filter)
	}
}

func TestCurrentAttributionAllocatesActiveSubscriptionsAndIncludesRatedAPIKeyCost(t *testing.T) {
	now := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
	monthStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store := &usageCostStoreStub{
		prices: []SubscriptionPrice{
			{ID: 1, CredentialID: 10, Currency: "CNY", PeriodAmount: "1000", BillingPeriod: "MONTH", EffectiveAt: monthStart, CredentialEffectiveAt: monthStart},
			{ID: 2, CredentialID: 20, Currency: "CNY", PeriodAmount: "200", BillingPeriod: "MONTH", EffectiveAt: monthStart, CredentialEffectiveAt: monthStart},
			{ID: 3, CredentialID: 30, Currency: "USD", PeriodAmount: "50", BillingPeriod: "MONTH", EffectiveAt: monthStart, CredentialEffectiveAt: monthStart, CredentialEndAt: pointerTime(monthStart.Add(24 * time.Hour))},
		},
		usage: map[int64][]domain.UsageShare{
			10: {{PrincipalID: 101, Tokens: 70}, {PrincipalID: 102, Tokens: 30}},
			20: {{PrincipalID: 101, Tokens: 10}},
		},
		identities: []PrincipalIdentity{
			{ID: 101, Name: "Alpha", Type: "MEMBER"},
			{ID: 102, Name: "Beta", Type: "APPLICATION"},
		},
		apiAttribution: []PrincipalTotal{{
			PrincipalID: 101, PrincipalName: "Alpha", PrincipalType: "MEMBER",
			BillingType: "API_KEY", Currency: "USD", Amount: "0.008", Tokens: 25,
		}},
	}
	service := NewUsageCostQuery(store)
	service.now = func() time.Time { return now }
	result, err := service.CurrentAttribution(context.Background(), admin.Identity{ID: 1}, CurrentAttributionFilter{
		From: monthStart, To: monthStart.AddDate(0, 1, 0), Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantItems := []PrincipalTotal{
		{PrincipalID: 101, PrincipalName: "Alpha", PrincipalType: "MEMBER", BillingType: "SUBSCRIPTION", Currency: "CNY", Amount: "900", Tokens: 80},
		{PrincipalID: 102, PrincipalName: "Beta", PrincipalType: "APPLICATION", BillingType: "SUBSCRIPTION", Currency: "CNY", Amount: "300", Tokens: 30},
		{PrincipalID: 101, PrincipalName: "Alpha", PrincipalType: "MEMBER", BillingType: "API_KEY", Currency: "USD", Amount: "0.008", Tokens: 25},
	}
	if !reflect.DeepEqual(result.Items, wantItems) || result.Total != 3 {
		t.Fatalf("items=%+v total=%d", result.Items, result.Total)
	}
	if len(result.Totals) != 2 || result.Totals[0].BillingType != "API_KEY" ||
		result.Totals[0].TotalAmount != "0.008" || result.Totals[1].BillingType != "SUBSCRIPTION" ||
		result.Totals[1].TotalAmount != "1200" || result.Totals[1].AllocatedAmount != "1200" ||
		result.Totals[1].UnallocatedAmount != "0" {
		t.Fatalf("totals=%+v", result.Totals)
	}
}

func TestCurrentAttributionKeepsSubscriptionUnallocatedWithoutUsage(t *testing.T) {
	now := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
	monthStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	store := &usageCostStoreStub{prices: []SubscriptionPrice{{
		ID: 1, CredentialID: 10, Currency: "CNY", PeriodAmount: "1000", BillingPeriod: "MONTH",
		EffectiveAt: monthStart, CredentialEffectiveAt: monthStart,
	}}}
	service := NewUsageCostQuery(store)
	service.now = func() time.Time { return now }
	result, err := service.CurrentAttribution(context.Background(), admin.Identity{ID: 1}, CurrentAttributionFilter{
		From: monthStart, To: monthStart.AddDate(0, 1, 0), Limit: 100,
	})
	if err != nil || len(result.Items) != 0 || len(result.Totals) != 1 ||
		result.Totals[0].AllocatedAmount != "0" || result.Totals[0].UnallocatedAmount != "1000" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCurrentAttributionRequiresRangeContainingNow(t *testing.T) {
	now := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
	service := NewUsageCostQuery(&usageCostStoreStub{})
	service.now = func() time.Time { return now }
	_, err := service.CurrentAttribution(context.Background(), admin.Identity{ID: 1}, CurrentAttributionFilter{
		From: now.Add(-48 * time.Hour), To: now.Add(-24 * time.Hour), Limit: 100,
	})
	if !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func pointerTime(value time.Time) *time.Time { return &value }
