// Package anthropicclaude 管理 Claude Code 个人订阅的长期 OAuth Token。
package anthropicclaude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const (
	AdapterCode = mgmt.AuthAdapterClaudeCode
	// APIBaseURL 是订阅 OAuth Token 唯一允许的推理主机；长期 Token 不随服务商端点配置发往其他地址。
	APIBaseURL     = "https://api.anthropic.com"
	OAuthBeta      = "oauth-2025-04-20"
	credentialKind = "claude_code_setup_token"
)

// 订阅 Token 只接受带 Claude Code 身份的推理请求，缺少系统提示时上游返回 429 rate_limit_error。
const (
	ClaudeCodeBeta         = "claude-code-20250219"
	ClaudeCodeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."
	// DefaultClaudeCodeVersion 是未配置 CLAUDE_CODE_VERSION 时声明的客户端版本。上游开始拒绝旧版本时，
	// 先通过配置更新版本，再在下次发版时同步此默认值。
	DefaultClaudeCodeVersion = "2.1.293"
)

// claudeCodeVersionPattern 与 config 包对 CLAUDE_CODE_VERSION 的校验保持一致。
var claudeCodeVersionPattern = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,6}$`)

// ValidClaudeCodeVersion 判断版本号是否为 x.y.z 形式，避免把任意文本写入 User-Agent。
func ValidClaudeCodeVersion(version string) bool {
	return claudeCodeVersionPattern.MatchString(version)
}

// ClaudeCodeUserAgent 返回订阅请求声明的 User-Agent；版本为空或无效时使用默认版本。
func ClaudeCodeUserAgent(version string) string {
	if !ValidClaudeCodeVersion(version) {
		version = DefaultClaudeCodeVersion
	}
	return "claude-cli/" + version + " (external, cli)"
}

const maxUpstreamErrorMessage = 300

// UpstreamError 从 Anthropic 错误响应中提取 error.type 与截断后的 error.message，仅用于诊断日志；
// 不返回原始响应体，响应无法解析时返回空值。
func UpstreamError(body []byte) (errorType, message string) {
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return "", ""
	}
	message = strings.ToValidUTF8(strings.TrimSpace(payload.Error.Message), "")
	if runes := []rune(message); len(runes) > maxUpstreamErrorMessage {
		message = string(runes[:maxUpstreamErrorMessage]) + "…"
	}
	return strings.TrimSpace(payload.Error.Type), message
}

var (
	errInvalidCredential = errors.New("invalid Claude Code setup token")
	errProbeUnsupported  = errors.New("setup token cannot read Claude subscription usage")
)

type storedCredential struct {
	Kind        string `json:"kind"`
	AccessToken string `json:"access_token"`
}

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

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

func (a *Adapter) Inspect(raw []byte) (mgmt.SubscriptionInspection, error) {
	if _, err := parseCredential(raw); err != nil {
		return mgmt.SubscriptionInspection{}, err
	}
	return mgmt.SubscriptionInspection{}, nil
}

// quotaStatus 把订阅窗口的已用百分比换算为额度状态。
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

// Probe 仅为满足 SubscriptionAdapter 接口而保留。setup-token 只有 user:inference 权限，
// 额度接口 /api/oauth/usage 会以 403 oauth_scope_insufficient 拒绝（要求 user:profile），
// 因此 Claude 订阅的连接测试走推理探测，不会调用此方法。
func (a *Adapter) Probe(context.Context, []byte, *catalog.OutboundProxy) (mgmt.SubscriptionProbe, error) {
	return mgmt.SubscriptionProbe{}, errProbeUnsupported
}

// setup-token 是长期 OAuth Token，没有可供第三方调用的公开刷新接口。
func (a *Adapter) RefreshIfNeeded(context.Context, []byte, *catalog.OutboundProxy) ([]byte, bool, error) {
	return nil, false, nil
}

func (a *Adapter) NeedsRefresh(raw []byte) (bool, error) {
	_, err := parseCredential(raw)
	return false, err
}
