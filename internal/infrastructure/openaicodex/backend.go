package openaicodex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const (
	// 路径与 Codex CLI 0.171.0 的 backend-client 保持一致。
	codexBackendURL         = "https://chatgpt.com/backend-api/wham"
	codexBackendUserAgent   = "codex-cli/0.171.0"
	maxAccountResponseBytes = 1 << 20
)

type backendWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
	ResetAt            int64   `json:"reset_at"`
}

type backendRateLimit struct {
	Allowed      *bool          `json:"allowed"`
	LimitReached bool           `json:"limit_reached"`
	Primary      *backendWindow `json:"primary_window"`
	Secondary    *backendWindow `json:"secondary_window"`
}

type backendUsage struct {
	AccountID            string            `json:"account_id"`
	PlanType             string            `json:"plan_type"`
	RateLimit            *backendRateLimit `json:"rate_limit"`
	RateLimitReachedType *struct {
		Type string `json:"type"`
	} `json:"rate_limit_reached_type"`
	AdditionalRateLimits []struct {
		LimitName      string            `json:"limit_name"`
		MeteredFeature string            `json:"metered_feature"`
		RateLimit      *backendRateLimit `json:"rate_limit"`
	} `json:"additional_rate_limits"`
	ResetCredits *struct {
		AvailableCount int `json:"available_count"`
	} `json:"rate_limit_reset_credits"`
}

type backendResetCredits struct {
	AvailableCount *int `json:"available_count"`
	Credits        []struct {
		ID          string  `json:"id"`
		ResetType   string  `json:"reset_type"`
		Status      string  `json:"status"`
		GrantedAt   string  `json:"granted_at"`
		ExpiresAt   *string `json:"expires_at"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
	} `json:"credits"`
}

func (a *Adapter) prepareAccountCredential(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, mgmt.SubscriptionInspection, bool, error) {
	credential, err := ensureLastRefresh(raw)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, false, refreshFailure("CREDENTIAL_INVALID", "invalid Codex credential")
	}
	needed, err := a.NeedsRefresh(credential)
	if err != nil {
		clear(credential)
		return nil, mgmt.SubscriptionInspection{}, false, refreshFailure("CREDENTIAL_INVALID", "invalid Codex credential")
	}
	rotated := false
	if needed {
		updated, refreshErr := a.refreshToken(ctx, credential, proxy)
		clear(credential)
		if refreshErr != nil {
			return nil, mgmt.SubscriptionInspection{}, false, refreshErr
		}
		credential, rotated = updated, true
	}
	inspection, err := a.Inspect(credential)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, false, preserveRotatedCredential(err, credential, rotated)
	}
	return credential, inspection, rotated, nil
}

func preserveRotatedCredential(err error, credential []byte, rotated bool) error {
	if rotated {
		return &sessionFailure{cause: err, credential: credential}
	}
	clear(credential)
	return err
}

func (a *Adapter) readAccountLimits(ctx context.Context, credential []byte, proxy *catalog.OutboundProxy) (rateLimitsResponse, error) {
	var usage backendUsage
	if err := a.accountRequest(ctx, credential, proxy, http.MethodGet, "/usage", nil, &usage); err != nil {
		return rateLimitsResponse{}, err
	}
	cache, err := parseCredential(credential)
	if err != nil {
		return rateLimitsResponse{}, refreshFailure("CREDENTIAL_INVALID", "invalid Codex credential")
	}
	if usage.AccountID == "" {
		return rateLimitsResponse{}, refreshFailure("UPSTREAM_INVALID_RESPONSE", "Codex usage account is missing")
	}
	if usage.AccountID != cache.Tokens.AccountID {
		return rateLimitsResponse{}, refreshFailure("UPSTREAM_AUTH_FAILED", "Codex usage account does not match credential")
	}
	limits := usage.rateLimits()
	// 与官方 app-server 一致：重置卡详情失败时仍返回额度及 usage 中的可用数量。
	if usage.ResetCredits != nil {
		limits.RateLimitResetCredits = &rateLimitResetCreditsWire{AvailableCount: usage.ResetCredits.AvailableCount}
	}
	cardCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var cards backendResetCredits
	if err := a.accountRequest(cardCtx, credential, proxy, http.MethodGet, "/rate-limit-reset-credits", nil, &cards); err == nil {
		if detailed, ok := cards.toRateLimitResetCredits(); ok {
			limits.RateLimitResetCredits = detailed
		}
	}
	return limits, nil
}

func (u backendUsage) rateLimits() rateLimitsResponse {
	result := rateLimitsResponse{RateLimits: makeCodexRateLimit("codex", "", u.PlanType, u.RateLimit, u.reachedType())}
	for index, extra := range u.AdditionalRateLimits {
		code := strings.TrimSpace(extra.MeteredFeature)
		if code == "" {
			code = "codex.additional." + strconv.Itoa(index+1)
		}
		if result.RateLimitsByLimitID == nil {
			result.RateLimitsByLimitID = make(map[string]codexRateLimit)
			result.RateLimitsByLimitID["codex"] = *result.RateLimits
		}
		result.RateLimitsByLimitID[code] = *makeCodexRateLimit(code, extra.LimitName, u.PlanType, extra.RateLimit, nil)
	}
	return result
}

func (u backendUsage) reachedType() *string {
	if u.RateLimitReachedType == nil || u.RateLimitReachedType.Type == "" {
		return nil
	}
	value := u.RateLimitReachedType.Type
	return &value
}

func makeCodexRateLimit(code, name, plan string, source *backendRateLimit, reached *string) *codexRateLimit {
	limit := &codexRateLimit{LimitID: code, PlanType: plan, RateLimitReachedType: reached}
	if name != "" {
		limit.LimitName = &name
	}
	if source == nil {
		return limit
	}
	if source.LimitReached || source.Allowed != nil && !*source.Allowed {
		if limit.RateLimitReachedType == nil {
			value := "rate_limit_reached"
			limit.RateLimitReachedType = &value
		}
	}
	limit.Primary = source.Primary.toWindow()
	limit.Secondary = source.Secondary.toWindow()
	return limit
}

func (w *backendWindow) toWindow() *rateLimitWindow {
	if w == nil {
		return nil
	}
	return &rateLimitWindow{UsedPercent: w.UsedPercent, WindowDurationSeconds: &w.LimitWindowSeconds, ResetsAt: &w.ResetAt}
}

func (r backendResetCredits) toRateLimitResetCredits() (*rateLimitResetCreditsWire, bool) {
	if r.AvailableCount == nil || *r.AvailableCount < 0 {
		return nil, false
	}
	result := &rateLimitResetCreditsWire{AvailableCount: *r.AvailableCount, Credits: make([]rateLimitResetCreditWire, 0, len(r.Credits))}
	for _, card := range r.Credits {
		granted, err := time.Parse(time.RFC3339Nano, card.GrantedAt)
		if err != nil || card.ID == "" {
			return nil, false
		}
		var expires *int64
		if card.ExpiresAt != nil {
			parsed, err := time.Parse(time.RFC3339Nano, *card.ExpiresAt)
			if err != nil {
				return nil, false
			}
			value := parsed.UTC().Unix()
			expires = &value
		}
		resetType := card.ResetType
		if resetType == "codex_rate_limits" {
			resetType = "codexRateLimits"
		}
		result.Credits = append(result.Credits, rateLimitResetCreditWire{
			ID: card.ID, ResetType: resetType, Status: card.Status,
			GrantedAt: granted.UTC().Unix(), ExpiresAt: expires, Title: card.Title, Description: card.Description,
		})
	}
	return result, true
}

func (a *Adapter) consumeAccountResetCredit(ctx context.Context, credential []byte, proxy *catalog.OutboundProxy, idempotencyKey, creditID string) (string, error) {
	payload := struct {
		RedeemRequestID string `json:"redeem_request_id"`
		CreditID        string `json:"credit_id,omitempty"`
	}{RedeemRequestID: idempotencyKey, CreditID: creditID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", refreshFailure("SUBSCRIPTION_REFRESH_FAILED", "cannot encode Codex reset credit request")
	}
	defer clear(encoded)
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var result struct {
		Code string `json:"code"`
	}
	if err := a.accountRequest(requestCtx, credential, proxy, http.MethodPost, "/rate-limit-reset-credits/consume", encoded, &result); err != nil {
		return "", err
	}
	switch result.Code {
	case "reset":
		return "reset", nil
	case "already_redeemed":
		return "alreadyRedeemed", nil
	case "nothing_to_reset":
		return "nothingToReset", nil
	case "no_credit":
		return "noCredit", nil
	default:
		return "", refreshFailure("UPSTREAM_INVALID_RESPONSE", "invalid Codex reset credit response")
	}
}

func (a *Adapter) accountRequest(ctx context.Context, credential []byte, proxy *catalog.OutboundProxy, method, path string, payload []byte, target any) error {
	cache, err := parseCredential(credential)
	if err != nil {
		return refreshFailure("CREDENTIAL_INVALID", "invalid Codex credential")
	}
	request, err := http.NewRequestWithContext(ctx, method, a.backendURL+path, bytes.NewReader(payload))
	if err != nil {
		return refreshFailure("UPSTREAM_INVALID_RESPONSE", "cannot create Codex account request")
	}
	request.Header.Set("Authorization", "Bearer "+cache.Tokens.AccessToken)
	request.Header.Set("ChatGPT-Account-Id", cache.Tokens.AccountID)
	request.Header.Set("User-Agent", codexBackendUserAgent)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client, cleanup, err := provider.ClientWithProxy(ctx, a.refreshHTTP, proxy, provider.ProxyRequestLog{
		Logger: a.logger, Operation: "subscription_account", ProviderCode: catalog.OpenAIOfficialCode, Protocol: "OPENAI",
	})
	if err != nil {
		return refreshFailure("SUBSCRIPTION_UNAVAILABLE", "invalid Codex proxy configuration")
	}
	defer cleanup()
	response, err := client.Do(request)
	if err != nil {
		return refreshFailure(provider.TransportErrorCode(ctx, err, proxy != nil), "Codex account request transport failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAccountResponseBytes+1))
	if err != nil {
		return refreshFailure("UPSTREAM_INVALID_RESPONSE", "cannot read Codex account response")
	}
	defer clear(body)
	if len(body) > maxAccountResponseBytes {
		return refreshFailure("UPSTREAM_INVALID_RESPONSE", "Codex account response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifyAccountRejection(response.StatusCode)
	}
	if len(body) == 0 || json.Unmarshal(body, target) != nil {
		return refreshFailure("UPSTREAM_INVALID_RESPONSE", "invalid Codex account response")
	}
	return nil
}

func classifyAccountRejection(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return refreshFailure("UPSTREAM_AUTH_FAILED", "Codex account request was rejected")
	case http.StatusTooManyRequests:
		return refreshFailure("UPSTREAM_RATE_LIMITED", "Codex account request was rate limited")
	default:
		if status >= 500 {
			return refreshFailure("UPSTREAM_UNAVAILABLE", "Codex account service is unavailable")
		}
		return refreshFailure("SUBSCRIPTION_UNAVAILABLE", "Codex account request failed")
	}
}
