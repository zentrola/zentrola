package usage

import "time"

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
	QuotaGroupIDs          []int64
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
	// CacheCreationInputTokens 为缓存写入 Token 数，包含在 InputTokens 中；
	// nil 表示上游未返回或协议不区分缓存写入。
	CacheCreationInputTokens *int64
}
