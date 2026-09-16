package management

import (
	"context"
	"errors"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type providerInitIDs struct{ next int64 }

func (g *providerInitIDs) NextID(context.Context) (int64, error) { g.next++; return g.next, nil }

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

func (s *providerInitializationState) Providers(_ context.Context, page Page, _ string) ([]Provider, error) {
	if page.After != 0 {
		return []Provider{}, nil
	}
	return append([]Provider(nil), s.providers...), nil
}
func (s *providerInitializationState) CreateProvider(_ context.Context, provider Provider) error {
	s.providers = append(s.providers, provider)
	return nil
}
func (s *providerInitializationState) UpdateProvider(_ context.Context, provider Provider) error {
	for index := range s.providers {
		if s.providers[index].ID == provider.ID {
			s.providers[index] = provider
			return nil
		}
	}
	return errors.New("provider not found")
}
func (s *providerInitializationState) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

func TestInitializeOfficialProvidersCreatesMissingTemplatesAndSynchronizesLocalizedNames(t *testing.T) {
	customWebsite := "https://custom.example.com"
	existing := Provider{
		ID: 8, Code: "openai-official", Name: "管理员自定义名称", Type: "OFFICIAL", Status: "DISABLED",
		Website:   &customWebsite,
		Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://custom.example.com/v1"}},
	}
	state := &providerInitializationState{providers: []Provider{existing}}
	service := New(providerInitializationStore{state}, &providerInitIDs{next: 100}, nil, nil)
	actor := admin.Identity{ID: 1}

	result, err := service.InitializeOfficialProviders(context.Background(), actor, ProviderInitializeInput{Locale: "zh-CN"}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 13 || result.Created != 12 || result.Updated != 1 || result.Existing != 1 || len(state.providers) != 13 || len(state.audits) != 13 {
		t.Fatalf("unexpected initialization result: %+v providers=%d audits=%d", result, len(state.providers), len(state.audits))
	}
	if state.providers[0].Name != "OpenAI" || state.providers[0].Website == nil || *state.providers[0].Website != "https://openai.com" || state.providers[0].Endpoints[0].BaseURL != existing.Endpoints[0].BaseURL || state.providers[0].Status != existing.Status {
		t.Fatalf("unexpected synchronized provider: %+v", state.providers[0])
	}
	for _, provider := range state.providers[1:] {
		if provider.Type != "OFFICIAL" || provider.Status != "DISABLED" || len(provider.Endpoints) == 0 || provider.Name == "" || provider.Website == nil || *provider.Website == "" {
			t.Fatalf("invalid initialized provider: %+v", provider)
		}
	}

	result, err = service.InitializeOfficialProviders(context.Background(), actor, ProviderInitializeInput{Locale: "zh-CN"}, appsec.RequestMeta{})
	if err != nil || result.Created != 0 || result.Updated != 0 || result.Existing != 13 || len(state.providers) != 13 || len(state.audits) != 13 {
		t.Fatalf("initialization is not idempotent: %+v err=%v", result, err)
	}

	result, err = service.InitializeOfficialProviders(context.Background(), actor, ProviderInitializeInput{Locale: "en-US"}, appsec.RequestMeta{})
	if err != nil || result.Created != 0 || result.Updated != 7 || result.Existing != 13 || len(state.providers) != 13 || len(state.audits) != 20 {
		t.Fatalf("localized names were not synchronized: %+v err=%v", result, err)
	}
	if state.providers[0].Name != "OpenAI" {
		t.Fatalf("unexpected English provider name: %q", state.providers[0].Name)
	}
}

func TestInitializeOfficialProvidersRejectsUnsupportedLocale(t *testing.T) {
	state := &providerInitializationState{}
	service := New(providerInitializationStore{state}, &providerInitIDs{}, nil, nil)
	_, err := service.InitializeOfficialProviders(
		context.Background(),
		admin.Identity{ID: 1},
		ProviderInitializeInput{Locale: "fr-FR"},
		appsec.RequestMeta{},
	)
	if !errors.Is(err, appsec.ErrInvalidArgument) || len(state.providers) != 0 {
		t.Fatalf("unsupported locale was accepted: %v", err)
	}
}
