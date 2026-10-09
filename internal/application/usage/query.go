package usage

import (
	"context"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type Filter struct {
	PrincipalID, ModelID, ProviderID, ResourceID *int64
	PrincipalType                                string
	From, To                                     time.Time
	After                                        int64
	Limit                                        int32
	ProbeNext                                    bool
}
type Row struct {
	ClientProtocol    string    `json:"clientProtocol"`
	ID                int64     `json:"id,string"`
	RequestID         string    `json:"requestId"`
	PrincipalID       int64     `json:"principalId,string"`
	PrincipalName     string    `json:"principalName"`
	PrincipalType     string    `json:"principalType"`
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
type Page struct {
	Items []Row
	Total int64
}
type StatisticDimension string

const (
	StatisticMember      StatisticDimension = "member"
	StatisticApplication StatisticDimension = "application"
	StatisticModel       StatisticDimension = "model"
	StatisticProvider    StatisticDimension = "provider"
)

type StatisticFilter struct {
	Dimension StatisticDimension
	From, To  time.Time
	After     int64
	Limit     int32
	ProbeNext bool
}
type StatisticRow struct {
	EntityID          int64  `json:"entityId,string"`
	Name              string `json:"name"`
	Code              string `json:"code"`
	Count             int64  `json:"count"`
	Successful        int64  `json:"successful"`
	InputTokens       int64  `json:"inputTokens"`
	OutputTokens      int64  `json:"outputTokens"`
	CachedInputTokens int64  `json:"cachedInputTokens"`
	Tokens            int64  `json:"tokens"`
	OverallTokens     int64  `json:"overallTokens"`
	AverageLatencyMS  int64  `json:"averageLatencyMs"`
}
type StatisticPage struct {
	Items []StatisticRow
	Total int64
}
type TokenRank struct {
	PrincipalID int64  `json:"principalId,string"`
	Name        string `json:"name"`
	Tokens      int64  `json:"tokens"`
}
type ClientModelRank struct {
	ModelID   int64  `json:"modelId,string"`
	ModelCode string `json:"modelCode"`
	ModelName string `json:"modelName"`
	Requests  int64  `json:"requests"`
	Tokens    int64  `json:"tokens"`
}
type ProviderRank struct {
	ProviderID   int64  `json:"providerId,string"`
	ProviderName string `json:"providerName"`
	Calls        int64  `json:"calls"`
	Tokens       int64  `json:"tokens"`
}
type Dashboard struct {
	ActiveMemberCount  int64             `json:"activeMemberCount"`
	ModelCount         int64             `json:"modelCount"`
	ProviderCount      int64             `json:"providerCount"`
	TotalTokens        int64             `json:"totalTokens"`
	TokenRanking       []TokenRank       `json:"tokenRanking"`
	ClientModelRanking []ClientModelRank `json:"clientModelRanking"`
	ProviderRanking    []ProviderRank    `json:"providerRanking"`
}
type QueryStore interface {
	Query(context.Context, admin.Identity, Filter) (Page, error)
	Statistics(context.Context, admin.Identity, StatisticFilter) (StatisticPage, error)
	Dashboard(context.Context, admin.Identity, time.Time, time.Time) (Dashboard, error)
	TokenUsage(context.Context, int64, time.Time, time.Time) (int64, error)
}
type QueryService struct{ store QueryStore }

func NewQuery(store QueryStore) *QueryService { return &QueryService{store} }
func (s *QueryService) Query(ctx context.Context, a admin.Identity, f Filter) (Page, error) {
	if a.ID <= 0 {
		return Page{}, appsec.ErrUnauthenticated
	}
	if f.After < 0 || f.Limit < 1 || f.Limit > 100 || f.From.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return Page{}, appsec.ErrInvalidArgument
	}
	if f.PrincipalType != "" && f.PrincipalType != "MEMBER" && f.PrincipalType != "APPLICATION" {
		return Page{}, appsec.ErrInvalidArgument
	}
	for _, id := range []*int64{f.PrincipalID, f.ModelID, f.ProviderID, f.ResourceID} {
		if id != nil && *id <= 0 {
			return Page{}, appsec.ErrInvalidArgument
		}
	}
	f.From, f.To = f.From.UTC(), f.To.UTC()
	return s.store.Query(ctx, a, f)
}

func (s *QueryService) Statistics(ctx context.Context, a admin.Identity, f StatisticFilter) (StatisticPage, error) {
	if a.ID <= 0 {
		return StatisticPage{}, appsec.ErrUnauthenticated
	}
	if (f.Dimension != StatisticMember && f.Dimension != StatisticApplication && f.Dimension != StatisticModel && f.Dimension != StatisticProvider) ||
		f.After < 0 || f.Limit < 1 || f.Limit > 100 || f.From.IsZero() || !f.To.After(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return StatisticPage{}, appsec.ErrInvalidArgument
	}
	f.From, f.To = f.From.UTC(), f.To.UTC()
	return s.store.Statistics(ctx, a, f)
}

func (s *QueryService) Dashboard(ctx context.Context, a admin.Identity, from, to time.Time) (Dashboard, error) {
	if a.ID <= 0 {
		return Dashboard{}, appsec.ErrUnauthenticated
	}
	if from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return Dashboard{}, appsec.ErrInvalidArgument
	}
	from, to = from.UTC(), to.UTC()
	return s.store.Dashboard(ctx, a, from, to)
}
