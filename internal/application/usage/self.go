package usage

import (
	"context"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
)

// SelfUsage 是成员指定时间范围内的 Token 用量快照。
type SelfUsage struct {
	Tokens int64     `json:"tokens"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
}

// Self 查询已认证成员指定时间范围内的 Token 用量。
func (s *QueryService) Self(ctx context.Context, identity appsec.PrincipalIdentity, from, to time.Time) (SelfUsage, error) {
	if identity.ID <= 0 {
		return SelfUsage{}, appsec.ErrUnauthenticated
	}
	if from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return SelfUsage{}, appsec.ErrInvalidArgument
	}
	from, to = from.UTC(), to.UTC()
	tokens, err := s.store.TokenUsage(ctx, identity.ID, from, to)
	if err != nil {
		return SelfUsage{}, err
	}
	return SelfUsage{Tokens: tokens, From: from, To: to}, nil
}
