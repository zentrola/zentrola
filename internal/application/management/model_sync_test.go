package management

import (
	"context"
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

func TestConnectionTestEndpointSelection(t *testing.T) {
	provider := Provider{Endpoints: []ProviderEndpoint{
		{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"},
		{ProtocolType: "ANTHROPIC", BaseURL: "https://api.example.com/anthropic"},
	}}
	protocol, baseURL := preferredProviderEndpoint(provider)
	if protocol != "ANTHROPIC" || baseURL != "https://api.example.com/anthropic" {
		t.Fatalf("default connection test endpoint = %s %s", protocol, baseURL)
	}
	protocol, baseURL = providerEndpoint(provider, "OPENAI")
	if protocol != "OPENAI" || baseURL != "https://api.example.com/v1" {
		t.Fatalf("selected connection test endpoint = %s %s", protocol, baseURL)
	}
}

type syncIDs struct{ next int64 }

func (g *syncIDs) NextID(context.Context) (int64, error) { g.next++; return g.next, nil }

type syncCipher struct{ Cipher }

func (syncCipher) Decrypt(catalog.SealedCredential, catalog.CredentialOwner) ([]byte, error) {
	return []byte("provider-key"), nil
}

type syncDiscoverer struct {
	models       []DiscoveredModel
	result       ConnectionResult
	source       ModelDiscoverySource
	supportsCode string
}

func (d *syncDiscoverer) Supports(providerCode string) bool {
	code := d.supportsCode
	if code == "" {
		code = catalog.DeepSeekOfficialCode
	}
	return providerCode == code
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
	providers         []Provider
	providerType      string
	countProviderType string
}

func (r *providerCapabilityReader) Providers(_ context.Context, _ Page, providerType string) ([]Provider, error) {
	r.providerType = providerType
	result := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		if providerType == "" || provider.Type == providerType {
			result = append(result, provider)
		}
	}
	return result, nil
}

func (r *providerCapabilityReader) CountProviders(_ context.Context, providerType string) (int64, error) {
	r.countProviderType = providerType
	var count int64
	for _, provider := range r.providers {
		if providerType == "" || provider.Type == providerType {
			count++
		}
	}
	return count, nil
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
func (s *syncState) Resources(_ context.Context, page Page) ([]Resource, error) {
	if page.After != 0 || s.resource.ID == 0 {
		return []Resource{}, nil
	}
	return []Resource{s.resource.Resource}, nil
}
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
		resource: ResourceRecord{Resource: Resource{ID: 10, ProviderID: 20, Name: "Official key", AuthType: AuthTypeAPIKey, UpdatedAt: now}},
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
	actor := admin.Identity{ID: 1}

	result, err := service.SyncProviderModels(context.Background(), actor, 20, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Source != "PROVIDER" || result.Discovered != 3 || result.Created != 2 || result.Updated != 1 || result.Mapped != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if discoverer.source.ProviderCode != state.provider.Code || len(discoverer.source.Endpoints) != 1 ||
		discoverer.source.Endpoints[0] != state.provider.Endpoints[0] {
		t.Fatalf("discovery source = %+v, want provider code and endpoints from %+v", discoverer.source, state.provider)
	}
	if len(state.models) != 3 || state.models[0].Name != "Upstream name" || state.models[0].Status != "ACTIVE" || state.models[0].Remark != "保留说明" || len(state.models[0].InputModalities) != 2 || len(state.models[0].OutputModalities) != 1 || state.models[1].Name != "New Model" || state.models[1].Status != "ACTIVE" || state.models[2].Name != "Deepseek v4 flash vision exp" || state.models[2].Status != "ACTIVE" {
		t.Fatalf("existing model metadata was not selectively updated or synchronized models were not enabled: %+v", state.models)
	}
	for _, model := range state.models {
		if model.PublisherProviderID == nil || *model.PublisherProviderID != state.provider.ID || model.PublisherProviderName == nil || *model.PublisherProviderName != state.provider.Name {
			t.Fatalf("model publisher was not synchronized: %+v", model)
		}
	}
	if len(state.mappings) != 3 || state.mappings[0].UpstreamModelCode != "custom-existing" || state.mappings[0].Priority != 7 || state.mappings[1].UpstreamModelCode != "" || state.mappings[2].UpstreamModelCode != "" {
		t.Fatalf("existing mapping was overwritten: %+v", state.mappings)
	}
	if len(state.audits) != 1 || state.audits[0].ErrorCode != "" {
		t.Fatalf("successful sync was not audited: %+v", state.audits)
	}

	result, err = service.SyncProviderModels(context.Background(), actor, 20, appsec.RequestMeta{})
	if err != nil || result.Created != 0 || result.Updated != 0 || result.Mapped != 0 || len(state.models) != 3 || len(state.mappings) != 3 {
		t.Fatalf("sync is not idempotent: result=%+v err=%v", result, err)
	}
}

func TestSyncProviderModelsFallsBackToBuiltInCatalogWithoutCredential(t *testing.T) {
	state := &syncState{
		provider: Provider{ID: 20, Code: catalog.DeepSeekOfficialCode, Name: "DeepSeek"},
	}
	service := New(
		syncStore{state},
		&syncIDs{},
		syncCipher{},
		nil,
		WithModelDiscoverer(&syncDiscoverer{}),
	)

	result, err := service.SyncProviderModels(
		context.Background(),
		admin.Identity{ID: 1},
		20,
		appsec.RequestMeta{},
	)
	if err != nil || !result.OK || result.Source != "BUILTIN" || result.Discovered != 2 || result.Created != 2 || result.Mapped != 2 {
		t.Fatalf("unexpected built-in fallback result: %+v err=%v", result, err)
	}
	if len(state.models) != 2 || state.models[0].Code != "deepseek-flash" || state.models[0].Status != "ACTIVE" || state.models[1].Status != "ACTIVE" ||
		len(state.models[0].InputModalities) != 2 || state.models[0].InputModalities[0] != "TEXT" ||
		state.models[0].InputModalities[1] != "IMAGE" || len(state.mappings) != 2 ||
		state.mappings[0].UpstreamModelCode != "" || state.mappings[1].UpstreamModelCode != "" {
		t.Fatalf("built-in catalog was not persisted: models=%+v mappings=%+v", state.models, state.mappings)
	}
}

func TestBuiltInModelSyncPreservesManuallyAdjustedModalities(t *testing.T) {
	publisherID := int64(99)
	publisherName := "旧厂商"
	state := &syncState{
		provider: Provider{ID: 20, Code: catalog.DeepSeekOfficialCode, Name: "DeepSeek"},
		models: []Model{{
			ID: 30, Code: "deepseek-flash", Name: "旧名称", Status: "ACTIVE",
			InputModalities: []string{"IMAGE"}, OutputModalities: []string{"AUDIO"},
			PublisherProviderID: &publisherID, PublisherProviderName: &publisherName,
		}},
	}
	service := New(syncStore{state}, &syncIDs{next: 100}, syncCipher{}, nil)

	result, err := service.SyncProviderModels(
		context.Background(),
		admin.Identity{ID: 1},
		20,
		appsec.RequestMeta{},
	)
	if err != nil || result.Source != "BUILTIN" || result.Created != 1 || result.Updated != 1 || result.Mapped != 2 {
		t.Fatalf("unexpected built-in update result: %+v err=%v", result, err)
	}
	existing := state.models[0]
	if existing.Name != "DeepSeek V4.1 Flash" ||
		len(existing.InputModalities) != 1 || existing.InputModalities[0] != "IMAGE" ||
		len(existing.OutputModalities) != 1 || existing.OutputModalities[0] != "AUDIO" ||
		existing.PublisherProviderID == nil || *existing.PublisherProviderID != state.provider.ID {
		t.Fatalf("manual modalities were overwritten during built-in sync: %+v", existing)
	}
}

func TestSyncProviderModelsRequiresCredentialWhenBuiltInCatalogIsEmpty(t *testing.T) {
	state := &syncState{provider: Provider{ID: 20, Code: "provider-custom", Name: "Custom"}}
	service := New(
		syncStore{state},
		&syncIDs{},
		syncCipher{},
		nil,
		WithModelDiscoverer(&syncDiscoverer{supportsCode: "provider-custom"}),
	)

	_, err := service.SyncProviderModels(context.Background(), admin.Identity{ID: 1}, 20, appsec.RequestMeta{})
	if !errors.Is(err, ErrModelSyncCredentialRequired) {
		t.Fatalf("error=%v; want model sync credential required", err)
	}
}

func TestSyncResourceModelsBackfillsPublisherWhenOfficialNameIsUnchanged(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	state := &syncState{
		resource: ResourceRecord{Resource: Resource{ID: 10, ProviderID: 20, Name: "Official key", UpdatedAt: now}},
		provider: Provider{ID: 20, Code: catalog.DeepSeekOfficialCode, Name: "DeepSeek"},
		models:   []Model{{ID: 30, Code: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", Status: "DISABLED", InputModalities: []string{"TEXT"}, OutputModalities: []string{"TEXT"}}},
		mappings: []ProviderMapping{{ID: 40, ProviderID: 20, ModelID: 30, UpstreamModelCode: "deepseek-v4-pro", Priority: 100}},
	}
	discoverer := &syncDiscoverer{
		models: []DiscoveredModel{{Code: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"}},
		result: ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200},
	}
	service := New(syncStore{state}, &syncIDs{}, syncCipher{}, nil, WithModelDiscoverer(discoverer))

	result, err := service.SyncResourceModels(context.Background(), admin.Identity{ID: 1}, 10, appsec.RequestMeta{})
	if err != nil || result.Updated != 1 || result.Created != 0 || result.Mapped != 0 {
		t.Fatalf("unexpected publisher backfill result: %+v err=%v", result, err)
	}
	model := state.models[0]
	if model.PublisherProviderID == nil || *model.PublisherProviderID != state.provider.ID || model.PublisherProviderName == nil || *model.PublisherProviderName != state.provider.Name {
		t.Fatalf("publisher was not backfilled: %+v", model)
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
	result, err := service.SyncResourceModels(context.Background(), admin.Identity{ID: 1}, 10, appsec.RequestMeta{})
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

	result, err := service.Providers(context.Background(), admin.Identity{}, Page{Limit: 20}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || !result.Items[0].ModelSyncSupported || result.Items[1].ModelSyncSupported {
		t.Fatalf("unexpected model sync capabilities: %+v", result.Items)
	}
}

func TestProvidersFiltersByValidatedProviderType(t *testing.T) {
	reader := &providerCapabilityReader{providers: []Provider{
		{ID: 1, Type: string(catalog.Official)},
		{ID: 2, Type: string(catalog.Custom)},
	}}
	service := New(providerCapabilityStore{reader: reader}, nil, nil, nil)

	result, err := service.Providers(context.Background(), admin.Identity{}, Page{Limit: 20}, string(catalog.Official))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 1 || result.Total != 1 ||
		reader.providerType != string(catalog.Official) || reader.countProviderType != string(catalog.Official) {
		t.Fatalf("provider type filter was not forwarded consistently: result=%+v reader=%+v", result, reader)
	}
	if _, err := service.Providers(context.Background(), admin.Identity{}, Page{Limit: 20}, "UNKNOWN"); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("invalid provider type error = %v", err)
	}
}

func TestModelDisplayName(t *testing.T) {
	if got := modelDisplayName("deepseek-v4-flash-vision-exp"); got != "Deepseek v4 flash vision exp" {
		t.Fatalf("modelDisplayName() = %q", got)
	}
}

func TestEnrichDiscoveredModelsUsesBuiltInMetadataWithoutAddingModels(t *testing.T) {
	discovered := enrichDiscoveredModels(catalog.DeepSeekOfficialCode, []DiscoveredModel{{
		Code: "deepseek-flash", Name: "deepseek-flash",
	}})
	if len(discovered) != 1 || discovered[0].Name != "DeepSeek V4.1 Flash" ||
		len(discovered[0].InputModalities) != 2 || discovered[0].InputModalities[0] != "TEXT" ||
		discovered[0].InputModalities[1] != "IMAGE" ||
		len(discovered[0].OutputModalities) != 1 || discovered[0].OutputModalities[0] != "TEXT" {
		t.Fatalf("built-in metadata was not merged: %+v", discovered)
	}
	discovered[0].InputModalities[0] = "IMAGE"
	again := enrichDiscoveredModels(catalog.DeepSeekOfficialCode, []DiscoveredModel{{Code: "deepseek-flash"}})
	if again[0].InputModalities[0] != "TEXT" {
		t.Fatalf("built-in metadata was mutated: %+v", again)
	}
}
