package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

const (
	defaultRouteCooldown      = time.Minute
	responseInspectionTimeout = 2 * time.Second
)

var errResponseInspectionTimeout = errors.New("upstream error response inspection timed out")

func (s *Service) routes(ctx context.Context, identity appsec.PrincipalIdentity, model, protocol string) ([]Route, error) {
	if store, ok := s.store.(CandidateStore); ok {
		return store.ResolveCandidates(ctx, identity, model, protocol)
	}
	route, err := s.store.Resolve(ctx, identity, model, protocol)
	if err != nil {
		return nil, err
	}
	return []Route{route}, nil
}

func (s *Service) acquire(ctx context.Context, route Route) bool {
	if s.state == nil {
		return true
	}
	allowed, err := s.state.Acquire(ctx, route)
	return err != nil || allowed
}

func (s *Service) nextRoute(ctx context.Context, routes []Route, start int) int {
	for index := start; index < len(routes); index++ {
		if s.acquire(ctx, routes[index]) {
			return index
		}
	}
	return -1
}

func (s *Service) forwardCandidates(ctx context.Context, identity appsec.PrincipalIdentity, request Request, parsed Parsed, routes []Route) (*Response, error) {
	originalBody := request.Body
	index := s.nextRoute(ctx, routes, 0)
	if index < 0 {
		// ResolveCandidates 已经确认存在配置完整的候选路由；此处全部被
		// RouteState 拒绝只表示短期冷却，不能误报为缺少 Provider 配置。
		return nil, ErrRouteCooldown
	}
	for index >= 0 {
		route := routes[index]
		if !validModel(route.UpstreamModel) {
			next := s.nextRoute(ctx, routes, index+1)
			if next < 0 {
				return nil, ErrRoute
			}
			index = next
			continue
		}
		credential, err := s.cipher.Decrypt(route.Credential, catalog.CredentialOwner{ProviderID: route.ProviderID, ResourceID: route.ResourceID})
		if err != nil {
			s.block(ctx, identity, route, ResourceBlock{Reason: "CREDENTIAL_UNRECOVERABLE", ErrorCode: "CREDENTIAL_UNRECOVERABLE"})
			next := s.nextRoute(ctx, routes, index+1)
			if next < 0 {
				return nil, ErrCredential
			}
			index = next
			continue
		}
		route.Proxy, err = s.decryptProxy(route)
		if err != nil {
			clear(credential)
			s.cooldown(ctx, route, defaultRouteCooldown)
			next := s.nextRoute(ctx, routes, index+1)
			if next < 0 {
				return nil, err
			}
			index = next
			continue
		}
		if route.AuthType == "SUBSCRIPTION" {
			credential, _, err = s.refreshCredential(ctx, route, credential)
			if err != nil {
				clear(credential)
				var refreshFailure *SubscriptionRefreshError
				if errors.As(err, &refreshFailure) && refreshFailure.Permanent {
					s.block(ctx, identity, route, ResourceBlock{
						Reason: subscriptionRefreshBlockReason(refreshFailure.Code), ErrorCode: refreshFailure.Code,
					})
				} else {
					s.cooldown(ctx, route, defaultRouteCooldown)
				}
				next := s.nextRoute(ctx, routes, index+1)
				if next < 0 {
					return nil, err
				}
				index = next
				continue
			}
		}
		request.Body = parsed.Rewrite(originalBody, route.UpstreamModel)
		attempt := s.startAttempt(request.Trace, route)
		response, openErr := s.upstream.Open(ctx, route, request, credential)
		clear(credential)
		if openErr != nil {
			if errors.Is(openErr, ErrCredential) {
				s.block(ctx, identity, route, ResourceBlock{Reason: "CREDENTIAL_UNRECOVERABLE", ErrorCode: "CREDENTIAL_UNRECOVERABLE"})
			}
			if localOpenError(openErr) {
				s.discardAttempt(request.Trace, attempt)
			} else {
				s.failAttempt(request.Trace, attempt, failureCode(openErr))
			}
			if retryableOpenError(openErr) {
				if !errors.Is(openErr, ErrRoute) && !errors.Is(openErr, ErrCredential) && !errors.Is(openErr, ErrProxyServer) {
					s.cooldown(ctx, route, defaultRouteCooldown)
				}
				next := s.nextRoute(ctx, routes, index+1)
				if next >= 0 {
					index = next
					continue
				}
			}
			return nil, openErr
		}

		decision := inspectResponse(response)
		response.ErrorType = decision.errorCode
		if decision.permanent != nil {
			s.block(ctx, identity, route, *decision.permanent)
		} else if decision.retry {
			s.cooldown(ctx, route, decision.cooldown)
		} else if s.state != nil {
			_ = s.state.Healthy(ctx, route)
		}
		if decision.retry {
			next := s.nextRoute(ctx, routes, index+1)
			if next >= 0 {
				s.failAttempt(request.Trace, attempt, responseErrorType(response))
				response.Body.Close()
				index = next
				continue
			}
		}
		selectedRoute := route
		response.route = &selectedRoute
		s.recordActiveRoute(ctx, selectedRoute)
		return response, nil
	}
	return nil, ErrUpstream
}

func (s *Service) recordActiveRoute(ctx context.Context, route Route) {
	if s.activeRoutes == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	// 仪表盘状态属于旁路观测数据，Redis 写入失败不能影响网关请求。
	_ = s.activeRoutes.RecordActiveRoute(recordCtx, route)
}

func (s *Service) refreshCredential(ctx context.Context, route Route, credential []byte) ([]byte, bool, error) {
	refresh := s.subscriptionRefresher(route.AuthAdapter)
	if refresh == nil {
		return credential, false, ErrSubscription
	}
	if inspector, ok := refresh.(SubscriptionRefreshInspector); ok {
		needed, err := inspector.NeedsRefresh(credential)
		if err != nil {
			return credential, false, classifySubscriptionRefreshError(refresh, err)
		}
		if !needed {
			return credential, false, nil
		}
	}
	refreshCtx := ctx
	var cancel context.CancelFunc
	if s.refreshTimeout > 0 {
		refreshCtx, cancel = context.WithTimeout(ctx, s.refreshTimeout)
		defer cancel()
	}
	if s.refreshCoordinator != nil && !hasSubscriptionRefreshLease(refreshCtx) {
		release, acquired, err := s.refreshCoordinator.AcquireSubscriptionRefreshResource(refreshCtx, route.ResourceID)
		if err != nil || !acquired || release == nil {
			return credential, false, ErrSubscription
		}
		defer release()
		refreshCtx = withSubscriptionRefreshLease(refreshCtx)
	}
	// 保留无 Redis 协调器时的兼容回退；生产服务始终注入 Redis
	// SubscriptionRefreshCoordinator，因此不会在外部刷新期间持有 PostgreSQL 连接。
	if s.refreshCoordinator == nil {
		if locker, ok := s.store.(SubscriptionRefreshLocker); ok {
			lockedCtx, unlock, err := locker.LockSubscriptionRefresh(refreshCtx, route.ResourceID)
			if err != nil || unlock == nil {
				return credential, false, ErrSubscription
			}
			defer unlock()
			refreshCtx = lockedCtx
		}
	}
	if loader, ok := s.store.(CredentialLoader); ok {
		sealed, err := loader.LoadResourceCredential(refreshCtx, route)
		if err != nil {
			return credential, false, ErrSubscription
		}
		latest, err := s.cipher.Decrypt(sealed, catalog.CredentialOwner{
			ProviderID: route.ProviderID, ResourceID: route.ResourceID,
		})
		if err != nil {
			return credential, false, &SubscriptionRefreshError{Code: ErrCredential.Code, Permanent: true}
		}
		clear(credential)
		credential = latest
		route.Credential = sealed
	}
	updated, changed, err := refresh.RefreshIfNeeded(refreshCtx, credential, route.Proxy)
	if err != nil {
		clear(updated)
		return credential, false, classifySubscriptionRefreshError(refresh, err)
	}
	if !changed {
		clear(updated)
		return credential, false, nil
	}
	if len(updated) == 0 {
		return credential, false, ErrSubscription
	}
	if inspector, ok := refresh.(SubscriptionRefreshMetadataInspector); ok {
		refreshedAt, expiresAt, inspectErr := inspector.CredentialRefreshMetadata(updated)
		if inspectErr != nil {
			clear(updated)
			return credential, false, classifySubscriptionRefreshError(refresh, inspectErr)
		}
		route.CredentialRefreshedAt, route.CredentialExpiresAt = refreshedAt, expiresAt
	}
	encryptor, encryptOK := s.cipher.(CredentialEncryptor)
	updater, updateOK := s.store.(CredentialUpdater)
	if !encryptOK || !updateOK {
		clear(updated)
		return credential, false, ErrSubscription
	}
	sealed, err := encryptor.Encrypt(updated, catalog.CredentialOwner{ProviderID: route.ProviderID, ResourceID: route.ResourceID})
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(refreshCtx), 3*time.Second)
	defer cancel()
	if err != nil || updater.UpdateResourceCredential(persistCtx, route, sealed) != nil {
		clear(updated)
		return credential, false, ErrSubscription
	}
	clear(credential)
	return updated, true, nil
}

func (s *Service) startAttempt(event *usage.Event, route Route) *usage.Attempt {
	if event == nil {
		return nil
	}
	attempt := &usage.Attempt{
		AttemptNo: int32(len(event.Attempts) + 1), ProviderID: route.ProviderID,
		ProviderModelID: route.ProviderModelID, ResourceID: route.ResourceID,
		ModelID: route.ModelID, StartedAt: time.Now().UTC(),
	}
	event.Attempt = attempt
	return attempt
}

func (s *Service) failAttempt(event *usage.Event, attempt *usage.Attempt, code string) {
	if event == nil || attempt == nil {
		return
	}
	attempt.CompletedAt = time.Now().UTC()
	attempt.Status = usage.Failed
	attempt.ErrorType = code
	event.Attempts = append(event.Attempts, *attempt)
	event.Attempt = nil
}

func (s *Service) discardAttempt(event *usage.Event, attempt *usage.Attempt) {
	if event != nil && event.Attempt == attempt {
		event.Attempt = nil
	}
}

func (s *Service) cooldown(ctx context.Context, route Route, duration time.Duration) {
	if s.state != nil {
		_ = s.state.Cooldown(ctx, route, duration)
	}
}

func (s *Service) block(ctx context.Context, identity appsec.PrincipalIdentity, route Route, block ResourceBlock) {
	if blocker, ok := s.store.(ResourceBlocker); ok {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = blocker.BlockResource(persistCtx, identity, route.ResourceID, block)
	}
}

func (s *Service) blockSystemResource(ctx context.Context, resourceID int64, block ResourceBlock) {
	if blocker, ok := s.store.(SystemResourceBlocker); ok {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = blocker.BlockResourceSystem(persistCtx, resourceID, block)
	}
}

func retryableOpenError(err error) bool {
	return errors.Is(err, ErrRoute) || errors.Is(err, ErrCredential) || errors.Is(err, ErrSubscription) || errors.Is(err, ErrProxy) || errors.Is(err, ErrProxyServer) || errors.Is(err, ErrUpstream) || errors.Is(err, ErrTimeout)
}

func classifySubscriptionRefreshError(refresh SubscriptionRefresher, err error) error {
	if err == nil {
		return ErrSubscription
	}
	if classifier, ok := refresh.(SubscriptionRefreshErrorClassifier); ok {
		code, permanent := classifier.ClassifyRefreshError(err)
		if code != "" {
			return &SubscriptionRefreshError{Code: code, Permanent: permanent}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &SubscriptionRefreshError{Code: "UPSTREAM_TIMEOUT"}
	}
	return &SubscriptionRefreshError{Code: ErrSubscription.Code}
}

func subscriptionRefreshBlockReason(code string) string {
	switch code {
	case "UPSTREAM_AUTH_FAILED":
		return "AUTHENTICATION"
	case "UPSTREAM_BILLING_BLOCKED", "SUBSCRIPTION_EXPIRED":
		return "BILLING"
	case "ACCOUNT_SUSPENDED", "UPSTREAM_ACCOUNT_SUSPENDED":
		return "ACCOUNT_SUSPENDED"
	case "CREDENTIAL_REVOKED":
		return "CREDENTIAL_REVOKED"
	default:
		return "CREDENTIAL_UNRECOVERABLE"
	}
}

func localOpenError(err error) bool {
	return errors.Is(err, ErrRoute) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrCredential) || errors.Is(err, ErrSubscription) || errors.Is(err, ErrProxy)
}

func failureCode(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	return "UPSTREAM_UNAVAILABLE"
}

type responseDecision struct {
	retry     bool
	cooldown  time.Duration
	permanent *ResourceBlock
	errorCode string
}

func inspectResponse(response *Response) responseDecision {
	if response == nil || response.Body == nil {
		return responseDecision{retry: true, cooldown: defaultRouteCooldown}
	}
	status := response.Status
	if status < 400 {
		return responseDecision{}
	}
	prefix, body, readErr := readResponsePrefix(response.Body, 64<<10, responseInspectionTimeout)
	response.Body = body
	if readErr != nil {
		return responseDecision{retry: true, cooldown: defaultRouteCooldown}
	}
	lower := strings.ToLower(string(prefix))
	block := func(reason, code string) responseDecision {
		return responseDecision{retry: true, permanent: &ResourceBlock{Reason: reason, ErrorCode: code, HTTPStatus: int32(status)}, errorCode: code}
	}
	if status == http.StatusUnauthorized {
		return block("AUTHENTICATION", "UPSTREAM_AUTHENTICATION_FAILED")
	}
	if status == http.StatusPaymentRequired || containsAny(lower,
		"insufficient_balance", "insufficient balance", "credit balance", "payment required",
		"billing_error", "billing error", "subscription expired", "subscription has expired",
		"plan expired", "subscription inactive", "no active subscription") {
		return block("BILLING", "UPSTREAM_BILLING_BLOCKED")
	}
	if status == http.StatusForbidden && containsAny(lower, "account suspended", "account_suspended", "account disabled", "account_disabled") {
		return block("ACCOUNT_SUSPENDED", "UPSTREAM_ACCOUNT_SUSPENDED")
	}
	if status == http.StatusTooManyRequests || status == http.StatusForbidden || status == http.StatusNotFound || status >= 500 {
		return responseDecision{retry: true, cooldown: retryAfter(response.Headers)}
	}
	return responseDecision{}
}

func responseErrorType(response *Response) string {
	if response != nil && response.ErrorType != "" {
		return response.ErrorType
	}
	if response == nil {
		return "UPSTREAM_UNAVAILABLE"
	}
	return "UPSTREAM_HTTP_" + strconv.Itoa(response.Status)
}

func retryAfter(headers map[string][]string) time.Duration {
	return retryAfterAt(headers, time.Now().UTC())
}

func retryAfterAt(headers map[string][]string, now time.Time) time.Duration {
	raw := strings.TrimSpace(http.Header(headers).Get("Retry-After"))
	seconds, err := strconv.Atoi(raw)
	if err == nil && seconds > 0 && seconds <= 3600 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, parseErr := http.ParseTime(raw); parseErr == nil {
		delay := retryAt.Sub(now)
		if delay > 0 && delay <= time.Hour {
			return delay
		}
	}
	return defaultRouteCooldown
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

type prefixedBody struct {
	io.Reader
	closer io.Closer
}

func (body *prefixedBody) Close() error { return body.closer.Close() }

type prefixReadResult struct {
	prefix []byte
	err    error
}

func readResponsePrefix(source io.ReadCloser, limit int64, timeout time.Duration) ([]byte, io.ReadCloser, error) {
	result := make(chan prefixReadResult, 1)
	go func() {
		prefix, err := io.ReadAll(io.LimitReader(source, limit))
		result <- prefixReadResult{prefix: prefix, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case read := <-result:
		return read.prefix, &prefixedBody{Reader: io.MultiReader(bytes.NewReader(read.prefix), source), closer: source}, read.err
	case <-timer.C:
		_ = source.Close()
		return nil, http.NoBody, errResponseInspectionTimeout
	}
}
