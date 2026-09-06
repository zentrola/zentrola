// Package principal 定义治理主体和调用凭证的机器语义。
package principal

type Type string

const (
	Member      Type = "MEMBER"
	Application Type = "APPLICATION" // 仅保留类型，不提供 MVP 创建或调用流程。
)

func (t Type) SupportedInMVP() bool { return t == Member }

type KeyStatus string

const (
	KeyActive   KeyStatus = "ACTIVE"
	KeyDisabled KeyStatus = "DISABLED"
	KeyRevoked  KeyStatus = "REVOKED"
)
