package security

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/health"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type adminStoreFake struct {
	account                *admin.Account
	attemptErr             error
	active                 admin.Identity
	activeErr              error
	hasAdmin, hasAny       bool
	hasAdminErr, hasAnyErr error
	changeErr              error
	changedHash            string
	initialUsername        string
	initialHash            string
	initialErr             error
}

func (s *adminStoreFake) ChangePassword(_ context.Context, _ admin.Identity, hash string, _ RequestMeta) error {
	s.changedHash = hash
	return s.changeErr
}
func (s *adminStoreFake) Attempt(_ context.Context, _ string, _ RequestMeta, evaluate func(*admin.Account) admin.LoginDecision) (admin.Identity, error) {
	decision := evaluate(s.account)
	if s.attemptErr != nil {
		return admin.Identity{}, s.attemptErr
	}
	if !decision.Allowed {
		if decision.State.LockedUntil != nil {
			return admin.Identity{}, &AccountLockedError{LockedUntil: *decision.State.LockedUntil}
		}
		return admin.Identity{}, ErrUnauthenticated
	}
	return decision.State.Identity, nil
}
func (s *adminStoreFake) GetActive(context.Context, int64) (admin.Identity, error) {
	return s.active, s.activeErr
}
func (s *adminStoreFake) EnsureInitial(_ context.Context, username string, hash func() (string, error)) error {
	return s.captureInitial(username, hash)
}
func (s *adminStoreFake) CreateFirstAdmin(_ context.Context, username string, hash func() (string, error), _ RequestMeta) error {
	return s.captureInitial(username, hash)
}
func (s *adminStoreFake) captureInitial(username string, hash func() (string, error)) error {
	s.initialUsername = username
	value, err := hash()
	s.initialHash = value
	if err != nil {
		return err
	}
	return s.initialErr
}
func (s *adminStoreFake) HasAnyAdmin(context.Context) (bool, error) { return s.hasAny, s.hasAnyErr }
func (s *adminStoreFake) HasAdmin(context.Context) (bool, error)    { return s.hasAdmin, s.hasAdminErr }

type passwordsFake struct {
	hashErr   error
	verify    bool
	dummyRuns int
}

func (p *passwordsFake) Hash(value string) (string, error) {
	if p.hashErr != nil {
		return "", p.hashErr
	}
	return "hashed:" + value, nil
}
func (p *passwordsFake) Verify(string, string) bool { return p.verify }
func (p *passwordsFake) DummyVerify(string)         { p.dummyRuns++ }

type tokensFake struct {
	issued    string
	expires   time.Time
	issueErr  error
	verified  admin.Identity
	verifyErr error
}

func (t *tokensFake) Issue(admin.Identity) (string, time.Time, error) {
	return t.issued, t.expires, t.issueErr
}
func (t *tokensFake) Verify(string) (admin.Identity, error) { return t.verified, t.verifyErr }

func newAdminTestService(store *adminStoreFake, passwords *passwordsFake, tokens *tokensFake) *AdminService {
	service := NewAdmin(store, passwords, tokens, admin.LoginPolicy{MaxFailures: 2, LockDuration: time.Minute})
	service.now = func() time.Time { return time.Date(2026, 10, 2, 3, 4, 5, 0, time.UTC) }
	return service
}

func TestAdminInitializationAndSetupStatus(t *testing.T) {
	ctx := context.Background()
	store := &adminStoreFake{}
	passwords := &passwordsFake{}
	service := newAdminTestService(store, passwords, &tokensFake{})

	if status, err := service.SetupStatus(ctx); err != nil || !status.Required {
		t.Fatalf("expected setup to be required: status=%+v err=%v", status, err)
	}
	store.hasAny = true
	if status, err := service.SetupStatus(ctx); err != nil || status.Required {
		t.Fatalf("expected setup to be complete: status=%+v err=%v", status, err)
	}
	store.hasAnyErr = errors.New("database failed")
	if _, err := service.SetupStatus(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	store.hasAnyErr = nil
	if err := service.Bootstrap(ctx, "admin", "secret1"); err != nil {
		t.Fatal(err)
	}
	if store.initialUsername != "admin" || store.initialHash != "hashed:secret1" {
		t.Fatalf("unexpected initial admin: %q %q", store.initialUsername, store.initialHash)
	}
	if err := service.Initialize(ctx, " admin", "secret1", RequestMeta{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid username rejection, got %v", err)
	}
	passwords.hashErr = errors.New("hash failed")
	if err := service.Initialize(ctx, "admin", "secret1", RequestMeta{}); err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected password implementation error to be preserved, got %v", err)
	}
}

func TestAdminReadinessStates(t *testing.T) {
	ctx := context.Background()
	store := &adminStoreFake{hasAdmin: true}
	service := newAdminTestService(store, &passwordsFake{}, &tokensFake{})
	if err := service.Check(ctx); err != nil {
		t.Fatal(err)
	}
	store.hasAdmin = false
	if err := service.Check(ctx); !errors.Is(err, health.ErrPending) {
		t.Fatalf("expected pending, got %v", err)
	}
	store.hasAny = true
	if err := service.Check(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
	store.hasAdminErr = errors.New("query failed")
	if err := service.Check(ctx); err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected store error to be preserved, got %v", err)
	}
}

func TestAdminLoginAndLocking(t *testing.T) {
	ctx := context.Background()
	identity := admin.Identity{ID: 7, Username: "admin", CredentialVersion: 3}
	store := &adminStoreFake{account: &admin.Account{Identity: identity, PasswordHash: "hash", Active: true}}
	passwords := &passwordsFake{verify: true}
	tokens := &tokensFake{issued: "token", expires: time.Date(2026, 10, 2, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))}
	service := newAdminTestService(store, passwords, tokens)

	result, err := service.Login(ctx, "admin", "secret1", RequestMeta{})
	if err != nil || result.Token != "token" || result.TokenType != "Bearer" || result.ExpiresAt.Location() != time.UTC {
		t.Fatalf("unexpected login result: %+v err=%v", result, err)
	}
	tokens.issueErr = errors.New("sign failed")
	if _, err := service.Login(ctx, "admin", "secret1", RequestMeta{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected token issue failure, got %v", err)
	}

	passwords.verify = false
	store.account.FailedLogins = 1
	if _, err := service.Login(ctx, "admin", "wrong", RequestMeta{}); err == nil {
		t.Fatal("expected locked login failure")
	} else {
		var locked *AccountLockedError
		if !errors.As(err, &locked) || !locked.LockedUntil.Equal(service.now().Add(time.Minute)) {
			t.Fatalf("unexpected lock result: %v", err)
		}
	}
	store.account = nil
	if _, err := service.Login(ctx, "missing", "wrong", RequestMeta{}); !errors.Is(err, ErrUnauthenticated) || passwords.dummyRuns == 0 {
		t.Fatalf("unknown account did not perform dummy verification: err=%v runs=%d", err, passwords.dummyRuns)
	}
	if _, err := service.Login(ctx, " ", "wrong", RequestMeta{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("invalid username was not rejected: %v", err)
	}
}

func TestChangePasswordAndTokenVersion(t *testing.T) {
	ctx := context.Background()
	identity := admin.Identity{ID: 7, Username: "admin", CredentialVersion: 3}
	store := &adminStoreFake{account: &admin.Account{Identity: identity, PasswordHash: "hash", Active: true}, active: identity}
	passwords := &passwordsFake{verify: true}
	tokens := &tokensFake{verified: identity}
	service := newAdminTestService(store, passwords, tokens)

	if err := service.ChangePassword(ctx, identity, "secret1", "secret2", RequestMeta{}); err != nil || store.changedHash != "hashed:secret2" {
		t.Fatalf("unexpected password change: hash=%q err=%v", store.changedHash, err)
	}
	if err := service.ChangePassword(ctx, identity, "secret1", "secret1", RequestMeta{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid password change, got %v", err)
	}
	passwords.verify = false
	if err := service.ChangePassword(ctx, identity, "wrong", "secret2", RequestMeta{}); !errors.Is(err, ErrCurrentPassword) {
		t.Fatalf("expected current password error, got %v", err)
	}

	passwords.verify = true
	if authenticated, err := service.Authenticate(ctx, "token"); err != nil || authenticated != identity {
		t.Fatalf("unexpected authentication: %+v err=%v", authenticated, err)
	}
	store.active.CredentialVersion++
	if _, err := service.Authenticate(ctx, "token"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected stale token rejection, got %v", err)
	}
	tokens.verifyErr = errors.New("bad signature")
	if _, err := service.Authenticate(ctx, "token"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected invalid token rejection, got %v", err)
	}
}
