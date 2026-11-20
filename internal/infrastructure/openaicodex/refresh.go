package openaicodex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const (
	// 与官方 Codex CLI 的 ChatGPT OAuth 刷新请求保持一致。该端点不是公开承诺稳定的 API。
	codexRefreshURL      = "https://auth.openai.com/oauth/token"
	codexOAuthClientID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	maxRefreshReplyBytes = 64 << 10
)

type refreshReply struct {
	IDToken      *string `json:"id_token"`
	AccessToken  *string `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
}

// 刷新可以只返回部分 Token；缺省或 null 保留原值，但不能接受空响应或显式空 Token。
func (r refreshReply) valid() bool {
	present := false
	for _, value := range []*string{r.IDToken, r.AccessToken, r.RefreshToken} {
		if value != nil {
			if strings.TrimSpace(*value) == "" {
				return false
			}
			present = true
		}
	}
	return present
}

func newRefreshHTTPClient() *http.Client {
	// 无服务商代理时保留操作系统代理设置；配置了服务商代理时由 ClientWithProxy 覆盖。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: transport, Timeout: 45 * time.Second, CheckRedirect: provider.RejectRedirect}
}

func (a *Adapter) refreshToken(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, error) {
	cache, err := parseCredential(raw)
	if err != nil {
		return nil, err
	}
	requestBody, err := json.Marshal(struct {
		GrantType     string `json:"grant_type"`
		ClientID      string `json:"client_id"`
		RefreshToken  string `json:"refresh_token"`
		RefreshReason string `json:"refresh_reason"`
	}{"refresh_token", codexOAuthClientID, cache.Tokens.RefreshToken, "proactive_expiry"})
	if err != nil {
		return nil, err
	}
	defer clear(requestBody)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.refreshURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, refreshFailure("SUBSCRIPTION_REFRESH_FAILED", "cannot create Codex token refresh request")
	}
	request.Header.Set("Content-Type", "application/json")
	client, cleanup, err := provider.ClientWithProxy(ctx, a.refreshHTTP, proxy, provider.ProxyRequestLog{
		Logger: a.logger, Operation: "subscription_refresh", ProviderCode: catalog.OpenAIOfficialCode, Protocol: "OPENAI",
	})
	if err != nil {
		return nil, refreshFailure("SUBSCRIPTION_REFRESH_FAILED", "invalid Codex proxy configuration")
	}
	defer cleanup()
	response, err := client.Do(request)
	if err != nil {
		code := provider.TransportErrorCode(ctx, err, proxy != nil)
		return nil, refreshFailure(code, "Codex token refresh transport failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRefreshReplyBytes+1))
	if err != nil {
		return nil, refreshFailure("UPSTREAM_INVALID_RESPONSE", "cannot read Codex token refresh response")
	}
	defer clear(body)
	if len(body) > maxRefreshReplyBytes {
		return nil, refreshFailure("UPSTREAM_INVALID_RESPONSE", "Codex token refresh response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, classifyTokenRejection(response.StatusCode, body)
	}
	var tokens refreshReply
	if json.Unmarshal(body, &tokens) != nil || !tokens.valid() {
		return nil, refreshFailure("UPSTREAM_INVALID_RESPONSE", "invalid Codex token refresh response")
	}
	updated, err := replaceAuthTokens(raw, tokens, time.Now().UTC())
	if err != nil {
		return nil, refreshFailure("UPSTREAM_INVALID_RESPONSE", "cannot save Codex token refresh response")
	}
	return updated, nil
}

func refreshFailure(code, message string) error {
	return &connectionFailure{code: code, cause: errors.New(message)}
}

func classifyTokenRejection(status int, body []byte) error {
	code := oauthErrorCode(body)
	switch code {
	case "token_expired", "refresh_token_expired":
		return refreshFailure("CREDENTIAL_INVALID", "Codex refresh token expired or invalid")
	case "refresh_token_reused", "refresh_token_revoked", "refresh_token_invalidated", "token_revoked":
		return refreshFailure("CREDENTIAL_REVOKED", "Codex refresh token was reused or revoked")
	case "invalid_grant":
		if status == http.StatusBadRequest {
			return refreshFailure("CREDENTIAL_INVALID", "Codex refresh token expired or invalid")
		}
	}
	switch status {
	case http.StatusTooManyRequests:
		return refreshFailure("UPSTREAM_RATE_LIMITED", "Codex token refresh was rate limited")
	case http.StatusUnauthorized:
		return refreshFailure("UPSTREAM_AUTH_FAILED", "Codex token refresh was rejected")
	default:
		return refreshFailure("SUBSCRIPTION_REFRESH_FAILED", "Codex token refresh was rejected")
	}
}

// oauthErrorCode 兼容标准 OAuth error 字符串、嵌套 error.code 和顶层 code。
// 原始错误内容可能包含凭据，只返回用于分类的错误码，不把响应正文交给日志或调用方。
func oauthErrorCode(body []byte) string {
	var reply struct {
		Error json.RawMessage `json:"error"`
		Code  string          `json:"code"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return ""
	}
	var code string
	if json.Unmarshal(reply.Error, &code) != nil {
		var nested struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(reply.Error, &nested) == nil {
			code = nested.Code
		}
	}
	if strings.TrimSpace(code) == "" {
		code = reply.Code
	}
	return strings.ToLower(strings.TrimSpace(code))
}

func replaceAuthTokens(raw []byte, tokens refreshReply, refreshedAt time.Time) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(document["tokens"], &saved); err != nil {
		return nil, err
	}
	for key, value := range map[string]*string{
		"id_token": tokens.IDToken, "access_token": tokens.AccessToken, "refresh_token": tokens.RefreshToken,
	} {
		if value == nil {
			continue
		}
		encoded, err := json.Marshal(*value)
		if err != nil {
			return nil, err
		}
		saved[key] = encoded
	}
	encodedTokens, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	encodedTime, err := json.Marshal(refreshedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	document["tokens"] = encodedTokens
	document["last_refresh"] = encodedTime
	updated, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	if _, err := parseCredential(updated); err != nil {
		clear(updated)
		return nil, err
	}
	return updated, nil
}
