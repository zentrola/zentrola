package management

import (
	"context"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type providerInitIDs struct{ next int64 }

func (g *providerInitIDs) NextID() (int64, error) { g.next++; return g.next, nil }

type providerInitializationStore struct{ state *providerInitializationState }

func (s providerInitializationStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.state)
}
func (s providerInitializationStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.state)
}

type providerInitializationState struct {
	Writer
	providers []Provider
	audits    []Audit
}

func (s *providerInitializationState) Providers(_ context.Context, page Page) ([]Provider, error) {
	if page.After != 0 {
		return []Provider{}, nil
	}
	return append([]Provider(nil), s.providers...), nil
}
func (s *providerInitializationState) CreateProvider(_ context.Context, provider Provider) error {
	s.providers = append(s.providers, provider)
	return nil
}
func (s *providerInitializationState) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

func TestInitializeOfficialProvidersOnlyCreatesMissingTemplates(t *testing.T) {
	existing := Provider{
		ID: 8, Code: "openai-official", Name: "管理员自定义名称", Type: "OFFICIAL", Status: "DISABLED",
		Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://custom.example.com/v1"}},
	}
	state := &providerInitializationState{providers: []Provider{existing}}
	service := New(providerInitializationStore{state}, &providerInitIDs{next: 100}, nil, nil)
	actor := admin.Identity{ID: 1, OrganizationID: 2}

	result, err := service.InitializeOfficialProviders(context.Background(), actor, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 7 || result.Created != 6 || result.Existing != 1 || len(state.providers) != 7 || len(state.audits) != 6 {
		t.Fatalf("unexpected initialization result: %+v providers=%d audits=%d", result, len(state.providers), len(state.audits))
	}
	if state.providers[0].Name != existing.Name || state.providers[0].Endpoints[0].BaseURL != existing.Endpoints[0].BaseURL {
		t.Fatalf("existing provider was overwritten: %+v", state.providers[0])
	}
	for _, provider := range state.providers[1:] {
		if provider.Type != "OFFICIAL" || provider.Status != "DISABLED" || len(provider.Endpoints) == 0 {
			t.Fatalf("invalid initialized provider: %+v", provider)
		}
	}

	result, err = service.InitializeOfficialProviders(context.Background(), actor, appsec.RequestMeta{})
	if err != nil || result.Created != 0 || result.Existing != 7 || len(state.providers) != 7 || len(state.audits) != 6 {
		t.Fatalf("initialization is not idempotent: %+v err=%v", result, err)
	}
}
