// Package quota 定义月度 Token 配额的稳定领域类型。
package quota

import "time"

type ScopeType string

const (
	Principal ScopeType = "PRINCIPAL"
	Group     ScopeType = "GROUP"
)

type Scope struct {
	Type  ScopeType `json:"scopeType"`
	ID    int64     `json:"scopeId,string"`
	Limit int64     `json:"monthlyTokenLimit,string"`
}

type Decision struct {
	Allowed bool
	Scope   *Scope
	Used    int64
}

type Status struct {
	ScopeType         ScopeType `json:"scopeType"`
	ScopeID           int64     `json:"scopeId,string"`
	MonthlyTokenLimit int64     `json:"monthlyTokenLimit,string"`
	UsedTokens        int64     `json:"usedTokens,string"`
	RemainingTokens   int64     `json:"remainingTokens,string"`
	UsedPercent       float64   `json:"usedPercent"`
	Level             string    `json:"level"`
	PeriodStart       time.Time `json:"periodStart"`
	PeriodEnd         time.Time `json:"periodEnd"`
}

func Period(at time.Time) (time.Time, time.Time) {
	utc := at.UTC()
	start := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func NewStatus(scope Scope, used int64, at time.Time) Status {
	start, end := Period(at)
	if scope.Limit <= 0 {
		return Status{
			ScopeType: scope.Type, ScopeID: scope.ID, MonthlyTokenLimit: scope.Limit,
			UsedTokens: used, RemainingTokens: 0, UsedPercent: 0,
			Level: "EXHAUSTED", PeriodStart: start, PeriodEnd: end,
		}
	}
	remaining := scope.Limit - used
	if remaining < 0 {
		remaining = 0
	}
	percent := float64(used) * 100 / float64(scope.Limit)
	level := "NORMAL"
	switch {
	case percent >= 100:
		level = "EXHAUSTED"
	case percent >= 80:
		level = "WARNING"
	case percent >= 50:
		level = "NOTICE"
	}
	return Status{
		ScopeType: scope.Type, ScopeID: scope.ID, MonthlyTokenLimit: scope.Limit,
		UsedTokens: used, RemainingTokens: remaining, UsedPercent: percent,
		Level: level, PeriodStart: start, PeriodEnd: end,
	}
}
