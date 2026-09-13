package postgres

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/health"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

func TestStage2Integration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids := idgen.New(pool)
	store := NewSecurityStore(pool, ids)
	passwords, err := cryptosec.NewPasswords(4)
	if err != nil {
		t.Fatal(err)
	}
	var entropy [32]byte
	_, _ = rand.Read(entropy[:])
	tokens, err := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(entropy[:]))
	if err != nil {
		t.Fatal(err)
	}
	admins := appsec.NewAdmin(store, passwords, tokens, admin.LoginPolicy{MaxFailures: 5, LockDuration: 15 * time.Minute})
	const username = "test-admin"
	const password = "test-only-password-123"
	if err := admins.Bootstrap(ctx, username, password); err != nil {
		t.Fatal(err)
	}
	if err := admins.Bootstrap(ctx, "replacement-admin", "different-password-123"); err != nil {
		t.Fatal(err)
	}
	keys := appsec.NewKeys(store, ids)
	var logs bytes.Buffer
	router := httptransport.NewRouter(logging.New(&logs, "json", slog.LevelInfo), health.New(), config.CORS{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: admins, Keys: keys})
	request := func(method, path, token, key string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			req.Header.Set("x-api-key", key)
		}
		req.Header.Set("User-Agent", "  ") // 空白 Header 不能导致审计失败和登录失败计数回滚。
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	login := func(user, pw string) *httptest.ResponseRecorder {
		return request("POST", "/api/v1/auth/login", "", "", map[string]string{"username": user, "password": pw})
	}
	decodeLogin := func(rec *httptest.ResponseRecorder) appsec.LoginResult {
		t.Helper()
		if rec.Code != 200 {
			t.Fatalf("login failed with %d", rec.Code)
		}
		var body struct {
			Data appsec.LoginResult `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data
	}
	initial := decodeLogin(login(username, password))
	actor, err := admins.Authenticate(ctx, initial.Token)
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider(id,provider_code,provider_name,provider_type,status,created_by,updated_by,created_at,updated_at)
		VALUES($1,'security-test-provider','Security Test Provider','OFFICIAL','ACTIVE','system','system',now(),now())`, providerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at)
		VALUES($1,'ANTHROPIC','https://security.example.com','system','system',now(),now())`, providerID); err != nil {
		t.Fatal(err)
	}
	if rec := request("GET", "/api/v1/me", initial.Token, "", nil); rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("protected identity endpoint failed")
	}

	t.Run("login lockout and retry details", func(t *testing.T) {
		unknown := login("nonexistent", password)
		if unknown.Code != 401 {
			t.Fatal("unknown account should fail uniformly")
		}
		for range 4 {
			if rec := login(username, "wrong-password"); rec.Code != 401 {
				t.Fatal("wrong password accepted")
			}
		}
		var lockedUntil time.Time
		for _, pw := range []string{"wrong-password", password, "wrong-again"} {
			rec := login(username, pw)
			var body struct {
				Code string                          `json:"code"`
				Data httptransport.LoginLockResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 429 || body.Code != "ACCOUNT_LOCKED" || body.Data.RetryAfterSeconds <= 0 || body.Data.RetryAfterSeconds > 900 ||
				rec.Header().Get("Retry-After") != strconv.FormatInt(body.Data.RetryAfterSeconds, 10) {
				t.Fatalf("missing lock details: %d %s", rec.Code, rec.Body.String())
			}
			if !lockedUntil.IsZero() && !body.Data.LockedUntil.Equal(lockedUntil) {
				t.Fatal("retry extended the lock")
			}
			lockedUntil = body.Data.LockedUntil
		}
		row, err := dbgen.New(pool).GetAdminForLogin(ctx, username)
		if err != nil || row.FailedLoginCount != 5 || !row.LockedUntil.Valid {
			t.Fatal("lockout state not persisted", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE admin_user SET locked_until=$1 WHERE id=$2", time.Now().Add(-time.Second), actor.ID); err != nil {
			t.Fatal(err)
		}
		decodeLogin(login(username, password))
		row, err = dbgen.New(pool).GetAdminForLogin(ctx, username)
		if err != nil || row.FailedLoginCount != 0 || row.LockedUntil.Valid || !row.LastLoginAt.Valid {
			t.Fatal("successful login did not reset state", err)
		}
	})
	t.Run("concurrent login cannot bypass counter", func(t *testing.T) {
		var wg sync.WaitGroup
		for range 12 {
			wg.Go(func() {
				_, err := admins.Login(ctx, username, "wrong-password", appsec.RequestMeta{})
				if !errors.Is(err, appsec.ErrUnauthenticated) {
					t.Error("unexpected concurrent login result")
				}
			})
		}
		wg.Wait()
		row, err := dbgen.New(pool).GetAdminForLogin(ctx, username)
		if err != nil || row.FailedLoginCount != 5 || !row.LockedUntil.Valid {
			t.Fatal("concurrent failures lost lockout state", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE admin_user SET locked_until=$1 WHERE id=$2", time.Now().Add(-time.Second), actor.ID); err != nil {
			t.Fatal(err)
		}
		decodeLogin(login(username, password))
	})
	t.Run("JWT cannot bypass disabled admin", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "UPDATE admin_user SET status='DISABLED' WHERE id=$1", actor.ID); err != nil {
			t.Fatal(err)
		}
		if rec := request("GET", "/api/v1/me", initial.Token, "", nil); rec.Code != 401 {
			t.Fatal("disabled admin accepted")
		}
		if _, err := pool.Exec(ctx, "UPDATE admin_user SET status='ACTIVE' WHERE id=$1", actor.ID); err != nil {
			t.Fatal(err)
		}
	})

	principalID, _ := ids.NextID(ctx)
	if _, err := pool.Exec(ctx, `INSERT INTO principal (id,principal_type,name,status,created_by,updated_by,created_at,updated_at) VALUES ($1,'MEMBER','Test Member','ACTIVE','system','system',now(),now())`, principalID); err != nil {
		t.Fatal(err)
	}
	var issued appsec.CreatedKey
	t.Run("key issuance and authentication isolation", func(t *testing.T) {
		rec := request("POST", "/api/v1/members/"+strconv.FormatInt(principalID, 10)+"/keys", initial.Token, "", map[string]string{"name": "Test Key"})
		if rec.Code != 201 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("key creation failed with %d", rec.Code)
		}
		var result struct {
			Data appsec.CreatedKey `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		issued = result.Data
		digest := sha256.Sum256([]byte(issued.Key))
		row, err := dbgen.New(pool).GetAccessKeyByHash(ctx, digest[:])
		if err != nil || len(row.KeyHash) != 32 || row.MaskedKey != issued.MaskedKey {
			t.Fatal("key digest storage incorrect", err)
		}
		identity, err := keys.Authenticate(ctx, issued.Key)
		if err != nil || identity.ID != principalID {
			t.Fatal("key authentication failed", err)
		}
		if rec := request("GET", "/api/v1/me", issued.Key, "", nil); rec.Code != 401 {
			t.Fatal("Access Key accepted as Admin JWT")
		}
		for _, headers := range [][2]string{{initial.Token, ""}, {"", initial.Token}, {initial.Token, issued.Key}} {
			rec := request("POST", "/anthropic/v1/messages", headers[0], headers[1], map[string]any{})
			if rec.Code != 401 || strings.Contains(rec.Body.String(), `"code"`) || !strings.Contains(rec.Body.String(), `"authentication_error"`) {
				t.Fatal("Gateway auth was bypassed or protocol error was wrapped")
			}
		}
		if rec := request("POST", "/anthropic/v1/messages", "", issued.Key, map[string]any{}); rec.Code != 501 {
			t.Fatal("valid key must reach the explicitly unimplemented Gateway")
		}
		if _, err := pool.Exec(ctx, "UPDATE principal_access_key SET expires_at=$1 WHERE id=$2", time.Now().Add(-time.Second), issued.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := keys.Authenticate(ctx, issued.Key); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("expired key accepted")
		}
		if _, err := pool.Exec(ctx, "UPDATE principal_access_key SET expires_at=NULL WHERE id=$1", issued.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE principal SET status='DISABLED' WHERE id=$1", principalID); err != nil {
			t.Fatal(err)
		}
		if _, err := keys.Authenticate(ctx, issued.Key); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("disabled principal accepted")
		}
		if _, err := pool.Exec(ctx, "UPDATE principal SET status='ACTIVE' WHERE id=$1", principalID); err != nil {
			t.Fatal(err)
		}
		if rec := request("POST", "/api/v1/access-keys/"+strconv.FormatInt(issued.ID, 10)+"/revoke", initial.Token, "", nil); rec.Code != 200 {
			t.Fatal("revoke failed")
		}
		if _, err := keys.Authenticate(ctx, issued.Key); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("revoked key remained valid")
		}
		if err := keys.Revoke(ctx, actor, issued.ID, appsec.RequestMeta{}); err != nil {
			t.Fatal("revoke is not idempotent")
		}
		foreign := actor
		foreign.ID = 0
		if _, err := keys.Create(ctx, foreign, principalID, "Invalid Actor", nil, appsec.RequestMeta{}); err == nil {
			t.Fatal("invalid administrator key creation accepted")
		}
	})
	t.Run("credential recovery preserves admin access", func(t *testing.T) {
		master, err := cryptosec.LoadMasterKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
		if err != nil {
			t.Fatal(err)
		}
		c, _ := cryptosec.NewCredentials(master)
		resourceID, _ := ids.NextID(ctx)
		owner := cryptosec.CredentialOwner{ProviderID: providerID, ResourceID: resourceID}
		sealed, _ := c.Encrypt([]byte("test-provider-secret"), owner)
		if _, err := pool.Exec(ctx, `INSERT INTO provider_credential (id,provider_id,resource_name,credential_ciphertext,credential_nonce,key_version,status,created_by,updated_by,created_at,updated_at) VALUES ($1,$2,'Test Resource',$3,$4,$5,'ACTIVE','system','system',now(),now())`, resourceID, providerID, sealed.Ciphertext, sealed.Nonce, sealed.KeyVersion); err != nil {
			t.Fatal(err)
		}
		if err := store.ValidateCredentials(ctx, c); err != nil {
			t.Fatal("valid resource was rejected", err)
		}
		otherMaster, err := cryptosec.LoadMasterKey("AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=")
		if err != nil {
			t.Fatal(err)
		}
		otherCipher, err := cryptosec.NewCredentials(otherMaster)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.ValidateCredentials(ctx, otherCipher); err == nil {
			t.Fatal("wrong MASTER_KEY was accepted")
		}
		var isDeleted bool
		if err := pool.QueryRow(ctx, `SELECT is_deleted FROM provider_credential WHERE id=$1`, resourceID).Scan(&isDeleted); err != nil || isDeleted {
			t.Fatal("credential validation failure modified the resource", err)
		}
		if err := admins.Check(ctx); err != nil {
			t.Fatal("credential recovery disabled admin plane")
		}
	})
	t.Run("audit and logs exclude secrets", func(t *testing.T) {
		var content string
		if err := pool.QueryRow(ctx, `SELECT string_agg(row_to_json(l)::text, E'\n') FROM operation_log l`).Scan(&content); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{password, initial.Token, issued.Key, "test-provider-secret"} {
			if secret != "" && strings.Contains(content+logs.String(), secret) {
				t.Fatal("secret leaked into audit or logs")
			}
		}
		for _, event := range []string{"LOGIN_SUCCESS", "LOGIN_FAILED", "LOGIN_LOCKED", "ACCESS_KEY_CREATE", "ACCESS_KEY_REVOKE", "RESOURCE_STATUS_CHANGE"} {
			if !strings.Contains(content, event) {
				t.Fatalf("audit missing %s", event)
			}
		}
		var hashes int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log WHERE before_data::text LIKE '%password%' OR after_data::text LIKE '%password%'`).Scan(&hashes); err != nil || hashes != 0 {
			t.Fatal("password field in snapshots")
		}
	})
}
