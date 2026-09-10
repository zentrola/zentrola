package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type SealedCredential = catalog.SealedCredential
type CredentialOwner = catalog.CredentialOwner
type Credentials struct {
	aead cipher.AEAD
}

func NewCredentials(master *MasterKey) (*Credentials, error) {
	if master == nil || len(master.value) != 32 {
		return nil, errors.New("invalid Master Key")
	}
	block, err := aes.NewCipher(master.value)
	if err != nil {
		return nil, errors.New("cannot initialize credential encryption")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("cannot initialize credential encryption")
	}
	return &Credentials{aead: aead}, nil
}
func (c *Credentials) Encrypt(plain []byte, owner CredentialOwner) (SealedCredential, error) {
	if len(plain) == 0 || !owner.Valid() {
		return SealedCredential{}, errors.New("credential and owner are required")
	}
	nonce := make([]byte, c.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return SealedCredential{Ciphertext: c.aead.Seal(nil, nonce, plain, owner.AAD()), Nonce: nonce, KeyVersion: 2}, nil
}
func (c *Credentials) Decrypt(sealed SealedCredential, owner CredentialOwner) ([]byte, error) {
	if len(sealed.Nonce) != c.aead.NonceSize() || !owner.Valid() {
		return nil, errors.New("credential unavailable")
	}
	ciphertext := sealed.Ciphertext
	var aad []byte
	switch sealed.KeyVersion {
	case 1:
		if len(ciphertext) <= 8 {
			return nil, errors.New("credential unavailable")
		}
		organizationID := int64(binary.BigEndian.Uint64(ciphertext[:8]))
		if organizationID <= 0 {
			return nil, errors.New("credential unavailable")
		}
		ciphertext = ciphertext[8:]
		aad = owner.LegacyAAD(organizationID)
	case 2:
		aad = owner.AAD()
	default:
		return nil, errors.New("credential unavailable")
	}
	plain, err := c.aead.Open(nil, sealed.Nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("credential unavailable")
	}
	return plain, nil
}

func (c *Credentials) EncryptProviderProxy(plain []byte, owner catalog.ProviderProxyOwner) (SealedCredential, error) {
	if len(plain) == 0 || !owner.Valid() {
		return SealedCredential{}, errors.New("provider proxy secret and owner are required")
	}
	nonce := make([]byte, c.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return SealedCredential{Ciphertext: c.aead.Seal(nil, nonce, plain, owner.AAD()), Nonce: nonce, KeyVersion: 1}, nil
}

func (c *Credentials) DecryptProviderProxy(sealed SealedCredential, owner catalog.ProviderProxyOwner) ([]byte, error) {
	if sealed.KeyVersion != 1 || len(sealed.Nonce) != c.aead.NonceSize() || !owner.Valid() {
		return nil, errors.New("provider proxy secret unavailable")
	}
	plain, err := c.aead.Open(nil, sealed.Nonce, sealed.Ciphertext, owner.AAD())
	if err != nil {
		return nil, errors.New("provider proxy secret unavailable")
	}
	return plain, nil
}
