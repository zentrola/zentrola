package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

func TestOpenAIIntegration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids := idgen.New(pool)
	securityStore := NewSecurityStore(pool, ids)
	passwords, _ := cryptosec.NewPasswords(4)
	entropy := make([]byte, 32)
	_, _ = rand.Read(entropy)
	tokens, _ := cryptosec.NewJWT(base64.StdEncoding.EncodeToString(entropy))
	admins := appsec.NewAdmin(securityStore, passwords, tokens, admin.LoginPolicy{MaxFailures: 5, LockDuration: time.Minute})
	if err := admins.Bootstrap(ctx, "admin", "openai-test-password"); err != nil {
		t.Fatal(err)
	}
	login, err := admins.Login(ctx, "admin", "openai-test-password", appsec.RequestMeta{})
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
	flash := createActiveTestModel(t, ctx, management, actor, "deepseek-v4-flash", "DeepSeek V4 Flash", []string{"TEXT"})
	pro := createActiveTestModel(t, ctx, management, actor, "deepseek-v4-pro", "DeepSeek V4 Pro", []string{"TEXT"})
	claude := createActiveTestModel(t, ctx, management, actor, "claude-sonnet", "Claude Sonnet", []string{"TEXT", "IMAGE"})
	provider := createTestProvider(t, ctx, pool, management, actor, "DeepSeek 测试服务商",
		[]mgmt.ProviderEndpoint{
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.deepseek.com/anthropic"},
			{ProtocolType: "OPENAI", BaseURL: "https://api.deepseek.com"},
		},
		[]mgmt.ProviderMappingInput{
			{ModelID: flash.ID, UpstreamModelCode: "deepseek-v4-flash"},
			{ModelID: pro.ID, UpstreamModelCode: "deepseek-v4-pro"},
		})
	anthropicProvider := createTestProvider(t, ctx, pool, management, actor, "Anthropic 测试服务商",
		[]mgmt.ProviderEndpoint{{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"}},
		[]mgmt.ProviderMappingInput{{ModelID: claude.ID, UpstreamModelCode: "claude-sonnet"}})
	member, err := management.CreateMember(ctx, actor, "OpenAI integration", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	group, err := management.CreateGroup(ctx, actor, "openai-test", "OpenAI group", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	modelID, claudeID := flash.ID, claude.ID
	resource, err := management.CreateResource(ctx, actor, provider.ID, "OpenAI resource", "openai-upstream-secret", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	originalResourceID := resource.ID
	if err := management.SetProviderStatus(ctx, actor, provider.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	anthropicResource, err := management.CreateResource(ctx, actor, anthropicProvider.ID, "Anthropic resource", "anthropic-upstream-secret", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := management.SetProviderStatus(ctx, actor, anthropicProvider.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := management.SetGroupModel(ctx, actor, group.ID, modelID, true, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	keys := appsec.NewKeys(securityStore, ids)
	key, err := keys.Create(ctx, actor, member.ID, "openai-key", nil, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := management.SetMemberStatus(ctx, actor, member.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := management.SetGroupMember(ctx, actor, group.ID, member.ID, true, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	usageStore := NewUsageStore(pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	writer, err := usageapp.NewWriter(usageStore, logger, usageapp.Options{QueueSize: 100, BatchSize: 100, FlushInterval: time.Hour, WriteTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close(context.Background()) })
	var calls atomic.Int32
	const chunk = "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_test\",\"function\":{\"arguments\":\"{\\\"n\\\":9007199254740993}\"}}]}}]}\n\n"
	const tail = "data: {\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":7,\"prompt_cache_hit_tokens\":2}}\n\ndata: [DONE]\n\n"
	upstream := gatewayOpenFunc(func(c context.Context, r gw.Route, q gw.Request, secret []byte) (*gw.Response, error) {
		calls.Add(1)
		wantSecret := "openai-upstream-secret"
		if r.ProviderID == anthropicProvider.ID {
			wantSecret = "anthropic-upstream-secret"
		}
		if string(secret) != wantSecret {
			t.Error("wrong shared resource credential")
		}
		media := "application/json"
		status := 200
		body := `{"object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"prompt_cache_hit_tokens":2}}`
		var reader io.ReadCloser
		if q.Protocol == gw.AnthropicProtocol {
			wantBaseURL := "https://api.deepseek.com/anthropic"
			if r.ProviderID == anthropicProvider.ID {
				wantBaseURL = "https://api.anthropic.com"
			}
			if r.BaseURL != wantBaseURL {
				t.Error("Anthropic URL changed")
			}
			body = `{"id":"msg_compat","type":"message","role":"assistant","model":"claude-sonnet","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":7}}`
		} else {
			if r.BaseURL != "https://api.deepseek.com" {
				t.Error("wrong OpenAI route")
			}
			if q.Protocol == gw.OpenAIImagesProtocol {
				if q.Path != "/v1/images/generations" {
					t.Error("wrong OpenAI Images route")
				}
				body = `{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`
			} else if q.Protocol == gw.OpenAIResponsesProtocol {
				if q.Path != "/v1/responses" {
					t.Error("wrong OpenAI Responses route")
				}
				body = `{"object":"response","status":"completed","usage":{"input_tokens":12,"output_tokens":7}}`
			} else if q.Path != "/v1/chat/completions" {
				t.Error("wrong OpenAI Chat Completions route")
			}
			switch {
			case strings.Contains(string(q.Body), `"scenario":"timeout"`):
				<-c.Done()
				return nil, gw.ErrTimeout
			case strings.Contains(string(q.Body), `"scenario":"cancel"`):
				media = "text/event-stream"
				reader = &usageInterruptedBody{strings.NewReader(chunk), c, true}
			case strings.Contains(string(q.Body), `"scenario":"truncate"`):
				media = "text/event-stream"
				reader = &usageInterruptedBody{strings.NewReader(chunk), c, false}
			case strings.Contains(string(q.Body), `"stream":true`):
				media = "text/event-stream"
				body = chunk + tail
			case strings.Contains(string(q.Body), `"scenario":"rate"`):
				status = 429
				body = `{"error":{"message":"slow down","type":"rate_limit_error"}}`
			case strings.Contains(string(q.Body), `"scenario":"tool"`):
				body = `{"object":"chat.completion","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_test","type":"function","function":{"name":"sum","arguments":"{\"n\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7}}`
			case strings.Contains(string(q.Body), `"role":"tool"`):
				if !strings.Contains(string(q.Body), `"tool_call_id":"call_test"`) || !strings.Contains(string(q.Body), `9007199254740993`) {
					t.Error("tool result mutated")
				}
			}
		}
		if reader == nil {
			reader = io.NopCloser(strings.NewReader(body))
		}
		return &gw.Response{Status: status, Headers: map[string][]string{"Content-Type": {media}, "Retry-After": {"2"}}, Body: reader}, nil
	})
	cfg := httptransport.GatewayOptions{MaxBodyBytes: 1 << 20, RequestTimeout: 200 * time.Millisecond, BodyReadTimeout: time.Second, WriteTimeout: time.Second}
	service := gw.New(NewGatewayStore(pool), cipher, gw.NewCompatibleUpstream(upstream, upstream))
	server := httptest.NewServer(httptransport.NewRouter(logger, health.New(), httptransport.CORSOptions{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: admins, Keys: keys, Management: management, Gateway: httptransport.NewGatewayHandler(service, cfg, logger, writer), OpenAI: httptransport.NewOpenAIGatewayHandler(service, cfg, logger, writer), Usage: usageapp.NewQuery(usageStore), UsageWriter: writer}))
	defer server.Close()
	requestIDs := map[string]string{}
	call := func(name, method, path, body, credential string, want int) []byte {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		requestIDs[name] = resp.Header.Get("X-Request-ID")
		if resp.StatusCode != want {
			t.Fatalf("%s status %d: %s", name, resp.StatusCode, data)
		}
		if want >= 400 && strings.HasPrefix(path, "/v1/") {
			var value map[string]json.RawMessage
			if json.Unmarshal(data, &value) != nil || value["error"] == nil || value["data"] != nil || value["type"] != nil {
				t.Fatal("non-native OpenAI error envelope")
			}
		}
		return data
	}
	list := call("models", "GET", "/v1/models", "", key.Key, 200)
	var discovery struct {
		Object string     `json:"object"`
		Data   []gw.Model `json:"data"`
	}
	_ = json.Unmarshal(list, &discovery)
	if discovery.Object != "list" || len(discovery.Data) != 1 || discovery.Data[0].ID != "deepseek-v4-flash" {
		t.Fatal("model discovery leaked unauthorized models")
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"normal", `{"model":"deepseek-v4-flash","stream":null}`, 200}, {"stream", `{"model":"deepseek-v4-flash","stream":true,"stream_options":{"include_usage":true}}`, 200}, {"tool", `{"model":"deepseek-v4-flash","scenario":"tool"}`, 200}, {"tool_result", `{"model":"deepseek-v4-flash","messages":[{"role":"tool","tool_call_id":"call_test","content":"9007199254740993"}]}`, 200}, {"rate", `{"model":"deepseek-v4-flash","scenario":"rate"}`, 429}, {"timeout", `{"model":"deepseek-v4-flash","scenario":"timeout"}`, 504}, {"truncate", `{"model":"deepseek-v4-flash","scenario":"truncate"}`, 200}, {"denied", `{"model":"deepseek-v4-pro"}`, 403}, {"bad", `{`, 400}} {
		body := call(tc.name, "POST", "/v1/chat/completions", tc.body, key.Key, tc.status)
		if tc.name == "stream" && string(body) != chunk+tail {
			t.Fatal("SSE changed")
		}
	}
	call("responses", "POST", "/v1/responses", `{"model":"deepseek-v4-flash","input":"hello"}`, key.Key, 200)
	images := call("images", "POST", "/v1/images/generations", `{"model":"deepseek-v4-flash","prompt":"draw an otter"}`, key.Key, 200)
	if !strings.Contains(string(images), `"b64_json":"aW1hZ2U="`) {
		t.Fatal("Images response changed")
	}
	call("anthropic", "POST", "/anthropic/v1/messages", `{"model":"deepseek-v4-flash"}`, key.Key, 200)
	before := calls.Load()
	call("no_key", "POST", "/v1/chat/completions", `{}`, "", 401)
	call("admin_key", "POST", "/v1/chat/completions", `{}`, login.Token, 401)
	call("method", "GET", "/v1/chat/completions", "", key.Key, 405)
	call("invalid_responses", "POST", "/v1/responses", `{}`, key.Key, 400)
	call("query", "GET", "/v1/models?bad=1", "", key.Key, 400)
	if calls.Load() != before {
		t.Fatal("invalid request reached upstream")
	}
	// 仅配置 Anthropic endpoint 的 Claude 模型可由 OpenAI 协议调用，响应仍为 OpenAI 格式。
	if err := management.SetGroupModel(ctx, actor, group.ID, claudeID, true, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	converted := call("protocol_fallback", "POST", "/v1/chat/completions", `{"model":"claude-sonnet","messages":[{"role":"user","content":"hello"}]}`, key.Key, 200)
	if !strings.Contains(string(converted), `"object":"chat.completion"`) {
		t.Fatal("Anthropic fallback did not return OpenAI response")
	}
	before = calls.Load()
	call("images_anthropic_only", "POST", "/v1/images/generations", `{"model":"claude-sonnet","prompt":"draw an otter"}`, key.Key, 503)
	if calls.Load() != before {
		t.Fatal("Images request reached an Anthropic-only route")
	}
	if err := management.DeleteResource(ctx, actor, resource.ID, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if err := management.DeleteResource(ctx, actor, anthropicResource.ID, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	list = call("disabled_models", "GET", "/v1/models", "", key.Key, 200)
	_ = json.Unmarshal(list, &discovery)
	if len(discovery.Data) != 0 {
		t.Fatal("disabled resource still discoverable")
	}
	call("disabled", "POST", "/v1/chat/completions", `{"model":"deepseek-v4-flash"}`, key.Key, 503)
	resource, err = management.CreateResource(ctx, actor, provider.ID, "OpenAI replacement resource", "openai-upstream-secret", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	req, _ := http.NewRequestWithContext(cancelCtx, "POST", server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4-flash","scenario":"cancel"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key.Key)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	first := make([]byte, len(chunk))
	_, err = io.ReadFull(resp.Body, first)
	if err != nil {
		t.Fatal(err)
	}
	requestIDs["cancel"] = resp.Header.Get("X-Request-ID")
	queued := writer.Metrics().Queued
	cancel()
	resp.Body.Close()
	until := time.Now().Add(time.Second)
	for writer.Metrics().Queued == queued && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	if err := keys.Revoke(ctx, actor, key.ID, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	call("revoked", "GET", "/v1/models", "", key.Key, 401)
	if err := writer.Close(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := usageapp.NewQuery(usageStore).Query(ctx, actor, usageapp.Filter{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]usageapp.Row{}
	for _, r := range page.Items {
		byID[r.RequestID] = r
	}
	tokenValue := func(value *int64) any {
		if value == nil {
			return nil
		}
		return *value
	}
	for _, name := range []string{"normal", "stream", "tool", "tool_result"} {
		r := byID[requestIDs[name]]
		if r.ClientProtocol != "OPENAI_CHAT" || r.Status != "SUCCESS" || r.InputTokens == nil || *r.InputTokens != 12 || r.OutputTokens == nil || *r.OutputTokens != 7 || r.ResourceID != originalResourceID {
			t.Fatalf("OpenAI usage %s invalid: input=%v output=%v cached=%v row=%+v", name, tokenValue(r.InputTokens), tokenValue(r.OutputTokens), tokenValue(r.CachedInputTokens), r)
		}
	}
	if r := byID[requestIDs["responses"]]; r.ClientProtocol != "OPENAI_RESPONSES" || r.ProviderModelID != byID[requestIDs["normal"]].ProviderModelID {
		t.Fatalf("OpenAI Responses did not share the OpenAI provider endpoint: %+v", r)
	}
	if r := byID[requestIDs["images"]]; r.ClientProtocol != "OPENAI_IMAGES" || r.Status != "SUCCESS" || r.InputTokens != nil || r.OutputTokens != nil || r.ProviderModelID != byID[requestIDs["normal"]].ProviderModelID {
		t.Fatalf("OpenAI Images usage invalid: %+v", r)
	}
	if r := byID[requestIDs["anthropic"]]; r.ClientProtocol != "ANTHROPIC_MESSAGES" || r.Status != "SUCCESS" || r.ProviderModelID != byID[requestIDs["normal"]].ProviderModelID {
		t.Fatal("shared provider-model attribution lost")
	}
	for _, name := range []string{"wrong_protocol", "images_anthropic_only", "denied", "disabled", "bad"} {
		if _, ok := byID[requestIDs[name]]; ok {
			t.Fatal("local failure was recorded as upstream usage")
		}
	}
	if r := byID[requestIDs["cancel"]]; r.Status != "CANCELLED" || r.OutputTokens != nil {
		t.Fatal("cancelled usage incomplete")
	}
}
