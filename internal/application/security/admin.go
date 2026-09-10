package security

import (
	"context"
	"errors"
	"github.com/zentrola/zentrola/internal/application/health"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type AdminService struct {
	store     AdminStore
	passwords Passwords
	tokens    Tokens
	policy    admin.LoginPolicy
	now       func() time.Time
}
type LoginResult struct {
	Token     string    `json:"token"`
	TokenType string    `json:"tokenType"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func NewAdmin(store AdminStore, passwords Passwords, tokens Tokens, policy admin.LoginPolicy) *AdminService {
	return &AdminService{store: store, passwords: passwords, tokens: tokens, policy: policy, now: func() time.Time { return time.Now().UTC() }}
}

func (s *AdminService) Bootstrap(ctx context.Context, username, password string) error {
	return s.store.EnsureInitial(ctx, username, s.initialPassword(username, password))
}

type SetupStatus struct {
	Required bool `json:"required"`
}

func (s *AdminService) SetupStatus(ctx context.Context) (SetupStatus, error) {
	exists, err := s.store.HasAnyAdmin(ctx)
	if err != nil {
		return SetupStatus{}, ErrUnavailable
	}
	return SetupStatus{Required: !exists}, nil
}

func (s *AdminService) Initialize(ctx context.Context, username, password string, meta RequestMeta) error {
	return s.store.CreateFirstAdmin(ctx, username, s.initialPassword(username, password), meta)
}

func (s *AdminService) initialPassword(username, password string) func() (string, error) {
	return func() (string, error) {
		if len(username) == 0 || len(username) > 64 || !utf8.ValidString(username) || strings.TrimSpace(username) != username ||
			strings.IndexFunc(username, unicode.IsControl) >= 0 || !utf8.ValidString(password) || utf8.RuneCountInString(password) < 6 || len(password) > 72 || strings.ContainsRune(password, 0) {
			return "", ErrInvalidArgument
		}
		return s.passwords.Hash(password)
	}
}

func (s *AdminService) Check(ctx context.Context) error {
	ok, err := s.store.HasAdmin(ctx)
	if err != nil {
		return err
	}
	if !ok {
		exists, err := s.store.HasAnyAdmin(ctx)
		if err != nil {
			return ErrUnavailable
		}
		if !exists {
			return health.ErrPending
		}
		return ErrUnavailable
	}
	return nil
}

func (s *AdminService) Login(ctx context.Context, username, password string, meta RequestMeta) (LoginResult, error) {
	identity, err := s.verifyCredentials(ctx, username, password, meta)
	if err != nil {
		return LoginResult{}, err
	}
	token, expires, err := s.tokens.Issue(identity)
	if err != nil {
		return LoginResult{}, ErrUnavailable
	}
	return LoginResult{Token: token, TokenType: "Bearer", ExpiresAt: expires}, nil
}

func (s *AdminService) verifyCredentials(ctx context.Context, username, password string, meta RequestMeta) (admin.Identity, error) {
	if len(username) > 64 || strings.TrimSpace(username) == "" || strings.ContainsRune(username, 0) {
		s.passwords.DummyVerify("invalid input")
		return admin.Identity{}, ErrUnauthenticated
	}
	return s.store.Attempt(ctx, username, meta, func(account *admin.Account) admin.LoginDecision {
		if account == nil {
			s.passwords.DummyVerify(password)
			return admin.LoginDecision{}
		}
		matches := false
		if len(password) > 72 {
			s.passwords.DummyVerify("invalid input")
		} else {
			matches = s.passwords.Verify(account.PasswordHash, password)
		}
		return account.Attempt(matches, s.now(), s.policy)
	})
}

func (s *AdminService) ChangePassword(ctx context.Context, actor admin.Identity, current, next string, meta RequestMeta) error {
	if current == "" || current == next || !utf8.ValidString(next) || utf8.RuneCountInString(next) < 6 || len(next) > 72 || strings.ContainsRune(next, 0) {
		return ErrInvalidArgument
	}
	// 复用登录失败计数与锁定策略，避免通过修改密码接口无限猜测当前密码。
	verified, err := s.verifyCredentials(ctx, actor.Username, current, meta)
	if err != nil {
		var locked *AccountLockedError
		if errors.Is(err, ErrUnauthenticated) && !errors.As(err, &locked) {
			return ErrCurrentPassword
		}
		return err
	}
	if verified.ID != actor.ID || verified.CredentialVersion != actor.CredentialVersion {
		return ErrUnauthenticated
	}
	hash, err := s.passwords.Hash(next)
	if err != nil {
		return ErrUnavailable
	}
	return s.store.ChangePassword(ctx, actor, hash, meta)
}

func (s *AdminService) Authenticate(ctx context.Context, token string) (admin.Identity, error) {
	claims, err := s.tokens.Verify(token)
	if err != nil {
		return admin.Identity{}, ErrUnauthenticated
	}
	identity, err := s.store.GetActive(ctx, claims.ID)
	if err != nil {
		return admin.Identity{}, err
	}
	if identity.CredentialVersion != claims.CredentialVersion {
		return admin.Identity{}, ErrUnauthenticated
	}
	return identity, nil
}
