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
	ErrConflict   = errors.New("conflict")
	ErrCredential = errors.New("credential unrecoverable")
	ErrProvider   = errors.New("provider unavailable")
)

type Page struct {
	After int64
	Limit int32
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
	ID               int64     `json:"id,string"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	InputModalities  []string  `json:"inputModalities"`
	OutputModalities []string  `json:"outputModalities"`
	Remark           string    `json:"remark"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type ModelInput struct {
	Code             string   `json:"code" binding:"required"`
	Name             string   `json:"name" binding:"required"`
	InputModalities  []string `json:"inputModalities" binding:"required" enums:"TEXT,IMAGE,AUDIO,VIDEO"`
	OutputModalities []string `json:"outputModalities" binding:"required" enums:"TEXT,IMAGE,AUDIO,VIDEO"`
	Remark           string   `json:"remark"`
}
type Provider struct {
	ID                 int64                    `json:"id,string"`
	Code               string                   `json:"code"`
	Name               string                   `json:"name"`
	Type               string                   `json:"type"`
	Website            *string                  `json:"website"`
	Endpoints          []ProviderEndpoint       `json:"endpoints"`
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
	ProtocolType string `json:"protocolType" enums:"OPENAI_CHAT,OPENAI_RESPONSES,ANTHROPIC_MESSAGES"`
	BaseURL      string `json:"baseUrl"`
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
	UpstreamModelCode string `json:"upstreamModelCode" binding:"required"`
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
type Resource struct {
	ID                   int64     `json:"id,string"`
	ProviderID           int64     `json:"providerId,string"`
	Name                 string    `json:"name"`
	Status               string    `json:"status"`
	CredentialConfigured bool      `json:"credentialConfigured"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}
type ResourceRecord struct {
	Resource
	Sealed catalog.SealedCredential `json:"-"`
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
	Member(context.Context, int64) (Member, error)
	Groups(context.Context, Page, string) ([]Group, error)
	Group(context.Context, int64) (Group, error)
	MemberGroups(context.Context, int64, Page) ([]Group, error)
	GroupMembers(context.Context, int64, Page) ([]Member, error)
	GroupModels(context.Context, int64, Page) ([]Model, error)
	Models(context.Context, Page, string) ([]Model, error)
	Model(context.Context, int64) (Model, error)
	Providers(context.Context, Page) ([]Provider, error)
	Provider(context.Context, int64) (Provider, error)
	ProviderMappings(context.Context, int64) ([]ProviderMapping, error)
	Resources(context.Context, Page) ([]Resource, error)
	Resource(context.Context, int64) (ResourceRecord, error)
	Keys(context.Context, int64, Page) ([]Key, error)
	Operations(context.Context, Page) ([]Operation, error)
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
	CreateProvider(context.Context, Provider) error
	UpdateProvider(context.Context, Provider) error
	DeleteProvider(context.Context, int64, time.Time) error
	SetProviderStatus(context.Context, int64, string) error
	CreateProviderMapping(context.Context, ProviderMapping) error
	UpdateProviderMapping(context.Context, ProviderMapping) error
	DeleteProviderMapping(context.Context, int64, int64, time.Time) error
	CreateResource(context.Context, ResourceRecord) error
	UpdateResource(context.Context, ResourceRecord) error
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
	OK         bool   `json:"ok"`
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	LatencyMS  int64  `json:"latencyMs"`
}
type ConnectionTester interface {
	Test(context.Context, string, string, []byte, *catalog.OutboundProxy) ConnectionResult
}
