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

	"github.com/zentrola/zentrola/internal/application/health"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

type connectionTestFunc func(context.Context, string, string, []byte, *catalog.OutboundProxy) mgmt.ConnectionResult

func (f connectionTestFunc) Test(ctx context.Context, protocol, url string, key []byte, proxy *catalog.OutboundProxy) mgmt.ConnectionResult {
	return f(ctx, protocol, url, key, proxy)
}

type failedAuditIDs struct{}

func (failedAuditIDs) NextID(context.Context) (int64, error) {
	return 0, errors.New("test ID failure")
}

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
	ids := idgen.New(pool)
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
	tester := connectionTestFunc(func(_ context.Context, _ string, url string, key []byte, _ *catalog.OutboundProxy) mgmt.ConnectionResult {
		if url != "https://provider.example.com/v1" || string(key) != credential {
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
	if member.Status != "DISABLED" {
		t.Fatalf("new member status=%s, want DISABLED", member.Status)
	}
	group := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"code": "engineering", "name": "研发"}, 201))
	models := stage3Data[struct {
		Items []mgmt.Model `json:"items"`
	}](t, request("GET", "/api/v1/models", nil, 200)).Items
	providers := stage3Data[struct {
		Items []mgmt.Provider `json:"items"`
	}](t, request("GET", "/api/v1/providers", nil, 200)).Items
	if len(models) != 0 || len(providers) != 0 {
		t.Fatal("startup must not create models or providers")
	}
	model := stage3Data[mgmt.Model](t, request("POST", "/api/v1/models", mgmt.ModelInput{
		Code: "management-base-model", Name: "管理端基础模型", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"},
	}, 201))
	request("PATCH", "/api/v1/models/"+sid(model.ID)+"/status", map[string]string{"status": "ACTIVE"}, 200)
	mappedModel := stage3Data[mgmt.Model](t, request("POST", "/api/v1/models", mgmt.ModelInput{
		Code: "management-mapped-model", Name: "管理端映射模型", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"},
	}, 201))
	customProviderInput := mgmt.ProviderInput{
		Name:      "管理端服务商",
		Endpoints: []mgmt.ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://provider.example.com/v1"}},
		Mappings: []mgmt.ProviderMappingInput{
			{ModelID: mappedModel.ID, UpstreamModelCode: "vendor-model-v1"},
			{ModelID: model.ID, UpstreamModelCode: "vendor-model-v2"},
		},
	}
	customProvider := stage3Data[mgmt.Provider](t, request("POST", "/api/v1/providers", customProviderInput, 201))
	missingCredential := request("PATCH", "/api/v1/providers/"+sid(customProvider.ID)+"/status", map[string]string{"status": "ACTIVE"}, 409)
	if !strings.Contains(missingCredential.Body.String(), `"code":"PROVIDER_CREDENTIAL_REQUIRED"`) {
		t.Fatalf("provider without a credential returned an unexpected error: %s", missingCredential.Body.String())
	}
	provider := customProvider
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanup, "DELETE FROM provider_model WHERE provider_id=$1", customProvider.ID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanup, "DELETE FROM provider_endpoint WHERE provider_id=$1", customProvider.ID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanup, "DELETE FROM provider WHERE id=$1", customProvider.ID); err != nil {
			t.Error(err)
		}
	})
	customProviderPath := "/api/v1/providers/" + sid(customProvider.ID)
	providerDetail := stage3Data[mgmt.ProviderDetail](t, request("GET", customProviderPath, nil, 200))
	if len(providerDetail.Mappings) != 2 || providerDetail.Mappings[0].ModelID != mappedModel.ID || providerDetail.Mappings[0].UpstreamModelCode != "vendor-model-v1" || providerDetail.Mappings[0].Priority != 100 {
		t.Fatalf("unexpected provider mappings: %+v", providerDetail.Mappings)
	}
	mappingID := providerDetail.Mappings[0].ID
	customProviderInput.Mappings[0].UpstreamModelCode = "vendor-model-v2"
	customProviderInput.Mappings = customProviderInput.Mappings[:1]
	stage3Data[mgmt.Provider](t, request("PUT", customProviderPath, customProviderInput, 200))
	providerDetail = stage3Data[mgmt.ProviderDetail](t, request("GET", customProviderPath, nil, 200))
	if len(providerDetail.Mappings) != 1 || providerDetail.Mappings[0].ID != mappingID || providerDetail.Mappings[0].UpstreamModelCode != "vendor-model-v2" {
		t.Fatalf("provider mapping update not preserved: %+v", providerDetail.Mappings)
	}
	var deletedMappings int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM provider_model WHERE provider_id=$1 AND is_deleted", customProvider.ID).Scan(&deletedMappings); err != nil || deletedMappings != 1 {
		t.Fatalf("unchecked mapping was not logically deleted: count=%d err=%v", deletedMappings, err)
	}
	memberPath := "/api/v1/members/" + sid(member.ID)
	createdKey := stage3Data[appsec.CreatedKey](t, request("POST", memberPath+"/keys", map[string]string{"name": "Claude Code"}, 201))
	virtualKey := createdKey.Key
	request("PATCH", memberPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
	groupPath := "/api/v1/groups/" + sid(group.ID)
	memberLink := groupPath + "/members/" + sid(member.ID)
	modelLink := groupPath + "/models/" + sid(model.ID)
	q := dbgen.New(pool)
	allowed := func(want bool) {
		t.Helper()
		ok, err := q.HasGroupModelPermission(ctx, dbgen.HasGroupModelPermissionParams{PrincipalID: member.ID, ModelID: model.ID})
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

	t.Run("key issuance and member model status", func(t *testing.T) {
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
		request("PATCH", groupPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		allowed(false)
		activeGroups := stage3Data[struct {
			Items []mgmt.Group `json:"items"`
		}](t, request("GET", "/api/v1/groups?status=ACTIVE", nil, 200)).Items
		for _, activeGroup := range activeGroups {
			if activeGroup.ID == group.ID {
				t.Fatal("disabled group returned by active group filter")
			}
		}
		request("PATCH", groupPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
		allowed(true)
		request("POST", "/api/v1/access-keys/"+sid(createdKey.ID)+"/revoke", nil, 200)
		if _, err := keys.Authenticate(ctx, virtualKey); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("revoked key accepted")
		}
		newer := stage3Data[appsec.CreatedKey](t, request("POST", memberPath+"/keys", map[string]string{"name": "新设备"}, 201))
		type keyPage struct {
			Items []mgmt.Key `json:"items"`
			Next  *string    `json:"nextCursor"`
			Total int64      `json:"total"`
		}
		first := stage3Data[keyPage](t, request("GET", memberPath+"/keys?limit=1", nil, 200))
		if len(first.Items) != 1 || first.Items[0].ID != newer.ID || first.Next == nil || *first.Next != sid(newer.ID) || first.Total != 2 {
			t.Fatal("key list must return the newest key first")
		}
		olderResponse := request("GET", memberPath+"/keys?after="+*first.Next, nil, 200)
		older := stage3Data[keyPage](t, olderResponse)
		if len(older.Items) != 1 || older.Items[0].ID != createdKey.ID || older.Next != nil {
			t.Fatal("key pagination must return older keys without duplicates")
		}
		allResponse := request("GET", memberPath+"/keys", nil, 200)
		if strings.Contains(allResponse.Body.String(), newer.Key) || strings.Contains(allResponse.Body.String(), virtualKey) || strings.Contains(allResponse.Body.String(), "hash") {
			t.Fatal("key history leaked a secret")
		}
		request("POST", "/api/v1/access-keys/"+sid(newer.ID)+"/revoke", nil, 200)
		latestRevoked := stage3Data[keyPage](t, request("GET", memberPath+"/keys?limit=1", nil, 200))
		if len(latestRevoked.Items) != 1 || latestRevoked.Items[0].ID != newer.ID || latestRevoked.Items[0].Status != "REVOKED" {
			t.Fatal("latest key lookup must not silently fall back to an older key")
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
		if err := pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE NOT is_deleted) FROM principal_group_model_permission WHERE group_id=$1 AND model_id=$2", group.ID, model.ID).Scan(&history, &active); err != nil || history != 2 || active != 1 {
			t.Fatal("grant recreated old row")
		}
		if err := pool.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE NOT is_deleted) FROM principal_group_membership WHERE group_id=$1 AND principal_id=$2", group.ID, member.ID).Scan(&history, &active); err != nil || history != 2 || active != 1 {
			t.Fatal("membership recreated old row")
		}
	})

	var first mgmt.Resource
	t.Run("resource encrypted storage recovery and singleton", func(t *testing.T) {
		create := func(name string) mgmt.Resource {
			return stage3Data[mgmt.Resource](t, request("POST", "/api/v1/resources", map[string]string{"providerId": sid(provider.ID), "name": name, "credential": credential}, 201))
		}
		first = create("主资源")
		request("PATCH", "/api/v1/providers/"+sid(provider.ID)+"/status", map[string]string{"status": "ACTIVE"}, 200)
		if first.Status != "ACTIVE" {
			t.Fatal("new provider credential should be active")
		}
		firstPath := "/api/v1/resources/" + sid(first.ID)
		request("POST", "/api/v1/resources", map[string]string{"providerId": sid(provider.ID), "name": "备用配置", "credential": credential}, 409)
		var encrypted []byte
		if err := pool.QueryRow(ctx, "SELECT credential_ciphertext FROM provider_credential WHERE id=$1", first.ID).Scan(&encrypted); err != nil || bytes.Contains(encrypted, []byte(credential)) {
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
		request("PATCH", firstPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		request("PATCH", firstPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
		request("PATCH", firstPath+"/status", map[string]string{"status": "DISABLED"}, 200)
		if _, err := pool.Exec(ctx, "UPDATE provider_credential SET credential_ciphertext=decode(repeat('00',32),'hex') WHERE id=$1", first.ID); err != nil {
			t.Fatal(err)
		}
		request("PATCH", firstPath+"/status", map[string]string{"status": "ACTIVE"}, 422)
		result = stage3Data[mgmt.ConnectionResult](t, request("POST", firstPath+"/test-connection", nil, 200))
		if result.OK || result.Code != "CREDENTIAL_UNRECOVERABLE" {
			t.Fatal("unrecoverable credential test accepted")
		}
		request("PUT", firstPath+"/credential", map[string]string{"credential": credential}, 200)
		recovered := stage3Data[mgmt.Resource](t, request("GET", firstPath, nil, 200))
		if recovered.Status != "ACTIVE" {
			t.Fatalf("replaced credential status=%q; want ACTIVE", recovered.Status)
		}
	})

	t.Run("connection test releases transaction and detects replacement", func(t *testing.T) {
		changing := mgmt.New(NewManagementStore(pool, ids), ids, cipher, connectionTestFunc(func(ctx context.Context, _, _ string, _ []byte, _ *catalog.OutboundProxy) mgmt.ConnectionResult {
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

	t.Run("validation pagination and authorization boundaries", func(t *testing.T) {
		request("POST", "/api/v1/groups", map[string]string{"code": "engineering", "name": "重复"}, 409)
		request("POST", "/api/v1/members", map[string]string{"name": " ", "organizationId": "1"}, 400)
		request("POST", "/api/v1/resources", map[string]string{"providerId": sid(provider.ID), "name": "bad", "credential": "key\nheader"}, 400)
		request("PATCH", memberPath+"/status", map[string]string{"status": "DELETED"}, 400)
		for _, path := range []string{"/api/v1/members/bad", "/api/v1/members?after=-1", "/api/v1/members?limit=101", "/api/v1/members?limit=1&limit=2"} {
			request("GET", path, nil, 400)
		}
		earlier := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]string{"name": "分页较早成员"}, 201))
		latest := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]string{"name": "分页最新成员"}, 201))
		type memberPage struct {
			Items []mgmt.Member `json:"items"`
			Next  *string       `json:"nextCursor"`
			Total int64         `json:"total"`
		}
		page := stage3Data[memberPage](t, request("GET", "/api/v1/members?limit=1", nil, 200))
		if len(page.Items) != 1 || page.Items[0].ID != latest.ID || page.Next == nil || *page.Next != sid(latest.ID) || page.Total != 3 {
			t.Fatal("descending first page must contain the latest member and its cursor")
		}
		inserted := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]string{"name": "翻页期间新增成员"}, 201))
		second := stage3Data[memberPage](t, request("GET", "/api/v1/members?limit=1&after="+*page.Next, nil, 200))
		if len(second.Items) != 1 || second.Items[0].ID != earlier.ID || second.Next == nil || second.Total != 4 {
			t.Fatal("descending second page skipped or repeated a member after insertion")
		}
		tail := stage3Data[memberPage](t, request("GET", "/api/v1/members?after="+*second.Next, nil, 200))
		if len(tail.Items) != 1 || tail.Items[0].ID != member.ID || tail.Next != nil {
			t.Fatal("descending final page must contain only the original member")
		}
		refreshed := stage3Data[memberPage](t, request("GET", "/api/v1/members?limit=1&after=0", nil, 200))
		if len(refreshed.Items) != 1 || refreshed.Items[0].ID != inserted.ID {
			t.Fatal("refresh must show the newly inserted member first")
		}
		assertDescending := func(path string) {
			t.Helper()
			type item struct {
				ID int64 `json:"id,string"`
			}
			page := stage3Data[struct {
				Items []item `json:"items"`
			}](t, request("GET", path, nil, 200))
			for i := 1; i < len(page.Items); i++ {
				if page.Items[i-1].ID <= page.Items[i].ID {
					t.Fatalf("%s is not ordered by descending ID: %d before %d", path, page.Items[i-1].ID, page.Items[i].ID)
				}
			}
		}
		for _, path := range []string{
			"/api/v1/groups",
			"/api/v1/models",
			"/api/v1/providers",
			"/api/v1/resources",
			"/api/v1/operation-logs",
			groupPath + "/members",
			groupPath + "/models",
		} {
			assertDescending(path)
		}
		badActor := actor
		badActor.ID = 0
		if _, err := service.Members(ctx, badActor, mgmt.Page{Limit: 50}); !errors.Is(err, appsec.ErrUnauthenticated) {
			t.Fatal("invalid administrator accepted")
		}
		if _, err := pool.Exec(ctx, "UPDATE principal_group SET is_deleted=true WHERE id=$1", group.ID); err != nil {
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
		created := stage3Data[appsec.CreatedKey](t, request("POST", path+"/keys", map[string]string{"name": "删除测试 Key"}, 201))
		request("PATCH", path+"/status", map[string]string{"status": "ACTIVE"}, 200)
		request("PUT", groupMembers+"/"+sid(m.ID), nil, 200)
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
		if err := pool.QueryRow(ctx, "SELECT p.is_deleted, k.status='REVOKED' AND k.revoked_at IS NOT NULL, g.is_deleted FROM principal p JOIN principal_access_key k ON k.principal_id=p.id JOIN principal_group_membership g ON g.principal_id=p.id WHERE p.id=$1", m.ID).Scan(&deleted, &revoked, &unlinked); err != nil || !deleted || !revoked || !unlinked {
			t.Fatalf("incomplete member deletion: %v %v %v %v", deleted, revoked, unlinked, err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM operation_log WHERE target_id=$1 AND operation_type IN ('MEMBER_CREATE','MEMBER_DELETE')", m.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal("member history missing or deletion audit duplicated")
		}
	})

	t.Run("member creation and editing manage groups atomically", func(t *testing.T) {
		group := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", map[string]string{"name": "用户编辑分组"}, 201))
		groupID := sid(group.ID)
		request("POST", "/api/v1/members", map[string]any{
			"name":     "重复分组用户",
			"groupIds": []string{groupID, groupID},
		}, 400)
		request("POST", "/api/v1/members", map[string]any{
			"name":     "不存在分组用户",
			"groupIds": []string{"9223372036854775806"},
		}, 404)
		member := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]any{
			"name":     "创建时入组用户",
			"remark":   "原备注",
			"groupIds": []string{groupID},
		}, 201))
		path := "/api/v1/members/" + sid(member.ID)
		memberGroups := stage3Data[struct {
			Items []mgmt.Group `json:"items"`
		}](t, request("GET", path+"/groups", nil, 200)).Items
		if len(memberGroups) != 1 || memberGroups[0].ID != group.ID {
			t.Fatal("member creation did not save selected groups")
		}

		updated := stage3Data[mgmt.Member](t, request("PUT", path, map[string]any{
			"name":     "编辑后的用户",
			"remark":   "新备注",
			"groupIds": []string{},
		}, 200))
		if updated.ID != member.ID || updated.Name != "编辑后的用户" || updated.Remark == nil || *updated.Remark != "新备注" {
			t.Fatal("member edit changed identity or lost metadata")
		}
		memberGroups = stage3Data[struct {
			Items []mgmt.Group `json:"items"`
		}](t, request("GET", path+"/groups", nil, 200)).Items
		if len(memberGroups) != 0 {
			t.Fatal("member edit did not remove omitted group")
		}

		broken := mgmt.New(NewManagementStore(pool, failedAuditIDs{}), ids, cipher, tester)
		if _, err := broken.UpdateMemberWithGroups(ctx, actor, member.ID, "不得保存的用户名", "", []int64{group.ID}, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatalf("member edit ignored audit failure: %v", err)
		}
		unchanged := stage3Data[mgmt.Member](t, request("GET", path, nil, 200))
		if unchanged.Name != updated.Name {
			t.Fatal("member edit survived an audit rollback")
		}
		memberGroups = stage3Data[struct {
			Items []mgmt.Group `json:"items"`
		}](t, request("GET", path+"/groups", nil, 200)).Items
		if len(memberGroups) != 0 {
			t.Fatal("member group change survived an audit rollback")
		}

		if _, err := pool.Exec(ctx, "UPDATE principal_group SET status='DISABLED' WHERE id=$1", group.ID); err != nil {
			t.Fatal(err)
		}
		request("GET", "/api/v1/groups?status=UNKNOWN", nil, 400)
		activeGroups := stage3Data[struct {
			Items []mgmt.Group `json:"items"`
		}](t, request("GET", "/api/v1/groups?status=ACTIVE", nil, 200)).Items
		for _, activeGroup := range activeGroups {
			if activeGroup.ID == group.ID {
				t.Fatal("disabled group returned by active group filter")
			}
		}
	})

	t.Run("group creation assigns models and deletion removes permissions atomically", func(t *testing.T) {
		payload := map[string]any{
			"name":     "自动编码分组",
			"remark":   "创建时选择模型",
			"modelIds": []string{sid(model.ID)},
		}
		request("POST", "/api/v1/groups", map[string]any{
			"name":     "重复模型分组",
			"modelIds": []string{sid(model.ID), sid(model.ID)},
		}, 400)
		request("POST", "/api/v1/groups", map[string]any{
			"name":     "不存在模型分组",
			"modelIds": []string{"9223372036854775806"},
		}, 404)

		created := stage3Data[mgmt.Group](t, request("POST", "/api/v1/groups", payload, 201))
		if created.Code != "group-"+sid(created.ID) || created.Name != payload["name"] || created.Remark == nil || *created.Remark != payload["remark"] {
			t.Fatal("group server-generated fields or input fields were not preserved")
		}
		path := "/api/v1/groups/" + sid(created.ID)
		grants := stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", path+"/models", nil, 200)).Items
		if len(grants) != 1 || grants[0].ID != model.ID {
			t.Fatal("models selected during group creation were not granted")
		}
		request("PUT", path, map[string]any{
			"name":     "无效编辑",
			"modelIds": []string{sid(model.ID), sid(model.ID)},
		}, 400)
		updated := stage3Data[mgmt.Group](t, request("PUT", path, map[string]any{
			"name":     "编辑后的分组",
			"remark":   "编辑名称、备注和授权",
			"modelIds": []string{},
		}, 200))
		if updated.ID != created.ID || updated.Code != created.Code || updated.Name != "编辑后的分组" || updated.Remark == nil || *updated.Remark != "编辑名称、备注和授权" {
			t.Fatal("group edit changed identity or lost metadata")
		}
		grants = stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", path+"/models", nil, 200)).Items
		if len(grants) != 0 {
			t.Fatal("group edit did not revoke omitted model")
		}
		updated = stage3Data[mgmt.Group](t, request("PUT", path, map[string]any{
			"name":     updated.Name,
			"remark":   *updated.Remark,
			"modelIds": []string{sid(model.ID)},
		}, 200))
		grants = stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", path+"/models", nil, 200)).Items
		if len(grants) != 1 || grants[0].ID != model.ID {
			t.Fatal("group edit did not grant selected model")
		}

		linkedMember := stage3Data[mgmt.Member](t, request("POST", "/api/v1/members", map[string]string{"name": "分组删除校验用户"}, 201))
		linkedMemberPath := "/api/v1/members/" + sid(linkedMember.ID)
		request("POST", linkedMemberPath+"/keys", map[string]string{"name": "分组权限测试 Key"}, 201)
		request("PATCH", linkedMemberPath+"/status", map[string]string{"status": "ACTIVE"}, 200)
		request("PUT", path+"/members/"+sid(linkedMember.ID), nil, 200)
		permission := func(want bool) {
			t.Helper()
			ok, err := q.HasGroupModelPermission(ctx, dbgen.HasGroupModelPermissionParams{
				PrincipalID: linkedMember.ID,
				ModelID:     model.ID,
			})
			if err != nil || ok != want {
				t.Fatalf("deleted group permission=%v want=%v err=%v", ok, want, err)
			}
		}
		permission(true)

		broken := mgmt.New(NewManagementStore(pool, failedAuditIDs{}), ids, cipher, tester)
		if _, err := broken.UpdateGroupWithModels(ctx, actor, created.ID, "不得保存的分组名", "", []int64{model.ID}, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatalf("group edit ignored audit failure: %v", err)
		}
		unchanged := stage3Data[mgmt.Group](t, request("GET", path, nil, 200))
		if unchanged.Name != updated.Name {
			t.Fatal("group edit survived an audit rollback")
		}
		if err := broken.DeleteGroup(ctx, actor, created.ID, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatalf("group deletion ignored audit failure: %v", err)
		}
		request("GET", path, nil, 200)
		permission(true)
		if _, err := broken.CreateGroupWithModels(ctx, actor, "", "创建回滚分组", "", []int64{model.ID}, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnavailable) {
			t.Fatalf("group creation ignored audit failure: %v", err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM principal_group WHERE group_name='创建回滚分组'").Scan(&count); err != nil || count != 0 {
			t.Fatal("group creation survived an audit rollback")
		}

		savedToken := token
		token = ""
		request("DELETE", path, nil, 401)
		token = savedToken
		request("DELETE", "/api/v1/groups/bad", nil, 400)
		request("DELETE", path, nil, 200)
		request("GET", path, nil, 404)
		request("DELETE", path, nil, 404)
		permission(false)
		var deleted, membersDeleted, modelsDeleted bool
		if err := pool.QueryRow(ctx, `
			SELECT g.is_deleted, pg.is_deleted, permission.is_deleted
			FROM principal_group g
			JOIN principal_group_membership pg ON pg.group_id=g.id
			JOIN principal_group_model_permission permission ON permission.group_id=g.id
			WHERE g.id=$1`, created.ID).Scan(&deleted, &membersDeleted, &modelsDeleted); err != nil || !deleted || !membersDeleted || !modelsDeleted {
			t.Fatalf("incomplete group deletion: %v %v %v %v", deleted, membersDeleted, modelsDeleted, err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM operation_log WHERE target_id=$1 AND operation_type IN ('GROUP_CREATE','GROUP_DELETE')", created.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal("group creation/deletion audit count incorrect")
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM operation_log WHERE target_id=$1 AND operation_type='GROUP_UPDATE'", created.ID).Scan(&count); err != nil || count != 2 {
			t.Fatal("group edit audit count incorrect")
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
		activeModels := stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", "/api/v1/models?status=ACTIVE", nil, 200)).Items
		for _, candidate := range activeModels {
			if candidate.Status != "ACTIVE" || candidate.ID == created.ID {
				t.Fatal("active model filter returned a disabled model")
			}
		}
		disabledModels := stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", "/api/v1/models?status=DISABLED", nil, 200)).Items
		foundCreated := false
		for _, candidate := range disabledModels {
			if candidate.Status != "DISABLED" {
				t.Fatal("disabled model filter returned an active model")
			}
			foundCreated = foundCreated || candidate.ID == created.ID
		}
		if !foundCreated {
			t.Fatal("disabled model filter omitted the created model")
		}
		request("GET", "/api/v1/models?status=UNKNOWN", nil, 400)
		request("GET", "/api/v1/models?status=ACTIVE&status=DISABLED", nil, 400)
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
		activeModels = stage3Data[struct {
			Items []mgmt.Model `json:"items"`
		}](t, request("GET", "/api/v1/models?status=ACTIVE", nil, 200)).Items
		foundCreated = false
		for _, candidate := range activeModels {
			if candidate.ID == created.ID {
				foundCreated = len(candidate.InputModalities) == 2
			}
		}
		if !foundCreated {
			t.Fatal("active model filter lost model modality metadata")
		}
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
		conflicting.Code = model.Code
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
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM model WHERE model_code='model-must-rollback'").Scan(&count); err != nil || count != 0 {
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
		for _, event := range []string{"MEMBER_CREATE", "MEMBER_STATUS_CHANGE", "ACCESS_KEY_CREATE", "ACCESS_KEY_REVOKE", "GROUP_CREATE", "GROUP_UPDATE", "GROUP_STATUS_CHANGE", "GROUP_DELETE", "GROUP_MEMBER_ADD", "GROUP_MEMBER_REMOVE", "GROUP_MODEL_GRANT", "GROUP_MODEL_REVOKE", "MODEL_CREATE", "MODEL_UPDATE", "MODEL_STATUS_CHANGE", "PROVIDER_CREATE", "PROVIDER_UPDATE", "RESOURCE_CREATE", "RESOURCE_CREDENTIAL_UPDATE", "RESOURCE_STATUS_CHANGE", "RESOURCE_CONNECTION_TEST"} {
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
