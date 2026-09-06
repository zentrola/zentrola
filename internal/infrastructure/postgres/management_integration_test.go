package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	"github.com/zentrola/zentrola/internal/application/health"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

type connectionTestFunc func(context.Context, string, []byte) mgmt.ConnectionResult

func (f connectionTestFunc) Test(ctx context.Context, url string, key []byte) mgmt.ConnectionResult {
	return f(ctx, url, key)
}

type failedAuditIDs struct{}

func (failedAuditIDs) NextID() (int64, error) { return 0, errors.New("test ID failure") }

type synchronizedLogs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedLogs) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *synchronizedLogs) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }
func stage3Data[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var body struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}

func TestStage3Integration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids, err := idgen.New(4)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.New(NewBootstrapStore(pool), ids, "sonnet", "opus").Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	securityStore := NewSecurityStore(pool, ids)
	passwords, err := cryptosec.NewPasswords(4)
	if err != nil {
		t.Fatal(err)
	}
	entropy := make([]byte, 32)
	_, _ = rand.Read(entropy)
	tokens, err := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(entropy))
	if err != nil {
		t.Fatal(err)
	}
	admins := appsec.NewAdmin(securityStore, passwords, tokens, admin.LoginPolicy{MaxFailures: 5, LockDuration: 15 * time.Minute})
	if err := admins.Bootstrap(ctx, "admin", "test-stage3-password"); err != nil {
		t.Fatal(err)
	}
	master, err := cryptosec.LoadMasterKey("", "", filepath.Join(t.TempDir(), "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := cryptosec.NewCredentials(master)
	if err != nil {
		t.Fatal(err)
	}
	const credential = "stage3-private-provider-key"
	tester := connectionTestFunc(func(_ context.Context, url string, key []byte) mgmt.ConnectionResult {
		if url != "https://api.anthropic.com" || string(key) != credential {
			t.Error("unexpected connection test inputs")
		}
		return mgmt.ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200}
	})
	service := mgmt.New(NewManagementStore(pool, ids), ids, cipher, tester)
	keys := appsec.NewKeys(securityStore, ids)
	var logs synchronizedLogs
	router := httptransport.NewRouter(logging.New(&logs, "json", slog.LevelInfo), health.New(), config.CORS{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: admins, Keys: keys, Management: service})
	token := ""
	request := func(method, path string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if want != 0 && rec.Code != want {
			t.Fatalf("%s %s: got %d, want %d; safe body=%s", method, path, rec.Code, want, rec.Body.String())
		}
		var envelope struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Code == "" || envelope.RequestID != rec.Header().Get("X-Request-ID") {
			t.Fatal("management response contract failed")
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("management response may be cached")
		}
		return rec
	}
	request("GET", "/api/v1/members", nil, 401)
	login := stage3Data[appsec.LoginResult](t, request("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "test-stage3-password"}, 200))
	token = login.Token
	actor, err := admins.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	sid := func(id int64) string { return strconv.FormatInt(id, 10) }
	member := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]string{"name": "开发者", "remark": "测试成员"}, 201))
	group := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"code": "engineering", "name": "研发"}, 201))
	models := stage3Data[struct {
		Items []mgmt.Model `json:"items"`
	}](t, request("GET", "/api/v1/models", nil, 200)).Items
	providers := stage3Data[struct {
		Items []mgmt.Provider `json:"items"`
	}](t, request("GET", "/api/v1/providers", nil, 200)).Items
	if len(models) != 2 || len(providers) != 1 {
		t.Fatal("bootstrap catalogs unavailable")
	}
	model := models[0]
	provider := providers[0]
	memberPath := "/api/v1/members/" + sid(member.ID)
	groupPath := "/api/v1/groups/" + sid(group.ID)
	memberLink := groupPath + "/members/" + sid(member.ID)
	modelLink := groupPath + "/models/" + sid(model.ID)
	q := dbgen.New(pool)
	allowed := func(want bool) {
		t.Helper()
		ok, err := q.HasGroupModelPermission(ctx, dbgen.HasGroupModelPermissionParams{OrganizationID: actor.OrganizationID, PrincipalID: member.ID, ModelID: model.ID})
		if err != nil || ok != want {
			t.Fatalf("permission=%v want=%v err=%v", ok, want, err)
		}
	}

	t.Run("API governance chain and default deny", func(t *testing.T) {
		allowed(false)
		request("PUT", memberLink, nil, 200)
		request("PUT", modelLink, nil, 200)
		allowed(true)
		request("GET", memberPath, nil, 200)
		request("GET", groupPath, nil, 200)
		members := stage3Data[struct {
			Items []mgmt.Member `json:"items"`
		}](t, request("GET", groupPath+"/members", nil, 200)).Items
		grants := stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", groupPath+"/models", nil, 200)).Items
		if len(members) != 1 || members[0].ID != member.ID || len(grants) != 1 {
			t.Fatal("group association read failed")
		}
		request("PUT", memberLink, nil, 200)
		request("PUT", modelLink, nil, 200)
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM operation_log WHERE operation_type='GROUP_MODEL_GRANT'").Scan(&count); err != nil || count != 1 {
			t.Fatal("idempotent grant duplicated audit")
		}
	})

	var virtualKey string
	t.Run("key issuance and member model status", func(t *testing.T) {
		created := stage3Data[appsec.CreatedKey](t, request("POST", memberPath+"/keys", map[string]string{"name": "Claude Code"}, 201))
		virtualKey = created.Key
		if _, err := keys.Authenticate(ctx, virtualKey); err != nil {
			t.Fatal(err)
		}
		listed := request("GET", memberPath+"/keys", nil, 200)
		if strings.Contains(listed.Body.String(), virtualKey) || strings.Contains(listed.Body.String(), "hash") {
			t.Fatal("key list leaked secret")
		}
		request("PATCH", memberPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		allowed(false)
		if _, err := keys.Authenticate(ctx, virtualKey); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("disabled member key accepted")
		}
		request("PATCH", memberPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
		allowed(true)
		request("PATCH", "/api/v1/models/"+sid(model.ID)+"/status", map[string]string{"status": "DISABLED"}, 200)
		allowed(false)
		request("PATCH", "/api/v1/models/"+sid(model.ID)+"/status", map[string]string{"status": "ACTIVE"}, 200)
		allowed(true)
		request("POST", "/api/v1/access-keys/"+sid(created.ID)+"/revoke", nil, 200)
		if _, err := keys.Authenticate(ctx, virtualKey); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("revoked key accepted")
		}
	})

	t.Run("union and irreversible relationship deletion", func(t *testing.T) {
		other := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"code": "other", "name": "另一组"}, 201))
		otherPath := "/api/v1/groups/" + sid(other.ID)
		request("PUT", otherPath+"/members/"+sid(member.ID), nil, 200)
		request("PUT", otherPath+"/models/"+sid(model.ID), nil, 200)
		request("DELETE", modelLink, nil, 200)
		allowed(true)
		request("DELETE", otherPath+"/members/"+sid(member.ID), nil, 200)
		allowed(false)
		request("PUT", modelLink, nil, 200)
		allowed(true)
		request("DELETE", memberLink, nil, 200)
		allowed(false)
		request("PUT", memberLink, nil, 200)
		allowed(true)
		var history, active int
		if err := pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE NOT is_deleted) FROM group_model_permission WHERE group_id=$1 AND model_id=$2", group.ID, model.ID).Scan(&history, &active); err != nil || history != 2 || active != 1 {
			t.Fatal("grant recreated old row")
		}
		if err := pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE NOT is_deleted) FROM principal_group WHERE group_id=$1 AND principal_id=$2", group.ID, member.ID).Scan(&history, &active); err != nil || history != 2 || active != 1 {
			t.Fatal("membership recreated old row")
		}
	})

	var first, second mgmt.Resource
	t.Run("resource encrypted storage recovery and singleton", func(t *testing.T) {
		create := func(name string) mgmt.Resource {
			return stage3Data[mgmt.Resource](t, request("POST", "/api/v1/resources", map[string]string{"providerId": sid(provider.ID), "name": name, "credential": credential}, 201))
		}
		first = create("主资源")
		second = create("备用配置")
		if first.Status != "DISABLED" {
			t.Fatal("new resource should await explicit activation")
		}
		firstPath := "/api/v1/resources/" + sid(first.ID)
		secondPath := "/api/v1/resources/" + sid(second.ID)
		var encrypted []byte
		if err := pool.QueryRow(ctx, "SELECT credential_ciphertext FROM ai_resource WHERE id=$1", first.ID).Scan(&encrypted); err != nil || bytes.Contains(encrypted, []byte(credential)) {
			t.Fatal("resource stored plaintext")
		}
		for _, path := range []string{"/api/v1/resources", firstPath} {
			rec := request("GET", path, nil, 200)
			if strings.Contains(rec.Body.String(), credential) || strings.Contains(strings.ToLower(rec.Body.String()), "ciphertext") || strings.Contains(rec.Body.String(), "nonce") {
				t.Fatal("resource response leaked encryption data")
			}
		}
		result := stage3Data[mgmt.ConnectionResult](t, request("POST", firstPath+"/test-connection", nil, 200))
		if !result.OK {
			t.Fatal("connection test failed")
		}
		request("PATCH", firstPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
		request("PATCH", secondPath+"/status", map[string]string{"status": "ACTIVE"}, 409)
		request("PATCH", firstPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for _, path := range []string{firstPath, secondPath} {
			wg.Go(func() { codes <- request("PATCH", path+"/status", map[string]string{"status": "ACTIVE"}, 0).Code })
		}
		wg.Wait()
		close(codes)
		ok, conflict := 0, 0
		for code := range codes {
			if code == 200 {
				ok++
			} else if code == 409 {
				conflict++
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatal("concurrent activation broke resource singleton")
		}
		request("PATCH", firstPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		request("PATCH", secondPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		if _, err := pool.Exec(ctx, "UPDATE ai_resource SET credential_ciphertext=decode(repeat('00',32),'hex') WHERE id=$1", first.ID); err != nil {
			t.Fatal(err)
		}
		request("PATCH", firstPath+"/status", map[string]string{"status": "ACTIVE"}, 422)
		result = stage3Data[mgmt.ConnectionResult](t, request("POST", firstPath+"/test-connection", nil, 200))
		if result.OK || result.Code != "CREDENTIAL_UNRECOVERABLE" {
			t.Fatal("unrecoverable credential test accepted")
		}
		request("PUT", firstPath+"/credential", map[string]string{"credential": credential}, 200)
		request("PATCH", firstPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
	})

	t.Run("connection test releases transaction and detects replacement", func(t *testing.T) {
		changing := mgmt.New(NewManagementStore(pool, ids), ids, cipher, connectionTestFunc(func(ctx context.Context, _ string, _ []byte) mgmt.ConnectionResult {
			if err := service.UpdateCredential(ctx, actor, first.ID, credential, appsec.RequestMeta{}); err != nil {
				t.Error(err)
			}
			return mgmt.ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200}
		}))
		result, err := changing.TestResource(ctx, actor, first.ID, appsec.RequestMeta{})
		if err != nil || result.OK || result.Code != "RESOURCE_CHANGED" {
			t.Fatalf("stale connection result accepted: %+v %v", result, err)
		}
	})

	t.Run("validation pagination and organization boundaries", func(t *testing.T) {
		request("POST", "/api/v1/groups", map[string]string{"code": "engineering", "name": "重复"}, 409)
		request("POST", "/api/v1/members", map[string]string{"name": " ", "organizationId": "1"}, 400)
		request("POST", "/api/v1/resources", map[string]string{"providerId": sid(provider.ID), "name": "bad", "credential": "key\nheader"}, 400)
		request("PATCH", memberPath+"/status", map[string]string{"status": "DELETED"}, 400)
		for _, path := range []string{"/api/v1/members/bad", "/api/v1/members?after=-1", "/api/v1/members?limit=101", "/api/v1/members?limit=1&limit=2"} {
			request("GET", path, nil, 400)
		}
		page := stage3Data[struct {
			Items []mgmt.Member `json:"items"`
			Next  *string       `json:"nextCursor"`
		}](t, request("GET", "/api/v1/members?limit=1", nil, 200))
		if len(page.Items) != 1 || page.Next == nil {
			t.Fatal("pagination cursor missing")
		}
		request("GET", "/api/v1/members?after="+*page.Next, nil, 200)
		foreignID, _ := ids.NextID()
		if _, err := pool.Exec(ctx, "INSERT INTO principal(id,organization_id,principal_type,name,status,created_by,updated_by,created_at,updated_at) VALUES($1,$2,'MEMBER','foreign','ACTIVE','system','system',now(),now())", foreignID, actor.OrganizationID+1); err != nil {
			t.Fatal(err)
		}
		foreignPath := "/api/v1/members/" + sid(foreignID)
		request("GET", foreignPath, nil, 404)
		request("DELETE", foreignPath, nil, 404)
		request("PATCH", foreignPath+"/status", map[string]string{"status": "DISABLED"}, 404)
		request("PUT", groupPath+"/members/"+sid(foreignID), nil, 404)
		request("POST", foreignPath+"/keys", map[string]string{"name": "foreign key"}, 404)
		badActor := actor
		badActor.OrganizationID++
		if _, err := service.Members(ctx, badActor, mgmt.Page{Limit: 50}); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("forged organization accepted")
		}
		if _, err := pool.Exec(ctx, "UPDATE ai_group SET is_deleted=true WHERE id=$1", group.ID); err != nil {
			t.Fatal(err)
		}
		request("GET", groupPath, nil, 404)
		request("PUT", memberLink, nil, 404)
		replacement := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"code": "engineering", "name": "新研发"}, 201))
		if replacement.ID == group.ID {
			t.Fatal("deleted group resurrected")
		}
	})

	t.Run("audit failure rolls back mutation", func(t *testing.T) {
		broken := mgmt.New(NewManagementStore(pool, failedAuditIDs{}), ids, cipher, tester)
		if _, err := broken.CreateMember(ctx, actor, "must-rollback", "", appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatal("audit failure was ignored")
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM principal WHERE name='must-rollback'").Scan(&count); err != nil || count != 0 {
			t.Fatal("unaudited member committed")
		}
	})

	t.Run("member deletion revokes keys removes groups and audits atomically", func(t *testing.T) {
		m := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]string{"name": "待删除成员"}, 201))
		path := "/api/v1/members/" + sid(m.ID)
		g := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"code": "delete-member", "name": "删除测试组"}, 201))
		groupMembers := "/api/v1/groups/" + sid(g.ID) + "/members"
		request("PUT", groupMembers+"/"+sid(m.ID), nil, 200)
		created := stage3Data[appsec.CreatedKey](t, request("POST", path+"/keys", map[string]string{"name": "删除测试 Key"}, 201))
		broken := mgmt.New(NewManagementStore(pool, failedAuditIDs{}), ids, cipher, tester)
		if err := broken.DeleteMember(ctx, actor, m.ID, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatalf("audit failure ignored: %v", err)
		}
		request("GET", path, nil, 200)
		if _, err := keys.Authenticate(ctx, created.Key); err != nil {
			t.Fatalf("rolled back deletion revoked key: %v", err)
		}
		savedToken := token
		token = ""
		request("DELETE", path, nil, 401)
		token = savedToken
		request("DELETE", "/api/v1/members/bad", nil, 400)
		request("DELETE", path, nil, 200)
		request("GET", path, nil, 404)
		request("DELETE", path, nil, 404)
		request("PATCH", path+"/status", map[string]string{"status": "ACTIVE"}, 404)
		request("POST", path+"/keys", map[string]string{"name": "不得签发"}, 404)
		if _, err := keys.Authenticate(ctx, created.Key); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("deleted member key accepted")
		}
		linked := stage3Data[struct {
			Items []mgmt.Member `json:"items"`
		}](t, request("GET", groupMembers, nil, 200))
		if len(linked.Items) != 0 {
			t.Fatal("deleted member still listed in group")
		}
		var deleted, revoked, unlinked bool
		if err := pool.QueryRow(ctx, "SELECT p.is_deleted, k.status='REVOKED' AND k.revoked_at IS NOT NULL, g.is_deleted FROM principal p JOIN access_key k ON k.principal_id=p.id JOIN principal_group g ON g.principal_id=p.id WHERE p.id=$1", m.ID).Scan(&deleted, &revoked, &unlinked); err != nil || !deleted || !revoked || !unlinked {
			t.Fatalf("incomplete member deletion: %v %v %v %v", deleted, revoked, unlinked, err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM operation_log WHERE target_id=$1 AND operation_type IN ('MEMBER_CREATE','MEMBER_DELETE')", m.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal("member history missing or deletion audit duplicated")
		}
	})

	t.Run("official model creation editing validation and audit", func(t *testing.T) {
		input := mgmt.ModelInput{Code: "official-model-test", Name: "官方模型测试", InputModalities: []string{"TEXT", "IMAGE"}, OutputModalities: []string{"TEXT"}, Remark: "用途说明"}
		savedToken := token
		token = ""
		request("POST", "/api/v1/models", input, 401)
		request("PUT", "/api/v1/models/1", input, 401)
		token = savedToken
		created := stage3Data[mgmt.Model](t, request("POST", "/api/v1/models", input, 201))
		path := "/api/v1/models/" + sid(created.ID)
		if created.Status != "DISABLED" || len(created.InputModalities) != 2 || created.Remark != input.Remark {
			t.Fatal("model metadata/default status lost")
		}
		request("POST", "/api/v1/models", input, 409)
		for _, invalid := range []any{nil, []string{}, []string{"TEXT", "TEXT"}, []string{"IMAGE", "UNKNOWN"}, "TEXT", []any{"TEXT", nil}, []any{[]string{"TEXT"}}} {
			for _, field := range []string{"inputModalities", "outputModalities"} {
				payload := map[string]any{"code": "invalid-model", "name": "错误模型", "inputModalities": []string{"TEXT"}, "outputModalities": []string{"TEXT"}}
				payload[field] = invalid
				request("POST", "/api/v1/models", payload, 400)
				request("PUT", path, payload, 400)
			}
		}
		request("GET", "/api/v1/models/bad", nil, 400)
		request("GET", "/api/v1/models/1", nil, 404)
		request("PUT", "/api/v1/models/1", input, 404)
		request("PATCH", path+"/status", map[string]string{"status": "ACTIVE"}, 200)
		g := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"code": "model-metadata-test", "name": "模型测试组"}, 201))
		grantPath := "/api/v1/groups/" + sid(g.ID) + "/models"
		request("PUT", grantPath+"/"+sid(created.ID), nil, 200)
		input.Code, input.Name, input.Remark = "official-model-renamed", "官方模型新名称", "更新说明"
		input.OutputModalities = []string{"TEXT", "AUDIO"}
		updated := stage3Data[mgmt.Model](t, request("PUT", path, input, 200))
		if updated.ID != created.ID || updated.Status != "ACTIVE" || updated.Code != input.Code || !updated.CreatedAt.Equal(created.CreatedAt) {
			t.Fatal("edit changed model identity/status or failed to save")
		}
		detail := stage3Data[mgmt.Model](t, request("GET", path, nil, 200))
		if detail.Remark != input.Remark || len(detail.OutputModalities) != 2 {
			t.Fatal("model details lost JSONB metadata")
		}
		grants := stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", grantPath, nil, 200)).Items
		if len(grants) != 1 || grants[0].ID != created.ID || grants[0].Code != input.Code || len(grants[0].OutputModalities) != 2 {
			t.Fatal("model edit broke group association")
		}
		conflicting := input
		conflicting.Code = models[0].Code
		request("PUT", path, conflicting, 409)
		broken := mgmt.New(NewManagementStore(pool, failedAuditIDs{}), ids, cipher, tester)
		input.Code = "model-must-rollback"
		if _, err := broken.CreateModel(ctx, actor, input, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatal("model creation ignored audit failure")
		}
		if _, err := broken.UpdateModel(ctx, actor, created.ID, input, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatal("model edit ignored audit failure")
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM ai_model WHERE model_code='model-must-rollback'").Scan(&count); err != nil || count != 0 {
			t.Fatal("unaudited model mutation committed")
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM operation_log WHERE target_id=$1 AND operation_type IN ('MODEL_CREATE','MODEL_UPDATE')", created.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal("model audit count incorrect")
		}
	})

	t.Run("audit API contains all management events without secrets", func(t *testing.T) {
		rec := request("GET", "/api/v1/operation-logs?limit=100", nil, 200)
		var stored string
		if err := pool.QueryRow(ctx, "SELECT string_agg(row_to_json(l)::text,' ') FROM operation_log l").Scan(&stored); err != nil {
			t.Fatal(err)
		}
		for _, event := range []string{"MEMBER_CREATE", "MEMBER_STATUS_CHANGE", "ACCESS_KEY_CREATE", "ACCESS_KEY_REVOKE", "GROUP_CREATE", "GROUP_MEMBER_ADD", "GROUP_MEMBER_REMOVE", "GROUP_MODEL_GRANT", "GROUP_MODEL_REVOKE", "MODEL_STATUS_CHANGE", "RESOURCE_CREATE", "RESOURCE_CREDENTIAL_UPDATE", "RESOURCE_STATUS_CHANGE", "RESOURCE_CONNECTION_TEST"} {
			if !strings.Contains(stored, event) {
				t.Errorf("missing audit %s", event)
			}
		}
		for _, secret := range []string{credential, virtualKey, token, "test-stage3-password"} {
			if secret != "" && strings.Contains(stored+logs.String()+rec.Body.String(), secret) {
				t.Fatal("sensitive value leaked into audit or logs")
			}
		}
	})
}
