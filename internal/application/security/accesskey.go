package security

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/shared"
	"strings"
	"time"
)

const (
	keyMarker       = "vk-"
	legacyKeyMarker = "zt_vk_"
	keyPrefixChars  = 8
	keySuffixChars  = 4
	keyMask         = "********"
)

type Keys struct {
	store KeyStore
	ids   shared.IDGenerator
}
type CreatedKey struct {
	ID        int64      `json:"id,string"`
	Key       string     `json:"key"`
	MaskedKey string     `json:"maskedKey"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func NewKeys(store KeyStore, ids shared.IDGenerator) *Keys { return &Keys{store: store, ids: ids} }

func (s *Keys) Create(ctx context.Context, actor admin.Identity, principalID int64, name string, expires *time.Time, meta RequestMeta) (CreatedKey, error) {
	now := time.Now().UTC()
	if principalID <= 0 || strings.TrimSpace(name) == "" || strings.ContainsRune(name, 0) || len(name) > 128 || (expires != nil && !expires.After(now)) {
		return CreatedKey{}, ErrInvalidArgument
	}
	var entropy [32]byte
	_, _ = rand.Read(entropy[:])
	full := keyMarker + base64.RawURLEncoding.EncodeToString(entropy[:])
	digest := sha256.Sum256([]byte(full))
	id, err := s.ids.NextID()
	if err != nil {
		return CreatedKey{}, ErrUnavailable
	}
	maskedKey := full[:len(keyMarker)+keyPrefixChars] + keyMask + full[len(full)-keySuffixChars:]
	row := KeyRecord{ID: id, OrganizationID: actor.OrganizationID, PrincipalID: principalID, Hash: digest[:], MaskedKey: maskedKey, Name: name, ExpiresAt: expires, CreatedAt: now}
	if err := s.store.Create(ctx, actor, row, meta); err != nil {
		return CreatedKey{}, err
	}
	// 完整值仅存在于本次创建返回值，不传给 Repository 或操作日志。
	return CreatedKey{ID: id, Key: full, MaskedKey: row.MaskedKey, Name: name, ExpiresAt: expires}, nil
}
func (s *Keys) Revoke(ctx context.Context, actor admin.Identity, keyID int64, meta RequestMeta) error {
	if keyID <= 0 {
		return ErrInvalidArgument
	}
	return s.store.Revoke(ctx, actor, keyID, meta)
}
func (s *Keys) Authenticate(ctx context.Context, full string) (PrincipalIdentity, error) {
	marker := keyMarker
	// 已签发的旧密钥继续按原文摘要认证，不修改现有凭证。
	if strings.HasPrefix(full, legacyKeyMarker) {
		marker = legacyKeyMarker
	}
	if !strings.HasPrefix(full, marker) || len(full) != len(marker)+43 {
		return PrincipalIdentity{}, ErrUnauthenticated
	}
	if raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(full, marker)); err != nil || len(raw) != 32 {
		return PrincipalIdentity{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(full))
	return s.store.Authenticate(ctx, digest[:], time.Now().UTC())
}
