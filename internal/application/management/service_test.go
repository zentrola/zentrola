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

type subscriptionAdapterStub struct {
	probe   SubscriptionProbe
	onProbe func(*catalog.OutboundProxy)
}

func (subscriptionAdapterStub) Code() string              { return AuthAdapterOpenAICodex }
func (subscriptionAdapterStub) Supports(code string) bool { return code == AuthAdapterOpenAICodex }
func (subscriptionAdapterStub) SupportsProvider(provider Provider) bool {
	return provider.Code == "openai-official"
}
func (subscriptionAdapterStub) Inspect([]byte) (SubscriptionInspection, error) {
	return SubscriptionInspection{}, nil
}
func (s subscriptionAdapterStub) Probe(_ context.Context, _ []byte, proxy *catalog.OutboundProxy) (SubscriptionProbe, error) {
	if s.onProbe != nil {
		s.onProbe(proxy)
	}
	return s.probe, nil
}

type claudeSubscriptionAdapterStub struct{ subscriptionAdapterStub }

func (claudeSubscriptionAdapterStub) Code() string              { return AuthAdapterClaudeCode }
func (claudeSubscriptionAdapterStub) Supports(code string) bool { return code == AuthAdapterClaudeCode }
func (claudeSubscriptionAdapterStub) SupportsProvider(provider Provider) bool {
	return provider.Code == catalog.AnthropicOfficialCode
}

func TestProviderCapabilitiesIncludeMatchingSubscriptionAdapter(t *testing.T) {
	service := &Service{subscriptions: []SubscriptionAdapter{
		subscriptionAdapterStub{}, claudeSubscriptionAdapterStub{},
	}}
	openAI := service.withProviderCapabilities(Provider{Code: catalog.OpenAIOfficialCode})
	if len(openAI.AuthAdapters) != 2 || openAI.AuthAdapters[1] != AuthAdapterOpenAICodex {
		t.Fatalf("unexpected OpenAI adapters: %v", openAI.AuthAdapters)
	}
	claude := service.withProviderCapabilities(Provider{Code: catalog.AnthropicOfficialCode})
	if len(claude.AuthAdapters) != 2 || claude.AuthAdapters[1] != AuthAdapterClaudeCode {
		t.Fatalf("unexpected Anthropic adapters: %v", claude.AuthAdapters)
	}
}

type resourceCreateWriter struct {
	Writer
	created ResourceRecord
	quotas  []ResourceQuota
}

func (w *resourceCreateWriter) Provider(context.Context, int64) (Provider, error) {
	return Provider{ID: 40, Code: "openai-official"}, nil
}
func (w *resourceCreateWriter) CreateResource(_ context.Context, resource ResourceRecord) error {
	w.created = resource
	return nil
}
func (w *resourceCreateWriter) ReplaceResourceQuotas(_ context.Context, _ int64, quotas []ResourceQuota) error {
	w.quotas = append([]ResourceQuota(nil), quotas...)
	return nil
}
func (*resourceCreateWriter) Audit(context.Context, Audit, appsec.RequestMeta) error { return nil }

type resourceCreateStore struct{ writer *resourceCreateWriter }

func (s resourceCreateStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.writer)
}
func (s resourceCreateStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

type fixedMemberID struct{ id int64 }

func (g fixedMemberID) NextID(context.Context) (int64, error) { return g.id, nil }

type connectionTesterFunc func(context.Context, ConnectionTarget, []byte, *catalog.OutboundProxy) ConnectionResult

func (f connectionTesterFunc) Test(ctx context.Context, target ConnectionTarget, credential []byte, proxy *catalog.OutboundProxy) ConnectionResult {
	return f(ctx, target, credential, proxy)
}

type resourceTestWriter struct {
	Writer
	resource ResourceRecord
	provider Provider
	model    Model
	mappings []ProviderMapping
	updated  ResourceRecord
	quotas   []ResourceQuota
	restored bool
}

func (w *resourceTestWriter) Resource(context.Context, int64) (ResourceRecord, error) {
	return w.resource, nil
}
func (w *resourceTestWriter) Provider(context.Context, int64) (Provider, error) {
	return w.provider, nil
}
func (w *resourceTestWriter) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return w.mappings, nil
}
func (w *resourceTestWriter) Model(context.Context, int64) (Model, error) {
	return w.model, nil
}
func (w *resourceTestWriter) UpdateResource(_ context.Context, resource ResourceRecord) error {
	w.updated = resource
	w.resource = resource
	return nil
}
func (w *resourceTestWriter) ReplaceResourceQuotas(_ context.Context, _ int64, quotas []ResourceQuota) error {
	w.quotas = append([]ResourceQuota(nil), quotas...)
	return nil
}
func (w *resourceTestWriter) RestoreResourceRuntime(context.Context, int64, time.Time) error {
	w.restored = true
	return nil
}
func (w *resourceTestWriter) BlockResourceRuntime(_ context.Context, _ int64, reason, code string, status *int32, at time.Time) error {
	w.resource.RuntimeStatus = "BLOCKED"
	w.resource.BlockedReason = stringPointer(reason)
	w.resource.BlockedAt = &at
	w.resource.LastErrorAt = &at
	w.resource.LastHTTPStatus = status
	w.resource.LastErrorCode = stringPointer(code)
	w.resource.UpdatedAt = at
	w.updated = w.resource
	return nil
}
func (*resourceTestWriter) Audit(context.Context, Audit, appsec.RequestMeta) error { return nil }

type resourceTestStore struct{ writer *resourceTestWriter }

func (s resourceTestStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.writer)
}
func (s resourceTestStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

type memberCreateWriter struct {
	Writer
	created Member
	audit   Audit
}

func (w *memberCreateWriter) CreateMember(_ context.Context, member Member) error {
	w.created = member
	return nil
}

func (w *memberCreateWriter) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	w.audit = audit
	return nil
}

type memberCreateStore struct{ writer *memberCreateWriter }

func (s memberCreateStore) Read(_ context.Context, _ admin.Identity, _ func(Reader) error) error {
	return nil
}

func (s memberCreateStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

type credentialUpdateWriter struct {
	Writer
	resource ResourceRecord
	updated  ResourceRecord
	audit    Audit
}

func (w *credentialUpdateWriter) Resource(context.Context, int64) (ResourceRecord, error) {
	return w.resource, nil
}

func (w *credentialUpdateWriter) UpdateResource(_ context.Context, resource ResourceRecord) error {
	w.updated = resource
	return nil
}

func (w *credentialUpdateWriter) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	w.audit = audit
	return nil
}

type credentialUpdateStore struct{ writer *credentialUpdateWriter }

func (s credentialUpdateStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.writer)
}

func (s credentialUpdateStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

type memberSuggestionReader struct {
	Reader
	page   Page
	name   string
	result []Member
	called bool
}

func (r *memberSuggestionReader) MemberSuggestions(_ context.Context, page Page, name string) ([]Member, error) {
	r.called = true
	r.page = page
	r.name = name
	return r.result, nil
}
func (r *memberSuggestionReader) CountMemberSuggestions(context.Context, string) (int64, error) {
	return int64(len(r.result)), nil
}

type memberSuggestionStore struct{ reader *memberSuggestionReader }

func (s memberSuggestionStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.reader)
}

func (s memberSuggestionStore) Write(_ context.Context, _ admin.Identity, _ func(Writer) error) error {
	return nil
}

func TestCreateMemberDefaultsToDisabled(t *testing.T) {
	writer := &memberCreateWriter{}
	service := New(memberCreateStore{writer: writer}, fixedMemberID{id: 42}, nil, nil)

	created, err := service.CreateMember(context.Background(), admin.Identity{}, "新用户", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "DISABLED" || writer.created.Status != "DISABLED" {
		t.Fatalf("created status=%q, persisted status=%q; want DISABLED", created.Status, writer.created.Status)
	}
	after, ok := writer.audit.After.(Member)
	if !ok || after.Status != "DISABLED" {
		t.Fatalf("audit after=%#v; want disabled member", writer.audit.After)
	}
}

func TestUpdateCredentialPersistsResource(t *testing.T) {
	writer := &credentialUpdateWriter{resource: ResourceRecord{Resource: Resource{
		ID: 48, ProviderID: 40, Name: "深度求索 API Key", AuthType: AuthTypeAPIKey,
	}}}
	service := New(credentialUpdateStore{writer: writer}, nil, providerTestCipher{}, nil)

	err := service.UpdateCredential(
		context.Background(),
		admin.Identity{ID: 1},
		48,
		"replacement-credential",
		appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(writer.updated.Sealed.Ciphertext) != "replacement-credential" {
		t.Fatal("updated credential was not persisted")
	}
	after, ok := writer.audit.After.(map[string]any)
	if !ok || after["credentialConfigured"] != true || after["authType"] != AuthTypeAPIKey {
		t.Fatalf("audit after=%#v; want configured API key credential", writer.audit.After)
	}
}

func TestExportSubscriptionCredentialReturnsAuthFileAndAudits(t *testing.T) {
	subscriptionType := SubscriptionPersonal
	writer := &credentialUpdateWriter{resource: ResourceRecord{
		Resource: Resource{
			ID: 48, ProviderID: 40, Name: "OpenAI 个人订阅", AuthType: AuthTypeSubscription,
			AuthAdapter: AuthAdapterOpenAICodex, SubscriptionType: &subscriptionType,
		},
		Sealed: catalog.SealedCredential{Ciphertext: []byte(`{"auth_mode":"chatgpt"}`), KeyVersion: 1},
	}}
	service := New(credentialUpdateStore{writer: writer}, nil, providerTestCipher{}, nil)

	credential, err := service.ExportSubscriptionCredential(
		context.Background(),
		admin.Identity{ID: 1},
		48,
		appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(credential)
	if got := string(credential); got != `{"auth_mode":"chatgpt"}` {
		t.Fatalf("credential=%q; want exported auth.json", got)
	}
	if writer.audit.Event != "RESOURCE_CREDENTIAL_EXPORT" || writer.audit.ID != 48 {
		t.Fatalf("audit=%+v; want credential export event", writer.audit)
	}
	if after, ok := writer.audit.After.(map[string]string); !ok || after["authAdapter"] != AuthAdapterOpenAICodex {
		t.Fatalf("audit after=%#v; want non-sensitive adapter metadata", writer.audit.After)
	}
}

func TestExportSubscriptionCredentialRejectsAPIKey(t *testing.T) {
	writer := &credentialUpdateWriter{resource: ResourceRecord{Resource: Resource{
		ID: 48, ProviderID: 40, Name: "OpenAI API Key", AuthType: AuthTypeAPIKey,
		AuthAdapter: AuthAdapterAPIKey,
	}}}
	service := New(credentialUpdateStore{writer: writer}, nil, providerTestCipher{}, nil)

	credential, err := service.ExportSubscriptionCredential(
		context.Background(),
		admin.Identity{ID: 1},
		48,
		appsec.RequestMeta{},
	)
	if !errors.Is(err, ErrCredentialExportUnsupported) || credential != nil {
		t.Fatalf("credential=%q error=%v; want unsupported export", credential, err)
	}
}

func TestResourceInferenceProbeBlocksBillingFailure(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{ID: 48, ProviderID: 40, Name: "上游账号", AuthType: AuthTypeAPIKey, AuthAdapter: AuthAdapterAPIKey, RuntimeStatus: "HEALTHY", UpdatedAt: updatedAt},
			Sealed:   catalog.SealedCredential{Ciphertext: []byte("provider-key"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: "anthropic-official", Endpoints: []ProviderEndpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"},
		}},
		mappings: []ProviderMapping{{ProviderID: 40, UpstreamModelCode: "claude-test", Priority: 0}},
	}
	tester := connectionTesterFunc(func(_ context.Context, target ConnectionTarget, credential []byte, _ *catalog.OutboundProxy) ConnectionResult {
		if target.ProviderCode != "anthropic-official" || target.Protocol != "ANTHROPIC" || target.BaseURL != "https://api.anthropic.com" || target.UpstreamModelCode != "claude-test" || string(credential) != "provider-key" {
			t.Fatalf("unexpected inference target: %+v", target)
		}
		return ConnectionResult{Code: "UPSTREAM_BILLING_BLOCKED", HTTPStatus: 403}
	})
	service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, tester)

	result, err := service.TestResource(context.Background(), admin.Identity{ID: 1}, 48, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || writer.restored {
		t.Fatal("billing failure must not restore the resource")
	}
	if writer.updated.RuntimeStatus != "BLOCKED" || writer.updated.BlockedReason == nil || *writer.updated.BlockedReason != "BILLING" || writer.updated.LastHTTPStatus == nil || *writer.updated.LastHTTPStatus != 403 {
		t.Fatalf("unexpected blocked resource: %+v", writer.updated.Resource)
	}
}

func TestResourceInferenceProbeUsesSelectedProtocol(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{ID: 48, ProviderID: 40, Name: "上游账号", AuthType: AuthTypeAPIKey, AuthAdapter: AuthAdapterAPIKey, RuntimeStatus: "HEALTHY", UpdatedAt: updatedAt},
			Sealed:   catalog.SealedCredential{Ciphertext: []byte("provider-key"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: "custom-provider", Endpoints: []ProviderEndpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.example.com/anthropic"},
		}},
		model:    Model{ID: 90, Code: "logical-model"},
		mappings: []ProviderMapping{{ProviderID: 40, ModelID: 90, UpstreamModelCode: "", Priority: 0}},
	}
	tester := connectionTesterFunc(func(_ context.Context, target ConnectionTarget, _ []byte, _ *catalog.OutboundProxy) ConnectionResult {
		if target.ProviderCode != "custom-provider" || target.Protocol != "OPENAI" || target.BaseURL != "https://api.example.com/v1" || target.UpstreamModelCode != "logical-model" {
			t.Fatalf("unexpected selected inference target: %+v", target)
		}
		return ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200}
	})
	service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, tester)

	result, err := service.TestResourceProtocol(context.Background(), admin.Identity{ID: 1}, 48, "openai", appsec.RequestMeta{})
	if err != nil || !result.OK || result.Code != "OK" || !writer.restored {
		t.Fatalf("selected protocol test failed: result=%+v restored=%v err=%v", result, writer.restored, err)
	}
}

func TestPersonalSubscriptionConnectionUsesQuotaProbeWithoutModelMapping(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	reset := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	percent := 20.0
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{
				ID: 48, ProviderID: 40, Name: "OpenAI 个人订阅",
				AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterOpenAICodex,
				RuntimeStatus: "HEALTHY", UpdatedAt: updatedAt,
			},
			Sealed: catalog.SealedCredential{Ciphertext: []byte("imported-auth-cache"), KeyVersion: 1},
		},
		provider: Provider{
			ID: 40, Code: "openai-official", ProxyEnabled: true,
			ProxyURLSealed: catalog.SealedCredential{Ciphertext: []byte("http://proxy.example.com:8080"), KeyVersion: 1},
		},
	}
	proxySeen := false
	service := New(
		resourceTestStore{writer: writer},
		nil,
		providerTestCipher{},
		nil,
		WithSubscriptionAdapter(subscriptionAdapterStub{probe: SubscriptionProbe{
			Inspection: SubscriptionInspection{AccountRef: "account-1", PlanCode: "plus"},
			Credential: []byte("refreshed-auth-cache"),
			Quotas: []ResourceQuota{{
				Code: "codex.primary", Status: QuotaAvailable, UsedPercent: &percent, ResetsAt: &reset,
			}},
		}, onProbe: func(proxy *catalog.OutboundProxy) {
			proxySeen = proxy != nil && proxy.URL == "http://proxy.example.com:8080"
		}}),
	)

	result, err := service.TestResource(
		context.Background(),
		admin.Identity{ID: 1},
		48,
		appsec.RequestMeta{},
	)
	if err != nil || !result.OK || result.Code != "OK" {
		t.Fatalf("unexpected subscription test result: %+v err=%v", result, err)
	}
	if !proxySeen || !writer.restored || writer.updated.QuotaStatus != QuotaAvailable || len(writer.quotas) != 1 {
		t.Fatalf("subscription state was not refreshed: updated=%+v quotas=%+v", writer.updated.Resource, writer.quotas)
	}
	if string(writer.updated.Sealed.Ciphertext) != "refreshed-auth-cache" {
		t.Fatalf("refreshed credential was not persisted: %q", writer.updated.Sealed.Ciphertext)
	}
}

func TestCreatePersonalSubscriptionProbesAndPersistsQuota(t *testing.T) {
	local := time.FixedZone("CST", 8*60*60)
	reset := time.Now().In(local).Add(time.Hour).Truncate(time.Second)
	effectiveAt := time.Date(2026, 9, 1, 8, 0, 0, 0, local)
	expiresAt := effectiveAt.Add(24 * time.Hour)
	percent := 20.0
	writer := &resourceCreateWriter{}
	service := New(resourceCreateStore{writer: writer}, fixedMemberID{id: 48}, providerTestCipher{}, nil,
		WithSubscriptionAdapter(subscriptionAdapterStub{probe: SubscriptionProbe{
			Inspection: SubscriptionInspection{AccountRef: "account-1", PlanCode: "plus"},
			Credential: []byte("refreshed-auth-cache"),
			Quotas:     []ResourceQuota{{Code: "codex.primary", Status: QuotaAvailable, UsedPercent: &percent, ResetsAt: &reset}},
		}}),
	)

	created, err := service.CreateAuthenticationResource(context.Background(), admin.Identity{ID: 1}, CreateResourceInput{
		ProviderID: 40, Name: "个人订阅", Credential: "imported-auth-cache",
		AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterOpenAICodex, Priority: 10,
		EffectiveAt: &effectiveAt, ExpiresAt: &expiresAt,
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if created.SubscriptionType == nil || *created.SubscriptionType != SubscriptionPersonal || created.QuotaStatus != QuotaAvailable || created.PlanCode == nil || *created.PlanCode != "plus" {
		t.Fatalf("unexpected subscription resource: %+v", created)
	}
	if created.EffectiveAt == nil || created.ExpiresAt == nil || created.QuotaResetsAt == nil ||
		created.EffectiveAt.Location() != time.UTC || created.ExpiresAt.Location() != time.UTC || created.QuotaResetsAt.Location() != time.UTC ||
		writer.quotas[0].ResetsAt == nil || writer.quotas[0].ResetsAt.Location() != time.UTC {
		t.Fatalf("resource times were not normalized to UTC: created=%+v quotas=%+v", created, writer.quotas)
	}
	if string(writer.created.Sealed.Ciphertext) != "refreshed-auth-cache" || len(writer.quotas) != 1 || writer.quotas[0].Code != "codex.primary" {
		t.Fatalf("credential or quotas were not persisted: sealed=%q quotas=%+v", writer.created.Sealed.Ciphertext, writer.quotas)
	}
}

func TestMemberSuggestionsQueriesByName(t *testing.T) {
	reader := &memberSuggestionReader{result: []Member{{ID: 42, Name: "林知远"}}}
	service := New(memberSuggestionStore{reader: reader}, nil, nil, nil)
	page := Page{Limit: 8}

	result, err := service.MemberSuggestions(context.Background(), admin.Identity{}, page, "林知")
	if err != nil {
		t.Fatal(err)
	}
	if !reader.called || reader.page != page || reader.name != "林知" {
		t.Fatalf("query page=%#v name=%q called=%v", reader.page, reader.name, reader.called)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "林知远" || result.Total != 1 {
		t.Fatalf("result=%#v; want matching member", result)
	}
}

func TestMemberSuggestionsRejectsEmptyName(t *testing.T) {
	reader := &memberSuggestionReader{}
	service := New(memberSuggestionStore{reader: reader}, nil, nil, nil)

	_, err := service.MemberSuggestions(context.Background(), admin.Identity{}, Page{Limit: 8}, "")
	if !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("error=%v; want invalid argument", err)
	}
	if reader.called {
		t.Fatal("reader should not be called for an empty name")
	}
}
