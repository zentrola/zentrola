package catalog

import "encoding/binary"

// SealedCredential 只允许在应用内部传递，不能序列化进管理响应或审计快照。
type SealedCredential struct {
	Ciphertext, Nonce []byte
	KeyVersion        int32
}

type CredentialOwner struct{ ProviderID, ResourceID int64 }

func (o CredentialOwner) Valid() bool {
	return o.ProviderID > 0 && o.ResourceID > 0
}

// ProviderProxyOwner 为服务商代理秘密提供独立 AAD，避免与资源凭证混用。
type ProviderProxyOwner struct {
	ProviderID int64
	Field      string
}

func (o ProviderProxyOwner) Valid() bool {
	return o.ProviderID > 0 && (o.Field == "url" || o.Field == "headers")
}
func (o ProviderProxyOwner) AAD() []byte {
	data := []byte("zentrola:provider-proxy:v1:" + o.Field + ":")
	return binary.BigEndian.AppendUint64(data, uint64(o.ProviderID))
}

// OutboundProxy 仅在一次上游调用期间保存解密后的代理配置，不可序列化或记录日志。
type OutboundProxy struct {
	URL     string            `json:"-"`
	Headers map[string]string `json:"-"`
}

func (o CredentialOwner) AAD() []byte {
	data := []byte("zentrola:resource:v2:")
	data = binary.BigEndian.AppendUint64(data, uint64(o.ProviderID))
	return binary.BigEndian.AppendUint64(data, uint64(o.ResourceID))
}

// LegacyAAD 仅用于解密从 organization 架构迁移的 v1 资源凭据。
func (o CredentialOwner) LegacyAAD(organizationID int64) []byte {
	data := []byte("zentrola:resource:v1:")
	data = binary.BigEndian.AppendUint64(data, uint64(organizationID))
	data = binary.BigEndian.AppendUint64(data, uint64(o.ProviderID))
	return binary.BigEndian.AppendUint64(data, uint64(o.ResourceID))
}
