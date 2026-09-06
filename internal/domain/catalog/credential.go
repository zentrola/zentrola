package catalog

import "encoding/binary"

// SealedCredential 只允许在应用内部传递，不能序列化进管理响应或审计快照。
type SealedCredential struct {
	Ciphertext, Nonce []byte
	KeyVersion        int32
}

type CredentialOwner struct{ OrganizationID, ProviderID, ResourceID int64 }

func (o CredentialOwner) Valid() bool {
	return o.OrganizationID > 0 && o.ProviderID > 0 && o.ResourceID > 0
}
func (o CredentialOwner) AAD() []byte {
	data := []byte("zentrola:resource:v1:")
	data = binary.BigEndian.AppendUint64(data, uint64(o.OrganizationID))
	data = binary.BigEndian.AppendUint64(data, uint64(o.ProviderID))
	return binary.BigEndian.AppendUint64(data, uint64(o.ResourceID))
}
