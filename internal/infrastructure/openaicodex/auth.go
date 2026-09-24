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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const (
	AdapterCode         = mgmt.AuthAdapterOpenAICodex
	defaultRefreshAhead = 30 * time.Minute
)

var (
	errInvalidCredential    = errors.New("invalid Codex ChatGPT credential")
	errAppServerUnavailable = errors.New("Codex app-server is unavailable")
)

type connectionFailure struct {
	code  string
	cause error
}

func (e *connectionFailure) Error() string          { return e.cause.Error() }
func (e *connectionFailure) Unwrap() error          { return e.cause }
func (e *connectionFailure) ConnectionCode() string { return e.code }

type Adapter struct {
	executable   string
	refreshAhead time.Duration
	runSession   accountSessionRunner
}

type accountRPC interface {
	call(context.Context, int64, string, any, any) error
}

type accountSessionRunner func(context.Context, []byte, *catalog.OutboundProxy, func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error)

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
	adapter := &Adapter{executable: executable, refreshAhead: defaultRefreshAhead}
	adapter.runSession = adapter.runAccountSession
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

func (a *Adapter) ClassifyRefreshError(err error) (string, bool) {
	if err == nil {
		return "SUBSCRIPTION_REFRESH_FAILED", false
	}
	var failure *connectionFailure
	if errors.As(err, &failure) && failure.code != "" {
		return failure.code, permanentRefreshCode(failure.code)
	}
	if errors.Is(err, errInvalidCredential) {
		return "CREDENTIAL_INVALID", true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "UPSTREAM_TIMEOUT", false
	}
	if errors.Is(err, errAppServerUnavailable) {
		return "CODEX_APP_SERVER_UNAVAILABLE", false
	}
	if strings.Contains(strings.ToLower(err.Error()), "authentication failed") {
		return "UPSTREAM_AUTH_FAILED", true
	}
	return "SUBSCRIPTION_REFRESH_FAILED", false
}

func permanentRefreshCode(code string) bool {
	switch code {
	case "CREDENTIAL_INVALID", "CREDENTIAL_REVOKED", "CREDENTIAL_UNRECOVERABLE",
		"UPSTREAM_AUTH_FAILED", "UPSTREAM_BILLING_BLOCKED", "SUBSCRIPTION_EXPIRED",
		"UPSTREAM_ACCOUNT_SUSPENDED":
		return true
	default:
		return false
	}
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
	var limits rateLimitsResponse
	credential, inspection, err := a.runSession(ctx, raw, proxy, func(client accountRPC) error {
		return client.call(ctx, 2, "account/rateLimits/read", nil, &limits)
	})
	if err != nil {
		code := "SUBSCRIPTION_UNAVAILABLE"
		switch {
		case errors.Is(err, errInvalidCredential):
			code = "CREDENTIAL_INVALID"
		case errors.Is(err, errAppServerUnavailable):
			code = "CODEX_APP_SERVER_UNAVAILABLE"
		case errors.Is(err, context.DeadlineExceeded):
			code = "UPSTREAM_TIMEOUT"
		}
		return mgmt.SubscriptionProbe{}, &connectionFailure{
			code: code, cause: fmt.Errorf("cannot read Codex ChatGPT rate limits: %w", err),
		}
	}
	if plan := limits.planType(); plan != "" {
		inspection.PlanCode = plan
	}
	observedAt := time.Now().UTC().Truncate(time.Microsecond)
	return mgmt.SubscriptionProbe{
		Inspection:   inspection,
		Credential:   credential,
		Quotas:       limits.quotas(observedAt),
		ResetCredits: limits.resetCredits(),
	}, nil
}

func (a *Adapter) refreshCredential(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, mgmt.SubscriptionInspection, error) {
	var account accountResponse
	updated, inspection, err := a.runSession(ctx, raw, proxy, func(client accountRPC) error {
		return client.call(ctx, 2, "account/read", map[string]bool{"refreshToken": true}, &account)
	})
	if err != nil {
		code, _ := a.ClassifyRefreshError(err)
		return nil, mgmt.SubscriptionInspection{}, &connectionFailure{
			code: code, cause: fmt.Errorf("Codex ChatGPT authentication failed: %w", err),
		}
	}
	if account.Account == nil || account.Account.Type != "chatgpt" {
		clear(updated)
		return nil, mgmt.SubscriptionInspection{}, errors.New("Codex ChatGPT authentication failed")
	}
	if account.Account.PlanType != "" {
		inspection.PlanCode = account.Account.PlanType
	}
	return updated, inspection, nil
}

func (a *Adapter) runAccountSession(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
	directory, err := os.MkdirTemp("", "zentrola-codex-auth-")
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, fmt.Errorf("%w: cannot create isolated credential directory", errAppServerUnavailable)
	}
	defer os.RemoveAll(directory)
	authPath := filepath.Join(directory, "auth.json")
	normalized, err := ensureLastRefresh(raw)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	if err := os.WriteFile(authPath, normalized, 0o600); err != nil {
		clear(normalized)
		return nil, mgmt.SubscriptionInspection{}, fmt.Errorf("%w: cannot stage credential", errAppServerUnavailable)
	}
	clear(normalized)

	client, err := startClient(ctx, a.executable, directory, proxy)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	defer client.close()
	if err := client.initialize(ctx); err != nil {
		return nil, mgmt.SubscriptionInspection{}, fmt.Errorf("%w: initialization failed: %v", errAppServerUnavailable, err)
	}
	if err := action(client); err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	updated, err := os.ReadFile(authPath)
	if err != nil {
		return nil, mgmt.SubscriptionInspection{}, fmt.Errorf("%w: cannot read refreshed credential", errAppServerUnavailable)
	}
	normalized, err = ensureLastRefresh(updated)
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
	return updated, inspection, nil
}

func (a *Adapter) ConsumeResetCredit(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy, idempotencyKey, creditID string) (mgmt.ResetCreditConsume, error) {
	if strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(creditID) != creditID {
		return mgmt.ResetCreditConsume{}, errors.New("invalid rate-limit reset request")
	}
	params := map[string]string{"idempotencyKey": idempotencyKey}
	if creditID != "" {
		params["creditId"] = creditID
	}
	var consumed struct {
		Outcome string `json:"outcome"`
	}
	var limits rateLimitsResponse
	credential, inspection, err := a.runSession(ctx, raw, proxy, func(client accountRPC) error {
		if err := client.call(ctx, 2, "account/rateLimitResetCredit/consume", params, &consumed); err != nil {
			return err
		}
		return client.call(ctx, 3, "account/rateLimits/read", nil, &limits)
	})
	if err != nil {
		return mgmt.ResetCreditConsume{}, fmt.Errorf("cannot consume Codex ChatGPT rate-limit reset: %w", err)
	}
	if consumed.Outcome != "reset" && consumed.Outcome != "alreadyRedeemed" && consumed.Outcome != "nothingToReset" && consumed.Outcome != "noCredit" {
		clear(credential)
		return mgmt.ResetCreditConsume{}, errors.New("invalid Codex ChatGPT rate-limit reset response")
	}
	if plan := limits.planType(); plan != "" {
		inspection.PlanCode = plan
	}
	observedAt := time.Now().UTC().Truncate(time.Microsecond)
	return mgmt.ResetCreditConsume{Outcome: consumed.Outcome, Probe: mgmt.SubscriptionProbe{
		Inspection:   inspection,
		Credential:   credential,
		Quotas:       limits.quotas(observedAt),
		ResetCredits: limits.resetCredits(),
	}}, nil
}

// RefreshIfNeeded 在短期 access token 临近过期时交给官方 app-server 刷新。
// 返回的更新凭据仍是完整 auth.json，调用方必须立即加密保存并清除明文。
func (a *Adapter) RefreshIfNeeded(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, bool, error) {
	needed, err := a.NeedsRefresh(raw)
	if err != nil {
		code, _ := a.ClassifyRefreshError(err)
		return nil, false, &connectionFailure{code: code, cause: err}
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
		return nil, fmt.Errorf("%w: cannot open input", errAppServerUnavailable)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("%w: cannot open output", errAppServerUnavailable)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("%w: cannot start process", errAppServerUnavailable)
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

type rateLimitsResponse struct {
	RateLimits            *codexRateLimit            `json:"rateLimits"`
	RateLimitsByLimitID   map[string]codexRateLimit  `json:"rateLimitsByLimitId"`
	RateLimitResetCredits *rateLimitResetCreditsWire `json:"rateLimitResetCredits"`
}

type codexRateLimit struct {
	LimitID              string           `json:"limitId"`
	LimitName            *string          `json:"limitName"`
	PlanType             string           `json:"planType"`
	Primary              *rateLimitWindow `json:"primary"`
	Secondary            *rateLimitWindow `json:"secondary"`
	RateLimitReachedType *string          `json:"rateLimitReachedType"`
}

type rateLimitWindow struct {
	UsedPercent        float64 `json:"usedPercent"`
	WindowDurationMins *int64  `json:"windowDurationMins"`
	ResetsAt           *int64  `json:"resetsAt"`
}

type rateLimitResetCreditsWire struct {
	AvailableCount int                        `json:"availableCount"`
	Credits        []rateLimitResetCreditWire `json:"credits"`
}

type rateLimitResetCreditWire struct {
	ID          string  `json:"id"`
	ResetType   string  `json:"resetType"`
	Status      string  `json:"status"`
	GrantedAt   int64   `json:"grantedAt"`
	ExpiresAt   *int64  `json:"expiresAt"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

func (r rateLimitsResponse) orderedLimits() []codexRateLimit {
	if len(r.RateLimitsByLimitID) == 0 {
		if r.RateLimits == nil {
			return nil
		}
		return []codexRateLimit{*r.RateLimits}
	}
	keys := make([]string, 0, len(r.RateLimitsByLimitID))
	for key := range r.RateLimitsByLimitID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	limits := make([]codexRateLimit, 0, len(keys))
	for _, key := range keys {
		limit := r.RateLimitsByLimitID[key]
		if strings.TrimSpace(limit.LimitID) == "" {
			limit.LimitID = key
		}
		limits = append(limits, limit)
	}
	return limits
}

func (r rateLimitsResponse) planType() string {
	for _, limit := range r.orderedLimits() {
		if strings.TrimSpace(limit.PlanType) != "" {
			return strings.TrimSpace(limit.PlanType)
		}
	}
	return ""
}

func (r rateLimitsResponse) quotas(observedAt time.Time) []mgmt.ResourceQuota {
	limits := r.orderedLimits()
	quotas := make([]mgmt.ResourceQuota, 0, len(limits)*2)
	for index, limit := range limits {
		code := strings.TrimSpace(limit.LimitID)
		if code == "" {
			code = "codex"
			if index > 0 {
				code += "." + strconv.Itoa(index+1)
			}
		}
		if limit.Primary != nil {
			quotas = append(quotas, officialWindowQuota(code+".primary", limit.LimitName, limit.Primary, limit.RateLimitReachedType, observedAt))
		}
		if limit.Secondary != nil {
			quotas = append(quotas, officialWindowQuota(code+".secondary", limit.LimitName, limit.Secondary, limit.RateLimitReachedType, observedAt))
		}
	}
	return quotas
}

func officialWindowQuota(code string, name *string, window *rateLimitWindow, reachedType *string, observedAt time.Time) mgmt.ResourceQuota {
	percent := window.UsedPercent
	status := mgmt.QuotaAvailable
	if reachedType != nil || percent >= 100 {
		status = mgmt.QuotaExhausted
	} else if percent >= 90 {
		status = mgmt.QuotaNearLimit
	}
	var durationSeconds *int64
	if window.WindowDurationMins != nil && *window.WindowDurationMins >= 0 && *window.WindowDurationMins <= (1<<63-1)/60 {
		value := *window.WindowDurationMins * 60
		durationSeconds = &value
	}
	var resetsAt *time.Time
	if window.ResetsAt != nil && *window.ResetsAt > 0 {
		value := time.Unix(*window.ResetsAt, 0).UTC()
		resetsAt = &value
	}
	return mgmt.ResourceQuota{
		Code: code, Name: name, Status: status, UsedPercent: &percent,
		WindowDurationSeconds: durationSeconds, ResetsAt: resetsAt, ReachedType: reachedType,
		ObservedAt: observedAt,
	}
}

func (r rateLimitsResponse) resetCredits() *mgmt.RateLimitResetCredits {
	if r.RateLimitResetCredits == nil {
		return nil
	}
	result := &mgmt.RateLimitResetCredits{
		AvailableCount: r.RateLimitResetCredits.AvailableCount,
		Credits:        make([]mgmt.RateLimitResetCredit, 0, len(r.RateLimitResetCredits.Credits)),
	}
	for _, credit := range r.RateLimitResetCredits.Credits {
		if strings.TrimSpace(credit.ID) == "" {
			continue
		}
		var expiresAt *time.Time
		if credit.ExpiresAt != nil && *credit.ExpiresAt > 0 {
			value := time.Unix(*credit.ExpiresAt, 0).UTC()
			expiresAt = &value
		}
		result.Credits = append(result.Credits, mgmt.RateLimitResetCredit{
			ID: credit.ID, ResetType: credit.ResetType, Status: credit.Status,
			GrantedAt: time.Unix(credit.GrantedAt, 0).UTC(), ExpiresAt: expiresAt,
			Title: credit.Title, Description: credit.Description,
		})
	}
	return result
}
