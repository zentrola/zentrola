package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

const adminIssuer = "zentrola"
const adminAudience = "zentrola-admin"

type AdminClaims struct {
	OrganizationID string `json:"organization_id"`
	Kind           string `json:"kind"`
	jwt.RegisteredClaims
}
type JWT struct {
	key []byte
	now func() time.Time
}

func NewJWT(encoded string) (*JWT, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) < 32 {
		return nil, errors.New("ADMIN_JWT_SECRET must be Base64 with at least 32 random bytes")
	}
	return &JWT{key: key, now: time.Now}, nil
}

func (j *JWT) ValidateIndependentMaster(master *MasterKey) error {
	if master == nil || len(master.value) != 32 || subtle.ConstantTimeCompare(j.key, master.value) == 1 {
		return errors.New("JWT signing key must be independent from Master Key")
	}
	return nil
}
func (j *JWT) Issue(identity admin.Identity) (string, time.Time, error) {
	if identity.ID <= 0 || identity.OrganizationID <= 0 {
		return "", time.Time{}, errors.New("invalid admin identity")
	}
	now := j.now().UTC().Truncate(time.Second)
	expires := now.Add(8 * time.Hour)
	var entropy [16]byte
	_, _ = rand.Read(entropy[:])
	claims := AdminClaims{OrganizationID: strconv.FormatInt(identity.OrganizationID, 10), Kind: "ADMIN", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: adminIssuer, Subject: strconv.FormatInt(identity.ID, 10), Audience: jwt.ClaimStrings{adminAudience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), ID: base64.RawURLEncoding.EncodeToString(entropy[:]),
	}}
	encoded, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.key)
	return encoded, expires, err
}
func (j *JWT) Verify(encoded string) (admin.Identity, error) {
	if len(encoded) > 4096 {
		return admin.Identity{}, errors.New("invalid admin token")
	}
	claims := new(AdminClaims)
	token, err := jwt.ParseWithClaims(encoded, claims, func(token *jwt.Token) (any, error) { return j.key, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(adminIssuer), jwt.WithAudience(adminAudience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(j.now))
	if err != nil || token == nil || !token.Valid || claims.Kind != "ADMIN" || claims.ID == "" || claims.IssuedAt == nil || claims.ExpiresAt == nil || !claims.ExpiresAt.After(claims.IssuedAt.Time) {
		return admin.Identity{}, errors.New("invalid admin token")
	}
	id, e1 := strconv.ParseInt(claims.Subject, 10, 64)
	org, e2 := strconv.ParseInt(claims.OrganizationID, 10, 64)
	if e1 != nil || e2 != nil || id <= 0 || org <= 0 {
		return admin.Identity{}, errors.New("invalid admin token")
	}
	return admin.Identity{ID: id, OrganizationID: org}, nil
}
