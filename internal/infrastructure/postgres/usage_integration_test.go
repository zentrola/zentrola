package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	app "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/usage"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

type usageInterruptedBody struct {
	io.Reader
	ctx  context.Context
	wait bool
}

func (b *usageInterruptedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		if b.wait {
			<-b.ctx.Done()
			return 0, b.ctx.Err()
		}
		return 0, io.ErrUnexpectedEOF
	}
	return n, err
}
func (b *usageInterruptedBody) Close() error { return nil }

func TestStage5Integration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids, err := idgen.New(8)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.New(NewBootstrapStore(pool), ids, "sonnet-test", "opus-test").Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	securityStore := NewSecurityStore(pool, ids)
	passwords, _ := cryptosec.NewPasswords(4)
	entropy := make([]byte, 32)
	_, _ = rand.Read(entropy)
	tokens, _ := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(entropy))
	admins := appsec.NewAdmin(securityStore, passwords, tokens, admin.LoginPolicy{MaxFailures: 5, LockDuration: time.Minute})
	if err := admins.Bootstrap(ctx, "admin", "stage5-test-password"); err != nil {
		t.Fatal(err)
	}
	login, err := admins.Login(ctx, "admin", "stage5-test-password", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := admins.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	master, err := cryptosec.LoadMasterKey("", "", filepath.Join(t.TempDir(), "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	cipher, _ := cryptosec.NewCredentials(master)
	management := mgmt.New(NewManagementStore(pool, ids), ids, cipher, nil)
	member, err := management.CreateMember(ctx, actor, "Usage member", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	group, err := management.CreateGroup(ctx, actor, "usage-test", "Usage group", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	models, _ := management.Models(ctx, actor, mgmt.Page{Limit: 50}, "")
	var modelID int64
	for _, m := range models {
		if m.Code == "claude-sonnet" {
			modelID = m.ID
		}
	}
	providers, _ := management.Providers(ctx, actor, mgmt.Page{Limit: 50})
	resource, err := management.CreateResource(ctx, actor, providers[0].ID, "Usage resource", "secret-not-in-usage", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := management.SetResourceStatus(ctx, actor, resource.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := management.SetGroupMember(ctx, actor, group.ID, member.ID, true, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := management.SetGroupModel(ctx, actor, group.ID, modelID, true, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	keys := appsec.NewKeys(securityStore, ids)
	key, err := keys.Create(ctx, actor, member.ID, "usage-test", nil, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	store := NewUsageStore(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	writer, err := app.NewWriter(store, ids, logger, app.Options{QueueSize: 50, BatchSize: 50, FlushInterval: time.Hour, WriteTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close(context.Background()) })
	const start = "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":1,\"cache_read_input_tokens\":4}}}\n\n"
	upstream := gatewayOpenFunc(func(c context.Context, r gw.Route, q gw.Request, secret []byte) (*gw.Response, error) {
		status := 200
		media := "application/json"
		body := `{"type":"message","content":[{"type":"text","text":"private response"}],"usage":{"input_tokens":11,"output_tokens":7,"cache_read_input_tokens":4}}`
		var reader io.ReadCloser
		switch {
		case strings.Contains(string(q.Body), `"timeout"`):
			<-c.Done()
			return nil, gw.ErrTimeout
		case strings.Contains(string(q.Body), `"stream":true`):
			media = "text/event-stream"
			body = start + "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
		case strings.Contains(string(q.Body), `"cancel"`):
			media = "text/event-stream"
			reader = &usageInterruptedBody{strings.NewReader(start), c, true}
		case strings.Contains(string(q.Body), `"truncate"`):
			media = "text/event-stream"
			reader = &usageInterruptedBody{strings.NewReader(start), c, false}
		case strings.Contains(string(q.Body), `"rate"`):
			status = 429
			body = `{"type":"error","error":{"message":"private upstream diagnostic"}}`
		case strings.Contains(string(q.Body), `"missing"`):
			body = `{"type":"message","content":[]}`
		case q.Path == "/v1/messages/count_tokens":
			body = `{"input_tokens":999}`
		}
		if reader == nil {
			reader = io.NopCloser(strings.NewReader(body))
		}
		return &gw.Response{Status: status, Headers: map[string][]string{"Content-Type": {media}}, Body: reader}, nil
	})
	cfg := config.Gateway{MaxBodyBytes: 1 << 20, RequestTimeout: 200 * time.Millisecond, BodyReadTimeout: time.Second, WriteTimeout: time.Second}
	handler := httptransport.NewGatewayHandler(gw.New(NewGatewayStore(pool), cipher, upstream), cfg, logger, writer)
	server := httptest.NewServer(httptransport.NewRouter(logger, health.New(), config.CORS{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: admins, Keys: keys, Management: management, Gateway: handler, Usage: app.NewQuery(store), UsageWriter: writer}))
	defer server.Close()
	requestIDs := map[string]string{}
	call := func(name, body, path string, want int) {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", key.Key)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s status %d", name, resp.StatusCode)
		}
		requestIDs[name] = resp.Header.Get("X-Request-ID")
	}
	for _, scenario := range []string{"normal", "missing", "rate", "timeout", "truncate"} {
		want := 200
		if scenario == "rate" {
			want = 429
		}
		if scenario == "timeout" {
			want = 504
		}
		call(scenario, fmt.Sprintf(`{"model":"claude-sonnet","scenario":%q}`, scenario), "/anthropic/v1/messages", want)
	}
	call("stream", `{"model":"claude-sonnet","stream":true}`, "/anthropic/v1/messages", 200)
	call("denied", `{"model":"claude-opus"}`, "/anthropic/v1/messages", 403)
	call("invalid", `{`, "/anthropic/v1/messages", 400)
	call("count", `{"model":"claude-sonnet"}`, "/anthropic/v1/messages/count_tokens", 200)
	cancelCtx, cancel := context.WithCancel(ctx)
	req, _ := http.NewRequestWithContext(cancelCtx, "POST", server.URL+"/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet","scenario":"cancel"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", key.Key)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	first := make([]byte, len(start))
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatal(err)
	}
	requestIDs["cancel"] = resp.Header.Get("X-Request-ID")
	cancel()
	resp.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for writer.Metrics().Queued < 9 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if writer.Metrics().Queued != 9 {
		t.Fatal("cancelled handler did not submit usage")
	}
	var before int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ai_request").Scan(&before); err != nil || before != 0 {
		t.Fatal("batch was prematurely persisted")
	}
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := app.NewQuery(store).Query(ctx, actor, app.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100})
	if err != nil || len(rows) != 9 {
		t.Fatalf("query: rows=%d err=%v", len(rows), err)
	}
	byID := map[string]app.Row{}
	for _, r := range rows {
		byID[r.RequestID] = r
		if r.PrincipalID != member.ID {
			t.Fatal("wrong principal attribution")
		}
	}
	for _, name := range []string{"normal", "stream"} {
		r := byID[requestIDs[name]]
		if r.Status != "SUCCESS" || r.ModelID == nil || *r.ModelID != modelID || r.ResourceID == nil || *r.ResourceID != resource.ID || r.ProviderID == nil || *r.ProviderID != providers[0].ID || r.AttemptNo == nil || *r.AttemptNo != 1 || r.InputTokens == nil || *r.InputTokens != 11 || r.OutputTokens == nil || *r.OutputTokens != 7 || r.CachedInputTokens == nil || *r.CachedInputTokens != 4 {
			t.Fatalf("wrong usage attribution: %+v", r)
		}
	}
	if r := byID[requestIDs["cancel"]]; r.Status != "CANCELLED" || r.InputTokens == nil || r.OutputTokens != nil {
		t.Fatal("cancelled usage incorrect")
	}
	for _, name := range []string{"truncate", "rate", "timeout"} {
		r := byID[requestIDs[name]]
		if r.Status != "FAILED" || r.UsageID == nil || r.OutputTokens != nil {
			t.Fatalf("failed attempt %s incorrect", name)
		}
	}
	for _, name := range []string{"denied", "invalid"} {
		r := byID[requestIDs[name]]
		if r.Status != "FAILED" || r.UsageID != nil {
			t.Fatal("local rejection fabricated attempt")
		}
	}
	if r := byID[requestIDs["missing"]]; r.Status != "SUCCESS" || r.InputTokens != nil || r.OutputTokens != nil {
		t.Fatal("missing counts became zero")
	}
	if _, ok := byID[requestIDs["count"]]; ok {
		t.Fatal("count_tokens recorded as inference")
	}
	// 查询 API：组合过滤、分页、时间、认证、非法参数和组织边界。
	for _, tc := range []struct {
		path          string
		authenticated bool
		status        int
	}{
		{"/api/v1/usage?memberId=" + fmt.Sprint(member.ID) + "&modelId=" + fmt.Sprint(modelID) + "&resourceId=" + fmt.Sprint(resource.ID) + "&limit=2", true, 200},
		{"/api/v1/usage/writer", true, 200}, {"/api/v1/usage", false, 401}, {"/api/v1/usage?limit=101", true, 400}, {"/api/v1/usage?resourceId=-1", true, 400}, {"/api/v1/usage?from=invalid", true, 400}, {"/api/v1/usage?limit=1&limit=2", true, 400},
	} {
		req, _ := http.NewRequest("GET", server.URL+tc.path, nil)
		if tc.authenticated {
			req.Header.Set("Authorization", "Bearer "+login.Token)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status || strings.Contains(string(body), "private response") || strings.Contains(string(body), "secret-not-in-usage") {
			t.Fatal("query isolation/validation failed")
		}
	}
	forged := actor
	filter := app.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100, PrincipalID: &member.ID, ModelID: &modelID, ResourceID: &resource.ID}
	filtered, err := app.NewQuery(store).Query(ctx, actor, filter)
	if err != nil || len(filtered) != 7 {
		t.Fatal("combined filters did not isolate attempts")
	}
	for i := 1; i < len(filtered); i++ {
		if filtered[i-1].ID <= filtered[i].ID {
			t.Fatal("usage records are not ordered by descending ID")
		}
	}
	filter.After = filtered[0].ID
	filter.Limit = 1
	page, err := app.NewQuery(store).Query(ctx, actor, filter)
	if err != nil || len(page) != 1 || page[0].ID != filtered[1].ID {
		t.Fatal("cursor pagination skipped/duplicated a row")
	}
	absent := int64(123)
	filter.PrincipalID = &absent
	if none, err := app.NewQuery(store).Query(ctx, actor, filter); err != nil || len(none) != 0 {
		t.Fatal("member filter ignored")
	}
	filter.PrincipalID = nil
	filter.From = time.Now().Add(time.Hour)
	filter.To = filter.From.Add(time.Hour)
	if none, err := app.NewQuery(store).Query(ctx, actor, filter); err != nil || len(none) != 0 {
		t.Fatal("time range ignored")
	}
	forged.OrganizationID++
	if _, err := store.Query(ctx, forged, app.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 10}); err == nil {
		t.Fatal("cross organization query accepted")
	}
	// 同事务回滚和幂等：第二张表约束失败时第一张表也不得保留。
	makeEvent := func(label string) domain.Event {
		rid, _ := ids.NextID()
		aid, _ := ids.NextID()
		now := time.Now().UTC()
		return domain.Event{ID: rid, OrganizationID: actor.OrganizationID, RequestID: label, PrincipalID: member.ID, ModelID: modelID, RequestAt: now, CompletedAt: now, Status: domain.Success, Attempt: &domain.Attempt{ID: aid, ProviderID: providers[0].ID, ProviderModelID: 1, ResourceID: resource.ID, ModelID: modelID, StartedAt: now, CompletedAt: now, Status: domain.Success}}
	}
	bad := makeEvent("usage-atomic-test")
	negative := int64(-1)
	bad.Attempt.InputTokens = &negative
	if err := store.WriteBatch(ctx, []domain.Event{bad}); err == nil {
		t.Fatal("invalid attempt accepted")
	}
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM ai_request WHERE request_id=$1", bad.RequestID).Scan(&count)
	if count != 0 {
		t.Fatal("partial fact committed")
	}
	good := makeEvent("usage-idempotent-test")
	for i := 0; i < 2; i++ {
		if err := store.WriteBatch(ctx, []domain.Event{good}); err != nil {
			t.Fatal(err)
		}
	}
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM usage_record WHERE request_id=$1", good.RequestID).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate usage recorded")
	}
}
