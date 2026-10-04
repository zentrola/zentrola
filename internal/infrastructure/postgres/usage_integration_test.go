package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	app "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/usage"
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
	ids := idgen.New(pool)
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
	master, err := cryptosec.LoadMasterKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if err != nil {
		t.Fatal(err)
	}
	cipher, _ := cryptosec.NewCredentials(master)
	management := mgmt.New(NewManagementStore(pool, ids), ids, cipher, nil)
	sonnet := createActiveTestModel(t, ctx, management, actor, "claude-sonnet", "Claude Sonnet", []string{"TEXT", "IMAGE"})
	opus := createActiveTestModel(t, ctx, management, actor, "claude-opus", "Claude Opus", []string{"TEXT", "IMAGE"})
	provider, err := management.CreateProvider(ctx, actor, mgmt.ProviderInput{
		Name:      "Anthropic 测试服务商",
		Endpoints: []mgmt.ProviderEndpoint{{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"}},
		Mappings: []mgmt.ProviderMappingInput{
			{ModelID: sonnet.ID, UpstreamModelCode: "sonnet-test"},
			{ModelID: opus.ID, UpstreamModelCode: "sonnet-test"},
		},
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	member, err := management.CreateMember(ctx, actor, "Usage member", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	group, err := management.CreateGroup(ctx, actor, "usage-test", "Usage group", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	modelID := sonnet.ID
	resource, err := management.CreateResource(ctx, actor, provider.ID, "Usage resource", "secret-not-in-usage", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := management.SetProviderStatus(ctx, actor, provider.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
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
	if err := management.SetMemberStatus(ctx, actor, member.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := management.SetGroupMember(ctx, actor, group.ID, member.ID, true, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	store := NewUsageStore(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	writer, err := app.NewWriter(store, logger, app.Options{QueueSize: 50, BatchSize: 50, FlushInterval: time.Hour, WriteTimeout: time.Second})
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
	cfg := httptransport.GatewayOptions{MaxBodyBytes: 1 << 20, RequestTimeout: 200 * time.Millisecond, BodyReadTimeout: time.Second, WriteTimeout: time.Second}
	handler := httptransport.NewGatewayHandler(gw.New(NewGatewayStore(pool), cipher, upstream), cfg, logger, writer)
	server := httptest.NewServer(httptransport.NewRouter(logger, health.New(), httptransport.CORSOptions{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: admins, Keys: keys, Management: management, Gateway: handler, Usage: app.NewQuery(store), UsageWriter: writer}))
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
	for writer.Metrics().Queued < 7 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if writer.Metrics().Queued != 7 {
		t.Fatal("cancelled handler did not submit usage")
	}
	var before int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM usage_record").Scan(&before); err != nil || before != 0 {
		t.Fatal("batch was prematurely persisted")
	}
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := app.NewQuery(store).Query(ctx, actor, app.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100})
	if err != nil || len(page.Items) != 7 || page.Total != 7 {
		t.Fatalf("query: page=%+v err=%v", page, err)
	}
	byID := map[string]app.Row{}
	for _, r := range page.Items {
		byID[r.RequestID] = r
		if r.ID <= 0 || r.PrincipalID != member.ID {
			t.Fatal("wrong principal attribution")
		}
	}
	for _, name := range []string{"normal", "stream"} {
		r := byID[requestIDs[name]]
		if r.Status != "SUCCESS" || r.ModelID != modelID || r.ResourceID != resource.ID || r.ProviderID != provider.ID || r.AttemptNo != 1 || r.InputTokens == nil || *r.InputTokens != 11 || r.OutputTokens == nil || *r.OutputTokens != 7 || r.CachedInputTokens == nil || *r.CachedInputTokens != 4 {
			t.Fatalf("wrong usage attribution: %+v", r)
		}
	}
	if r := byID[requestIDs["cancel"]]; r.Status != "CANCELLED" || r.InputTokens == nil || r.OutputTokens != nil {
		t.Fatal("cancelled usage incorrect")
	}
	for _, name := range []string{"truncate", "rate", "timeout"} {
		r := byID[requestIDs[name]]
		if r.Status != "FAILED" || r.OutputTokens != nil {
			t.Fatalf("failed attempt %s incorrect", name)
		}
	}
	for _, name := range []string{"denied", "invalid"} {
		if _, ok := byID[requestIDs[name]]; ok {
			t.Fatal("local rejection was recorded as upstream usage")
		}
	}
	if r := byID[requestIDs["missing"]]; r.Status != "SUCCESS" || r.InputTokens != nil || r.OutputTokens != nil {
		t.Fatal("missing counts became zero")
	}
	if _, ok := byID[requestIDs["count"]]; ok {
		t.Fatal("count_tokens recorded as inference")
	}
	dashboard, err := app.NewQuery(store).Dashboard(ctx, actor, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || dashboard.ActiveMemberCount < 1 || dashboard.ModelCount < 1 || dashboard.ProviderCount < 1 || dashboard.TotalTokens != 58 || len(dashboard.TokenRanking) != 1 || dashboard.TokenRanking[0].PrincipalID != member.ID || len(dashboard.ClientModelRanking) != 1 || dashboard.ClientModelRanking[0].ModelID != sonnet.ID || dashboard.ClientModelRanking[0].Requests != 7 || len(dashboard.ProviderRanking) != 1 || dashboard.ProviderRanking[0].ProviderID != provider.ID || dashboard.ProviderRanking[0].Calls != 7 {
		t.Fatalf("dashboard aggregation incorrect: result=%+v err=%v", dashboard, err)
	}
	var opusProviderModelID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM provider_model WHERE provider_id=$1 AND model_id=$2 AND NOT is_deleted`, provider.ID, opus.ID).Scan(&opusProviderModelID); err != nil {
		t.Fatal(err)
	}
	attemptID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	inputTokens, outputTokens := int64(2), int64(3)
	if err := store.WriteBatch(ctx, []domain.Event{{
		RequestID:      "same-upstream-model-code",
		ClientProtocol: gw.AnthropicProtocol,
		PrincipalID:    member.ID,
		ModelID:        opus.ID,
		RequestAt:      now,
		CompletedAt:    now,
		Status:         domain.Success,
		Attempt: &domain.Attempt{
			ID: attemptID, ProviderID: provider.ID, ProviderModelID: opusProviderModelID,
			ResourceID: resource.ID, ModelID: opus.ID, InputTokens: &inputTokens,
			OutputTokens: &outputTokens, StartedAt: now, CompletedAt: now, Status: domain.Success,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	backupProvider, err := management.CreateProvider(ctx, actor, mgmt.ProviderInput{
		Name:      "Anthropic 备用服务商",
		Endpoints: []mgmt.ProviderEndpoint{{ProtocolType: "ANTHROPIC", BaseURL: "https://backup.anthropic.example.com"}},
		Mappings:  []mgmt.ProviderMappingInput{{ModelID: sonnet.ID, UpstreamModelCode: "sonnet-backup"}},
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	backupResource, err := management.CreateResource(ctx, actor, backupProvider.ID, "Backup usage resource", "backup-secret-not-in-usage", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := management.SetProviderStatus(ctx, actor, backupProvider.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	var backupProviderModelID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM provider_model WHERE provider_id=$1 AND model_id=$2 AND NOT is_deleted`, backupProvider.ID, sonnet.ID).Scan(&backupProviderModelID); err != nil {
		t.Fatal(err)
	}
	backupAttemptID, err := ids.NextID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backupInputTokens, backupOutputTokens := int64(1), int64(1)
	if err := store.WriteBatch(ctx, []domain.Event{{
		RequestID:      "same-model-different-provider",
		ClientProtocol: gw.AnthropicProtocol,
		PrincipalID:    member.ID,
		ModelID:        sonnet.ID,
		RequestAt:      now,
		CompletedAt:    now,
		Status:         domain.Success,
		Attempt: &domain.Attempt{
			ID: backupAttemptID, ProviderID: backupProvider.ID, ProviderModelID: backupProviderModelID,
			ResourceID: backupResource.ID, ModelID: sonnet.ID, InputTokens: &backupInputTokens,
			OutputTokens: &backupOutputTokens, StartedAt: now, CompletedAt: now, Status: domain.Success,
		},
	}}); err != nil {
		t.Fatal(err)
	}
	dashboard, err = app.NewQuery(store).Dashboard(ctx, actor, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || dashboard.TotalTokens != 65 || len(dashboard.ClientModelRanking) != 2 || dashboard.ClientModelRanking[0].ModelID != sonnet.ID || dashboard.ClientModelRanking[0].Requests != 8 || dashboard.ClientModelRanking[1].ModelID != opus.ID || dashboard.ClientModelRanking[1].Requests != 1 || len(dashboard.ProviderRanking) != 2 || dashboard.ProviderRanking[0].ProviderID != provider.ID || dashboard.ProviderRanking[0].Calls != 8 || dashboard.ProviderRanking[0].Tokens != 63 || dashboard.ProviderRanking[1].ProviderID != backupProvider.ID || dashboard.ProviderRanking[1].Calls != 1 || dashboard.ProviderRanking[1].Tokens != 2 {
		t.Fatalf("model and provider rankings were not aggregated independently: result=%+v err=%v", dashboard, err)
	}
	selfRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/me/usage", nil)
	selfRequest.Header.Set("X-Api-Key", key.Key)
	selfResponse, err := server.Client().Do(selfRequest)
	if err != nil {
		t.Fatal(err)
	}
	var selfBody struct {
		Code string        `json:"code"`
		Data app.SelfUsage `json:"data"`
	}
	if decodeErr := json.NewDecoder(selfResponse.Body).Decode(&selfBody); decodeErr != nil {
		selfResponse.Body.Close()
		t.Fatal(decodeErr)
	}
	selfResponse.Body.Close()
	reportedAt := selfBody.Data.To.UTC()
	wantMonthStart := time.Date(reportedAt.Year(), reportedAt.Month(), 1, 0, 0, 0, 0, time.UTC)
	if selfResponse.StatusCode != http.StatusOK || selfBody.Code != "OK" || selfBody.Data.Tokens != 65 || !selfBody.Data.From.Equal(wantMonthStart) || selfBody.Data.To.Before(now) {
		t.Fatalf("self usage query incorrect: status=%d body=%+v", selfResponse.StatusCode, selfBody)
	}
	// 查询 API：组合过滤、分页、时间、认证和非法参数。
	dashboardRange := "?from=" + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano) + "&to=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	for _, tc := range []struct {
		path          string
		authenticated bool
		status        int
	}{
		{"/api/v1/usage?memberId=" + fmt.Sprint(member.ID) + "&modelId=" + fmt.Sprint(modelID) + "&resourceId=" + fmt.Sprint(resource.ID) + "&limit=2", true, 200},
		{"/api/v1/usage/dashboard" + dashboardRange, true, 200},
		{"/api/v1/usage/statistics" + dashboardRange + "&dimension=member&limit=1", true, 200},
		{"/api/v1/usage/statistics" + dashboardRange + "&dimension=model&limit=1", true, 200},
		{"/api/v1/usage/statistics" + dashboardRange + "&dimension=provider&limit=1", true, 200},
		{"/api/v1/usage/statistics" + dashboardRange + "&dimension=resource", true, 400},
		{"/api/v1/usage/dashboard", true, 400},
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
	filter := app.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100, PrincipalID: &member.ID, ModelID: &modelID, ResourceID: &resource.ID}
	filtered, err := app.NewQuery(store).Query(ctx, actor, filter)
	if err != nil || len(filtered.Items) != 7 || filtered.Total != 7 {
		t.Fatal("combined filters did not isolate attempts")
	}
	providerFilter := app.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100, ProviderID: &backupProvider.ID}
	providerUsage, err := app.NewQuery(store).Query(ctx, actor, providerFilter)
	if err != nil || len(providerUsage.Items) != 1 || providerUsage.Items[0].ProviderID != backupProvider.ID {
		t.Fatal("provider filter did not isolate attempts")
	}
	for _, dimension := range []app.StatisticDimension{app.StatisticMember, app.StatisticModel, app.StatisticProvider} {
		statistics, statisticErr := app.NewQuery(store).Statistics(ctx, actor, app.StatisticFilter{
			Dimension: dimension, From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100,
		})
		if statisticErr != nil || len(statistics.Items) == 0 || statistics.Total == 0 || statistics.Items[0].OverallTokens != 65 {
			t.Fatalf("%s statistics aggregation failed: result=%+v err=%v", dimension, statistics, statisticErr)
		}
	}
	for i := 1; i < len(filtered.Items); i++ {
		if filtered.Items[i-1].ID <= filtered.Items[i].ID {
			t.Fatal("usage records are not ordered by descending ID")
		}
	}
	filter.After = filtered.Items[0].ID
	filter.Limit = 1
	nextPage, err := app.NewQuery(store).Query(ctx, actor, filter)
	if err != nil || len(nextPage.Items) != 1 || nextPage.Items[0].ID != filtered.Items[1].ID || nextPage.Total != 7 {
		t.Fatal("cursor pagination skipped/duplicated a row")
	}
	absent := int64(123)
	filter.PrincipalID = &absent
	if none, err := app.NewQuery(store).Query(ctx, actor, filter); err != nil || len(none.Items) != 0 || none.Total != 0 {
		t.Fatal("member filter ignored")
	}
	filter.PrincipalID = nil
	filter.From = time.Now().Add(time.Hour)
	filter.To = filter.From.Add(time.Hour)
	if none, err := app.NewQuery(store).Query(ctx, actor, filter); err != nil || len(none.Items) != 0 || none.Total != 0 {
		t.Fatal("time range ignored")
	}
	// 约束失败不得留下记录，重复提交不得重复记账。
	makeEvent := func(label string) domain.Event {
		now := time.Now().UTC()
		return domain.Event{RequestID: label, ClientProtocol: gw.AnthropicProtocol, PrincipalID: member.ID, ModelID: modelID, RequestAt: now, CompletedAt: now, Status: domain.Success, Attempt: &domain.Attempt{ProviderID: provider.ID, ProviderModelID: 1, ResourceID: resource.ID, ModelID: modelID, StartedAt: now, CompletedAt: now, Status: domain.Success}}
	}
	bad := makeEvent("usage-atomic-test")
	negative := int64(-1)
	bad.Attempt.InputTokens = &negative
	if err := store.WriteBatch(ctx, []domain.Event{bad}); err == nil {
		t.Fatal("invalid attempt accepted")
	}
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM usage_record WHERE request_id=$1", bad.RequestID).Scan(&count)
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
