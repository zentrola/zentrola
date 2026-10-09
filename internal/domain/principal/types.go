// Package principal 定义治理主体和调用凭证的机器语义。
package principal

type Type string

const (
	Member      Type = "MEMBER"
	Application Type = "APPLICATION"
)

func (t Type) CanUseGateway() bool { return t == Member || t == Application }

type KeyStatus string

const (
	KeyActive   KeyStatus = "ACTIVE"
	KeyDisabled KeyStatus = "DISABLED"
	KeyRevoked  KeyStatus = "REVOKED"
)
