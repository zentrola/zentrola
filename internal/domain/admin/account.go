// Package admin 定义管理面身份和登录锁定规则。
package admin

import (
	"github.com/zentrola/zentrola/internal/domain/operation"
	"time"
)

type Identity struct {
	CredentialVersion int64  `json:"-"`
	ID                int64  `json:"id,string"`
	OrganizationID    int64  `json:"organizationId,string"`
	Username          string `json:"username"`
	DisplayName       string `json:"displayName"`
}

type Account struct {
	Identity
	PasswordHash string
	Active       bool
	FailedLogins int64
	LockedUntil  *time.Time
	LastLoginAt  *time.Time
}

type LoginPolicy struct {
	MaxFailures  int64
	LockDuration time.Duration
}
type LoginDecision struct {
	Allowed bool
	Event   operation.Type
	State   Account
}

func (a Account) Attempt(matches bool, now time.Time, policy LoginPolicy) LoginDecision {
	d := LoginDecision{State: a, Event: operation.LoginFailed}
	if a.LockedUntil != nil && now.Before(*a.LockedUntil) {
		d.Event = operation.LoginLocked
		return d
	}
	if !a.Active {
		return d
	}
	if a.LockedUntil != nil {
		d.State.LockedUntil = nil
		d.State.FailedLogins = 0
	}
	if matches {
		d.Allowed = true
		d.Event = operation.LoginSuccess
		d.State.FailedLogins = 0
		d.State.LockedUntil = nil
		d.State.LastLoginAt = &now
		return d
	}
	d.State.FailedLogins++
	if d.State.FailedLogins >= policy.MaxFailures {
		until := now.Add(policy.LockDuration)
		d.State.LockedUntil = &until
		d.Event = operation.LoginLocked
	}
	return d
}
