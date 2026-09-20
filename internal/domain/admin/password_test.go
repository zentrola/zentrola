package admin

import (
	"strings"
	"testing"
)

func TestValidNewPasswordPolicy(t *testing.T) {
	for _, value := range []string{"123456", "letters-AND_symbols!", strings.Repeat("a", 30)} {
		if !ValidNewPassword(value) {
			t.Fatalf("valid password rejected: %q", value)
		}
	}
	for _, value := range []string{"12345", strings.Repeat("a", 31), "12345\x00", "密码长度六位", "abc 123"} {
		if ValidNewPassword(value) {
			t.Fatalf("invalid password accepted: %q", value)
		}
	}
}
