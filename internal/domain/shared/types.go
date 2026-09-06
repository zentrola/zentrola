// Package shared 只保留当前各领域共用的基础类型，不建设通用业务框架。
package shared

type Status string

const (
	Active   Status = "ACTIVE"
	Disabled Status = "DISABLED"
)

func (s Status) Valid() bool { return s == Active || s == Disabled }

// IDGenerator 由应用依赖，基础设施提供正数 int64 ID 实现。
type IDGenerator interface{ NextID() (int64, error) }
