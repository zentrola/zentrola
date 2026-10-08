package usage

import (
	"time"

	"github.com/zentrola/zentrola/internal/domain/quota"
)

// Event 不持有 Prompt、响应文本、Credential 或数据库连接。
type Event struct {
	RequestID              string
	TraceID, SpanID        string
	ClientProtocol         string
	PrincipalID            int64
	ModelID                int64
	RequestAt, CompletedAt time.Time
	Status                 Status
	ErrorType              string
	QuotaScopes            []quota.Scope
	Attempt                *Attempt
	Attempts               []Attempt
}

type Attempt struct {
	ID                                               int64
	AttemptNo                                        int32
	ProviderID, ProviderModelID, ResourceID, ModelID int64
	StartedAt, CompletedAt                           time.Time
	Status                                           Status
	ErrorType                                        string
	InputTokens, OutputTokens, CachedInputTokens     *int64
}
