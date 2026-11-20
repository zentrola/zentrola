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

func TestSubscriptionSupportsConvertibleGatewayProtocols(t *testing.T) {
	for _, test := range []struct {
		name     string
		adapter  string
		protocol string
		want     bool
	}{
		{"Claude Code supports Anthropic Messages", mgmt.AuthAdapterClaudeCode, gw.AnthropicProtocol, true},
		{"Claude Code rejects OpenAI Responses", mgmt.AuthAdapterClaudeCode, gw.OpenAIResponsesProtocol, false},
		{"Codex supports OpenAI Responses", mgmt.AuthAdapterOpenAICodex, gw.OpenAIResponsesProtocol, true},
		{"Codex converts OpenAI Chat", mgmt.AuthAdapterOpenAICodex, gw.OpenAIProtocol, true},
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

func TestPreferExistingChatRoutes(t *testing.T) {
	routes := []gw.Route{
		{ResourceID: 1, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex},
		{ResourceID: 2, AuthType: mgmt.AuthTypeAPIKey},
		{ResourceID: 3, AuthType: mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex},
		{ResourceID: 4, AuthType: mgmt.AuthTypeAPIKey},
	}
	ordered := preferExistingChatRoutes(routes)
	for index, want := range []int64{2, 4, 1, 3} {
		if ordered[index].ResourceID != want {
			t.Fatalf("route %d = %d, want %d", index, ordered[index].ResourceID, want)
		}
	}
}
