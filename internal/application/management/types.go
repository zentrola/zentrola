// Package management 编排 P0-MVP 的管理用例，数据库记录和密文不直接作为响应。
package management

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

var (
	ErrConflict                     = errors.New("conflict")
	ErrCredential                   = errors.New("credential unrecoverable")
	ErrProvider                     = errors.New("provider unavailable")
	ErrProviderCredentialRequired   = errors.New("provider credential required")
	ErrProviderModelMappingRequired = errors.New("provider model mapping required")
	ErrModelSyncCredentialRequired  = errors.New("model sync credential required")
	ErrCredentialExportUnsupported  = errors.New("credential export unsupported")
	ErrResetCreditUnsupported       = errors.New("rate-limit reset credit unsupported")
)

const (
	AuthTypeAPIKey         = "API_KEY"
	AuthTypeSubscription   = "SUBSCRIPTION"
	AuthAdapterAPIKey      = "API_KEY"
	AuthAdapterOpenAICodex = "OPENAI_CODEX"
	AuthAdapterClaudeCode  = "ANTHROPIC_CLAUDE_CODE"
	SubscriptionPersonal   = "PERSONAL"
	QuotaAvailable         = "AVAILABLE"
	QuotaNearLimit         = "NEAR_LIMIT"
	QuotaExhausted         = "EXHAUSTED"
	QuotaUnknown           = "UNKNOWN"
)

type Page struct {
	After     int64
	Limit     int32
	ProbeNext bool
}
type PageData[T any] struct {
	Items []T
	Total int64
}
type Member struct {
	ID        int64     `json:"id,string"`
	Name      string    `json:"name"`
	Remark    *string   `json:"remark"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
type Group struct {
	ID        int64     `json:"id,string"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Remark    *string   `json:"remark"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
type Model struct {
	ID                    int64     `json:"id,string"`
	Code                  string    `json:"code"`
	Name                  string    `json:"name"`
	Status                string    `json:"status"`
	InputModalities       []string  `json:"inputModalities"`
	OutputModalities      []string  `json:"outputModalities"`
	Remark                string    `json:"remark"`
	PublisherProviderID   *int64    `json:"publisherProviderId,string"`
	PublisherProviderName *string   `json:"publisherProviderName"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type ModelInput struct {
	Code                string   `json:"code" binding:"required"`
	Name                string   `json:"name" binding:"required"`
	PublisherProviderID *int64   `json:"publisherProviderId,string"`
	InputModalities     []string `json:"inputModalities" binding:"required" enums:"TEXT,IMAGE,AUDIO,VIDEO"`
	OutputModalities    []string `json:"outputModalities" binding:"required" enums:"TEXT,IMAGE,AUDIO,VIDEO"`
	Remark              string   `json:"remark"`
}
type Provider struct {
	ID                 int64                    `json:"id,string"`
	Code               string                   `json:"code"`
	Name               string                   `json:"name"`
	Type               string                   `json:"type"`
	ModelCount         int64                    `json:"modelCount"`
	Website            *string                  `json:"website"`
	Endpoints          []ProviderEndpoint       `json:"endpoints"`
	ModelSyncSupported bool                     `json:"modelSyncSupported"`
	AuthAdapters       []string                 `json:"authAdapters"`
	ProxyEnabled       bool                     `json:"proxyEnabled"`
	ProxyURL           *string                  `json:"proxyUrl"`
	ProxyHeaders       []ProviderProxyHeader    `json:"proxyHeaders"`
	Status             string                   `json:"status"`
	CreatedAt          time.Time                `json:"createdAt"`
	UpdatedAt          time.Time                `json:"updatedAt"`
	ProxyURLSealed     catalog.SealedCredential `json:"-"`
	ProxyHeadersSealed catalog.SealedCredential `json:"-"`
}
type ProviderEndpoint struct {
	ProtocolType string `json:"protocolType" binding:"required" enums:"OPENAI,ANTHROPIC"`
	BaseURL      string `json:"baseUrl" binding:"required"`
}
type ProviderProxyHeader struct {
	Key        string `json:"key"`
	Configured bool   `json:"configured"`
}
type ProviderProxyHeaderInput struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}
type ProviderInput struct {
	Name         string                     `json:"name" binding:"required"`
	Website      string                     `json:"website"`
	Endpoints    []ProviderEndpoint         `json:"endpoints" binding:"required"`
	ProxyEnabled bool                       `json:"proxyEnabled"`
	ProxyURL     string                     `json:"proxyUrl"`
	ProxyHeaders []ProviderProxyHeaderInput `json:"proxyHeaders"`
	Mappings     []ProviderMappingInput     `json:"mappings" binding:"required"`
}
type ProviderMappingInput struct {
	ModelID           int64  `json:"modelId,string" binding:"required"`
	UpstreamModelCode string `json:"upstreamModelCode"`
	Priority          int32  `json:"priority,omitempty"`
}
type ProviderMapping struct {
	ID                int64     `json:"id,string"`
	ProviderID        int64     `json:"providerId,string"`
	ModelID           int64     `json:"modelId,string"`
	UpstreamModelCode string    `json:"upstreamModelCode"`
	Priority          int32     `json:"priority"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
type ProviderDetail struct {
	Provider
	Mappings []ProviderMapping `json:"mappings"`
}
type ProviderInitializeInput struct {
	Locale string `json:"locale" binding:"required" enums:"zh-CN,en-US"`
}
type ProviderInitializeResult struct {
	Total    int `json:"total"`
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Existing int `json:"existing"`
}
type Resource struct {
	ID                    int64      `json:"id,string"`
	ProviderID            int64      `json:"providerId,string"`
	Name                  string     `json:"name"`
	AuthType              string     `json:"authType"`
	AuthAdapter           string     `json:"authAdapter"`
	SubscriptionType      *string    `json:"subscriptionType"`
	PlanCode              *string    `json:"planCode"`
	ExternalAccountRef    *string    `json:"externalAccountRef"`
	Priority              int32      `json:"priority"`
	EffectiveAt           *time.Time `json:"effectiveAt"`
	ExpiresAt             *time.Time `json:"expiresAt"`
	QuotaStatus           string     `json:"quotaStatus"`
	QuotaCheckedAt        *time.Time `json:"quotaCheckedAt"`
	QuotaResetsAt         *time.Time `json:"quotaResetsAt"`
	CredentialRefreshedAt *time.Time `json:"-"`
	CredentialExpiresAt   *time.Time `json:"-"`
	RuntimeStatus         string     `json:"runtimeStatus"`
	BlockedReason         *string    `json:"blockedReason"`
	BlockedAt             *time.Time `json:"blockedAt"`
	LastErrorAt           *time.Time `json:"lastErrorAt"`
	LastHTTPStatus        *int32     `json:"lastHttpStatus"`
	LastErrorCode         *string    `json:"lastErrorCode"`
	CredentialConfigured  bool       `json:"credentialConfigured"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}
type ResourceQuota struct {
	Code                  string     `json:"code"`
	Name                  *string    `json:"name"`
	Status                string     `json:"status"`
	Unit                  *string    `json:"unit"`
	LimitValue            *string    `json:"limitValue"`
	UsedValue             *string    `json:"usedValue"`
	RemainingValue        *string    `json:"remainingValue"`
	UsedPercent           *float64   `json:"usedPercent"`
	WindowDurationSeconds *int64     `json:"windowDurationSeconds"`
	ResetsAt              *time.Time `json:"resetsAt"`
	ReachedType           *string    `json:"reachedType"`
	ObservedAt            time.Time  `json:"observedAt"`
}
type ResourceRecord struct {
	Resource
	Sealed catalog.SealedCredential `json:"-"`
}

type CreateResourceInput struct {
	ProviderID                              int64
	Name, Credential, AuthType, AuthAdapter string
	Priority                                int32
	EffectiveAt, ExpiresAt                  *time.Time
}

type SubscriptionInspection struct {
	AccountRef            string
	PlanCode              string
	ExpiresAt             *time.Time
	CredentialRefreshedAt *time.Time
	CredentialExpiresAt   *time.Time
}

type SubscriptionProbe struct {
	Inspection   SubscriptionInspection
	Credential   []byte
	Quotas       []ResourceQuota
	ResetCredits *RateLimitResetCredits
}

type RateLimitResetCredit struct {
	ID          string     `json:"id"`
	ResetType   string     `json:"resetType"`
	Status      string     `json:"status"`
	GrantedAt   time.Time  `json:"grantedAt"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
}

type RateLimitResetCredits struct {
	AvailableCount int                    `json:"availableCount"`
	Credits        []RateLimitResetCredit `json:"credits"`
}

type ResetCreditConsume struct {
	Outcome string
	Probe   SubscriptionProbe
}

type SubscriptionResetCreditConsumer interface {
	ConsumeResetCredit(context.Context, []byte, *catalog.OutboundProxy, string, string) (ResetCreditConsume, error)
}

// SubscriptionConnectionError 允许订阅适配器返回可安全展示的连接错误分类，
// 具体底层错误仍只保留在服务端。
type SubscriptionConnectionError interface {
	error
	ConnectionCode() string
}

type SubscriptionAdapter interface {
	Code() string
	Supports(string) bool
	SupportsProvider(Provider) bool
	Inspect([]byte) (SubscriptionInspection, error)
	Probe(context.Context, []byte, *catalog.OutboundProxy) (SubscriptionProbe, error)
}
type SubscriptionCredentialExporter interface {
	ExportCredential([]byte) ([]byte, error)
}
type Key struct {
	ID        int64      `json:"id,string"`
	Name      string     `json:"name"`
	MaskedKey string     `json:"maskedKey"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt"`
	CreatedAt time.Time  `json:"createdAt"`
}
type Operation struct {
	ID           int64           `json:"id,string"`
	OperatorName string          `json:"operatorName"`
	Type         string          `json:"type"`
	TargetType   string          `json:"targetType"`
	TargetID     *int64          `json:"targetId,string"`
	RequestID    *string         `json:"requestId"`
	Result       string          `json:"result"`
	ErrorCode    *string         `json:"errorCode"`
	Before       json.RawMessage `json:"before"`
	After        json.RawMessage `json:"after"`
	CreatedAt    time.Time       `json:"createdAt"`
}
type Audit struct {
	Event         operation.Type
	Target        string
	ID            int64
	Name          string
	Before, After any
	ErrorCode     string
}
type Reader interface {
	Members(context.Context, Page) ([]Member, error)
	CountMembers(context.Context) (int64, error)
	MemberSuggestions(context.Context, Page, string) ([]Member, error)
	CountMemberSuggestions(context.Context, string) (int64, error)
	Member(context.Context, int64) (Member, error)
	Groups(context.Context, Page, string) ([]Group, error)
	CountGroups(context.Context, string) (int64, error)
	Group(context.Context, int64) (Group, error)
	MemberGroups(context.Context, int64, Page) ([]Group, error)
	CountMemberGroups(context.Context, int64) (int64, error)
	GroupMembers(context.Context, int64, Page) ([]Member, error)
	CountGroupMembers(context.Context, int64) (int64, error)
	GroupModels(context.Context, int64, Page) ([]Model, error)
	CountGroupModels(context.Context, int64) (int64, error)
	Models(context.Context, Page, string) ([]Model, error)
	CountModels(context.Context, string) (int64, error)
	Model(context.Context, int64) (Model, error)
	Providers(context.Context, Page, string) ([]Provider, error)
	CountProviders(context.Context, string) (int64, error)
	Provider(context.Context, int64) (Provider, error)
	ProviderMappings(context.Context, int64) ([]ProviderMapping, error)
	ProviderCredentialConfigured(context.Context, int64) (bool, error)
	Resources(context.Context, Page) ([]Resource, error)
	CountResources(context.Context) (int64, error)
	Resource(context.Context, int64) (ResourceRecord, error)
	ResourceQuotas(context.Context, int64) ([]ResourceQuota, error)
	Keys(context.Context, int64, Page) ([]Key, error)
	CountKeys(context.Context, int64) (int64, error)
	Operations(context.Context, Page) ([]Operation, error)
	CountOperations(context.Context) (int64, error)
}
type Writer interface {
	Reader
	CreateMember(context.Context, Member) error
	UpdateMember(context.Context, Member) error
	SetMemberStatus(context.Context, int64, string) error
	DeleteMember(context.Context, int64) error
	CreateGroup(context.Context, Group) error
	UpdateGroup(context.Context, Group) error
	SetGroupStatus(context.Context, int64, string) error
	DeleteGroup(context.Context, int64) error
	SetGroupMember(context.Context, int64, int64, bool) (bool, error)
	SetGroupModel(context.Context, int64, int64, bool) (bool, error)
	SetModelStatus(context.Context, int64, string) error
	CreateModel(context.Context, Model) error
	UpdateModel(context.Context, Model) error
	DeleteModel(context.Context, int64, time.Time) error
	CreateProvider(context.Context, Provider) error
	UpdateProvider(context.Context, Provider) error
	DeleteProvider(context.Context, int64, time.Time) error
	SetProviderStatus(context.Context, int64, string) error
	CreateProviderMapping(context.Context, ProviderMapping) error
	UpdateProviderMapping(context.Context, ProviderMapping) error
	DeleteProviderMapping(context.Context, int64, int64, time.Time) error
	CreateResource(context.Context, ResourceRecord) error
	UpdateResource(context.Context, ResourceRecord) error
	BlockResourceRuntime(context.Context, int64, string, string, *int32, time.Time) error
	RestoreResourceRuntime(context.Context, int64, time.Time) error
	DeleteResource(context.Context, int64, time.Time) (bool, error)
	ReplaceResourceQuotas(context.Context, int64, []ResourceQuota) error
	Audit(context.Context, Audit, appsec.RequestMeta) error
}
type Store interface {
	Read(context.Context, admin.Identity, func(Reader) error) error
	Write(context.Context, admin.Identity, func(Writer) error) error
}
type Cipher interface {
	Encrypt([]byte, catalog.CredentialOwner) (catalog.SealedCredential, error)
	Decrypt(catalog.SealedCredential, catalog.CredentialOwner) ([]byte, error)
	EncryptProviderProxy([]byte, catalog.ProviderProxyOwner) (catalog.SealedCredential, error)
	DecryptProviderProxy(catalog.SealedCredential, catalog.ProviderProxyOwner) ([]byte, error)
}
type ConnectionResult struct {
	OK                     bool                   `json:"ok"`
	Code                   string                 `json:"code"`
	HTTPStatus             int                    `json:"httpStatus,omitempty"`
	LatencyMS              int64                  `json:"latencyMs"`
	ProviderModelMappingID int64                  `json:"providerModelMappingId,string,omitempty"`
	TestedModelID          int64                  `json:"testedModelId,string,omitempty"`
	TestedModelCode        string                 `json:"testedModelCode,omitempty"`
	ResetCredits           *RateLimitResetCredits `json:"resetCredits,omitempty"`
}

type ResetCreditConsumeResult struct {
	Outcome      string                 `json:"outcome"`
	ResetCredits *RateLimitResetCredits `json:"resetCredits,omitempty"`
}

type ConnectionTarget struct {
	ProviderCode      string
	Protocol          string
	BaseURL           string
	UpstreamModelCode string
	AuthType          string
	AuthAdapter       string
}

type ConnectionTester interface {
	Test(context.Context, ConnectionTarget, []byte, *catalog.OutboundProxy) ConnectionResult
}

type DiscoveredModel struct {
	Code             string
	Name             string
	InputModalities  []string
	OutputModalities []string
}

type ModelDiscoverySource struct {
	ProviderCode string
	Endpoints    []ProviderEndpoint
}

type ModelSyncResult struct {
	ConnectionResult
	Discovered int    `json:"discovered"`
	Created    int    `json:"created"`
	Updated    int    `json:"updated"`
	Mapped     int    `json:"mapped"`
	Source     string `json:"source"`
}

type ModelDiscoverer interface {
	Supports(providerCode string) bool
	Discover(context.Context, ModelDiscoverySource, []byte, *catalog.OutboundProxy) ([]DiscoveredModel, ConnectionResult)
}
