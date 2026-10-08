// Package gateway 编排 OpenAI 与 Anthropic Gateway 调用；不包含 HTTP 管理响应或数据库记录。
package gateway

import (
	"context"
	"encoding/json"
	"io"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	domainquota "github.com/zentrola/zentrola/internal/domain/quota"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

type Failure struct {
	Code, Type, Message string
	Status              int
}

func (f *Failure) Error() string { return f.Code }

// SubscriptionRefreshError 记录订阅刷新失败是否可重试，或必须等待管理员处理。
type SubscriptionRefreshError struct {
	Code      string
	Permanent bool
}

func (e *SubscriptionRefreshError) Error() string {
	if e == nil || e.Code == "" {
		return ErrSubscription.Code
	}
	return e.Code
}

func (e *SubscriptionRefreshError) Unwrap() error { return ErrSubscription }

var (
	ErrAuthentication = &Failure{"UNAUTHENTICATED", "authentication_error", "Authentication failed. Check the API key.", 401}
	ErrInvalid        = &Failure{"INVALID_REQUEST", "invalid_request_error", "The request is invalid. Check the request path, headers, and body.", 400}
	ErrModelUnknown   = &Failure{"MODEL_NOT_FOUND", "not_found_error", "The requested model is not configured.", 404}
	ErrModelDisabled  = &Failure{"MODEL_DISABLED", "permission_error", "The requested model is disabled. Enable it before retrying.", 403}
	ErrPermission     = &Failure{"MODEL_PERMISSION_DENIED", "permission_error", "The API key is not authorized to use the requested model.", 403}
	ErrRoute          = &Failure{"MODEL_ROUTE_UNAVAILABLE", "api_error", "No active provider route is available for the requested model. Enable a provider and configure its endpoint and model mapping.", 503}
	ErrRouteCooldown  = &Failure{"MODEL_ROUTE_COOLDOWN", "api_error", "All configured provider routes for the requested model are temporarily cooling down after upstream failures. Retry later.", 503}
	ErrResource       = &Failure{"RESOURCE_UNAVAILABLE", "api_error", "No active provider API key is available. Configure or enable the provider API key.", 503}
	ErrCredential     = &Failure{"CREDENTIAL_UNRECOVERABLE", "api_error", "The provider API key cannot be decrypted. Reconfigure the provider API key.", 503}
	ErrSubscription   = &Failure{"SUBSCRIPTION_REFRESH_FAILED", "api_error", "The provider subscription could not be refreshed. Retry later or reconfigure the subscription.", 503}
	ErrProxy          = &Failure{"PROXY_CONFIGURATION_UNRECOVERABLE", "api_error", "The provider proxy configuration cannot be decrypted or parsed. Reconfigure the provider proxy.", 503}
	ErrProxyAuth      = &Failure{"PROXY_AUTH_REJECTED", "api_error", "The provider proxy rejected authentication. Check its username, password, and access policy.", 502}
	ErrProxyServer    = &Failure{"PROXY_SERVER_UNAVAILABLE", "api_error", "The provider proxy server is unavailable. Check the proxy server and retry later.", 502}
	ErrUnavailable    = &Failure{"DEPENDENCY_UNAVAILABLE", "api_error", "An internal dependency is unavailable. Retry later or contact the administrator.", 503}
	ErrUpstream       = &Failure{"UPSTREAM_UNAVAILABLE", "api_error", "The upstream model provider is unavailable. Retry later.", 502}
	ErrTimeout        = &Failure{"UPSTREAM_TIMEOUT", "api_error", "The upstream model provider timed out. Retry later.", 504}
	ErrCancelled      = &Failure{"REQUEST_CANCELLED", "api_error", "The request was cancelled before completion.", 499}
	ErrBudgetExceeded = &Failure{"BUDGET_EXCEEDED", "rate_limit_error", "The monthly token quota has been reached. Ask an administrator to add quota or retry next month.", 429}
)

type Route struct {
	ModelID, ProviderID, ProviderModelID, ResourceID       int64
	ModelCode, ModelName, ProviderName                     string
	UpstreamModel, BaseURL, EndpointProtocol, NetworkScope string
	AuthType, AuthAdapter, SubscriptionType                string
	ResourcePriority                                       int32
	QuotaStatus                                            string
	ExpiresAt                                              *time.Time
	CredentialRefreshedAt, CredentialExpiresAt             *time.Time
	Credential                                             catalog.SealedCredential
	ProxyEnabled                                           bool
	ProxyURL, ProxyHeaders                                 catalog.SealedCredential
	Proxy                                                  *catalog.OutboundProxy
	QuotaScopes                                            []domainquota.Scope
	QuotaGroupIDs                                          []int64
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

type Provider struct {
	Name string `json:"name" example:"OpenAI"`
}
type ActiveModel struct {
	ModelName    string `json:"modelName"`
	ModelCode    string `json:"modelCode"`
	ProviderName string `json:"providerName"`
}
type ActiveModelReader interface {
	ActiveModels(context.Context) ([]ActiveModel, error)
}
type ActiveRouteRecorder interface {
	RecordActiveRoute(context.Context, Route) error
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
type SystemResourceBlocker interface {
	BlockResourceSystem(context.Context, int64, ResourceBlock) error
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

// SubscriptionRefreshCoordinator coordinates background subscription refreshes
// across multiple application instances. The scheduler lock only protects the
// scan; the resource lock protects one resource refresh.
type SubscriptionRefreshCoordinator interface {
	AcquireSubscriptionRefreshScheduler(context.Context) (func(), bool, error)
	AcquireSubscriptionRefreshResource(context.Context, int64) (func(), bool, error)
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
type SubscriptionRefreshErrorClassifier interface {
	ClassifyRefreshError(error) (code string, permanent bool)
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
type TokenQuotaAdmission interface {
	Check(context.Context, []domainquota.Scope, time.Time) (domainquota.Decision, error)
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
	route           *Route
}
type Upstream interface {
	Open(context.Context, Route, Request, []byte) (*Response, error)
}
type Service struct {
	store              Store
	cipher             Cipher
	upstream           Upstream
	state              RouteState
	activeRoutes       ActiveRouteRecorder
	refresh            []SubscriptionRefresher
	refreshCoordinator SubscriptionRefreshCoordinator
	refreshTimeout     time.Duration
	quotaAdmission     TokenQuotaAdmission
}

type Option func(*Service)

func WithRouteState(state RouteState) Option { return func(service *Service) { service.state = state } }
func WithActiveRouteRecorder(recorder ActiveRouteRecorder) Option {
	return func(service *Service) { service.activeRoutes = recorder }
}
func WithSubscriptionRefreshCoordinator(coordinator SubscriptionRefreshCoordinator) Option {
	return func(service *Service) { service.refreshCoordinator = coordinator }
}
func WithCredentialRefreshTimeout(timeout time.Duration) Option {
	return func(service *Service) {
		if timeout > 0 {
			service.refreshTimeout = timeout
		}
	}
}
func WithTokenQuotaAdmission(admission TokenQuotaAdmission) Option {
	return func(service *Service) { service.quotaAdmission = admission }
}
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

type subscriptionRefreshLeaseKey struct{}

func withSubscriptionRefreshLease(ctx context.Context) context.Context {
	return context.WithValue(ctx, subscriptionRefreshLeaseKey{}, true)
}

func hasSubscriptionRefreshLease(ctx context.Context) bool {
	value, _ := ctx.Value(subscriptionRefreshLeaseKey{}).(bool)
	return value
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
		if request.Path != "/v1/messages/count_tokens" && request.Protocol != OpenAIImagesProtocol {
			request.Trace.QuotaGroupIDs = append([]int64(nil), routes[0].QuotaGroupIDs...)
		}
	}
	if s.quotaAdmission != nil && len(routes) > 0 && len(routes[0].QuotaScopes) > 0 &&
		request.Path != "/v1/messages/count_tokens" && request.Protocol != OpenAIImagesProtocol {
		decision, quotaErr := s.quotaAdmission.Check(ctx, routes[0].QuotaScopes, time.Now().UTC())
		if quotaErr != nil {
			return nil, ErrUnavailable
		}
		if !decision.Allowed {
			return nil, ErrBudgetExceeded
		}
	}
	response, err := s.forwardCandidates(ctx, identity, request, parsed, routes)
	if response != nil {
		response.RequestedStream = parsed.Stream
	}
	return response, err
}

// ReportInvalidStream 将已开始传输后才发现的上游协议错误计入路由冷却。
// 此时不能安全重试，否则可能重复输出内容或执行工具。
func (s *Service) ReportInvalidStream(ctx context.Context, response *Response) {
	if s == nil || response == nil || response.route == nil {
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	s.cooldown(persistCtx, *response.route, defaultRouteCooldown)
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

// Provider 返回指定模型按当前网关路由顺序将使用的服务商。
func (s *Service) Provider(ctx context.Context, identity appsec.PrincipalIdentity, model string) (Provider, error) {
	if identity.ID <= 0 || identity.AccessKeyID <= 0 {
		return Provider{}, ErrAuthentication
	}
	if !validModel(model) {
		return Provider{}, ErrInvalid
	}
	routes, err := s.routes(ctx, identity, model, OpenAIResponsesProtocol)
	if err != nil {
		return Provider{}, err
	}
	if len(routes) == 0 || routes[0].ProviderName == "" {
		return Provider{}, ErrRoute
	}
	return Provider{Name: routes[0].ProviderName}, nil
}
