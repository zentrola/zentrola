package usage

import (
	"context"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type Filter struct {
	PrincipalID, ModelID, ResourceID *int64
	From, To                         time.Time
	After                            int64
	Limit                            int32
}
type Row struct {
	ClientProtocol    string    `json:"clientProtocol"`
	ID                int64     `json:"id,string"`
	RequestID         string    `json:"requestId"`
	PrincipalID       int64     `json:"principalId,string"`
	ModelID           *int64    `json:"modelId,string"`
	RequestAt         time.Time `json:"requestAt"`
	CompletedAt       time.Time `json:"completedAt"`
	LatencyMS         int64     `json:"latencyMs"`
	Status            string    `json:"status"`
	ErrorType         *string   `json:"errorType"`
	UsageID           *int64    `json:"usageId,string"`
	AttemptNo         *int64    `json:"attemptNo"`
	ProviderID        *int64    `json:"providerId,string"`
	ProviderModelID   *int64    `json:"providerModelId,string"`
	ResourceID        *int64    `json:"resourceId,string"`
	InputTokens       *int64    `json:"inputTokens"`
	OutputTokens      *int64    `json:"outputTokens"`
	CachedInputTokens *int64    `json:"cachedInputTokens"`
	AttemptStatus     *string   `json:"attemptStatus"`
	AttemptErrorType  *string   `json:"attemptErrorType"`
}
type QueryStore interface {
	Query(context.Context, admin.Identity, Filter) ([]Row, error)
}
type QueryService struct{ store QueryStore }

func NewQuery(store QueryStore) *QueryService { return &QueryService{store} }
func (s *QueryService) Query(ctx context.Context, a admin.Identity, f Filter) ([]Row, error) {
	if a.ID <= 0 || a.OrganizationID <= 0 {
		return nil, appsec.ErrUnauthenticated
	}
	if f.After < 0 || f.Limit < 1 || f.Limit > 100 || f.From.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return nil, appsec.ErrInvalidArgument
	}
	for _, id := range []*int64{f.PrincipalID, f.ModelID, f.ResourceID} {
		if id != nil && *id <= 0 {
			return nil, appsec.ErrInvalidArgument
		}
	}
	return s.store.Query(ctx, a, f)
}
