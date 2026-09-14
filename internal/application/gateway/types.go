// Package gateway 编排 OpenAI 与 Anthropic Gateway 调用；不包含 HTTP 管理响应或数据库记录。
package gateway

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

type Failure struct {
	Code, Type, Message string
	Status              int
}

func (f *Failure) Error() string { return f.Code }

var (
	ErrAuthentication = &Failure{"UNAUTHENTICATED", "authentication_error", "Authentication failed. Check the API key.", 401}
	ErrInvalid        = &Failure{"INVALID_REQUEST", "invalid_request_error", "The request is invalid. Check the request path, headers, and body.", 400}
	ErrModelUnknown   = &Failure{"MODEL_NOT_FOUND", "not_found_error", "The requested model is not configured.", 404}
	ErrModelDisabled  = &Failure{"MODEL_DISABLED", "permission_error", "The requested model is disabled. Enable it before retrying.", 403}
	ErrPermission     = &Failure{"MODEL_PERMISSION_DENIED", "permission_error", "The API key is not authorized to use the requested model.", 403}
	ErrRoute          = &Failure{"MODEL_ROUTE_UNAVAILABLE", "api_error", "No active provider route is available for the requested model. Enable a provider and configure its endpoint and model mapping.", 503}
	ErrResource       = &Failure{"RESOURCE_UNAVAILABLE", "api_error", "No active provider API key is available. Configure or enable the provider API key.", 503}
	ErrCredential     = &Failure{"CREDENTIAL_UNRECOVERABLE", "api_error", "The provider API key cannot be decrypted. Reconfigure the provider API key.", 503}
	ErrSubscription   = &Failure{"SUBSCRIPTION_REFRESH_FAILED", "api_error", "The provider subscription could not be refreshed. Retry later or reconfigure the subscription.", 503}
	ErrProxy          = &Failure{"PROXY_CONFIGURATION_UNRECOVERABLE", "api_error", "The provider proxy configuration cannot be decrypted or parsed. Reconfigure the provider proxy.", 503}
	ErrUnavailable    = &Failure{"DEPENDENCY_UNAVAILABLE", "api_error", "An internal dependency is unavailable. Retry later or contact the administrator.", 503}
	ErrUpstream       = &Failure{"UPSTREAM_UNAVAILABLE", "api_error", "The upstream model provider is unavailable. Retry later.", 502}
	ErrTimeout        = &Failure{"UPSTREAM_TIMEOUT", "api_error", "The upstream model provider timed out. Retry later.", 504}
	ErrCancelled      = &Failure{"REQUEST_CANCELLED", "api_error", "The request was cancelled before completion.", 499}
)

type Route struct {
	ModelID, ProviderID, ProviderModelID, ResourceID int64
	UpstreamModel, BaseURL, EndpointProtocol         string
	AuthType, AuthAdapter, SubscriptionType          string
	ResourcePriority                                 int32
	QuotaStatus                                      string
	ExpiresAt                                        *time.Time
	CredentialRefreshedAt, CredentialExpiresAt       *time.Time
	Credential                                       catalog.SealedCredential
	ProxyEnabled                                     bool
	ProxyURL, ProxyHeaders                           catalog.SealedCredential
	Proxy                                            *catalog.OutboundProxy
}

const AnthropicProtocol = "ANTHROPIC_MESSAGES"
const OpenAIProtocol = "OPENAI_CHAT"
const OpenAIResponsesProtocol = "OPENAI_RESPONSES"
const OpenAIImagesProtocol = "OPENAI_IMAGES"

// IsOpenAIProtocol 判断客户端协议是否属于 OpenAI 协议族。
func IsOpenAIProtocol(protocol string) bool {
	return protocol == OpenAIProtocol || protocol == OpenAIResponsesProtocol || protocol == OpenAIImagesProtocol
}

const AnthropicEndpoint = "ANTHROPIC"
const OpenAIEndpoint = "OPENAI"

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
type ModelStore interface {
	Models(context.Context, appsec.PrincipalIdentity) ([]Model, error)
}
type Store interface {
	Resolve(context.Context, appsec.PrincipalIdentity, string, ...string) (Route, error)
}
type CandidateStore interface {
	ResolveCandidates(context.Context, appsec.PrincipalIdentity, string, ...string) ([]Route, error)
}
type ResourceBlock struct {
	Reason, ErrorCode string
	HTTPStatus        int32
}
type ResourceBlocker interface {
	BlockResource(context.Context, appsec.PrincipalIdentity, int64, ResourceBlock) error
}
type CredentialUpdater interface {
	UpdateResourceCredential(context.Context, Route, catalog.SealedCredential) error
}
type CredentialRefreshMetadataUpdater interface {
	UpdateResourceCredentialRefreshMetadata(context.Context, Route) error
}
type CredentialLoader interface {
	LoadResourceCredential(context.Context, Route) (catalog.SealedCredential, error)
}
type SubscriptionRefreshLocker interface {
	LockSubscriptionRefresh(context.Context, int64) (context.Context, func(), error)
}
type SubscriptionCredential struct {
	ProviderID, ResourceID int64
	AuthAdapter            string
	CredentialRefreshedAt  *time.Time
	CredentialExpiresAt    *time.Time
	Credential             catalog.SealedCredential
	ProxyEnabled           bool
	ProxyURL               catalog.SealedCredential
	ProxyHeaders           catalog.SealedCredential
}
type SubscriptionCredentialLister interface {
	ListSubscriptionCredentials(context.Context, int64, int32, time.Time) ([]SubscriptionCredential, error)
}
type SubscriptionRefresher interface {
	Supports(string) bool
	RefreshIfNeeded(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error)
}
type SubscriptionRefreshInspector interface {
	NeedsRefresh([]byte) (bool, error)
}
type SubscriptionRefreshPlanner interface {
	RefreshBefore(time.Time) time.Time
}
type SubscriptionRefreshMetadataInspector interface {
	CredentialRefreshMetadata([]byte) (refreshedAt, expiresAt *time.Time, err error)
}
type RouteState interface {
	Acquire(context.Context, Route) (bool, error)
	Cooldown(context.Context, Route, time.Duration) error
	Healthy(context.Context, Route) error
}
type Cipher interface {
	Decrypt(catalog.SealedCredential, catalog.CredentialOwner) ([]byte, error)
	DecryptProviderProxy(catalog.SealedCredential, catalog.ProviderProxyOwner) ([]byte, error)
}
type CredentialEncryptor interface {
	Encrypt([]byte, catalog.CredentialOwner) (catalog.SealedCredential, error)
}

func (s *Service) decryptProxy(route Route) (*catalog.OutboundProxy, error) {
	if !route.ProxyEnabled {
		return nil, nil
	}
	urlBytes, err := s.cipher.DecryptProviderProxy(route.ProxyURL, catalog.ProviderProxyOwner{ProviderID: route.ProviderID, Field: "url"})
	if err != nil {
		return nil, ErrProxy
	}
	proxy := &catalog.OutboundProxy{URL: string(urlBytes), Headers: map[string]string{}}
	clear(urlBytes)
	if route.ProxyHeaders.KeyVersion == 0 {
		return proxy, nil
	}
	headerBytes, err := s.cipher.DecryptProviderProxy(route.ProxyHeaders, catalog.ProviderProxyOwner{ProviderID: route.ProviderID, Field: "headers"})
	if err != nil {
		return nil, ErrProxy
	}
	err = json.Unmarshal(headerBytes, &proxy.Headers)
	clear(headerBytes)
	if err != nil {
		return nil, ErrProxy
	}
	return proxy, nil
}

type Request struct {
	Trace                          *usage.Event
	Path, Version, Beta, RequestID string
	Protocol                       string
	BetaQuery, Development         bool
	Body                           []byte
	ProtocolHeaders                map[string][]string
}
type Response struct {
	Status          int
	Headers         map[string][]string
	Body            io.ReadCloser
	RequestedStream bool
	ErrorType       string
}
type Upstream interface {
	Open(context.Context, Route, Request, []byte) (*Response, error)
}
type Service struct {
	store     Store
	cipher    Cipher
	upstream  Upstream
	state     RouteState
	refresh   []SubscriptionRefresher
	refreshMu sync.Mutex
}

type Option func(*Service)

func WithRouteState(state RouteState) Option { return func(service *Service) { service.state = state } }
func WithSubscriptionRefresher(refresh SubscriptionRefresher) Option {
	return func(service *Service) {
		if refresh != nil {
			service.refresh = append(service.refresh, refresh)
		}
	}
}

func (s *Service) subscriptionRefresher(code string) SubscriptionRefresher {
	for _, refresh := range s.refresh {
		if refresh.Supports(code) {
			return refresh
		}
	}
	return nil
}
func New(store Store, cipher Cipher, upstream Upstream, options ...Option) *Service {
	service := &Service{store: store, cipher: cipher, upstream: upstream}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Forward(ctx context.Context, identity appsec.PrincipalIdentity, request Request) (*Response, error) {
	if identity.ID <= 0 || identity.AccessKeyID <= 0 {
		return nil, ErrAuthentication
	}
	if request.Protocol == "" {
		request.Protocol = AnthropicProtocol
	}
	if (request.Protocol == AnthropicProtocol && request.Path != "/v1/messages" && request.Path != "/v1/messages/count_tokens") ||
		(request.Protocol == OpenAIProtocol && request.Path != "/v1/chat/completions") ||
		(request.Protocol == OpenAIResponsesProtocol && request.Path != "/v1/responses") ||
		(request.Protocol == OpenAIImagesProtocol && request.Path != "/v1/images/generations") ||
		(request.Protocol != AnthropicProtocol && !IsOpenAIProtocol(request.Protocol)) {
		return nil, ErrInvalid
	}
	parseRequest := Parse
	if IsOpenAIProtocol(request.Protocol) {
		parseRequest = ParseOpenAI
	}
	parsed, err := parseRequest(request.Body)
	if err != nil {
		return nil, err
	}
	if request.Path == "/v1/messages/count_tokens" && parsed.Stream {
		return nil, ErrInvalid
	}
	routes, err := s.routes(ctx, identity, parsed.Model, request.Protocol)
	if err != nil {
		return nil, err
	}
	if request.Trace != nil && len(routes) > 0 {
		request.Trace.ModelID = routes[0].ModelID
	}
	response, err := s.forwardCandidates(ctx, identity, request, parsed, routes)
	if response != nil {
		response.RequestedStream = parsed.Stream
	}
	return response, err
}

func (s *Service) Models(ctx context.Context, identity appsec.PrincipalIdentity) ([]Model, error) {
	if identity.ID <= 0 || identity.AccessKeyID <= 0 {
		return nil, ErrAuthentication
	}
	store, ok := s.store.(ModelStore)
	if !ok {
		return nil, ErrUnavailable
	}
	return store.Models(ctx, identity)
}
