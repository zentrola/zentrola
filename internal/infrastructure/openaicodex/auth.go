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
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	errAppServerUnavailable = errors.New("codex app-server is unavailable")
	errAppServerStopped     = errors.New("codex app-server stopped unexpectedly")
	errSessionBusy          = errors.New("codex app-server session is busy")
)

// rpcError 是 app-server 返回的 JSON-RPC 错误。app-server 没有为认证失败定义专用错误码，
// 因此 JSON-RPC 错误一律视为可重试，不根据错误文本推断永久失败。
type rpcError struct {
	Code    int64
	Message string
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("codex app-server request failed (%d): %s", e.Code, e.Message)
}

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
	sessionSlot  chan struct{}
	logger       *slog.Logger
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

// WithLogger 设置诊断日志输出；未设置时使用 slog.Default。
func WithLogger(logger *slog.Logger) Option {
	return func(adapter *Adapter) {
		if logger != nil {
			adapter.logger = logger
		}
	}
}

func New(executable string, options ...Option) *Adapter {
	if strings.TrimSpace(executable) == "" {
		executable = "codex"
	}
	adapter := &Adapter{
		executable: executable, refreshAhead: defaultRefreshAhead, sessionSlot: make(chan struct{}, 1),
		logger: slog.Default(),
	}
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
	// 永久失败只依据类型化错误判定：凭据格式无效，或携带永久错误码的 connectionFailure。
	if errors.Is(err, errInvalidCredential) {
		return "CREDENTIAL_INVALID", true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "UPSTREAM_TIMEOUT", false
	}
	if errors.Is(err, errAppServerUnavailable) {
		return "CODEX_APP_SERVER_UNAVAILABLE", false
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
	credential, inspection, err := a.tryRunIsolatedSession(ctx, raw, proxy, func(client accountRPC) error {
		return client.call(ctx, 2, "account/rateLimits/read", nil, &limits)
	})
	if err != nil {
		code := subscriptionErrorCode(err)
		a.logSessionFailure(ctx, "subscription_probe", "codex subscription probe failed", code, err)
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

// logSessionFailure 记录 app-server 会话失败的脱敏原因。额度检测与凭据刷新只向调用方返回错误码，
// 兜底码的具体原因只能从这里的 app-server 错误和标准错误摘要中排查。
func (a *Adapter) logSessionFailure(ctx context.Context, operation, message, code string, err error) {
	args := []any{
		slog.String("operation", operation),
		slog.String("auth_adapter", AdapterCode),
		slog.String("error_code", code),
		slog.String("error", errorForLog(err)),
	}
	var failure *sessionFailure
	if errors.As(err, &failure) && failure.stderr != "" {
		args = append(args, slog.String("app_server_stderr", failure.stderr))
	}
	a.logger.WarnContext(ctx, message, args...)
}

func subscriptionErrorCode(err error) string {
	switch {
	case errors.Is(err, errSessionBusy):
		return "SUBSCRIPTION_CHECK_BUSY"
	case errors.Is(err, errInvalidCredential):
		return "CREDENTIAL_INVALID"
	case errors.Is(err, context.DeadlineExceeded):
		return "UPSTREAM_TIMEOUT"
	case errors.Is(err, errAppServerUnavailable):
		return "CODEX_APP_SERVER_UNAVAILABLE"
	default:
		return "SUBSCRIPTION_UNAVAILABLE"
	}
}

func (a *Adapter) refreshCredential(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy) ([]byte, mgmt.SubscriptionInspection, error) {
	var account accountResponse
	updated, inspection, err := a.runIsolatedSession(ctx, raw, proxy, func(client accountRPC) error {
		return client.call(ctx, 2, "account/read", map[string]bool{"refreshToken": true}, &account)
	})
	if err != nil {
		code, _ := a.ClassifyRefreshError(err)
		a.logSessionFailure(ctx, "subscription_refresh", "codex subscription refresh failed", code, err)
		return nil, mgmt.SubscriptionInspection{}, &connectionFailure{
			code: code, cause: fmt.Errorf("codex ChatGPT credential refresh failed: %w", err),
		}
	}
	if account.Account == nil || account.Account.Type != "chatgpt" {
		// 响应结构不符可能来自 app-server 版本不兼容或暂时异常，不能据此判定凭据失效，按可重试处理。
		var cause error = &connectionFailure{
			code: "UPSTREAM_INVALID_RESPONSE", cause: errors.New("codex app-server returned an unexpected account response"),
		}
		if credentialRotated(raw, updated) {
			return nil, mgmt.SubscriptionInspection{}, &sessionFailure{cause: cause, credential: updated}
		}
		clear(updated)
		return nil, mgmt.SubscriptionInspection{}, cause
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
	defer func() {
		client.close()
		client.stderr.reset()
	}()
	// app-server 启动后可能在任意 RPC 之前就用 refresh token 换取新 Token 并写回 auth.json；
	// 会话失败时仍要读回轮换后的凭据，否则存储中只剩已被使用过的 refresh token。
	failed := func(err error) error {
		client.close()
		return newSessionFailure(err, client.stderr.summary(), rotatedCredential(authPath, raw))
	}
	if err := client.initialize(ctx); err != nil {
		if ctx.Err() != nil {
			// 超时或取消时保留 context 错误，使调用方归类为超时，而不是 app-server 不可用。
			return nil, mgmt.SubscriptionInspection{}, failed(fmt.Errorf("codex app-server initialization interrupted: %w", err))
		}
		return nil, mgmt.SubscriptionInspection{}, failed(fmt.Errorf("%w: initialization failed: %v", errAppServerUnavailable, err))
	}
	if err := action(client); err != nil {
		return nil, mgmt.SubscriptionInspection{}, failed(err)
	}
	// 成功路径同样先关闭并等待 app-server 退出再读取 auth.json，避免读到尚未写回或写入中的凭据。
	client.close()
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

// 同一个适配器实例同时服务后台刷新和管理员检测；限制 Codex App Server 会话并发，
// 避免多个订阅在同一后端进程内同时读取账号额度或刷新认证。
func (a *Adapter) runIsolatedSession(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
	if err := ctx.Err(); err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	select {
	case a.sessionSlot <- struct{}{}:
		defer func() { <-a.sessionSlot }()
	case <-ctx.Done():
		return nil, mgmt.SubscriptionInspection{}, ctx.Err()
	}
	return a.runSession(ctx, raw, proxy, action)
}

func (a *Adapter) tryRunIsolatedSession(ctx context.Context, raw []byte, proxy *catalog.OutboundProxy, action func(accountRPC) error) ([]byte, mgmt.SubscriptionInspection, error) {
	if err := ctx.Err(); err != nil {
		return nil, mgmt.SubscriptionInspection{}, err
	}
	select {
	case a.sessionSlot <- struct{}{}:
		defer func() { <-a.sessionSlot }()
	default:
		return nil, mgmt.SubscriptionInspection{}, errSessionBusy
	}
	return a.runSession(ctx, raw, proxy, action)
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
	credential, inspection, err := a.runIsolatedSession(ctx, raw, proxy, func(client accountRPC) error {
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

// appServerWaitDelay 是 app-server 退出或被终止后等待其输出管道关闭的上限，
// 防止遗留的后代进程持有管道导致 Wait 无限阻塞。
const appServerWaitDelay = 2 * time.Second

type rpcClient struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	// lines 由 readLines 协程写入 app-server 的每行输出，读取结束时关闭。
	lines     chan []byte
	done      chan struct{}
	stderr    *stderrTail
	tree      *processTree
	closeOnce sync.Once
}

func startClient(ctx context.Context, executable, directory string, proxy *catalog.OutboundProxy) (*rpcClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("codex app-server start interrupted: %w", err)
	}
	command := exec.CommandContext(ctx, executable, "app-server", "--stdio", "-c", `cli_auth_credentials_store="file"`)
	// 子进程只继承白名单环境变量，再叠加服务商代理与隔离的 CODEX_HOME。
	environment, err := provider.EnvironmentWithProxy(ctx, codexEnvironment(os.Environ()), proxy, provider.ProxyRequestLog{
		Operation: "subscription_refresh", ProviderCode: catalog.OpenAIOfficialCode, Protocol: "OPENAI",
	})
	if err != nil {
		return nil, errors.New("invalid ChatGPT proxy configuration")
	}
	command.Env = append(environment, "CODEX_HOME="+directory)
	tree, err := newProcessTree()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot create process group", errAppServerUnavailable)
	}
	tree.configure(command)
	// context 结束时终止整棵进程树，而不仅是直接子进程。
	command.Cancel = tree.kill
	command.WaitDelay = appServerWaitDelay
	stdin, err := command.StdinPipe()
	if err != nil {
		tree.release()
		return nil, fmt.Errorf("%w: cannot open input", errAppServerUnavailable)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		tree.release()
		return nil, fmt.Errorf("%w: cannot open output", errAppServerUnavailable)
	}
	stderr := &stderrTail{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		stdin.Close()
		tree.release()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("codex app-server start interrupted: %w", ctxErr)
		}
		return nil, fmt.Errorf("%w: cannot start process", errAppServerUnavailable)
	}
	// 加入进程树失败时仍可终止直接子进程，不阻止会话继续。
	_ = tree.attach()
	client := &rpcClient{
		command: command, stdin: stdin, lines: make(chan []byte), done: make(chan struct{}), stderr: stderr, tree: tree,
	}
	go client.readLines(stdout)
	return client, nil
}

// readLines 在独立协程中逐行读取 app-server 输出，使 call 能同时响应 context 结束。
func (c *rpcClient) readLines(stdout io.Reader) {
	defer close(c.lines)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := bytes.Clone(scanner.Bytes())
		select {
		case c.lines <- line:
		case <-c.done:
			clear(line)
			return
		}
	}
}

// rotatedCredential 读取会话结束后 app-server 留下的 auth.json，仅当同一账号的 Token 已被轮换时返回规范化后的新凭据。
// 调用前必须等待 app-server 退出，避免读到写入中的文件。
func rotatedCredential(authPath string, original []byte) []byte {
	updated, err := os.ReadFile(authPath)
	if err != nil {
		return nil
	}
	defer clear(updated)
	if !credentialRotated(original, updated) {
		return nil
	}
	normalized, err := ensureLastRefresh(updated)
	if err != nil {
		return nil
	}
	return normalized
}

func credentialRotated(original, updated []byte) bool {
	previous, err := parseCredential(original)
	if err != nil {
		return false
	}
	next, err := parseCredential(updated)
	if err != nil || next.Tokens.AccountID != previous.Tokens.AccountID {
		return false
	}
	return next.Tokens.RefreshToken != previous.Tokens.RefreshToken || next.Tokens.AccessToken != previous.Tokens.AccessToken
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
	if err := ctx.Err(); err != nil {
		return interruptedRequest(method, err)
	}
	request := map[string]any{"id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	if err := writeJSONLine(c.stdin, request); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return interruptedRequest(method, ctxErr)
		}
		return errors.New("cannot write Codex app-server request")
	}
	for {
		var line []byte
		select {
		case <-ctx.Done():
			return interruptedRequest(method, ctx.Err())
		case received, ok := <-c.lines:
			if !ok {
				// app-server 因 context 结束被终止时，优先返回 context 错误，以便归类为超时或取消。
				if err := ctx.Err(); err != nil {
					return interruptedRequest(method, err)
				}
				return errAppServerStopped
			}
			line = received
		}
		matched, err := decodeResponse(line, id, target)
		clear(line)
		if matched {
			return err
		}
	}
}

// interruptedRequest 包装 context 错误，调用方仍可用 errors.Is 识别超时或取消。
func interruptedRequest(method string, err error) error {
	return fmt.Errorf("codex app-server request %s interrupted: %w", method, err)
}

// decodeResponse 解析一行 app-server 输出；matched 为 false 表示该行不是本次请求的响应。
func decodeResponse(line []byte, id int64, target any) (matched bool, err error) {
	var response struct {
		ID     *int64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(line, &response) != nil || response.ID == nil || *response.ID != id {
		return false, nil
	}
	if response.Error != nil {
		return true, &rpcError{Code: response.Error.Code, Message: response.Error.Message}
	}
	if target == nil {
		return true, nil
	}
	if err := json.Unmarshal(response.Result, target); err != nil {
		return true, errors.New("invalid Codex app-server response")
	}
	return true, nil
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
	c.closeOnce.Do(func() {
		close(c.done)
		c.stdin.Close()
		done := make(chan struct{})
		go func() {
			_ = c.command.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = c.tree.kill()
			<-done
		}
		// app-server 退出后清理仍在运行的后代进程。
		c.tree.release()
	})
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
