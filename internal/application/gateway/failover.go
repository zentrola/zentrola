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

const defaultRouteCooldown = time.Minute

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
				s.cooldown(ctx, route, defaultRouteCooldown)
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
				if !errors.Is(openErr, ErrRoute) && !errors.Is(openErr, ErrCredential) {
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
		return response, nil
	}
	return nil, ErrUpstream
}

func (s *Service) refreshCredential(ctx context.Context, route Route, credential []byte) ([]byte, bool, error) {
	refresh := s.subscriptionRefresher(route.AuthAdapter)
	if refresh == nil {
		return credential, false, ErrSubscription
	}
	if inspector, ok := refresh.(SubscriptionRefreshInspector); ok {
		needed, err := inspector.NeedsRefresh(credential)
		if err != nil {
			return credential, false, ErrSubscription
		}
		if !needed {
			return credential, false, nil
		}
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if locker, ok := s.store.(SubscriptionRefreshLocker); ok {
		lockedCtx, unlock, err := locker.LockSubscriptionRefresh(ctx, route.ResourceID)
		if err != nil || unlock == nil {
			return credential, false, ErrSubscription
		}
		defer unlock()
		ctx = lockedCtx
	}
	if loader, ok := s.store.(CredentialLoader); ok {
		sealed, err := loader.LoadResourceCredential(ctx, route)
		if err != nil {
			return credential, false, ErrSubscription
		}
		latest, err := s.cipher.Decrypt(sealed, catalog.CredentialOwner{
			ProviderID: route.ProviderID, ResourceID: route.ResourceID,
		})
		if err != nil {
			return credential, false, ErrSubscription
		}
		clear(credential)
		credential = latest
	}
	updated, changed, err := refresh.RefreshIfNeeded(ctx, credential, route.Proxy)
	if err != nil {
		clear(updated)
		return credential, false, ErrSubscription
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
			return credential, false, ErrSubscription
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
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
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
		_ = blocker.BlockResource(context.WithoutCancel(ctx), identity, route.ResourceID, block)
	}
}

func retryableOpenError(err error) bool {
	return errors.Is(err, ErrRoute) || errors.Is(err, ErrCredential) || errors.Is(err, ErrSubscription) || errors.Is(err, ErrProxy) || errors.Is(err, ErrUpstream) || errors.Is(err, ErrTimeout)
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
	prefix, body := readResponsePrefix(response.Body, 64<<10)
	response.Body = body
	lower := strings.ToLower(string(prefix))
	block := func(reason, code string) responseDecision {
		return responseDecision{retry: true, permanent: &ResourceBlock{Reason: reason, ErrorCode: code, HTTPStatus: int32(status)}, errorCode: code}
	}
	if status == http.StatusUnauthorized {
		return block("AUTHENTICATION", "UPSTREAM_AUTHENTICATION_FAILED")
	}
	if status == http.StatusPaymentRequired || containsAny(lower, "insufficient_balance", "insufficient balance", "credit balance", "payment required", "billing_error", "billing error") {
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
	raw := strings.TrimSpace(http.Header(headers).Get("Retry-After"))
	seconds, err := strconv.Atoi(raw)
	if err == nil && seconds > 0 && seconds <= 3600 {
		return time.Duration(seconds) * time.Second
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

func readResponsePrefix(source io.ReadCloser, limit int64) ([]byte, io.ReadCloser) {
	prefix, err := io.ReadAll(io.LimitReader(source, limit))
	if err != nil {
		return nil, source
	}
	return prefix, &prefixedBody{Reader: io.MultiReader(bytes.NewReader(prefix), source), closer: source}
}
