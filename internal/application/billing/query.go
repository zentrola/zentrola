package billing

import (
	"context"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type DocumentFilter struct {
	BillingType, DocumentType, Currency string
	From, To                            time.Time
	After                               int64
	Limit                               int32
}

type DocumentSummary struct {
	ID                 int64     `json:"id,string"`
	BillingType        string    `json:"billingType"`
	DocumentType       string    `json:"documentType"`
	Status             string    `json:"status"`
	CredentialID       int64     `json:"credentialId,string"`
	CredentialName     string    `json:"credentialName"`
	OriginalDocumentID *int64    `json:"originalDocumentId,string"`
	PeriodStart        time.Time `json:"periodStart"`
	PeriodEnd          time.Time `json:"periodEnd"`
	TotalTokens        int64     `json:"totalTokens"`
	TotalAmount        string    `json:"totalAmount"`
	Currency           string    `json:"currency"`
	CreatedAt          time.Time `json:"createdAt"`
}

type DocumentPage struct {
	Items []DocumentSummary
	Total int64
}

type DocumentItemDetail struct {
	ID              int64   `json:"id,string"`
	PrincipalID     int64   `json:"principalId,string"`
	PrincipalName   string  `json:"principalName"`
	PrincipalType   string  `json:"principalType"`
	UsageTokens     int64   `json:"usageTokens"`
	AllocationRatio *string `json:"allocationRatio"`
	Amount          string  `json:"amount"`
}

type DocumentDetail struct {
	DocumentSummary
	Items       []DocumentItemDetail `json:"items"`
	Original    *DocumentSummary     `json:"original"`
	Adjustments []DocumentSummary    `json:"adjustments"`
	RatingCount int64                `json:"ratingCount"`
}

type UnratedUsageFilter struct {
	Reason   string
	From, To time.Time
	After    int64
	Limit    int32
}

type UnratedUsage struct {
	UsageRecordID     int64     `json:"usageRecordId,string"`
	PrincipalID       int64     `json:"principalId,string"`
	PrincipalName     string    `json:"principalName"`
	PrincipalType     string    `json:"principalType"`
	CredentialID      int64     `json:"credentialId,string"`
	CredentialName    string    `json:"credentialName"`
	ProviderModelID   int64     `json:"providerModelId,string"`
	StartedAt         time.Time `json:"startedAt"`
	InputTokens       *int64    `json:"inputTokens"`
	CachedInputTokens *int64    `json:"cachedInputTokens"`
	OutputTokens      *int64    `json:"outputTokens"`
	Reason            string    `json:"reason"`
	WaitingSince      time.Time `json:"waitingSince"`
}

type UnratedUsagePage struct {
	Items []UnratedUsage
	Total int64
}

func (s *Service) Documents(ctx context.Context, actor admin.Identity, filter DocumentFilter) (DocumentPage, error) {
	if actor.ID <= 0 {
		return DocumentPage{}, appsec.ErrUnauthenticated
	}
	if s == nil || s.store == nil {
		return DocumentPage{}, appsec.ErrUnavailable
	}
	if !validQueryWindow(filter.From, filter.To) || filter.After < 0 || filter.Limit < 1 || filter.Limit > 100 ||
		!oneOf(filter.BillingType, "", "SUBSCRIPTION", "API_KEY") ||
		!oneOf(filter.DocumentType, "", "CHARGE", "ADJUSTMENT") ||
		!oneOf(filter.Currency, "", "CNY", "USD") {
		return DocumentPage{}, appsec.ErrInvalidArgument
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	return s.store.BillingDocuments(ctx, filter)
}

func (s *Service) Document(ctx context.Context, actor admin.Identity, id int64) (DocumentDetail, error) {
	if actor.ID <= 0 {
		return DocumentDetail{}, appsec.ErrUnauthenticated
	}
	if s == nil || s.store == nil {
		return DocumentDetail{}, appsec.ErrUnavailable
	}
	if id <= 0 {
		return DocumentDetail{}, appsec.ErrInvalidArgument
	}
	return s.store.BillingDocument(ctx, id)
}

func (s *Service) Unrated(ctx context.Context, actor admin.Identity, filter UnratedUsageFilter) (UnratedUsagePage, error) {
	if actor.ID <= 0 {
		return UnratedUsagePage{}, appsec.ErrUnauthenticated
	}
	if s == nil || s.store == nil {
		return UnratedUsagePage{}, appsec.ErrUnavailable
	}
	if !validQueryWindow(filter.From, filter.To) || filter.After < 0 || filter.Limit < 1 || filter.Limit > 100 ||
		!oneOf(filter.Reason, "", "INCOMPLETE_TOKENS", "MISSING_PRICE", "PENDING_RATING") {
		return UnratedUsagePage{}, appsec.ErrInvalidArgument
	}
	filter.From, filter.To = filter.From.UTC(), filter.To.UTC()
	return s.store.UnratedUsage(ctx, filter)
}

func validQueryWindow(from, to time.Time) bool {
	return !from.IsZero() && to.After(from) && to.Sub(from) <= 366*24*time.Hour
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
