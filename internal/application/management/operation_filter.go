package management

import (
	"regexp"
	"strings"
	"unicode/utf8"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// maxOperationKeywordLength 是操作日志关键字的最大字符数（按 Unicode 字符计）。
const maxOperationKeywordLength = 100

var operationTypePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,47}$`)

// OperationFilter 是操作日志的服务端过滤条件，零值表示不过滤。
// Keyword 对操作人名称、操作类型、目标类型、目标名称、目标 ID、请求 ID 和结果做不区分大小写的包含匹配；
// Type 精确匹配操作类型；Result 只能是 SUCCESS 或 FAILED。
type OperationFilter struct {
	Keyword string
	Type    string
	Result  string
}

// NewOperationFilter 规范化并校验查询参数：关键字去除首尾空白后最多 100 个字符，
// 操作类型须为大写下划线编码，结果只接受 SUCCESS 或 FAILED。
func NewOperationFilter(keyword, operationType, result string) (OperationFilter, error) {
	filter := OperationFilter{
		Keyword: strings.TrimSpace(keyword),
		Type:    strings.TrimSpace(operationType),
		Result:  strings.TrimSpace(result),
	}
	if !filter.valid() {
		return OperationFilter{}, appsec.ErrInvalidArgument
	}
	return filter, nil
}

func (f OperationFilter) valid() bool {
	if f.Keyword != strings.TrimSpace(f.Keyword) || !utf8.ValidString(f.Keyword) ||
		utf8.RuneCountInString(f.Keyword) > maxOperationKeywordLength || strings.ContainsRune(f.Keyword, 0) {
		return false
	}
	if f.Type != "" && !operationTypePattern.MatchString(f.Type) {
		return false
	}
	return f.Result == "" || f.Result == "SUCCESS" || f.Result == "FAILED"
}
