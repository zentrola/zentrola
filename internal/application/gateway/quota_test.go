package gateway

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	domainquota "github.com/zentrola/zentrola/internal/domain/quota"
	"github.com/zentrola/zentrola/internal/domain/usage"
)

type quotaAdmissionFunc func(context.Context, []domainquota.Scope, time.Time) (domainquota.Decision, error)

func (f quotaAdmissionFunc) Check(ctx context.Context, scopes []domainquota.Scope, at time.Time) (domainquota.Decision, error) {
	return f(ctx, scopes, at)
}

func TestForwardRejectsReachedMonthlyTokenQuotaBeforeOpeningUpstream(t *testing.T) {
	scope := domainquota.Scope{Type: domainquota.Principal, ID: 1, Limit: 100}
	routes := testRoutes()
	routes[0].QuotaScopes = []domainquota.Scope{scope}
	opened := false
	service := New(&failoverStore{routes: routes}, failoverCipher{}, upstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		opened = true
		return response(200, `{}`), nil
	}), WithTokenQuotaAdmission(quotaAdmissionFunc(func(_ context.Context, scopes []domainquota.Scope, _ time.Time) (domainquota.Decision, error) {
		return domainquota.Decision{Allowed: false, Scope: &scopes[0], Used: 100}, nil
	})))

	result, err := testForward(t, service, &usage.Event{})
	if result != nil || err != ErrBudgetExceeded || opened {
		t.Fatalf("quota rejection must happen before upstream: result=%v err=%v opened=%v", result, err, opened)
	}
}

func TestForwardAttachesCallTimeGroupsAndTokenCountBypassesQuota(t *testing.T) {
	scope := domainquota.Scope{Type: domainquota.Group, ID: 2, Limit: 200}
	routes := testRoutes()
	routes[0].QuotaScopes = []domainquota.Scope{scope}
	routes[0].QuotaGroupIDs = []int64{2, 3}
	checks := 0
	service := New(&failoverStore{routes: routes}, failoverCipher{}, upstreamFunc(func(context.Context, Route, Request, []byte) (*Response, error) {
		return &Response{Status: 200, Headers: map[string][]string{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}), WithTokenQuotaAdmission(quotaAdmissionFunc(func(_ context.Context, _ []domainquota.Scope, _ time.Time) (domainquota.Decision, error) {
		checks++
		return domainquota.Decision{Allowed: true}, nil
	})))

	trace := &usage.Event{}
	result, err := testForward(t, service, trace)
	if err != nil || result == nil || len(trace.QuotaGroupIDs) != 2 || trace.QuotaGroupIDs[0] != 2 || trace.QuotaGroupIDs[1] != 3 || checks != 1 {
		t.Fatalf("call-time groups were not preserved for usage metering: trace=%+v checks=%d err=%v", trace, checks, err)
	}
	_ = result.Body.Close()

	countTrace := &usage.Event{}
	result, err = service.Forward(context.Background(), appsec.PrincipalIdentity{ID: 1, AccessKeyID: 3}, Request{
		Path: "/v1/messages/count_tokens", Protocol: AnthropicProtocol, Body: []byte(`{"model":"claude-opus-5"}`), Trace: countTrace,
	})
	if err != nil || result == nil || checks != 1 || len(countTrace.QuotaGroupIDs) != 0 {
		t.Fatalf("count_tokens must bypass token quota admission: checks=%d err=%v", checks, err)
	}
	_ = result.Body.Close()
}
