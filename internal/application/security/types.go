// Package security 编排管理面认证、调用凭证和相关审计。
package security

import (
	"context"
	"errors"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"time"
)

var (
	ErrUnauthenticated    = errors.New("authentication failed")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrNotFound           = errors.New("object not found")
	ErrUnavailable        = errors.New("security service unavailable")
	ErrAlreadyInitialized = errors.New("administrator already initialized")
)

// AccountLockedError 保留服务端锁定截止时间，供登录接口返回重试提示。
type AccountLockedError struct {
	LockedUntil time.Time
}

func (e *AccountLockedError) Error() string { return "account temporarily locked" }
func (e *AccountLockedError) Unwrap() error { return ErrUnauthenticated }

type RequestMeta struct{ RequestID, Method, Path, IP, UserAgent string }
type PrincipalIdentity struct{ ID, OrganizationID, AccessKeyID int64 }

type Passwords interface {
	Hash(string) (string, error)
	Verify(string, string) bool
	DummyVerify(string)
}
type Tokens interface {
	Issue(admin.Identity) (string, time.Time, error)
	Verify(string) (admin.Identity, error)
}
type AdminStore interface {
	Attempt(context.Context, string, RequestMeta, func(*admin.Account) admin.LoginDecision) (admin.Identity, error)
	GetActive(context.Context, int64, int64) (admin.Identity, error)
	EnsureInitial(context.Context, string, func() (string, error)) error
	CreateFirstAdmin(context.Context, string, func() (string, error), RequestMeta) error
	HasAnyAdmin(context.Context) (bool, error)
	HasAdmin(context.Context) (bool, error)
}

type KeyRecord struct {
	ID, OrganizationID, PrincipalID int64
	Hash                            []byte
	Prefix, Name                    string
	ExpiresAt                       *time.Time
	CreatedAt                       time.Time
}
type KeyStore interface {
	Create(context.Context, admin.Identity, KeyRecord, RequestMeta) error
	Revoke(context.Context, admin.Identity, int64, RequestMeta) error
	Authenticate(context.Context, []byte, time.Time) (PrincipalIdentity, error)
}
