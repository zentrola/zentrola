package admin

import (
	"github.com/zentrola/zentrola/internal/domain/operation"
	"testing"
	"time"
)

func TestLockoutTransitions(t *testing.T) {
	now := time.Now().UTC()
	policy := LoginPolicy{MaxFailures: 5, LockDuration: 15 * time.Minute}
	a := Account{Active: true}
	for n := int64(1); n <= 5; n++ {
		d := a.Attempt(false, now, policy)
		a = d.State
		if a.FailedLogins != n || d.Allowed {
			t.Fatal("failure count incorrect")
		}
	}
	if a.LockedUntil == nil || !a.LockedUntil.Equal(now.Add(15*time.Minute)) {
		t.Fatal("lock deadline incorrect")
	}
	if d := a.Attempt(true, now, policy); d.Allowed || d.Event != operation.LoginLocked {
		t.Fatal("correct password bypassed lock")
	}
	if d := a.Attempt(false, *a.LockedUntil, policy); d.State.FailedLogins != 1 || d.State.LockedUntil != nil {
		t.Fatal("expired lock did not reset failure window")
	}
	if d := a.Attempt(true, *a.LockedUntil, policy); !d.Allowed || d.State.FailedLogins != 0 || d.State.LockedUntil != nil || d.State.LastLoginAt == nil {
		t.Fatal("success did not clear failure state")
	}
}
