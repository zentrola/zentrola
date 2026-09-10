package postgres

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/health"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

func TestFirstAdminSetupIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids := idgen.New(pool)
	passwords, _ := cryptosec.NewPasswords(4)
	var entropy [32]byte
	_, _ = rand.Read(entropy[:])
	tokens, _ := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(entropy[:]))
	service := appsec.NewAdmin(NewSecurityStore(pool, ids), passwords, tokens, admin.LoginPolicy{MaxFailures: 5, LockDuration: time.Minute})
	router := httptransport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(health.Check{Name: "admin", Run: service.Check}), config.CORS{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: service})
	request := func(method, path, contentType string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	setup := func(user, password string) *httptest.ResponseRecorder {
		return request("POST", "/api/v1/auth/setup", "application/json", map[string]string{"username": user, "password": password})
	}
	status := func(want bool) {
		t.Helper()
		w := request("GET", "/api/v1/auth/setup", "", nil)
		var r struct {
			Data appsec.SetupStatus `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != 200 || r.Data.Required != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unexpected setup state")
		}
	}
	status(true)
	if w := request("GET", "/health/ready", "", nil); w.Code != 503 || !strings.Contains(w.Body.String(), "NOT_INITIALIZED") {
		t.Fatal("empty system should be awaiting initialization")
	}
	for _, input := range [][2]string{{"", "test-password-123"}, {"bad\nname", "test-password-123"}, {"admin", "short"}, {"admin", strings.Repeat("x", 73)}, {"admin", "password\x00-123"}} {
		if w := setup(input[0], input[1]); w.Code != 400 {
			t.Fatalf("invalid setup accepted: username=%q password_length=%d status=%d", input[0], len(input[1]), w.Code)
		}
	}
	if w := request("POST", "/api/v1/auth/setup", "text/plain", map[string]string{"username": "admin", "password": "test-password-123"}); w.Code != 400 {
		t.Fatal("non-JSON setup accepted")
	}
	if w := request("POST", "/api/v1/auth/setup", "application/json", map[string]string{"username": "admin", "password": "test-password-123", "organizationId": "123"}); w.Code != 400 {
		t.Fatal("client organization accepted")
	}
	// 审计失败必须回滚管理员创建，允许重试，不留下半初始化状态。
	if _, err := pool.Exec(ctx, `ALTER TABLE operation_log ADD CONSTRAINT reject_setup_test CHECK(operation_type <> 'ADMIN_INITIALIZE')`); err != nil {
		t.Fatal(err)
	}
	if w := setup("owner", "test-password-123"); w.Code != 503 {
		t.Fatal("audit failure did not fail setup")
	}
	status(true)
	if _, err := pool.Exec(ctx, `ALTER TABLE operation_log DROP CONSTRAINT reject_setup_test`); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := setup("owner", "test-password-123")
			if w.Code == 201 {
				successes.Add(1)
			} else if w.Code != 409 || !strings.Contains(w.Body.String(), "ALREADY_INITIALIZED") {
				t.Errorf("unexpected setup race response %d", w.Code)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("more than one initializer won")
	}
	status(false)
	var count, audits int
	var hash, audit string
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_user`).Scan(&count); err != nil || count != 1 {
		t.Fatal("incorrect administrator count")
	}
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM admin_user`).Scan(&hash); err != nil || !passwords.Verify(hash, "test-password-123") {
		t.Fatal("password was not hashed correctly")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*), string_agg(row_to_json(l)::text,' ') FROM operation_log l WHERE operation_type='ADMIN_INITIALIZE'`).Scan(&audits, &audit); err != nil || audits != 1 || strings.Contains(audit, "test-password-123") || strings.Contains(audit, hash) {
		t.Fatal("invalid or unsafe initialization audit")
	}
	if w := request("POST", "/api/v1/auth/login", "application/json", map[string]string{"username": "owner", "password": "test-password-123"}); w.Code != 200 {
		t.Fatal("new administrator cannot log in")
	}
	if w := request("GET", "/health/ready", "", nil); w.Code != 200 {
		t.Fatal("initialized service not ready")
	}
	if w := setup("replacement", "different-password"); w.Code != 409 {
		t.Fatal("repeat setup accepted")
	}
	// 停用和软删除仍占据管理员记录；只有物理清空表才重新开放初始化。
	if _, err := pool.Exec(ctx, `UPDATE admin_user SET status='DISABLED',is_deleted=true`); err != nil {
		t.Fatal(err)
	}
	status(false)
	if w := setup("replacement", "different-password"); w.Code != 409 {
		t.Fatal("disabled history reopened setup")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM admin_user`); err != nil {
		t.Fatal(err)
	}
	status(true)
	if w := request("GET", "/health/ready", "", nil); w.Code != 503 || !strings.Contains(w.Body.String(), "NOT_INITIALIZED") {
		t.Fatal("empty administrator table should await initialization despite audit history")
	}
	var previousAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log`).Scan(&previousAudits); err != nil {
		t.Fatal(err)
	}
	if w := setup("replacement", "different-password"); w.Code != 201 {
		t.Fatal("existing audit history blocked initialization")
	}
	status(false)
	if w := request("POST", "/api/v1/auth/login", "application/json", map[string]string{"username": "replacement", "password": "different-password"}); w.Code != 200 {
		t.Fatal("replacement administrator cannot log in")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM operation_log`).Scan(&audits); err != nil || audits != previousAudits+2 {
		t.Fatal("administrator initialization did not preserve audit history")
	}
}
