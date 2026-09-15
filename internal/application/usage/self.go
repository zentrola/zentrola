package usage

import (
	"context"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// SelfUsage 是成员当前 UTC 自然月的 Token 用量快照。
type SelfUsage struct {
	Tokens int64     `json:"tokens"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

// Self 查询已认证成员当前 UTC 自然月的 Token 用量。
func (s *QueryService) Self(ctx context.Context, identity appsec.PrincipalIdentity, now time.Time) (SelfUsage, error) {
	if identity.ID <= 0 {
		return SelfUsage{}, appsec.ErrUnauthenticated
	}
	if now.IsZero() {
		return SelfUsage{}, appsec.ErrInvalidArgument
	}
	now = now.UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	tokens, err := s.store.TokenUsage(ctx, identity.ID, from, now)
	if err != nil {
		return SelfUsage{}, err
	}
	return SelfUsage{Tokens: tokens, From: from, To: now}, nil
}
