// Package openaicodex 管理 ChatGPT 个人订阅认证与额度查询。
package openaicodex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const (
	AdapterCode          = mgmt.AuthAdapterOpenAICodex
	defaultUsageEndpoint = "https://chatgpt.com/backend-api/wham/usage"
	defaultRefreshAhead  = 30 * time.Minute
)

var errInvalidCredential = errors.New("invalid Codex ChatGPT credential")

type Adapter struct {
	executable    string
	client        *http.Client
	usageEndpoint string
	refreshAhead  time.Duration
}

type Option func(*Adapter)

func WithRefreshAhead(value time.Duration) Option {
	return func(adapter *Adapter) {
		if value > 0 {
			adapter.refreshAhead = value
		}
	}
}

func New(executable string, options ...Option) *Adapter {
	if strings.TrimSpace(executable) == "" {
		executable = "codex"
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	adapter := &Adapter{
		executable: executable, usageEndpoint: defaultUsageEndpoint,
		client: &http.Client{Transport: transport, Timeout: 20 * time.Second}, refreshAhead: defaultRefreshAhead,
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func (a *Adapter) Code() string              { return AdapterCode }
func (a *Adapter) Supports(code string) bool { return code == AdapterCode }
func (a *Adapter) SupportsProvider(provider mgmt.Provider) bool {
	return provider.Code == "openai-official"
}

type authCache struct {
	AuthMode    string `json:"auth_mode"`
	LastRefresh string `json:"last_refresh"`
	Tokens      struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

func parseCredential(raw []byte) (authCache, error) {
	var cache authCache
	if len(raw) == 0 || len(raw) > 64<<10 || json.Unmarshal(raw, &cache) != nil ||
		cache.AuthMode != "chatgpt" || cache.Tokens.AccessToken == "" ||
		cache.Tokens.RefreshToken == "" || cache.Tokens.AccountID == "" {
		return authCache{}, errInvalidCredential
	}
	return cache, nil
}

// RequestCredential 返回一次上游调用所需的短期 Token 和账号标识。
// 返回的 Token 只能驻留在单次请求内，调用方不得记录或缓存到日志。
func RequestCredential(raw []byte) (accessToken, accountID string, expiresAt *time.Time, err error) {
	cache, err := parseCredential(raw)
	if err != nil {
		return "", "", nil, err
	}
	claims := jwtClaims(cache.Tokens.AccessToken)
	if expires, ok := claimUnix(claims, "exp"); ok {
		expiresAt = &expires
	}
	return cache.Tokens.AccessToken, cache.Tokens.AccountID, expiresAt, nil
}

func (a *Adapter) Inspect(raw []byte) (mgmt.SubscriptionInspection, error) {
	cache, err := parseCredential(raw)
	if err != nil {
		return mgmt.SubscriptionInspection{}, err
	}
	inspection := mgmt.SubscriptionInspection{AccountRef: cache.Tokens.AccountID}
	accessClaims := jwtClaims(cache.Tokens.AccessToken)
	if expires, ok := claimUnix(accessClaims, "exp"); ok {
		inspection.CredentialExpiresAt = &expires
	}
	if refreshed, ok := parseRefreshTime(cache.LastRefresh); ok {
		inspection.CredentialRefreshedAt = &refreshed
	} else if issued, ok := claimUnix(accessClaims, "iat"); ok {
		inspection.CredentialRefreshedAt = &issued
	}
	claims := jwtClaims(cache.Tokens.IDToken)
	if claims == nil {
		claims = jwtClaims(cache.Tokens.AccessToken)
	}
	if claims != nil {
		inspection.PlanCode = claimString(claims, "chatgpt_plan_type")
		if inspection.PlanCode == "" {
			if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
				inspection.PlanCode = claimString(auth, "chatgpt_plan_type")
			}
		}
	}
	return inspection, nil
}

func parseRefreshTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

// ensureLastRefresh 保证保存和导出的凭据包含 Codex auth.json 的刷新时间字段。
func ensureLastRefresh(raw []byte) ([]byte, error) {
	cache, err := parseCredential(raw)
	if err != nil {
		return nil, err
	}
	if _, ok := parseRefreshTime(cache.LastRefresh); ok {
		return bytes.Clone(raw), nil
	}
	refreshedAt := time.Now().UTC().Truncate(time.Second)
	if issued, ok := claimUnix(jwtClaims(cache.Tokens.AccessToken), "iat"); ok {
		refreshedAt = issued
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, errInvalidCredential
	}
	encoded, err := json.Marshal(refreshedAt.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	document["last_refresh"] = encoded
	return json.Marshal(document)
}

func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	defer clear(payload)
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	var claims map[string]any
	if decoder.Decode(&claims) != nil {
		return nil
	}
	return claims
}

func claimString(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return value
}

func claimUnix(claims map[string]any, key string) (time.Time, bool) {
	value, ok := claims[key].(json.Number)
	if !ok {
		return time.Time{}, false
	}
	seconds, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0).UTC(), true
}

func (a *Adapter) Probe(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) (mgmt.SubscriptionProbe, error) {
	cache, err := parseCredential(raw)
	if err != nil {
		return mgmt.SubscriptionProbe{}, err
	}
	credential, err := ensureLastRefresh(raw)
	if err != nil {
		return mgmt.SubscriptionProbe{}, err
	}
	inspection, err := a.Inspect(credential)
	if err != nil {
		clear(credential)
		return mgmt.SubscriptionProbe{}, err
	}
	usage, err := a.readUsage(ctx, cache.Tokens.AccessToken, cache.Tokens.AccountID, proxy)
	var statusError *usageHTTPError
	if errors.As(err, &statusError) && statusError.StatusCode == http.StatusUnauthorized {
		clear(credential)
		credential, inspection, err = a.refreshCredential(ctx, raw, proxy)
		if err != nil {
			return mgmt.SubscriptionProbe{}, err
		}
		refreshed, parseErr := parseCredential(credential)
		if parseErr != nil {
			clear(credential)
			return mgmt.SubscriptionProbe{}, parseErr
		}
		usage, err = a.readUsage(ctx, refreshed.Tokens.AccessToken, refreshed.Tokens.AccountID, proxy)
	}
	if err != nil {
		clear(credential)
		return mgmt.SubscriptionProbe{}, err
	}
	if usage.PlanType != "" {
		inspection.PlanCode = usage.PlanType
	}
	return mgmt.SubscriptionProbe{
		Inspection: inspection,
		Credential: credential,
		Quotas:     usage.quotas(time.Now().UTC().Truncate(time.Microsecond)),
	}, nil
}

func (a *Adapter) readUsage(ctx context.Context, accessToken, accountID string, proxy *catalog.OutboundProxy) (usageResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.usageEndpoint, nil)
	if err != nil {
		return usageResponse{}, errors.New("cannot create ChatGPT usage request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("ChatGPT-Account-Id", accountID)
	defer request.Header.Del("Authorization")

	client, cleanup, err := provider.ClientWithProxy(ctx, a.client, proxy, provider.ProxyRequestLog{
		Operation: "subscription_usage", ProviderCode: catalog.OpenAIOfficialCode, Protocol: "OPENAI",
	})
	if err != nil {
		return usageResponse{}, errors.New("invalid ChatGPT proxy configuration")
	}
	defer cleanup()
	response, err := client.Do(request)
	if err != nil {
		return usageResponse{}, errors.New("cannot reach ChatGPT usage service")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return usageResponse{}, &usageHTTPError{StatusCode: response.StatusCode}
	}
	var usage usageResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&usage); err != nil {
		return usageResponse{}, errors.New("invalid ChatGPT usage response")
	}
	if usage.AccountID != "" && usage.AccountID != accountID {
		return usageResponse{}, errors.New("ChatGPT usage account mismatch")
	}
	return usage, nil
}

type usageHTTPError struct{ StatusCode int }

func (e *usageHTTPError) Error() string {
	return fmt.Sprintf("ChatGPT usage request failed with HTTP %d", e.StatusCode)
}

func (a *Adapter) refreshCredential(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, mgmt.SubscriptionInspection, error) {
	directory, err := os.MkdirTemp("", "zentrola-codex-auth-")
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, errors.New("cannot create isolated Codex credential directory")
	}
	defer os.RemoveAll(directory)
	authPath := filepath.Join(directory, "auth.json")
	if err := os.WriteFile(authPath, raw, 0o600); err != nil {
		return nil, mgmt.SubscriptionInspection{}, errors.New("cannot stage Codex credential")
	}

	client, err := startClient(ctx, a.executable, directory, proxy)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	defer client.close()
	if err := client.initialize(ctx); err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	var account accountResponse
	if err := client.call(ctx, 2, "account/read", map[string]bool{"refreshToken": true}, &account); err != nil {
		return nil, mgmt.SubscriptionInspection{}, fmt.Errorf("Codex ChatGPT authentication failed: %w", err)
	}
	if account.Account == nil || account.Account.Type != "chatgpt" {
		return nil, mgmt.SubscriptionInspection{}, errors.New("Codex ChatGPT authentication failed")
	}
	updated, err := os.ReadFile(authPath)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, errors.New("cannot read refreshed Codex credential")
	}
	normalized, err := ensureLastRefresh(updated)
	clear(updated)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	updated = normalized
	inspection, err := a.Inspect(updated)
	if err != nil {
		clear(updated)
		return nil, mgmt.SubscriptionInspection{}, err
	}
	if account.Account.PlanType != "" {
		inspection.PlanCode = account.Account.PlanType
	}
	return updated, inspection, nil
}

// RefreshIfNeeded 在短期 access token 临近过期时交给官方 app-server 刷新。
// 返回的更新凭据仍是完整 auth.json，调用方必须立即加密保存并清除明文。
func (a *Adapter) RefreshIfNeeded(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, bool, error) {
	needed, err := a.NeedsRefresh(raw)
	if err != nil {
		return nil, false, err
	}
	if !needed {
		return nil, false, nil
	}
	updated, _, err := a.refreshCredential(ctx, raw, proxy)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func (a *Adapter) NeedsRefresh(raw []byte) (bool, error) {
	_, _, expiresAt, err := RequestCredential(raw)
	if err != nil {
		return false, err
	}
	if expiresAt != nil && expiresAt.After(time.Now().UTC().Add(a.refreshAhead)) {
		return false, nil
	}
	return true, nil
}

func (a *Adapter) RefreshBefore(now time.Time) time.Time {
	return now.UTC().Add(a.refreshAhead)
}

func (a *Adapter) CredentialRefreshMetadata(raw []byte) (refreshedAt, expiresAt *time.Time, err error) {
	inspection, err := a.Inspect(raw)
	if err != nil {
		return nil, nil, err
	}
	return inspection.CredentialRefreshedAt, inspection.CredentialExpiresAt, nil
}

func (a *Adapter) ExportCredential(raw []byte) ([]byte, error) {
	return ensureLastRefresh(raw)
}

type rpcClient struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
}

func startClient(ctx context.Context, executable, directory string, proxy *catalog.OutboundProxy) (*rpcClient, error) {
	command := exec.CommandContext(ctx, executable, "app-server", "--stdio", "-c", `cli_auth_credentials_store="file"`)
	environment := make([]string, 0, len(os.Environ())+1)
	for _, item := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(item), "CODEX_HOME=") {
			environment = append(environment, item)
		}
	}
	environment, err := provider.EnvironmentWithProxy(ctx, environment, proxy, provider.ProxyRequestLog{
		Operation: "subscription_refresh", ProviderCode: catalog.OpenAIOfficialCode, Protocol: "OPENAI",
	})
	if err != nil {
		return nil, errors.New("invalid ChatGPT proxy configuration")
	}
	command.Env = append(environment, "CODEX_HOME="+directory)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, errors.New("cannot open Codex app-server input")
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, errors.New("cannot open Codex app-server output")
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		stdin.Close()
		return nil, errors.New("Codex app-server is unavailable")
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	return &rpcClient{command: command, stdin: stdin, scanner: scanner}, nil
}

func (c *rpcClient) initialize(ctx context.Context) error {
	var result json.RawMessage
	if err := c.call(ctx, 1, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "zentrola", "version": "1"},
		"capabilities": map[string]any{},
	}, &result); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *rpcClient) notify(method string, params any) error {
	request := map[string]any{"method": method}
	if params != nil {
		request["params"] = params
	}
	return writeJSONLine(c.stdin, request)
}

func (c *rpcClient) call(ctx context.Context, id int64, method string, params any, target any) error {
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if err := writeJSONLine(c.stdin, request); err != nil {
		return errors.New("cannot write Codex app-server request")
	}
	for c.scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var response struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int64  `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(c.scanner.Bytes(), &response) != nil || response.ID == nil || *response.ID != id {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("Codex app-server request failed (%d): %s", response.Error.Code, response.Error.Message)
		}
		if target == nil {
			return nil
		}
		if err := json.Unmarshal(response.Result, target); err != nil {
			return errors.New("invalid Codex app-server response")
		}
		return nil
	}
	return errors.New("Codex app-server stopped unexpectedly")
}

func writeJSONLine(writer io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = writer.Write(encoded)
	clear(encoded)
	return err
}

func (c *rpcClient) close() {
	c.stdin.Close()
	done := make(chan struct{})
	go func() {
		_ = c.command.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = c.command.Process.Kill()
		<-done
	}
}

type accountResponse struct {
	Account *struct {
		Type     string `json:"type"`
		PlanType string `json:"planType"`
	} `json:"account"`
}

type usageResponse struct {
	AccountID            string                `json:"account_id"`
	PlanType             string                `json:"plan_type"`
	RateLimit            *directRateLimit      `json:"rate_limit"`
	AdditionalRateLimits []additionalRateLimit `json:"additional_rate_limits"`
	RateLimitReachedType *string               `json:"rate_limit_reached_type"`
}

type additionalRateLimit struct {
	LimitName      *string          `json:"limit_name"`
	MeteredFeature string           `json:"metered_feature"`
	RateLimit      *directRateLimit `json:"rate_limit"`
}

type directRateLimit struct {
	Allowed         *bool             `json:"allowed"`
	LimitReached    bool              `json:"limit_reached"`
	PrimaryWindow   *directRateWindow `json:"primary_window"`
	SecondaryWindow *directRateWindow `json:"secondary_window"`
}

type directRateWindow struct {
	UsedPercent        float64 `json:"used_percent"`
	LimitWindowSeconds *int64  `json:"limit_window_seconds"`
	ResetAfterSeconds  *int64  `json:"reset_after_seconds"`
	ResetAt            *int64  `json:"reset_at"`
}

func (r usageResponse) quotas(observedAt time.Time) []mgmt.ResourceQuota {
	quotas := make([]mgmt.ResourceQuota, 0, 2+len(r.AdditionalRateLimits)*2)
	quotas = appendRateLimitQuotas(quotas, "codex", nil, r.RateLimit, r.RateLimitReachedType, observedAt)
	for index, additional := range r.AdditionalRateLimits {
		code := strings.TrimSpace(additional.MeteredFeature)
		if code == "" {
			code = "additional." + strconv.Itoa(index+1)
		}
		quotas = appendRateLimitQuotas(quotas, code, additional.LimitName, additional.RateLimit, nil, observedAt)
	}
	return quotas
}

func appendRateLimitQuotas(quotas []mgmt.ResourceQuota, code string, name *string, limit *directRateLimit, reachedType *string, observedAt time.Time) []mgmt.ResourceQuota {
	if limit == nil {
		return quotas
	}
	if limit.PrimaryWindow != nil {
		quotas = append(quotas, windowQuota(code+".primary", name, limit.PrimaryWindow, limit, reachedType, observedAt))
	}
	if limit.SecondaryWindow != nil {
		quotas = append(quotas, windowQuota(code+".secondary", name, limit.SecondaryWindow, limit, reachedType, observedAt))
	}
	return quotas
}

func windowQuota(code string, name *string, window *directRateWindow, limit *directRateLimit, reachedType *string, observedAt time.Time) mgmt.ResourceQuota {
	percent := window.UsedPercent
	status := mgmt.QuotaAvailable
	if reachedType != nil || limit.LimitReached || (limit.Allowed != nil && !*limit.Allowed) || percent >= 100 {
		status = mgmt.QuotaExhausted
	} else if percent >= 90 {
		status = mgmt.QuotaNearLimit
	}
	var resetsAt *time.Time
	if window.ResetAt != nil {
		value := time.Unix(*window.ResetAt, 0).UTC()
		resetsAt = &value
	} else if window.ResetAfterSeconds != nil {
		value := observedAt.Add(time.Duration(*window.ResetAfterSeconds) * time.Second)
		resetsAt = &value
	}
	return mgmt.ResourceQuota{
		Code: code, Name: name, Status: status, UsedPercent: &percent,
		WindowDurationSeconds: window.LimitWindowSeconds, ResetsAt: resetsAt, ReachedType: reachedType,
		ObservedAt: observedAt,
	}
}
