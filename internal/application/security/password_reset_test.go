package security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type passwordResetStoreFake struct {
	username string
	hash     string
	err      error
}

func (s *passwordResetStoreFake) ResetPassword(_ context.Context, username, hash string) error {
	s.username, s.hash = username, hash
	return s.err
}

func TestPasswordReset(t *testing.T) {
	store := &passwordResetStoreFake{}
	passwords := &passwordsFake{}
	service := NewPasswordReset(store, passwords)
	plain, err := service.Reset(context.Background(), "admin")
	if err != nil || plain == "" || store.username != "admin" || store.hash != "hashed:"+plain {
		t.Fatalf("unexpected reset result: plain=%q username=%q hash=%q err=%v", plain, store.username, store.hash, err)
	}
	if len(plain) < 20 || strings.ContainsAny(plain, " \t\r\n") {
		t.Fatalf("generated password is unexpectedly weak or malformed: %q", plain)
	}
	if _, err := service.Reset(context.Background(), " admin"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected invalid username rejection, got %v", err)
	}
	passwords.hashErr = errors.New("hash failed")
	if _, err := service.Reset(context.Background(), "admin"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected hash failure mapping, got %v", err)
	}
	passwords.hashErr = nil
	store.err = ErrNotFound
	if plain, err := service.Reset(context.Background(), "missing"); plain != "" || !errors.Is(err, ErrNotFound) {
		t.Fatalf("store failure must not return plaintext: plain=%q err=%v", plain, err)
	}
}
