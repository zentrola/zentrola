package postgres

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

type gatewayOpenFunc func(context.Context, gw.Route, gw.Request, []byte) (*gw.Response, error)

func (f gatewayOpenFunc) Open(ctx context.Context, r gw.Route, q gw.Request, c []byte) (*gw.Response, error) {
	return f(ctx, r, q, c)
}

func TestStage4Integration(t *testing.T) {
	ctx, pool, _ := integrationDatabase(t)
	ids := idgen.New(pool)
	if err := bootstrap.New(NewBootstrapStore(pool), ids).Initialize(ctx); err != nil {
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
	if err := admins.Bootstrap(ctx, "admin", "stage4-test-password"); err != nil {
		t.Fatal(err)
	}
	login, err := admins.Login(ctx, "admin", "stage4-test-password", appsec.RequestMeta{})
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
	cipher, err := cryptosec.NewCredentials(master)
	if err != nil {
		t.Fatal(err)
	}
	management := mgmt.New(NewManagementStore(pool, ids), ids, cipher, nil)
	sonnet := createActiveTestModel(t, ctx, management, actor, "claude-sonnet", "Claude Sonnet", []string{"TEXT", "IMAGE"})
	opus := createActiveTestModel(t, ctx, management, actor, "claude-opus", "Claude Opus", []string{"TEXT", "IMAGE"})
	provider := createActiveTestProvider(t, ctx, pool, management, actor, "Anthropic 测试服务商",
		[]mgmt.ProviderEndpoint{{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"}},
		[]mgmt.ProviderMappingInput{
			{ModelID: sonnet.ID, UpstreamModelCode: "sonnet-upstream"},
			{ModelID: opus.ID, UpstreamModelCode: "opus-upstream"},
		})
	member, err := management.CreateMember(ctx, actor, "网关测试成员", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	group, err := management.CreateGroup(ctx, actor, "gateway-test", "网关测试组", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	modelID := sonnet.ID
	const providerKey = "stage4-provider-secret"
	resource, err := management.CreateResource(ctx, actor, provider.ID, "网关资源", providerKey, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	keys := appsec.NewKeys(securityStore, ids)
	key, err := keys.Create(ctx, actor, member.ID, "gateway", nil, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if err := management.SetMemberStatus(ctx, actor, member.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	streamRelease := make(chan struct{})
	defer close(streamRelease)
	streamCancelled := make(chan struct{}, 4)
	const firstEvent = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"model\":\"sonnet-upstream\"}}\n\n"
	const tailEvents = "event: future_event\ndata: {\"opaque\":true}\n\nevent: content_block_delta\ndata: {\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"id\\\":9007199254740993}\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var input struct {
			Model    string `json:"model"`
			Scenario string `json:"test_scenario"`
		}
		_ = json.Unmarshal(data, &input)
		if input.Model != "sonnet-upstream" {
			t.Error("logical model was not resolved")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "untrusted-upstream-id")
		w.Header().Set("Set-Cookie", "untrusted=secret")
		w.Header().Set("Authorization", "Bearer upstream-secret")
		w.Header().Set("Request-Id", "upstream-request-id")
		w.Header().Set("Connection", "Anthropic-Ratelimit-Hidden")
		w.Header().Set("Anthropic-Ratelimit-Hidden", "hop-by-hop")
		w.Header().Set("Anthropic-Ratelimit-Requests-Remaining", "10")
		if r.URL.Path == "/v1/messages/count_tokens" {
			io.WriteString(w, `{"input_tokens":42}`)
			return
		}
		switch input.Scenario {
		case "stream", "cancel":
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, firstEvent)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				streamCancelled <- struct{}{}
				return
			case <-streamRelease:
			}
			io.WriteString(w, tailEvents)
		case "complete_stream":
			w.Header().Set("Content-Type", "text/event-stream")
			payload := firstEvent + strings.Repeat("event: ping\ndata: {\"type\":\"ping\"}\n\n", 4096) + tailEvents
			for len(payload) > 0 {
				n := min(len(payload), 1001)
				io.WriteString(w, payload[:n])
				w.(http.Flusher).Flush()
				payload = payload[n:]
			}
		case "timeout":
			<-r.Context().Done()
		case "rate":
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"Slow down."}}`)
		case "truncate":
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(firstEvent), firstEvent)
			rw.Flush()
		case "tool_result":
			if !bytes.Contains(data, []byte(`"tool_use_id":"toolu_123"`)) || !bytes.Contains(data, []byte(`9007199254740993`)) {
				t.Error("tool result ID or integer corrupted")
			}
			io.WriteString(w, `{"type":"message","model":"sonnet-upstream","content":[{"type":"text","text":"Tool result received."}],"stop_reason":"end_turn"}`)
		default:
			io.WriteString(w, `{"type":"message","model":"sonnet-upstream","content":[{"type":"tool_use","id":"toolu_123","name":"read_file","input":{"path":"sample.txt"}}],"stop_reason":"tool_use"}`)
		}
	}))
	defer mock.Close()
	upstream := gatewayOpenFunc(func(ctx context.Context, route gw.Route, input gw.Request, credential []byte) (*gw.Response, error) {
		calls.Add(1)
		if string(credential) != providerKey || route.ResourceID != resource.ID || route.ModelID != modelID {
			t.Error("wrong resolved identity or credential")
		}
		if input.RequestID == "" || input.RequestID == "client-chosen-id" {
			t.Error("request ID was not generated")
		}
		req, err := http.NewRequestWithContext(ctx, "POST", mock.URL+input.Path, bytes.NewReader(input.Body))
		if err != nil {
			return nil, gw.ErrUpstream
		}
		resp, err := mock.Client().Do(req)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, gw.ErrTimeout
			}
			return nil, gw.ErrUpstream
		}
		return &gw.Response{Status: resp.StatusCode, Headers: resp.Header, Body: resp.Body}, nil
	})
	service := gw.New(NewGatewayStore(pool), cipher, upstream)
	var logs synchronizedLogs
	logger := logging.New(&logs, "json", slog.LevelInfo)
	cfg := config.Gateway{MaxBodyBytes: 4096, RequestTimeout: 5 * time.Second, HeaderTimeout: time.Second, BodyReadTimeout: time.Second, WriteTimeout: time.Second}
	makeRouter := func(cfg config.Gateway) http.Handler {
		return httptransport.NewRouter(logger, health.New(), config.CORS{}, time.Second, "prod", &httptransport.SecurityHandlers{Admin: admins, Keys: keys, Management: management, Gateway: httptransport.NewGatewayHandler(service, cfg, logger)})
	}
	server := httptest.NewServer(makeRouter(cfg))
	defer server.Close()
	request := func(method, path, credential, body string, want int) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", credential)
		req.Header.Set("X-Request-ID", "client-chosen-id")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			resp.Body.Close()
			t.Fatalf("%s %s: got %d want %d", method, path, resp.StatusCode, want)
		}
		if resp.Header.Get("X-Request-ID") == "" || resp.Header.Get("X-Request-ID") == "untrusted-upstream-id" {
			t.Fatal("request ID overwritten")
		}
		return resp
	}
	read := func(resp *http.Response) string {
		t.Helper()
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	const path = "/anthropic/v1/messages"
	const body = `{"model":"claude-sonnet","max_tokens":32,"messages":[{"role":"user","content":"private-prompt-marker"}]}`
	checkFailure := func(want int, code string) {
		t.Helper()
		before := calls.Load()
		resp := request("POST", path, key.Key, body, want)
		data := read(resp)
		if resp.Header.Get("X-Zentrola-Error-Code") != code || strings.Contains(data, `"requestId"`) || !strings.Contains(data, `"type":"error"`) || calls.Load() != before {
			t.Fatalf("incorrect rejection %s", code)
		}
	}

	t.Run("deny before upstream and resolve active resources", func(t *testing.T) {
		checkFailure(403, "MODEL_PERMISSION_DENIED")
		if err := management.SetGroupMember(ctx, actor, group.ID, member.ID, true, appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if err := management.SetGroupModel(ctx, actor, group.ID, modelID, true, appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if err := management.SetResourceStatus(ctx, actor, resource.ID, "DISABLED", appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		checkFailure(503, "RESOURCE_UNAVAILABLE")
		if err := management.SetResourceStatus(ctx, actor, resource.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if err := management.SetModelStatus(ctx, actor, modelID, "DISABLED", appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		checkFailure(403, "MODEL_DISABLED")
		if err := management.SetModelStatus(ctx, actor, modelID, "ACTIVE", appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM provider_endpoint WHERE provider_id=$1 AND protocol_type='ANTHROPIC'", provider.ID); err != nil {
			t.Fatal(err)
		}
		checkFailure(503, "MODEL_ROUTE_UNAVAILABLE")
		if _, err := pool.Exec(ctx, "INSERT INTO provider_endpoint(provider_id,protocol_type,base_url,created_by,updated_by,created_at,updated_at) VALUES($1,'ANTHROPIC','https://api.anthropic.com','system','system',now(),now())", provider.ID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("native tool loop token counting and upstream errors", func(t *testing.T) {
		resp := request("POST", path, key.Key, body, 200)
		data := read(resp)
		if !strings.Contains(data, `"id":"toolu_123"`) || !strings.Contains(data, `"stop_reason":"tool_use"`) || strings.Contains(data, `"code":"OK"`) {
			t.Fatal("native tool response changed")
		}
		for _, header := range []string{"Set-Cookie", "Authorization", "Anthropic-Ratelimit-Hidden"} {
			if resp.Header.Get(header) != "" {
				t.Fatal("unexpected upstream header forwarded")
			}
		}
		if resp.Header.Get("Request-Id") != "upstream-request-id" || resp.Header.Get("Anthropic-Ratelimit-Requests-Remaining") != "10" {
			t.Fatal("protocol response headers lost")
		}
		for _, field := range []string{"upstream_request_id", "upstream-request-id", "upstream_headers_ms", "first_byte_ms", "provider_id", "resource_id", "model_id"} {
			if !strings.Contains(logs.String(), field) {
				t.Fatalf("gateway access log missing %s: %s", field, logs.String())
			}
		}
		resultBody := `{"model":"claude-sonnet","test_scenario":"tool_result","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_123","name":"read_file","input":{"path":"sample.txt"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_123","content":"file contents","future":9007199254740993}]}]}`
		if data := read(request("POST", path, key.Key, resultBody, 200)); !strings.Contains(data, "Tool result received.") {
			t.Fatal("tool loop failed")
		}
		if data := read(request("POST", path+"/count_tokens", key.Key, body, 200)); data != `{"input_tokens":42}` {
			t.Fatal("count_tokens altered")
		}
		before := calls.Load()
		resp = request("POST", path, key.Key, `{"model":"claude-sonnet","test_scenario":"rate"}`, 429)
		if data := read(resp); data != `{"type":"error","error":{"type":"rate_limit_error","message":"Slow down."}}` || resp.Header.Get("Retry-After") != "2" || calls.Load() != before+1 {
			t.Fatal("upstream error changed or retried")
		}
	})
	t.Run("stream first byte arrives before completion and disconnect cancels", func(t *testing.T) {
		resp := request("POST", path, key.Key, `{"model":"claude-sonnet","stream":true,"test_scenario":"cancel"}`, 200)
		reader := bufio.NewReader(resp.Body)
		first, err := reader.ReadString('\n')
		if err != nil || first != "event: message_start\n" {
			t.Fatal("SSE was buffered or changed")
		}
		resp.Body.Close()
		select {
		case <-streamCancelled:
		case <-time.After(2 * time.Second):
			t.Fatal("client disconnect did not cancel upstream")
		}
	})
	t.Run("truncated stream aborts without appended JSON", func(t *testing.T) {
		resp := request("POST", path, key.Key, `{"model":"claude-sonnet","stream":true,"test_scenario":"truncate"}`, 200)
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err == nil || strings.Contains(string(data), "INTERNAL_ERROR") || strings.Contains(string(data), `"code"`) {
			t.Fatal("truncated stream was presented as complete or wrapped")
		}
	})
	t.Run("large SSE preserves all bytes and event order", func(t *testing.T) {
		data := read(request("POST", path, key.Key, `{"model":"claude-sonnet","stream":true,"test_scenario":"complete_stream"}`, 200))
		want := firstEvent + strings.Repeat("event: ping\ndata: {\"type\":\"ping\"}\n\n", 4096) + tailEvents
		if data != want {
			t.Fatal("stream bytes or order changed")
		}
	})
	t.Run("request timeout before headers", func(t *testing.T) {
		short := cfg
		short.RequestTimeout = 40 * time.Millisecond
		shortServer := httptest.NewServer(makeRouter(short))
		defer shortServer.Close()
		req, _ := http.NewRequest("POST", shortServer.URL+path, strings.NewReader(`{"model":"claude-sonnet","test_scenario":"timeout"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Api-Key", key.Key)
		resp, err := shortServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 504 || resp.Header.Get("X-Zentrola-Error-Code") != "UPSTREAM_TIMEOUT" {
			t.Fatal("request deadline not enforced")
		}
	})
	t.Run("invalid requests cannot reach upstream", func(t *testing.T) {
		before := calls.Load()
		read(request("POST", path, key.Key, `{"model":"missing"}`, 404))
		read(request("POST", path, key.Key, `{"model":"claude-sonnet","model":"claude-opus"}`, 400))
		read(request("POST", path, key.Key, strings.Repeat("x", 5000), 413))
		read(request("POST", path+"/count_tokens", key.Key, `{"model":"claude-sonnet","stream":true}`, 400))
		read(request("GET", path, key.Key, "", 405))
		read(request("POST", "/anthropic/unknown", key.Key, body, 404))
		read(request("POST", path+"?url=http://localhost", key.Key, body, 400))
		read(request("POST", path, login.Token, body, 401))
		if calls.Load() != before {
			t.Fatal("invalid or unauthorized request reached upstream")
		}
	})
	t.Run("credential recovery permission removal and revoke are immediate", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "UPDATE provider_credential SET credential_ciphertext=decode(repeat('00',32),'hex') WHERE id=$1", resource.ID); err != nil {
			t.Fatal(err)
		}
		checkFailure(503, "CREDENTIAL_UNRECOVERABLE")
		if err := management.UpdateCredential(ctx, actor, resource.ID, providerKey, appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if err := management.SetGroupModel(ctx, actor, group.ID, modelID, false, appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		checkFailure(403, "MODEL_PERMISSION_DENIED")
		if err := management.SetGroupModel(ctx, actor, group.ID, modelID, true, appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if err := management.SetMemberStatus(ctx, actor, member.ID, "DISABLED", appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		checkFailure(401, "UNAUTHENTICATED")
		if err := management.SetMemberStatus(ctx, actor, member.ID, "ACTIVE", appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if err := keys.Revoke(ctx, actor, key.ID, appsec.RequestMeta{}); err != nil {
			t.Fatal(err)
		}
		checkFailure(401, "UNAUTHENTICATED")
	})
	for _, secret := range []string{providerKey, key.Key, login.Token, "private-prompt-marker", "file contents"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("gateway log leaked sensitive request data")
		}
	}
}
