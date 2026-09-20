package admin

import (
	"strings"
	"unicode/utf8"
)

const (
	PasswordMinCharacters  = 6
	PasswordMaxCharacters  = 30
	passwordLegacyMaxBytes = 72
)

// ValidNewPassword 校验初始化、修改和重置时采用的新密码规则。
func ValidNewPassword(value string) bool {
	if len(value) < PasswordMinCharacters || len(value) > PasswordMaxCharacters {
		return false
	}
	for _, character := range []byte(value) {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}

// ValidPasswordInput 接受新规则以及规则调整前可能存在的密码，用于登录和当前密码验证。
func ValidPasswordInput(value string) bool {
	return value != "" && len(value) <= passwordLegacyMaxBytes && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0)
}
