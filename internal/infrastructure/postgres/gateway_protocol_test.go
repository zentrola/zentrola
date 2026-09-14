package postgres

import (
	"reflect"
	"testing"

	gw "github.com/zentrola/zentrola/internal/application/gateway"
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
