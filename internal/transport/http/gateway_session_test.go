package http

import (
	"net/http"
	"strings"
	"testing"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
)

func TestGatewayClientSessionHeaders(t *testing.T) {
	tests := []struct {
		name, protocol, sessionHeader, agentHeader string
		wantSession, wantAgent                     string
	}{
		{"codex", gw.OpenAIResponsesProtocol, "Session-Id", "", "codex-123", ""},
		{"claude", gw.AnthropicProtocol, "X-Claude-Code-Session-Id", "X-Claude-Code-Agent-Id", "claude-123", "agent-7"},
		{"other OpenAI route", gw.OpenAIProtocol, "Session-Id", "", "", ""},
		{"wrong client header", gw.AnthropicProtocol, "Session-Id", "", "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &http.Request{Header: make(http.Header)}
			if test.sessionHeader != "" {
				value := "claude-123"
				if test.sessionHeader == "Session-Id" {
					value = "codex-123"
				}
				request.Header.Set(test.sessionHeader, value)
			}
			if test.agentHeader != "" {
				request.Header.Set(test.agentHeader, "agent-7")
			}
			session, agent := gatewayClientSession(request, test.protocol)
			if session != test.wantSession || agent != test.wantAgent {
				t.Fatalf("session=%q agent=%q, want %q/%q", session, agent, test.wantSession, test.wantAgent)
			}
		})
	}
}

func TestGatewayClientSessionRejectsAmbiguousOrOversizedHeader(t *testing.T) {
	for _, values := range [][]string{{"one", "two"}, {strings.Repeat("x", 129)}, {" bad"}, {"bad\nvalue"}} {
		request := &http.Request{Header: make(http.Header)}
		request.Header[http.CanonicalHeaderKey("Session-Id")] = values
		if session, _ := gatewayClientSession(request, gw.OpenAIResponsesProtocol); session != "" {
			t.Fatalf("accepted unsafe header %q", values)
		}
	}
}
