package security

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestLoadMasterKey(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	master, err := LoadMasterKey(" \n" + encoded + "\n")
	if err != nil || !bytes.Equal(master.value, bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("valid MASTER_KEY was not loaded", err)
	}
	if _, err := LoadMasterKey(""); err == nil || !strings.Contains(err.Error(), "MASTER_KEY is required") {
		t.Fatal("missing MASTER_KEY was accepted", err)
	}
	if _, err := LoadMasterKey("invalid-secret"); err == nil || strings.Contains(err.Error(), "invalid-secret") {
		t.Fatal("invalid key leaked or was accepted", err)
	}
}
func TestCredentialAEAD(t *testing.T) {
	master, _ := decodeMaster(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	c, err := NewCredentials(master)
	if err != nil {
		t.Fatal(err)
	}
	owner := CredentialOwner{ProviderID: 2, ResourceID: 3}
	sealed, err := c.Encrypt([]byte("provider-secret"), owner)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := c.Decrypt(sealed, owner)
	if err != nil || string(plain) != "provider-secret" || sealed.KeyVersion != 2 {
		t.Fatal("round trip failed", err)
	}
	other, _ := decodeMaster(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	wrong, _ := NewCredentials(other)
	if _, err := wrong.Decrypt(sealed, owner); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := c.Decrypt(sealed, CredentialOwner{ProviderID: 2, ResourceID: 4}); err == nil {
		t.Fatal("ciphertext could be moved to a different resource")
	}
	legacyNonce := bytes.Repeat([]byte{9}, c.aead.NonceSize())
	legacyCiphertext := binary.BigEndian.AppendUint64(nil, 91)
	legacyCiphertext = append(legacyCiphertext, c.aead.Seal(nil, legacyNonce, []byte("legacy-provider-secret"), owner.LegacyAAD(91))...)
	legacy, err := c.Decrypt(SealedCredential{Ciphertext: legacyCiphertext, Nonce: legacyNonce, KeyVersion: 1}, owner)
	if err != nil || string(legacy) != "legacy-provider-secret" {
		t.Fatal("migrated v1 credential could not be decrypted", err)
	}
	second, _ := c.Encrypt([]byte("provider-secret"), owner)
	if bytes.Equal(sealed.Nonce, second.Nonce) || len(sealed.Nonce) != 12 {
		t.Fatal("nonce not fresh")
	}
	sealed.Ciphertext[0] ^= 1
	if _, err := c.Decrypt(sealed, owner); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	sealed.Nonce = []byte{1}
	if _, err := c.Decrypt(sealed, owner); err == nil {
		t.Fatal("invalid nonce accepted")
	}
	second.KeyVersion = 3
	if _, err := c.Decrypt(second, owner); err == nil {
		t.Fatal("unknown key version accepted")
	}
}

func TestProviderProxyAEADUsesIndependentOwner(t *testing.T) {
	master, _ := decodeMaster(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	c, _ := NewCredentials(master)
	owner := catalog.ProviderProxyOwner{ProviderID: 81, Field: "url"}
	secret := []byte("http://user:password@proxy.example.com:8080")
	sealed, err := c.EncryptProviderProxy(secret, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed.Ciphertext, []byte("password")) {
		t.Fatal("proxy password was stored in plaintext")
	}
	plain, err := c.DecryptProviderProxy(sealed, owner)
	if err != nil || !bytes.Equal(plain, secret) {
		t.Fatal("provider proxy round trip failed", err)
	}
	if _, err := c.DecryptProviderProxy(sealed, catalog.ProviderProxyOwner{ProviderID: 82, Field: "url"}); err == nil {
		t.Fatal("proxy ciphertext could be moved to another provider")
	}
	if _, err := c.DecryptProviderProxy(sealed, catalog.ProviderProxyOwner{ProviderID: 81, Field: "headers"}); err == nil {
		t.Fatal("proxy URL ciphertext could be reused as proxy headers")
	}
}
func TestJWTValidation(t *testing.T) {
	j, _ := NewJWT(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	if err := j.ValidateIndependentMaster(&MasterKey{value: bytes.Repeat([]byte{3}, 32)}); err == nil {
		t.Fatal("shared JWT and Master Key accepted")
	}
	if err := j.ValidateIndependentMaster(&MasterKey{value: bytes.Repeat([]byte{4}, 32)}); err != nil {
		t.Fatal("independent keys rejected")
	}
	now := time.Now().UTC().Truncate(time.Second)
	j.now = func() time.Time { return now }
	full, expires, err := j.Issue(admin.Identity{ID: 101, CredentialVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := j.Verify(full)
	if err != nil || identity.ID != 101 || identity.CredentialVersion != 3 {
		t.Fatal("JWT round trip failed")
	}
	if !expires.Equal(now.Add(8 * time.Hour)) {
		t.Fatal("unexpected JWT TTL")
	}
	j.now = func() time.Time { return expires }
	if _, err := j.Verify(full); err == nil {
		t.Fatal("expired JWT accepted")
	}
	j.now = func() time.Time { return now }
	claims := AdminClaims{Kind: "ADMIN", RegisteredClaims: jwt.RegisteredClaims{Issuer: adminIssuer, Subject: "101", Audience: jwt.ClaimStrings{adminAudience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), ID: "jti"}}
	// 升级前签发的 Token 没有凭证版本，兼容为 0；账号首次重置后不再匹配。
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"organization_id": "201", "kind": "ADMIN", "iss": adminIssuer, "sub": "101",
		"aud": adminAudience, "iat": now.Unix(), "exp": expires.Unix(), "jti": "legacy",
	}).SignedString(j.key)
	if err != nil {
		t.Fatal(err)
	}
	if identity, err := j.Verify(legacy); err != nil || identity.CredentialVersion != 0 {
		t.Fatal("legacy token version compatibility failed")
	}
	for _, test := range []struct {
		name   string
		mutate func(*AdminClaims)
		method jwt.SigningMethod
	}{
		{"wrong audience", func(c *AdminClaims) { c.Audience = jwt.ClaimStrings{"gateway"} }, jwt.SigningMethodHS256},
		{"wrong issuer", func(c *AdminClaims) { c.Issuer = "other" }, jwt.SigningMethodHS256},
		{"missing expiry", func(c *AdminClaims) { c.ExpiresAt = nil }, jwt.SigningMethodHS256},
		{"missing issued at", func(c *AdminClaims) { c.IssuedAt = nil }, jwt.SigningMethodHS256},
		{"missing jti", func(c *AdminClaims) { c.ID = "" }, jwt.SigningMethodHS256},
		{"wrong kind", func(c *AdminClaims) { c.Kind = "PRINCIPAL" }, jwt.SigningMethodHS256},
		{"negative credential version", func(c *AdminClaims) { c.CredentialVersion = -1 }, jwt.SigningMethodHS256},
		{"future issuance", func(c *AdminClaims) { c.IssuedAt = jwt.NewNumericDate(now.Add(time.Hour)) }, jwt.SigningMethodHS256},
		{"unexpected algorithm", func(c *AdminClaims) {}, jwt.SigningMethodHS384},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := claims
			test.mutate(&c)
			token, _ := jwt.NewWithClaims(test.method, c).SignedString(j.key)
			if _, err := j.Verify(token); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	other, _ := NewJWT(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)))
	if _, err := other.Verify(full); err == nil {
		t.Fatal("wrong signing key accepted")
	}
	if _, err := j.Verify("zt_vk_not-a-jwt"); err == nil {
		t.Fatal("Access Key accepted as JWT")
	}
}
func TestPasswordHash(t *testing.T) {
	p, err := NewPasswords(4)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := p.Hash("test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Verify(hash, "test-password-123") || p.Verify(hash, "wrong-password") {
		t.Fatal("password verification failed")
	}
	second, _ := p.Hash("test-password-123")
	if hash == second {
		t.Fatal("password salt was reused")
	}
	if _, err := p.Hash(strings.Repeat("a", 73)); err == nil {
		t.Fatal("bcrypt length limit ignored")
	}
	for _, password := range []string{"12345", "密码五位啊"} {
		if _, err := p.Hash(password); err == nil {
			t.Fatalf("short password accepted: %q", password)
		}
	}
	for _, password := range []string{"123456", "密码长度六位"} {
		if _, err := p.Hash(password); err != nil {
			t.Fatalf("six-character password rejected: %q: %v", password, err)
		}
	}
}
