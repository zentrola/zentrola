package security

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/admin"
)

type keyTestStore struct {
	row      KeyRecord
	lookups  int
	identity PrincipalIdentity
}

func (s *keyTestStore) Create(_ context.Context, _ admin.Identity, row KeyRecord, _ RequestMeta) error {
	s.row = row
	return nil
}
func (s *keyTestStore) Revoke(context.Context, admin.Identity, int64, RequestMeta) error { return nil }
func (s *keyTestStore) Authenticate(_ context.Context, digest []byte, _ time.Time) (PrincipalIdentity, error) {
	s.lookups++
	if !bytes.Equal(digest, s.row.Hash) {
		return PrincipalIdentity{}, ErrUnauthenticated
	}
	return s.identity, nil
}

type keyTestIDs struct{}

func (keyTestIDs) NextID() (int64, error) { return 123, nil }

func TestCreateVirtualKeyFormatAndAuthentication(t *testing.T) {
	store := &keyTestStore{identity: PrincipalIdentity{ID: 3, OrganizationID: 2, AccessKeyID: 123}}
	keys := NewKeys(store, keyTestIDs{})
	created, err := keys.Create(context.Background(), admin.Identity{ID: 1, OrganizationID: 2}, 3, "工作站", nil, RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Key, "vk-") || len(created.Key) != 46 {
		t.Fatal("unexpected virtual key format")
	}
	entropy, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(created.Key, "vk-"))
	if err != nil || len(entropy) != 32 {
		t.Fatal("virtual key must retain 32 random bytes")
	}
	wantMaskedKey := created.Key[:11] + "********" + created.Key[len(created.Key)-4:]
	if created.MaskedKey != wantMaskedKey || store.row.MaskedKey != created.MaskedKey {
		t.Fatal("unexpected masked key")
	}
	digest := sha256.Sum256([]byte(created.Key))
	if !bytes.Equal(store.row.Hash, digest[:]) {
		t.Fatal("stored hash must cover the complete key")
	}
	identity, err := keys.Authenticate(context.Background(), created.Key)
	if err != nil || identity != store.identity {
		t.Fatal("issued key did not authenticate")
	}
	if _, err := keys.Authenticate(context.Background(), created.MaskedKey); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("masked key must not authenticate")
	}
}

func TestLegacyVirtualKeyRemainsValidWithoutPrefixConversion(t *testing.T) {
	suffix := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xAB}, 32))
	full := "zt_vk_" + suffix
	digest := sha256.Sum256([]byte(full))
	store := &keyTestStore{row: KeyRecord{Hash: digest[:]}, identity: PrincipalIdentity{ID: 3}}
	keys := NewKeys(store, keyTestIDs{})
	if identity, err := keys.Authenticate(context.Background(), full); err != nil || identity != store.identity {
		t.Fatal("legacy key did not authenticate")
	}
	if _, err := keys.Authenticate(context.Background(), "vk-"+suffix); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("renaming a legacy key must not authenticate")
	}
}

func TestMalformedVirtualKeysDoNotQueryStore(t *testing.T) {
	store := &keyTestStore{}
	keys := NewKeys(store, keyTestIDs{})
	for _, full := range []string{"", "vk-123456", "zt_vk_123456", "sk-" + strings.Repeat("a", 43), "vk-" + strings.Repeat("!", 43), "zt_vk_" + strings.Repeat("!", 43), "vk-" + strings.Repeat("a", 44)} {
		if _, err := keys.Authenticate(context.Background(), full); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("malformed key must be rejected")
		}
	}
	if store.lookups != 0 {
		t.Fatal("malformed keys must not reach the store")
	}
}
