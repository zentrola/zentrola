package openaicodex

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

const (
	// app-server 标准错误输出只保留末尾片段，诊断所需的失败原因通常出现在最后几行。
	maxStderrCapture  = 16 << 10
	maxStderrLogRunes = 2048
	maxStderrLogLines = 20
	maxErrorLogRunes  = 512
	redactedValue     = "******"
)

// stderrTail 只保留 app-server 标准错误输出的末尾片段，避免异常进程持续输出时占用无界内存。
type stderrTail struct {
	mu        sync.Mutex
	buf       []byte
	truncated bool
}

func (t *stderrTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - maxStderrCapture; over > 0 {
		kept := copy(t.buf, t.buf[over:])
		clear(t.buf[kept:])
		t.buf = t.buf[:kept]
		t.truncated = true
	}
	return len(p), nil
}

// summary 返回脱敏后的末尾若干行；被截断时丢弃首个不完整行，避免残缺的凭据片段绕过脱敏规则。
func (t *stderrTail) summary() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	text := string(t.buf)
	if t.truncated {
		if index := strings.IndexByte(text, '\n'); index >= 0 {
			text = text[index+1:]
		} else {
			text = ""
		}
	}
	lines := strings.Split(sanitizeDiagnostic(text), "\n")
	kept := make([]string, 0, maxStderrLogLines)
	for index := len(lines) - 1; index >= 0 && len(kept) < maxStderrLogLines; index-- {
		if line := strings.TrimSpace(lines[index]); line != "" {
			kept = append(kept, line)
		}
	}
	for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
		kept[left], kept[right] = kept[right], kept[left]
	}
	result := strings.Join(kept, "\n")
	if runes := []rune(result); len(runes) > maxStderrLogRunes {
		result = "…" + string(runes[len(runes)-maxStderrLogRunes:])
	}
	return result
}

func (t *stderrTail) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	clear(t.buf)
	t.buf, t.truncated = nil, false
}

// sessionFailure 为 app-server 会话错误附加脱敏后的标准错误摘要，以及会话失败前已被轮换的凭据。
// Error 保持原始错误文本，不影响基于错误链和错误文本的失败分类。
type sessionFailure struct {
	cause      error
	stderr     string
	credential []byte
}

func newSessionFailure(cause error, stderr string, credential []byte) error {
	if stderr == "" && len(credential) == 0 {
		return cause
	}
	return &sessionFailure{cause: cause, stderr: stderr, credential: credential}
}

func (e *sessionFailure) Error() string { return e.cause.Error() }
func (e *sessionFailure) Unwrap() error { return e.cause }

// RotatedCredential 返回会话失败前 app-server 已轮换的完整 auth.json。refresh token 只能使用一次，
// 轮换后旧凭据已失效，调用方必须立即加密保存并清除明文。
func (e *sessionFailure) RotatedCredential() []byte { return e.credential }

var (
	ansiEscapePattern   = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	urlUserinfoPattern  = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@]+@`)
	urlQueryPattern     = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^\s?#"'<>]*)[?#][^\s"'<>]*`)
	authSchemePattern   = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[^\s"',;]+`)
	secretFieldPattern  = regexp.MustCompile(`(?i)((?:access|refresh|id)_?token|api_?key|client_?secret|secret|password|passwd|authorization|cookie|session_?id)("?\s*[:=]\s*"?)[^\s"',;&}]+`)
	jwtPattern          = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]*`)
	openAIKeyPattern    = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`)
	opaqueTokenPattern  = regexp.MustCompile(`[A-Za-z0-9_-]{40,}`)
	emailAddressPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

// sanitizeDiagnostic 移除终端控制序列并脱敏凭据、URL 用户信息与查询参数、邮箱等可能出现在
// app-server 错误或日志中的敏感内容，只保留可用于判断失败原因的文本。
func sanitizeDiagnostic(text string) string {
	text = strings.ToValidUTF8(text, "")
	text = ansiEscapePattern.ReplaceAllString(text, "")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	text = urlUserinfoPattern.ReplaceAllString(text, "${1}"+redactedValue+"@")
	text = urlQueryPattern.ReplaceAllString(text, "${1}?"+redactedValue)
	text = authSchemePattern.ReplaceAllString(text, "${1} "+redactedValue)
	text = secretFieldPattern.ReplaceAllString(text, "${1}${2}"+redactedValue)
	text = jwtPattern.ReplaceAllString(text, redactedValue)
	text = openAIKeyPattern.ReplaceAllString(text, redactedValue)
	text = opaqueTokenPattern.ReplaceAllStringFunc(text, func(match string) string {
		// 只脱敏同时包含字母和数字的长串，保留较长的普通标识符。
		if strings.ContainsAny(match, "0123456789") && strings.IndexFunc(match, unicode.IsLetter) >= 0 {
			return redactedValue
		}
		return match
	})
	return emailAddressPattern.ReplaceAllString(text, redactedValue)
}

// errorForLog 返回脱敏并截断后的单行错误文本。
func errorForLog(err error) string {
	text := strings.Join(strings.Fields(sanitizeDiagnostic(err.Error())), " ")
	if runes := []rune(text); len(runes) > maxErrorLogRunes {
		text = string(runes[:maxErrorLogRunes]) + "…"
	}
	return text
}
