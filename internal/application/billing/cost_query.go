package billing

import (
	"context"
	"errors"
	"sort"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

const maxUsageCostExportRows = int32(100000)

var ErrExportTooLarge = errors.New("usage cost export exceeds row limit")

type UsageCostFilter struct {
	PrincipalID, GroupID, ModelID, ProviderID, ResourceID *int64
	PrincipalType, ClientProtocol, Status                 string
	RatingStatus, BillingType, Currency                   string
	Sort, Order                                           string
	From, To                                              time.Time
	After                                                 int64
	Limit                                                 int32
}

type UsageCostTotal struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
	Rated    int64  `json:"rated"`
}

type UsageCostSummary struct {
	From              time.Time        `json:"from"`
	To                time.Time        `json:"to"`
	Requests          int64            `json:"requests"`
	Attempts          int64            `json:"attempts"`
	Successful        int64            `json:"successful"`
	InputTokens       int64            `json:"inputTokens"`
	CachedInputTokens int64            `json:"cachedInputTokens"`
	OutputTokens      int64            `json:"outputTokens"`
	Rated             int64            `json:"rated"`
	Shared            int64            `json:"shared"`
	Unrated           int64            `json:"unrated"`
	Totals            []UsageCostTotal `json:"totals"`
}

type UsageCostRow struct {
	ID                int64      `json:"id,string"`
	RequestID         string     `json:"requestId"`
	AttemptNo         int64      `json:"attemptNo"`
	PrincipalID       int64      `json:"principalId,string"`
	PrincipalName     string     `json:"principalName"`
	PrincipalType     string     `json:"principalType"`
	ModelID           int64      `json:"modelId,string"`
	ModelName         string     `json:"modelName"`
	ProviderID        int64      `json:"providerId,string"`
	ProviderName      string     `json:"providerName"`
	ProviderModelID   int64      `json:"providerModelId,string"`
	ResourceID        int64      `json:"resourceId,string"`
	ResourceName      string     `json:"resourceName"`
	ClientProtocol    string     `json:"clientProtocol"`
	Status            string     `json:"status"`
	ErrorType         *string    `json:"errorType"`
	StartedAt         time.Time  `json:"startedAt"`
	CompletedAt       time.Time  `json:"completedAt"`
	LatencyMS         int64      `json:"latencyMs"`
	InputTokens       *int64     `json:"inputTokens"`
	CachedInputTokens *int64     `json:"cachedInputTokens"`
	OutputTokens      *int64     `json:"outputTokens"`
	BillingType       string     `json:"billingType"`
	RatingStatus      string     `json:"ratingStatus"`
	RatingID          *int64     `json:"ratingId,string"`
	RatingRevision    *int32     `json:"ratingRevision"`
	Currency          *string    `json:"currency"`
	TotalCost         *string    `json:"totalCost"`
	RatedAt           *time.Time `json:"ratedAt"`
}

type UsageCostPage struct {
	Items []UsageCostRow
	Total int64
}

type UsageCostRating struct {
	ID               int64     `json:"id,string"`
	Revision         int32     `json:"revision"`
	ModelPriceID     int64     `json:"modelPriceId,string"`
	PriceEffectiveAt time.Time `json:"priceEffectiveAt"`
	InputPrice       string    `json:"inputPrice"`
	CachedInputPrice string    `json:"cachedInputPrice"`
	OutputPrice      string    `json:"outputPrice"`
	InputCost        string    `json:"inputCost"`
	CachedInputCost  string    `json:"cachedInputCost"`
	OutputCost       string    `json:"outputCost"`
	TotalCost        string    `json:"totalCost"`
	Currency         string    `json:"currency"`
	CreatedAt        time.Time `json:"createdAt"`
}

type SubscriptionAllocation struct {
	PriceID         int64      `json:"priceId,string"`
	Currency        string     `json:"currency"`
	PeriodAmount    string     `json:"periodAmount"`
	BillingPeriod   string     `json:"billingPeriod"`
	EffectiveAt     time.Time  `json:"effectiveAt"`
	DocumentID      *int64     `json:"documentId,string"`
	PeriodStart     *time.Time `json:"periodStart"`
	PeriodEnd       *time.Time `json:"periodEnd"`
	PrincipalAmount *string    `json:"principalAmount"`
}

type UsageCostDetail struct {
	UsageCostRow
	Ratings      []UsageCostRating       `json:"ratings"`
	Subscription *SubscriptionAllocation `json:"subscription"`
}

type PrincipalIdentity struct {
	ID   int64
	Name string
	Type string
}

type CurrentAttributionFilter struct {
	From, To time.Time
	After    int64
	Limit    int32
}

type UsageCostQueryStore interface {
	UsageCostSummary(context.Context, admin.Identity, UsageCostFilter) (UsageCostSummary, error)
	UsageCosts(context.Context, admin.Identity, UsageCostFilter) (UsageCostPage, error)
	UsageCost(context.Context, admin.Identity, int64) (UsageCostDetail, error)
	SubscriptionPrices(context.Context) ([]SubscriptionPrice, error)
	SubscriptionUsage(context.Context, int64, time.Time, time.Time) ([]domain.UsageShare, error)
	PrincipalIdentities(context.Context, []int64) ([]PrincipalIdentity, error)
	CurrentAPIKeyAttribution(context.Context, time.Time, time.Time) ([]PrincipalTotal, error)
}

type UsageCostQueryService struct {
	store UsageCostQueryStore
	now   func() time.Time
}

func NewUsageCostQuery(store UsageCostQueryStore) *UsageCostQueryService {
	return &UsageCostQueryService{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *UsageCostQueryService) CurrentAttribution(
	ctx context.Context,
	actor admin.Identity,
	filter CurrentAttributionFilter,
) (Statistics, error) {
	if actor.ID <= 0 {
		return Statistics{}, appsec.ErrUnauthenticated
	}
	if s == nil || s.store == nil || s.now == nil {
		return Statistics{}, appsec.ErrUnavailable
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	now := s.now().UTC()
	if filter.From.IsZero() || !filter.To.After(filter.From) || filter.To.Sub(filter.From) > 366*24*time.Hour ||
		filter.After < 0 || filter.Limit < 1 || filter.Limit > 100 || now.Before(filter.From) || !now.Before(filter.To) {
		return Statistics{}, appsec.ErrInvalidArgument
	}

	prices, err := s.store.SubscriptionPrices(ctx)
	if err != nil {
		return Statistics{}, err
	}
	items := make(map[attributionKey]PrincipalTotal)
	totals := make(map[attributionTotalKey]CurrencyTotal)
	principalIDs := make(map[int64]struct{})
	for _, pricesForCredential := range groupSubscriptionPrices(prices) {
		credential := pricesForCredential[0]
		if credential.CredentialEffectiveAt.After(now) ||
			credential.CredentialEndAt != nil && !credential.CredentialEndAt.After(now) {
			continue
		}
		price := priceAt(pricesForCredential, now)
		if price == nil {
			continue
		}
		usage, usageErr := s.store.SubscriptionUsage(ctx, price.CredentialID, filter.From, now)
		if usageErr != nil {
			return Statistics{}, usageErr
		}
		totalTokens, allocations, allocationErr := domain.Allocate(price.PeriodAmount, usage)
		if allocationErr != nil {
			return Statistics{}, allocationErr
		}
		totalKey := attributionTotalKey{BillingType: "SUBSCRIPTION", Currency: price.Currency}
		total := totals[totalKey]
		total.BillingType, total.Currency = totalKey.BillingType, totalKey.Currency
		total.TotalAmount, err = addAttributionAmount(total.TotalAmount, price.PeriodAmount)
		if err != nil {
			return Statistics{}, err
		}
		total.TotalTokens += totalTokens
		if len(allocations) == 0 {
			total.UnallocatedAmount, err = addAttributionAmount(total.UnallocatedAmount, price.PeriodAmount)
		} else {
			total.AllocatedAmount, err = addAttributionAmount(total.AllocatedAmount, price.PeriodAmount)
		}
		if err != nil {
			return Statistics{}, err
		}
		totals[totalKey] = total
		for _, allocation := range allocations {
			principalIDs[allocation.PrincipalID] = struct{}{}
			key := attributionKey{
				PrincipalID: allocation.PrincipalID, BillingType: "SUBSCRIPTION", Currency: price.Currency,
			}
			item := items[key]
			item.PrincipalID, item.BillingType, item.Currency = key.PrincipalID, key.BillingType, key.Currency
			item.Amount, err = addAttributionAmount(item.Amount, allocation.Amount)
			if err != nil {
				return Statistics{}, err
			}
			item.Tokens += allocation.Tokens
			items[key] = item
		}
	}

	apiItems, err := s.store.CurrentAPIKeyAttribution(ctx, filter.From, now)
	if err != nil {
		return Statistics{}, err
	}
	for _, value := range apiItems {
		key := attributionKey{PrincipalID: value.PrincipalID, BillingType: "API_KEY", Currency: value.Currency}
		item := items[key]
		if item.PrincipalID == 0 {
			item = value
		} else {
			item.Amount, err = addAttributionAmount(item.Amount, value.Amount)
			item.Tokens += value.Tokens
			if err != nil {
				return Statistics{}, err
			}
		}
		items[key] = item
		totalKey := attributionTotalKey{BillingType: "API_KEY", Currency: value.Currency}
		total := totals[totalKey]
		total.BillingType, total.Currency = totalKey.BillingType, totalKey.Currency
		total.TotalAmount, err = addAttributionAmount(total.TotalAmount, value.Amount)
		if err != nil {
			return Statistics{}, err
		}
		total.AllocatedAmount = total.TotalAmount
		total.UnallocatedAmount = "0"
		total.TotalTokens += value.Tokens
		totals[totalKey] = total
	}

	identities := make(map[int64]PrincipalIdentity, len(principalIDs))
	if len(principalIDs) > 0 {
		ids := make([]int64, 0, len(principalIDs))
		for id := range principalIDs {
			ids = append(ids, id)
		}
		values, identityErr := s.store.PrincipalIdentities(ctx, ids)
		if identityErr != nil {
			return Statistics{}, identityErr
		}
		for _, identity := range values {
			identities[identity.ID] = identity
		}
	}

	result := Statistics{From: filter.From, To: filter.To, Totals: make([]CurrencyTotal, 0, len(totals))}
	for _, total := range totals {
		total.TotalAmount = zeroAttributionAmount(total.TotalAmount)
		total.AllocatedAmount = zeroAttributionAmount(total.AllocatedAmount)
		total.UnallocatedAmount = zeroAttributionAmount(total.UnallocatedAmount)
		result.Totals = append(result.Totals, total)
	}
	sort.Slice(result.Totals, func(i, j int) bool {
		if result.Totals[i].BillingType != result.Totals[j].BillingType {
			return result.Totals[i].BillingType < result.Totals[j].BillingType
		}
		return result.Totals[i].Currency < result.Totals[j].Currency
	})
	allItems := make([]PrincipalTotal, 0, len(items))
	for _, item := range items {
		if item.BillingType == "SUBSCRIPTION" {
			identity, found := identities[item.PrincipalID]
			if !found {
				return Statistics{}, errors.New("billing attribution principal not found")
			}
			item.PrincipalName, item.PrincipalType = identity.Name, identity.Type
		}
		allItems = append(allItems, item)
	}
	sort.Slice(allItems, func(i, j int) bool {
		if allItems[i].Tokens != allItems[j].Tokens {
			return allItems[i].Tokens > allItems[j].Tokens
		}
		if allItems[i].BillingType != allItems[j].BillingType {
			return allItems[i].BillingType < allItems[j].BillingType
		}
		if allItems[i].Currency != allItems[j].Currency {
			return allItems[i].Currency < allItems[j].Currency
		}
		return allItems[i].PrincipalID < allItems[j].PrincipalID
	})
	result.Total = int64(len(allItems))
	start := int(filter.After)
	if start > len(allItems) {
		start = len(allItems)
	}
	end := start + int(filter.Limit)
	if end > len(allItems) {
		end = len(allItems)
	}
	result.Items = allItems[start:end]
	return result, nil
}

type attributionKey struct {
	PrincipalID int64
	BillingType string
	Currency    string
}

type attributionTotalKey struct {
	BillingType string
	Currency    string
}

func addAttributionAmount(left, right string) (string, error) {
	return domain.Add(zeroAttributionAmount(left), zeroAttributionAmount(right))
}

func zeroAttributionAmount(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func (s *UsageCostQueryService) Summary(ctx context.Context, actor admin.Identity, filter UsageCostFilter) (UsageCostSummary, error) {
	if err := validateUsageCostQuery(actor, &filter, false); err != nil {
		return UsageCostSummary{}, err
	}
	if s == nil || s.store == nil {
		return UsageCostSummary{}, appsec.ErrUnavailable
	}
	return s.store.UsageCostSummary(ctx, actor, filter)
}

func (s *UsageCostQueryService) Costs(ctx context.Context, actor admin.Identity, filter UsageCostFilter) (UsageCostPage, error) {
	if err := validateUsageCostQuery(actor, &filter, true); err != nil {
		return UsageCostPage{}, err
	}
	if s == nil || s.store == nil {
		return UsageCostPage{}, appsec.ErrUnavailable
	}
	return s.store.UsageCosts(ctx, actor, filter)
}

func (s *UsageCostQueryService) Cost(ctx context.Context, actor admin.Identity, id int64) (UsageCostDetail, error) {
	if actor.ID <= 0 {
		return UsageCostDetail{}, appsec.ErrUnauthenticated
	}
	if id <= 0 {
		return UsageCostDetail{}, appsec.ErrInvalidArgument
	}
	if s == nil || s.store == nil {
		return UsageCostDetail{}, appsec.ErrUnavailable
	}
	return s.store.UsageCost(ctx, actor, id)
}

func (s *UsageCostQueryService) Export(ctx context.Context, actor admin.Identity, filter UsageCostFilter) ([]UsageCostRow, error) {
	filter.After, filter.Limit = 0, maxUsageCostExportRows+1
	if err := validateUsageCostQuery(actor, &filter, true); err != nil {
		return nil, err
	}
	if s == nil || s.store == nil {
		return nil, appsec.ErrUnavailable
	}
	page, err := s.store.UsageCosts(ctx, actor, filter)
	if err != nil {
		return nil, err
	}
	if len(page.Items) > int(maxUsageCostExportRows) {
		return nil, ErrExportTooLarge
	}
	return page.Items, nil
}

func validateUsageCostQuery(actor admin.Identity, filter *UsageCostFilter, paged bool) error {
	if actor.ID <= 0 {
		return appsec.ErrUnauthenticated
	}
	if filter.From.IsZero() || !filter.To.After(filter.From) || filter.To.Sub(filter.From) > 366*24*time.Hour || filter.After < 0 {
		return appsec.ErrInvalidArgument
	}
	if paged && (filter.Limit < 1 || filter.Limit > maxUsageCostExportRows+1) {
		return appsec.ErrInvalidArgument
	}
	for _, id := range []*int64{filter.PrincipalID, filter.GroupID, filter.ModelID, filter.ProviderID, filter.ResourceID} {
		if id != nil && *id <= 0 {
			return appsec.ErrInvalidArgument
		}
	}
	if !oneOf(filter.PrincipalType, "", "MEMBER", "APPLICATION") ||
		!oneOf(filter.ClientProtocol, "", "OPENAI_CHAT", "OPENAI_RESPONSES", "OPENAI_IMAGES", "ANTHROPIC_MESSAGES") ||
		!oneOf(filter.Status, "", "SUCCESS", "FAILED", "CANCELLED") ||
		!oneOf(filter.RatingStatus, "", "RATED", "SUBSCRIPTION_SHARED", "INCOMPLETE_TOKENS", "MISSING_PRICE", "PENDING_RATING", "NOT_BILLABLE") ||
		!oneOf(filter.BillingType, "", "API_KEY", "SUBSCRIPTION") ||
		!oneOf(filter.Currency, "", "CNY", "USD") {
		return appsec.ErrInvalidArgument
	}
	if filter.Sort == "" {
		filter.Sort = "startedAt"
	}
	if filter.Order == "" {
		filter.Order = "desc"
	}
	if !oneOf(filter.Sort, "startedAt", "totalCost", "tokens", "latency") || !oneOf(filter.Order, "asc", "desc") {
		return appsec.ErrInvalidArgument
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	return nil
}
