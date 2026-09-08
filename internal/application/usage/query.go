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
	ModelID           int64     `json:"modelId,string"`
	RequestAt         time.Time `json:"requestAt"`
	CompletedAt       time.Time `json:"completedAt"`
	LatencyMS         int64     `json:"latencyMs"`
	Status            string    `json:"status"`
	ErrorType         *string   `json:"errorType"`
	AttemptNo         int64     `json:"attemptNo"`
	ProviderID        int64     `json:"providerId,string"`
	ProviderModelID   int64     `json:"providerModelId,string"`
	ResourceID        int64     `json:"resourceId,string"`
	InputTokens       *int64    `json:"inputTokens"`
	OutputTokens      *int64    `json:"outputTokens"`
	CachedInputTokens *int64    `json:"cachedInputTokens"`
}
type TokenRank struct {
	PrincipalID int64  `json:"principalId,string"`
	Name        string `json:"name"`
	Tokens      int64  `json:"tokens"`
}
type ModelRank struct {
	ModelID  int64  `json:"modelId,string"`
	Name     string `json:"name"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}
type Dashboard struct {
	ActiveMemberCount int64       `json:"activeMemberCount"`
	ModelCount        int64       `json:"modelCount"`
	ProviderCount     int64       `json:"providerCount"`
	TotalTokens       int64       `json:"totalTokens"`
	TokenRanking      []TokenRank `json:"tokenRanking"`
	ModelRanking      []ModelRank `json:"modelRanking"`
}
type QueryStore interface {
	Query(context.Context, admin.Identity, Filter) ([]Row, error)
	Dashboard(context.Context, admin.Identity, time.Time, time.Time) (Dashboard, error)
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

func (s *QueryService) Dashboard(ctx context.Context, a admin.Identity, from, to time.Time) (Dashboard, error) {
	if a.ID <= 0 || a.OrganizationID <= 0 {
		return Dashboard{}, appsec.ErrUnauthenticated
	}
	if from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return Dashboard{}, appsec.ErrInvalidArgument
	}
	return s.store.Dashboard(ctx, a, from, to)
}
