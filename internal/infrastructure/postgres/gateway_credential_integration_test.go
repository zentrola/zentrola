package postgres

import (
	"bytes"
	"errors"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestGatewayCredentialWriteRejectsStaleCredential(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	store := NewGatewayStore(pool)
	old := catalog.SealedCredential{Ciphertext: bytes.Repeat([]byte{1}, 32), Nonce: bytes.Repeat([]byte{2}, 12), KeyVersion: 1}
	admin := catalog.SealedCredential{Ciphertext: bytes.Repeat([]byte{3}, 32), Nonce: bytes.Repeat([]byte{4}, 12), KeyVersion: 2}
	refreshed := catalog.SealedCredential{Ciphertext: bytes.Repeat([]byte{5}, 32), Nonce: bytes.Repeat([]byte{6}, 12), KeyVersion: 1}
	if _, err := pool.Exec(ctx, `INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES (40,'credential-test','Credential Test','OFFICIAL','ACTIVE','system','system',now(),now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential
(id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,auth_type,auth_adapter,subscription_type,created_by,updated_by,created_at,updated_at)
VALUES (60,40,'Subscription',$1,$2,$3,'SUBSCRIPTION','OPENAI_CODEX','PERSONAL','system','system',now(),now())`, old.Ciphertext, old.Nonce, old.KeyVersion); err != nil {
		t.Fatal(err)
	}
	type writeCase struct {
		name  string
		write func(gw.Route) error
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	cases := []writeCase{
		{name: "credential", write: func(route gw.Route) error { return store.UpdateResourceCredential(ctx, route, refreshed) }},
		{name: "metadata", write: func(route gw.Route) error { return store.UpdateResourceCredentialRefreshMetadata(ctx, route) }},
	}
	for _, tc := range cases {
		for _, concurrentChange := range []bool{false, true} {
			name := tc.name + "/current"
			if concurrentChange {
				name = tc.name + "/stale"
			}
			t.Run(name, func(t *testing.T) {
				if _, err := pool.Exec(ctx, `UPDATE provider_credential SET credential_ciphertext=$1,credential_nonce=$2,key_version=$3,
credential_refreshed_at=NULL,credential_expires_at=NULL WHERE id=60`, old.Ciphertext, old.Nonce, old.KeyVersion); err != nil {
					t.Fatal(err)
				}
				route := gw.Route{ProviderID: 40, ResourceID: 60, CredentialExpiresAt: &expiresAt}
				var err error
				route.Credential, err = store.LoadResourceCredential(ctx, route)
				if err != nil {
					t.Fatal(err)
				}
				if concurrentChange {
					if _, err := pool.Exec(ctx, `UPDATE provider_credential SET credential_ciphertext=$1,credential_nonce=$2,key_version=$3 WHERE id=60`, admin.Ciphertext, admin.Nonce, admin.KeyVersion); err != nil {
						t.Fatal(err)
					}
				}
				err = tc.write(route)
				if concurrentChange && !errors.Is(err, gw.ErrUnavailable) {
					t.Fatalf("stale write error = %v, want unavailable", err)
				}
				if !concurrentChange && err != nil {
					t.Fatal(err)
				}
				var ciphertext, nonce []byte
				var keyVersion int32
				var hasExpiry bool
				if err := pool.QueryRow(ctx, `SELECT credential_ciphertext,credential_nonce,key_version,credential_expires_at IS NOT NULL
FROM provider_credential WHERE id=60`).Scan(&ciphertext, &nonce, &keyVersion, &hasExpiry); err != nil {
					t.Fatal(err)
				}
				want := old
				if concurrentChange {
					want = admin
				} else if tc.name == "credential" {
					want = refreshed
				}
				if !bytes.Equal(ciphertext, want.Ciphertext) || !bytes.Equal(nonce, want.Nonce) || keyVersion != want.KeyVersion || hasExpiry == concurrentChange {
					t.Fatal("credential or refresh metadata changed unexpectedly")
				}
			})
		}
	}
}
