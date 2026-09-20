// Package security 提供密码 Hash、签名令牌和根密钥的标准算法实现。
package security

import (
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/zentrola/zentrola/internal/domain/admin"
	"golang.org/x/crypto/bcrypt"
)

type Passwords struct {
	cost  int
	dummy string
}

func NewPasswords(cost int) (*Passwords, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, errors.New("invalid bcrypt cost")
	}
	var buf [32]byte
	_, _ = rand.Read(buf[:])
	dummy, err := bcrypt.GenerateFromPassword([]byte(base64.RawURLEncoding.EncodeToString(buf[:])), cost)
	if err != nil {
		return nil, errors.New("cannot initialize password verifier")
	}
	return &Passwords{cost: cost, dummy: string(dummy)}, nil
}
func (p *Passwords) Hash(password string) (string, error) {
	if !admin.ValidNewPassword(password) {
		return "", errors.New("password must contain 6 to 30 visible ASCII characters")
	}
	value, err := bcrypt.GenerateFromPassword([]byte(password), p.cost)
	if err != nil {
		return "", errors.New("cannot hash password")
	}
	return string(value), nil
}
func (p *Passwords) Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
func (p *Passwords) DummyVerify(password string) {
	_ = p.Verify(p.dummy, password)
}
