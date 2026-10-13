package http

import (
	"reflect"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

func TestModelRequestRoundTrip(t *testing.T) {
	publisherID := int64(17)
	input := mgmt.ModelInput{
		Code: "model-code", Name: "模型", PublisherProviderID: &publisherID,
		InputModalities: []string{"TEXT", "IMAGE"}, OutputModalities: []string{"TEXT"}, Remark: "备注",
	}
	if got := NewModelRequest(input).modelInput(); !reflect.DeepEqual(got, input) {
		t.Fatalf("round trip changed model input:\ngot  %+v\nwant %+v", got, input)
	}
}

func TestModelProviderPriorityRequestRequiresUTCSnapshot(t *testing.T) {
	request := ModelProviderPriorityRequest{}
	request.Mappings = []ModelProviderPriorityItemRequest{{ID: 11, Priority: 1, UpdatedAt: "2026-10-13T03:20:00Z"}}
	if !request.Valid() || request.inputs()[0].UpdatedAt.IsZero() {
		t.Fatal("valid UTC snapshot rejected")
	}
	for _, invalid := range []string{"2026-10-13T11:20:00+08:00", "2026-10-13T03:20:00", "invalid"} {
		request.Mappings[0].UpdatedAt = invalid
		if request.Valid() {
			t.Fatalf("invalid timestamp accepted: %s", invalid)
		}
	}
	request.Mappings[0].UpdatedAt = "2026-10-13T03:20:00Z"
	request.Mappings[0].Priority = 101
	if request.Valid() {
		t.Fatal("out-of-range priority accepted")
	}
}

func TestProviderRequestRoundTripPreservesOptionalCollections(t *testing.T) {
	tests := []struct {
		name  string
		input mgmt.ProviderInput
	}{
		{name: "nil collections", input: mgmt.ProviderInput{Name: "服务商"}},
		{name: "empty collections", input: mgmt.ProviderInput{
			Name: "服务商", Endpoints: []mgmt.ProviderEndpoint{},
			ProxyHeaders: []mgmt.ProviderProxyHeaderInput{}, Mappings: []mgmt.ProviderMappingInput{},
		}},
		{name: "all fields", input: mgmt.ProviderInput{
			Name: "服务商", Website: "https://example.com", ProxyEnabled: true,
			ProxyURL: "http://127.0.0.1:8080", UpdateProxyCredentials: true,
			Endpoints:    []mgmt.ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1", NetworkScope: "PRIVATE"}},
			ProxyHeaders: []mgmt.ProviderProxyHeaderInput{{Key: "X-Proxy-Key", Value: "secret"}},
			Mappings:     []mgmt.ProviderMappingInput{{ModelID: 19, UpstreamModelCode: "upstream", Priority: 250}},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NewProviderRequest(test.input).providerInput()
			if !reflect.DeepEqual(got, test.input) {
				t.Fatalf("round trip changed provider input:\ngot  %#v\nwant %#v", got, test.input)
			}
		})
	}
}
