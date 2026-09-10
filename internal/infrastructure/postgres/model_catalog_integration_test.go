package postgres

import (
	"bytes"
	"crypto/aes"
	stdcipher "crypto/cipher"
	"encoding/base64"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	"io/fs"
	"strings"
	"testing"
)

func TestModelModalitiesDatabaseConstraints(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	withFixture(t, ctx, pool, func(tx pgx.Tx) {
		for _, column := range []string{"input_modalities", "output_modalities"} {
			for _, invalid := range []string{`[]`, `null`, `{}`, `"TEXT"`, `["TEXT","TEXT"]`, `["UNKNOWN"]`, `[null]`, `["TEXT",null]`, `[1]`, `[["TEXT"]]`, `["TEXT",["IMAGE"]]`} {
				mustReject(t, ctx, tx, "23514", "UPDATE model SET "+column+"=$1::jsonb WHERE id=30", invalid)
			}
			mustReject(t, ctx, tx, "23502", "UPDATE model SET "+column+"=NULL WHERE id=30")
			mustExec(t, ctx, tx, "UPDATE model SET "+column+"='[\"TEXT\",\"IMAGE\",\"AUDIO\",\"VIDEO\"]'::jsonb WHERE id=30")
		}
	})
}

func TestModelCatalogUpgradePreservesExistingModels(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	source, _ := fs.Sub(migrations, "migrations")
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_model(id,model_code,display_name,model_type,status,created_by,updated_by,created_at,updated_at)
VALUES(1,'claude-sonnet','自定义显示名','CHAT','DISABLED','system','system',now(),now()),
(2,'deepseek-v4-flash','DeepSeek V4 Flash','CHAT','ACTIVE','system','system',now(),now()),
(3,'unknown-legacy-model','未知旧模型','CHAT','DISABLED','system','system',now(),now());`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err == nil || !strings.Contains(err.Error(), "Unknown legacy models") {
		t.Fatal("unknown legacy capabilities were guessed", err)
	}
	var legacyColumn bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_model' AND column_name='model_type')`).Scan(&legacyColumn); err != nil || !legacyColumn {
		t.Fatal("failed migration was not atomic")
	}
	// 仅移除本测试刚插入的未知记录，再验证安装目录中的已知模型。
	if _, err := pool.Exec(ctx, "DELETE FROM ai_model WHERE id=3"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT model_code='claude-sonnet' AND display_name='自定义显示名' AND status='DISABLED' AND input_modalities='["TEXT","IMAGE"]'::jsonb AND output_modalities='["TEXT"]'::jsonb AND remark='' FROM model WHERE id=1`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("legacy identity/status or backfill incorrect", err)
	}
	if err := pool.QueryRow(ctx, `SELECT input_modalities='["TEXT"]'::jsonb FROM model WHERE id=2`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("DeepSeek modality backfill incorrect", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal("repeated migration failed", err)
	}
}

func TestOrganizationRemovalPreservesExistingCredential(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	source, _ := fs.Sub(migrations, "migrations")
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 27); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM organization`); err != nil {
		t.Fatal(err)
	}

	const organizationID int64 = 101
	const providerID int64 = 102
	const resourceID int64 = 103
	masterBytes := bytes.Repeat([]byte{5}, 32)
	master, err := cryptosec.LoadMasterKey(base64.StdEncoding.EncodeToString(masterBytes), "", filepath.Join(t.TempDir(), "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	owner := cryptosec.CredentialOwner{ProviderID: providerID, ResourceID: resourceID}
	plain := []byte("credential-created-before-organization-removal")
	block, _ := aes.NewCipher(masterBytes)
	legacyAEAD, _ := stdcipher.NewGCM(block)
	legacyNonce := bytes.Repeat([]byte{7}, legacyAEAD.NonceSize())
	legacyCiphertext := legacyAEAD.Seal(nil, legacyNonce, plain, owner.LegacyAAD(organizationID))
	if _, err := pool.Exec(ctx, `INSERT INTO organization (id,organization_code,organization_name,status,created_by,updated_by,created_at,updated_at)
VALUES ($1,'default','Zentrola','ACTIVE','system','system',now(),now())`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider (id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
VALUES ($1,'upgrade-provider','Upgrade Provider','OFFICIAL','ACTIVE','system','system',now(),now())`, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_credential (id,organization_id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at)
VALUES ($1,$2,$3,'Upgrade Resource',$4,$5,1,'ACTIVE','system','system',now(),now())`, resourceID, organizationID, providerID, legacyCiphertext, legacyNonce); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}

	cipher, err := cryptosec.NewCredentials(master)
	if err != nil {
		t.Fatal(err)
	}
	var instanceProfileExists bool
	var ciphertext, migratedNonce []byte
	var keyVersion int32
	if err := pool.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.instance_profile') IS NOT NULL`).Scan(&instanceProfileExists); err != nil || instanceProfileExists {
		t.Fatal("instance_profile still exists", err)
	}
	if err := pool.QueryRow(ctx, `SELECT credential_ciphertext,credential_nonce,key_version FROM provider_credential WHERE id=$1`, resourceID).Scan(&ciphertext, &migratedNonce, &keyVersion); err != nil {
		t.Fatal(err)
	}
	opened, err := cipher.Decrypt(cryptosec.SealedCredential{Ciphertext: ciphertext, Nonce: migratedNonce, KeyVersion: keyVersion}, owner)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatal("credential cannot be decrypted after organization removal", err)
	}

	if _, err := provider.DownTo(ctx, 27); err != nil {
		t.Fatal(err)
	}
	var restoredOrganizationID int64
	var restoredCiphertext []byte
	if err := pool.QueryRow(ctx, `SELECT organization_id,credential_ciphertext FROM provider_credential WHERE id=$1`, resourceID).Scan(&restoredOrganizationID, &restoredCiphertext); err != nil || restoredOrganizationID != organizationID || !bytes.Equal(restoredCiphertext, legacyCiphertext) {
		t.Fatal("organization rollback did not restore legacy credential", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	v2, err := cipher.Encrypt([]byte("credential-created-after-organization-removal"), owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE provider_credential SET credential_ciphertext=$2,credential_nonce=$3,key_version=$4 WHERE id=$1`, resourceID, v2.Ciphertext, v2.Nonce, v2.KeyVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 27); err == nil || !strings.Contains(err.Error(), "Cannot roll back after v2 provider credentials") {
		t.Fatal("unsafe credential downgrade was allowed", err)
	}
}
