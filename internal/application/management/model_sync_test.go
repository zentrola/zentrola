package management

import (
	"context"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type syncIDs struct{ next int64 }

func (g *syncIDs) NextID(context.Context) (int64, error) { g.next++; return g.next, nil }

type syncCipher struct{ Cipher }

func (syncCipher) Decrypt(catalog.SealedCredential, catalog.CredentialOwner) ([]byte, error) {
	return []byte("provider-key"), nil
}

type syncDiscoverer struct {
	models []DiscoveredModel
	result ConnectionResult
	source ModelDiscoverySource
}

func (d *syncDiscoverer) Supports(providerCode string) bool {
	return providerCode == catalog.DeepSeekOfficialCode
}

func (d *syncDiscoverer) Discover(_ context.Context, source ModelDiscoverySource, _ []byte, _ *catalog.OutboundProxy) ([]DiscoveredModel, ConnectionResult) {
	d.source = source
	return d.models, d.result
}

type syncStore struct{ state *syncState }

func (s syncStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.state)
}
func (s syncStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.state)
}

type syncState struct {
	Writer
	resource ResourceRecord
	provider Provider
	models   []Model
	mappings []ProviderMapping
	audits   []Audit
}

type providerCapabilityReader struct {
	Reader
	providers []Provider
}

func (r *providerCapabilityReader) Providers(context.Context, Page) ([]Provider, error) {
	return append([]Provider(nil), r.providers...), nil
}

func (r *providerCapabilityReader) CountProviders(context.Context) (int64, error) {
	return int64(len(r.providers)), nil
}

type providerCapabilityStore struct{ reader *providerCapabilityReader }

func (s providerCapabilityStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.reader)
}

func (providerCapabilityStore) Write(context.Context, admin.Identity, func(Writer) error) error {
	return nil
}

func (s *syncState) Resource(context.Context, int64) (ResourceRecord, error) { return s.resource, nil }
func (s *syncState) Provider(context.Context, int64) (Provider, error)       { return s.provider, nil }
func (s *syncState) Models(_ context.Context, page Page, _ string) ([]Model, error) {
	if page.After != 0 {
		return []Model{}, nil
	}
	return append([]Model(nil), s.models...), nil
}
func (s *syncState) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return append([]ProviderMapping(nil), s.mappings...), nil
}
func (s *syncState) CreateModel(_ context.Context, model Model) error {
	s.models = append(s.models, model)
	return nil
}
func (s *syncState) UpdateModel(_ context.Context, model Model) error {
	for i := range s.models {
		if s.models[i].ID == model.ID {
			s.models[i] = model
			return nil
		}
	}
	return appsec.ErrUnavailable
}
func (s *syncState) CreateProviderMapping(_ context.Context, mapping ProviderMapping) error {
	s.mappings = append(s.mappings, mapping)
	return nil
}
func (s *syncState) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

func TestSyncResourceModelsCreatesMissingEntriesAndRefreshesExistingNames(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	state := &syncState{
		resource: ResourceRecord{Resource: Resource{ID: 10, ProviderID: 20, Name: "Official key", UpdatedAt: now}},
		provider: Provider{ID: 20, Code: catalog.DeepSeekOfficialCode, Name: "Official", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"}}},
		models: []Model{{
			ID: 30, Code: "existing", Name: "管理员名称", Status: "ACTIVE",
			InputModalities: []string{"TEXT", "IMAGE"}, OutputModalities: []string{"TEXT"}, Remark: "保留说明",
		}},
		mappings: []ProviderMapping{{ID: 40, ProviderID: 20, ModelID: 30, UpstreamModelCode: "custom-existing", Priority: 7}},
	}
	discoverer := &syncDiscoverer{
		models: []DiscoveredModel{
			{Code: "existing", Name: "Upstream name"},
			{Code: "new-model", Name: "New Model"},
			{Code: "deepseek-v4-flash-vision-exp", Name: "deepseek-v4-flash-vision-exp"},
		},
		result: ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200},
	}
	ids := &syncIDs{next: 100}
	service := New(syncStore{state}, ids, syncCipher{}, nil, WithModelDiscoverer(discoverer))
	actor := admin.Identity{ID: 1, OrganizationID: 2}

	result, err := service.SyncResourceModels(context.Background(), actor, 10, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Discovered != 3 || result.Created != 2 || result.Updated != 1 || result.Mapped != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if discoverer.source.ProviderCode != state.provider.Code {
		t.Fatalf("discovery provider code = %q, want %q", discoverer.source.ProviderCode, state.provider.Code)
	}
	if len(state.models) != 3 || state.models[0].Name != "Upstream name" || state.models[0].Status != "ACTIVE" || state.models[0].Remark != "保留说明" || len(state.models[0].InputModalities) != 2 || len(state.models[0].OutputModalities) != 1 || state.models[1].Name != "New Model" || state.models[1].Status != "DISABLED" || state.models[2].Name != "Deepseek v4 flash vision exp" || state.models[2].Status != "DISABLED" {
		t.Fatalf("existing model metadata was not selectively updated or new model was enabled: %+v", state.models)
	}
	if len(state.mappings) != 3 || state.mappings[0].UpstreamModelCode != "custom-existing" || state.mappings[0].Priority != 7 {
		t.Fatalf("existing mapping was overwritten: %+v", state.mappings)
	}
	if len(state.audits) != 1 || state.audits[0].ErrorCode != "" {
		t.Fatalf("successful sync was not audited: %+v", state.audits)
	}

	result, err = service.SyncResourceModels(context.Background(), actor, 10, appsec.RequestMeta{})
	if err != nil || result.Created != 0 || result.Updated != 0 || result.Mapped != 0 || len(state.models) != 3 || len(state.mappings) != 3 {
		t.Fatalf("sync is not idempotent: result=%+v err=%v", result, err)
	}
}

func TestSyncResourceModelsAuditsUpstreamFailureWithoutWrites(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	state := &syncState{
		resource: ResourceRecord{Resource: Resource{ID: 10, ProviderID: 20, Name: "Official key", UpdatedAt: now}},
		provider: Provider{ID: 20, Code: catalog.DeepSeekOfficialCode, Name: "Official", Endpoints: []ProviderEndpoint{{ProtocolType: "ANTHROPIC", BaseURL: "https://api.example.com"}}},
	}
	discoverer := &syncDiscoverer{result: ConnectionResult{Code: "UPSTREAM_AUTH_FAILED", HTTPStatus: 401}}
	service := New(syncStore{state}, &syncIDs{}, syncCipher{}, nil, WithModelDiscoverer(discoverer))
	result, err := service.SyncResourceModels(context.Background(), admin.Identity{ID: 1, OrganizationID: 2}, 10, appsec.RequestMeta{})
	if err != nil || result.OK || result.Code != "UPSTREAM_AUTH_FAILED" {
		t.Fatalf("unexpected failure result: %+v %v", result, err)
	}
	if len(state.models) != 0 || len(state.mappings) != 0 || len(state.audits) != 1 || state.audits[0].ErrorCode != "UPSTREAM_AUTH_FAILED" {
		t.Fatalf("failed sync wrote catalog data or missed audit: %+v", state)
	}
}

func TestProviderModelSyncCapabilityFollowsDiscovererSupport(t *testing.T) {
	reader := &providerCapabilityReader{providers: []Provider{
		{Code: catalog.DeepSeekOfficialCode},
		{Code: "provider-custom"},
	}}
	service := New(providerCapabilityStore{reader: reader}, nil, nil, nil, WithModelDiscoverer(&syncDiscoverer{}))

	result, err := service.Providers(context.Background(), admin.Identity{}, Page{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || !result.Items[0].ModelSyncSupported || result.Items[1].ModelSyncSupported {
		t.Fatalf("unexpected model sync capabilities: %+v", result.Items)
	}
}

func TestModelDisplayName(t *testing.T) {
	if got := modelDisplayName("deepseek-v4-flash-vision-exp"); got != "Deepseek v4 flash vision exp" {
		t.Fatalf("modelDisplayName() = %q", got)
	}
}
