package security

import (
	"context"
	"crypto/rand"
	"strings"
	"unicode"
	"unicode/utf8"
)

type PasswordResetStore interface {
	ResetPassword(context.Context, string, string) error
}

type PasswordReset struct {
	store     PasswordResetStore
	passwords Passwords
}

func NewPasswordReset(store PasswordResetStore, passwords Passwords) *PasswordReset {
	return &PasswordReset{store: store, passwords: passwords}
}

func ValidAdminUsername(username string) bool {
	return len(username) > 0 && len(username) <= 64 && utf8.ValidString(username) &&
		strings.TrimSpace(username) == username && strings.IndexFunc(username, unicode.IsControl) < 0
}

// Reset 仅在密码与审计事务提交成功后返回明文，调用方不得记录它。
func (s *PasswordReset) Reset(ctx context.Context, username string) (string, error) {
	if !ValidAdminUsername(username) {
		return "", ErrInvalidArgument
	}
	password := rand.Text()
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return "", ErrUnavailable
	}
	if err := s.store.ResetPassword(ctx, username, hash); err != nil {
		return "", err
	}
	return password, nil
}
