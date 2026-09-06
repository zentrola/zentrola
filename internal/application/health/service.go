// Package health 编排依赖就绪检查，不感知 HTTP 或具体数据库实现。
package health

import (
	"context"
	"errors"
)

var ErrPending = errors.New("not initialized")

type Check struct {
	Name string
	Run  func(context.Context) error
}

type Report struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

type Service struct {
	checks []Check
}

func New(checks ...Check) *Service {
	return &Service{checks: append([]Check(nil), checks...)}
}

// Pending 用于尚未实现的初始化依赖，禁止把占位检查报告为成功。
func Pending(context.Context) error { return ErrPending }

func (s *Service) Ready(ctx context.Context) Report {
	report := Report{Status: "READY", Checks: make(map[string]string, len(s.checks))}
	for _, check := range s.checks {
		err := check.Run(ctx)
		switch {
		case err == nil:
			report.Checks[check.Name] = "READY"
		case errors.Is(err, ErrPending):
			report.Checks[check.Name] = "NOT_INITIALIZED"
			report.Status = "NOT_READY"
		default:
			// 不向调用者暴露 DSN、网络拓扑或底层错误中的敏感内容。
			report.Checks[check.Name] = "UNAVAILABLE"
			report.Status = "NOT_READY"
		}
	}
	return report
}
