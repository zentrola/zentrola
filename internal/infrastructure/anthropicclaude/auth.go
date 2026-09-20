// Package anthropicclaude 管理 Claude Code 个人订阅的长期 OAuth Token。
package anthropicclaude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const (
	AdapterCode          = mgmt.AuthAdapterClaudeCode
	OAuthBeta            = "oauth-2025-04-20"
	defaultUsageEndpoint = "https://api.anthropic.com/api/oauth/usage"
	credentialKind       = "claude_code_setup_token"
)

var errInvalidCredential = errors.New("invalid Claude Code setup token")

type connectionFailure struct {
	code  string
	cause error
}

func (e *connectionFailure) Error() string          { return e.cause.Error() }
func (e *connectionFailure) Unwrap() error          { return e.cause }
func (e *connectionFailure) ConnectionCode() string { return e.code }

func connectionError(code string, cause error) error {
	return &connectionFailure{code: code, cause: cause}
}

type storedCredential struct {
	Kind        string `json:"kind"`
	AccessToken string `json:"access_token"`
}

type Adapter struct {
	client        *http.Client
	usageEndpoint string
}

func New() *Adapter {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = provider.PublicDialContext(&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second})
	return &Adapter{
		client:        &http.Client{Transport: transport, Timeout: 20 * time.Second},
		usageEndpoint: defaultUsageEndpoint,
	}
}

func (a *Adapter) Code() string              { return AdapterCode }
func (a *Adapter) Supports(code string) bool { return code == AdapterCode }
func (a *Adapter) SupportsProvider(provider mgmt.Provider) bool {
	return provider.Code == catalog.AnthropicOfficialCode
}

func validToken(token string) bool {
	if len(token) == 0 || len(token) > 4096 {
		return false
	}
	for _, value := range []byte(token) {
		if value < 33 || value > 126 {
			return false
		}
	}
	return true
}

func parseCredential(raw []byte) (storedCredential, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || len(trimmed) > 64<<10 {
		return storedCredential{}, errInvalidCredential
	}
	if trimmed[0] != '{' {
		token := string(trimmed)
		if !validToken(token) {
			return storedCredential{}, errInvalidCredential
		}
		return storedCredential{Kind: credentialKind, AccessToken: token}, nil
	}
	var credential storedCredential
	if json.Unmarshal(trimmed, &credential) != nil || credential.Kind != credentialKind || !validToken(credential.AccessToken) {
		return storedCredential{}, errInvalidCredential
	}
	return credential, nil
}

// RequestCredential 返回一次上游请求使用的 Bearer Token。
func RequestCredential(raw []byte) (string, error) {
	credential, err := parseCredential(raw)
	if err != nil {
		return "", err
	}
	return credential.AccessToken, nil
}

func canonicalCredential(credential storedCredential) ([]byte, error) {
	encoded, err := json.Marshal(credential)
	if err != nil {
		return nil, errInvalidCredential
	}
	return encoded, nil
}

func (a *Adapter) Inspect(raw []byte) (mgmt.SubscriptionInspection, error) {
	if _, err := parseCredential(raw); err != nil {
		return mgmt.SubscriptionInspection{}, err
	}
	return mgmt.SubscriptionInspection{}, nil
}

type usageWindow struct {
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at"`
}

type usageResponse struct {
	SubscriptionType string       `json:"subscription_type"`
	FiveHour         *usageWindow `json:"five_hour"`
	SevenDay         *usageWindow `json:"seven_day"`
	SevenDayOpus     *usageWindow `json:"seven_day_opus"`
}

func quotaStatus(utilization *float64) string {
	if utilization == nil {
		return mgmt.QuotaUnknown
	}
	if *utilization >= 100 {
		return mgmt.QuotaExhausted
	}
	if *utilization >= 90 {
		return mgmt.QuotaNearLimit
	}
	return mgmt.QuotaAvailable
}

func (u usageResponse) quotas(observedAt time.Time) []mgmt.ResourceQuota {
	windows := []struct {
		code, name string
		window     *usageWindow
	}{
		{code: "five_hour", name: "5 hours", window: u.FiveHour},
		{code: "seven_day", name: "7 days", window: u.SevenDay},
		{code: "seven_day_opus", name: "7 days · Opus", window: u.SevenDayOpus},
	}
	availabilityName := "Subscription"
	quotas := []mgmt.ResourceQuota{{
		Code: "subscription", Name: &availabilityName, Status: mgmt.QuotaAvailable, ObservedAt: observedAt,
	}}
	for _, item := range windows {
		if item.window == nil {
			continue
		}
		name, unit := item.name, "PERCENT"
		quotas = append(quotas, mgmt.ResourceQuota{
			Code: item.code, Name: &name, Status: quotaStatus(item.window.Utilization), Unit: &unit,
			UsedPercent: item.window.Utilization, ResetsAt: item.window.ResetsAt, ObservedAt: observedAt,
		})
	}
	return quotas
}

func (a *Adapter) Probe(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) (mgmt.SubscriptionProbe, error) {
	credential, err := parseCredential(raw)
	if err != nil {
		return mgmt.SubscriptionProbe{}, connectionError("CREDENTIAL_INVALID", err)
	}
	usage, err := a.readUsage(ctx, credential.AccessToken, proxy)
	if err != nil {
		return mgmt.SubscriptionProbe{}, err
	}
	canonical, err := canonicalCredential(credential)
	if err != nil {
		return mgmt.SubscriptionProbe{}, err
	}
	inspection := mgmt.SubscriptionInspection{PlanCode: usage.SubscriptionType}
	return mgmt.SubscriptionProbe{
		Inspection: inspection,
		Credential: canonical,
		Quotas:     usage.quotas(time.Now().UTC().Truncate(time.Microsecond)),
	}, nil
}

func (a *Adapter) readUsage(ctx context.Context, token string, proxy *catalog.OutboundProxy) (usageResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.usageEndpoint, nil)
	if err != nil {
		return usageResponse{}, connectionError("UPSTREAM_UNAVAILABLE", errors.New("cannot create Claude usage request"))
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("anthropic-beta", OAuthBeta)
	defer request.Header.Del("Authorization")

	client, cleanup, err := provider.ClientWithProxy(ctx, a.client, proxy, provider.ProxyRequestLog{
		Operation: "subscription_usage", ProviderCode: catalog.AnthropicOfficialCode, Protocol: "ANTHROPIC",
	})
	if err != nil {
		return usageResponse{}, connectionError("SUBSCRIPTION_UNAVAILABLE", errors.New("invalid Claude proxy configuration"))
	}
	defer cleanup()
	response, err := client.Do(request)
	if err != nil {
		code := "SUBSCRIPTION_UNAVAILABLE"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = "UPSTREAM_TIMEOUT"
		}
		return usageResponse{}, connectionError(code, errors.New("cannot reach Claude usage service"))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return usageResponse{}, connectionError(usageStatusCode(response.StatusCode), fmt.Errorf("Claude usage request failed with HTTP %d", response.StatusCode))
	}
	var usage usageResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if decoder.Decode(&usage) != nil {
		return usageResponse{}, connectionError("UPSTREAM_INVALID_RESPONSE", errors.New("invalid Claude usage response"))
	}
	return usage, nil
}

func usageStatusCode(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "UPSTREAM_AUTH_FAILED"
	case http.StatusPaymentRequired:
		return "UPSTREAM_BILLING_BLOCKED"
	case http.StatusTooManyRequests:
		return "UPSTREAM_RATE_LIMITED"
	default:
		return "UPSTREAM_UNAVAILABLE"
	}
}

// setup-token 是长期 OAuth Token，没有可供第三方调用的公开刷新接口。
func (a *Adapter) RefreshIfNeeded(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error) {
	return nil, false, nil
}

func (a *Adapter) NeedsRefresh(raw []byte) (bool, error) {
	_, err := parseCredential(raw)
	return false, err
}
