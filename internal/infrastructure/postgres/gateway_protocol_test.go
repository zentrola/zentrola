package postgres

import (
	"reflect"
	"testing"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

func TestGatewayEndpointProtocolPreference(t *testing.T) {
	for _, test := range []struct {
		name     string
		protocol string
		want     []string
	}{
		{"Anthropic prefers Anthropic", gw.AnthropicProtocol, []string{gw.AnthropicEndpoint, gw.OpenAIEndpoint}},
		{"OpenAI Chat prefers OpenAI", gw.OpenAIProtocol, []string{gw.OpenAIEndpoint, gw.AnthropicEndpoint}},
		{"OpenAI Responses prefers OpenAI", gw.OpenAIResponsesProtocol, []string{gw.OpenAIEndpoint, gw.AnthropicEndpoint}},
		{"OpenAI Images requires OpenAI", gw.OpenAIImagesProtocol, []string{gw.OpenAIEndpoint}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := gatewayEndpointProtocols(test.protocol); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestSubscriptionSupportsOnlyItsNativeGatewayProtocol(t *testing.T) {
	for _, test := range []struct {
		name     string
		adapter  string
		protocol string
		want     bool
	}{
		{"Claude Code supports Anthropic Messages", mgmt.AuthAdapterClaudeCode, gw.AnthropicProtocol, true},
		{"Claude Code rejects OpenAI Responses", mgmt.AuthAdapterClaudeCode, gw.OpenAIResponsesProtocol, false},
		{"Codex supports OpenAI Responses", mgmt.AuthAdapterOpenAICodex, gw.OpenAIResponsesProtocol, true},
		{"Codex rejects Anthropic Messages", mgmt.AuthAdapterOpenAICodex, gw.AnthropicProtocol, false},
		{"Unknown adapter is rejected", "UNKNOWN", gw.OpenAIResponsesProtocol, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := subscriptionSupportsProtocol(test.adapter, test.protocol); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
