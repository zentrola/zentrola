package anthropic

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
)

const claudeSubscriptionCredential = `{"kind":"claude_code_setup_token","access_token":"sk-ant-oat01-test"}`

func claudeSubscriptionTarget() mgmt.ConnectionTarget {
	return mgmt.ConnectionTarget{
		Protocol: "ANTHROPIC", BaseURL: "https://api.anthropic.com", UpstreamModelCode: "claude-haiku-5-5",
		AuthType: mgmt.AuthTypeSubscription, AuthAdapter: anthropicclaude.AdapterCode,
	}
}

func claudeSubscriptionRoute() gw.Route {
	return gw.Route{
		ProviderID: 36, ResourceID: 19896, BaseURL: "https://api.anthropic.com",
		AuthType: "SUBSCRIPTION", AuthAdapter: anthropicclaude.AdapterCode,
	}
}

func TestClaudeSubscriptionRequestsUseConfiguredClaudeCodeVersion(t *testing.T) {
	for _, tt := range []struct {
		name, version, want string
	}{
		{"default", "", "claude-cli/" + anthropicclaude.DefaultClaudeCodeVersion + " (external, cli)"},
		{"configured", "2.1.300", "claude-cli/2.1.300 (external, cli)"},
		{"invalid falls back", "2.1.300\r\nX-Injected: 1", "claude-cli/" + anthropicclaude.DefaultClaudeCodeVersion + " (external, cli)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var testerAgent, gatewayAgent string
			tester := NewConnectionTester()
			tester.UseClaudeCodeVersion(tt.version)
			tester.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				testerAgent = r.Header.Get("User-Agent")
				body := `{"type":"message","content":[{"type":"text","text":"OK"}]}`
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if result := tester.Test(context.Background(), claudeSubscriptionTarget(), []byte(claudeSubscriptionCredential), nil); !result.OK {
				t.Fatalf("connection test failed: %+v", result)
			}

			client := NewGatewayClient(time.Second)
			client.UseClaudeCodeVersion(tt.version)
			client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				gatewayAgent = r.Header.Get("User-Agent")
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
			})
			response, err := client.Open(context.Background(), claudeSubscriptionRoute(), gw.Request{Path: "/v1/messages", Body: []byte(`{}`)}, []byte(claudeSubscriptionCredential))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()

			if testerAgent != tt.want || gatewayAgent != tt.want {
				t.Fatalf("User-Agent tester=%q gateway=%q, want %q", testerAgent, gatewayAgent, tt.want)
			}
		})
	}
}

func TestClaudeSubscriptionRejectionLogsUpstreamError(t *testing.T) {
	rejection := `{"type":"error","error":{"type":"permission_error","message":"Claude Code version is no longer supported"},"request_id":"req_test"}`
	assertLogged := func(t *testing.T, output string) {
		t.Helper()
		for _, want := range []string{
			"claude subscription request rejected by upstream",
			`"upstream_status":403`,
			`"upstream_error_type":"permission_error"`,
			`"upstream_error_message":"Claude Code version is no longer supported"`,
			`"claude_code_user_agent":"claude-cli/2.1.300 (external, cli)"`,
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("rejection log missing %s: %s", want, output)
			}
		}
		if strings.Contains(output, "sk-ant-oat01-test") || strings.Contains(output, "req_test") {
			t.Fatalf("rejection log leaked credential or raw body: %s", output)
		}
	}

	t.Run("connection test", func(t *testing.T) {
		var logs bytes.Buffer
		tester := NewConnectionTester(slog.New(slog.NewJSONHandler(&logs, nil)))
		tester.UseClaudeCodeVersion("2.1.300")
		tester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(rejection))}, nil
		})
		result := tester.Test(context.Background(), claudeSubscriptionTarget(), []byte(claudeSubscriptionCredential), nil)
		if result.OK || result.HTTPStatus != http.StatusForbidden {
			t.Fatalf("unexpected result: %+v", result)
		}
		assertLogged(t, logs.String())
	})

	t.Run("gateway", func(t *testing.T) {
		var logs bytes.Buffer
		client := NewGatewayClient(time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
		client.UseClaudeCodeVersion("2.1.300")
		client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(rejection))}, nil
		})
		response, err := client.Open(context.Background(), claudeSubscriptionRoute(), gw.Request{Path: "/v1/messages", Body: []byte(`{}`), RequestID: "zentrola-request"}, []byte(claudeSubscriptionCredential))
		if err != nil {
			t.Fatal(err)
		}
		// 记录只能来自调用方已读取的正文，正文必须原样交给调用方。
		forwarded, err := io.ReadAll(response.Body)
		if err != nil || string(forwarded) != rejection {
			t.Fatalf("forwarded body = %q, err=%v", forwarded, err)
		}
		if strings.Contains(logs.String(), "rejected by upstream") {
			t.Fatal("rejection must be logged when the body closes, not before it is read")
		}
		response.Body.Close()
		response.Body.Close()
		output := logs.String()
		assertLogged(t, output)
		if strings.Count(output, "rejected by upstream") != 1 || !strings.Contains(output, `"request_id":"zentrola-request"`) || !strings.Contains(output, `"resource_id":19896`) {
			t.Fatalf("gateway rejection log must be written once with route details: %s", output)
		}
	})

	t.Run("API key route is not logged", func(t *testing.T) {
		var logs bytes.Buffer
		client := NewGatewayClient(time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
		client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(rejection))}, nil
		})
		response, err := client.Open(context.Background(), gw.Route{BaseURL: "https://api.anthropic.com", AuthType: "API_KEY"}, gw.Request{Path: "/v1/messages", Body: []byte(`{}`)}, []byte("test-key"))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(response.Body)
		response.Body.Close()
		if strings.Contains(logs.String(), "rejected by upstream") {
			t.Fatalf("API key route must keep its existing logging: %s", logs.String())
		}
	})
}
