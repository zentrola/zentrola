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

type resetCreditAdapterStub struct {
	subscriptionAdapterStub
	consume   ResetCreditConsume
	onConsume func(string, string, *catalog.OutboundProxy)
}

type subscriptionConnectionErrorStub string

func (e subscriptionConnectionErrorStub) Error() string          { return "subscription connection failed" }
func (e subscriptionConnectionErrorStub) ConnectionCode() string { return string(e) }

func TestSubscriptionConnectionCodeAllowsOnlyKnownSafeCodes(t *testing.T) {
	if code := subscriptionConnectionCode(subscriptionConnectionErrorStub("CODEX_APP_SERVER_UNAVAILABLE")); code != "CODEX_APP_SERVER_UNAVAILABLE" {
		t.Fatalf("unexpected connection code: %s", code)
	}
	if code := subscriptionConnectionCode(subscriptionConnectionErrorStub("UPSTREAM_AUTH_FAILED")); code != "UPSTREAM_AUTH_FAILED" {
		t.Fatalf("unexpected Anthropic authentication code: %s", code)
	}
	if code := subscriptionConnectionCode(subscriptionConnectionErrorStub("UNSAFE_INTERNAL_DETAIL")); code != "SUBSCRIPTION_UNAVAILABLE" {
		t.Fatalf("unexpected fallback connection code: %s", code)
	}
}

func (s resetCreditAdapterStub) ConsumeResetCredit(_ context.Context, _ []byte, proxy *catalog.OutboundProxy, idempotencyKey, creditID string) (ResetCreditConsume, error) {
	if s.onConsume != nil {
		s.onConsume(idempotencyKey, creditID, proxy)
	}
	return s.consume, nil
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
	created  ResourceRecord
	quotas   []ResourceQuota
	provider Provider
}

func (w *resourceCreateWriter) Provider(context.Context, int64) (Provider, error) {
	if w.provider.Code != "" {
		return w.provider, nil
	}
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
	models   []Model
	mappings []ProviderMapping
	updated  ResourceRecord
	quotas   []ResourceQuota
	restored bool
	audit    Audit
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
func (w *resourceTestWriter) Models(context.Context, Page, string) ([]Model, error) {
	if w.models != nil {
		return append([]Model(nil), w.models...), nil
	}
	if w.model.ID != 0 {
		return []Model{w.model}, nil
	}
	return []Model{}, nil
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
func (w *resourceTestWriter) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	w.audit = audit
	return nil
}

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
		model:    Model{ID: 90, Code: "claude-test", Status: "ACTIVE"},
		mappings: []ProviderMapping{{ID: 1, ProviderID: 40, ModelID: 90, UpstreamModelCode: "claude-test", Priority: 0}},
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
		model:    Model{ID: 90, Code: "logical-model", Status: "ACTIVE"},
		mappings: []ProviderMapping{{ID: 1, ProviderID: 40, ModelID: 90, UpstreamModelCode: "", Priority: 0}},
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

func TestResourceInferenceProbeUsesExplicitModelMapping(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{ID: 48, ProviderID: 40, Name: "上游账号", AuthType: AuthTypeAPIKey, AuthAdapter: AuthAdapterAPIKey, RuntimeStatus: "HEALTHY", UpdatedAt: updatedAt},
			Sealed:   catalog.SealedCredential{Ciphertext: []byte("provider-key"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: "custom-provider", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"}}},
		models: []Model{
			{ID: 90, Code: "model-one", Status: "ACTIVE"},
			{ID: 91, Code: "model-two", Status: "ACTIVE"},
			{ID: 92, Code: "model-disabled", Status: "DISABLED"},
		},
		mappings: []ProviderMapping{
			{ID: 1, ProviderID: 40, ModelID: 90, UpstreamModelCode: "upstream-one"},
			{ID: 2, ProviderID: 40, ModelID: 91, UpstreamModelCode: "upstream-two"},
			{ID: 3, ProviderID: 40, ModelID: 99, UpstreamModelCode: "deleted-model"},
			{ID: 4, ProviderID: 41, ModelID: 90, UpstreamModelCode: "other-provider"},
			{ID: 5, ProviderID: 40, ModelID: 92, UpstreamModelCode: "disabled-model"},
		},
	}
	testCalls := 0
	tester := connectionTesterFunc(func(_ context.Context, target ConnectionTarget, _ []byte, _ *catalog.OutboundProxy) ConnectionResult {
		testCalls++
		if target.UpstreamModelCode != "upstream-two" {
			t.Fatalf("unexpected selected model: %+v", target)
		}
		return ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200}
	})
	service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, tester)

	result, err := service.TestResourceSelection(context.Background(), admin.Identity{ID: 1}, 48, "OPENAI", 2, appsec.RequestMeta{})
	if err != nil || !result.OK {
		t.Fatalf("explicit model connection test failed: result=%+v err=%v", result, err)
	}
	if result.ProviderModelMappingID != 2 || result.TestedModelID != 91 || result.TestedModelCode != "upstream-two" {
		t.Fatalf("unexpected tested model metadata: %+v", result)
	}
	for _, invalidMappingID := range []int64{3, 4, 5, 999} {
		_, err = service.TestResourceSelection(context.Background(), admin.Identity{ID: 1}, 48, "OPENAI", invalidMappingID, appsec.RequestMeta{})
		if !errors.Is(err, appsec.ErrInvalidArgument) {
			t.Fatalf("mapping %d error=%v; want invalid argument", invalidMappingID, err)
		}
	}
	if testCalls != 1 {
		t.Fatalf("connection tester calls=%d; want 1", testCalls)
	}
}

func TestGoogleConnectionProbeModelRanking(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{code: "gemini-3.6-flash", want: 0},
		{code: "gemini-2.5-flash", want: 25},
		{code: "gemini-3.1-flash-lite", want: 10},
		{code: "gemini-3-flash-preview", want: 20},
		{code: "gemini-3-pro-image", want: 30},
		{code: "gemini-2.5-flash-preview-tts", want: 30},
		{code: "gemini-3.5-transcribe", want: 30},
		{code: "antigravity-preview-05-2026", want: 30},
	}
	for _, test := range tests {
		if got := connectionProbeModelRank(catalog.GoogleOfficialCode, test.code); got != test.want {
			t.Errorf("connectionProbeModelRank(%q) = %d, want %d", test.code, got, test.want)
		}
	}
	if got := connectionProbeModelRank(catalog.OpenAIOfficialCode, "gpt-test"); got != 0 {
		t.Fatalf("non-Google model rank = %d, want 0", got)
	}
}

func TestGoogleConnectionProbeIgnoresMappingsForMissingModels(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{ID: 48, ProviderID: 40, Name: "Google API Key", AuthType: AuthTypeAPIKey, AuthAdapter: AuthAdapterAPIKey, RuntimeStatus: "HEALTHY", UpdatedAt: updatedAt},
			Sealed:   catalog.SealedCredential{Ciphertext: []byte("provider-key"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: catalog.GoogleOfficialCode, Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"}}},
		models: []Model{
			{ID: 91, Code: "gemini-3-flash-preview", Status: "ACTIVE"},
			{ID: 92, Code: "gemini-3.6-flash", Status: "ACTIVE"},
		},
		mappings: []ProviderMapping{
			{ID: 1, ProviderID: 40, ModelID: 90, Priority: 100},
			{ID: 2, ProviderID: 40, ModelID: 91, Priority: 100},
			{ID: 3, ProviderID: 40, ModelID: 92, Priority: 100},
		},
	}
	tester := connectionTesterFunc(func(_ context.Context, target ConnectionTarget, _ []byte, _ *catalog.OutboundProxy) ConnectionResult {
		if target.UpstreamModelCode != "gemini-3.6-flash" {
			t.Fatalf("unexpected Google probe model: %+v", target)
		}
		return ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200}
	})
	service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, tester)

	result, err := service.TestResourceProtocol(context.Background(), admin.Identity{ID: 1}, 48, "OPENAI", appsec.RequestMeta{})
	if err != nil || !result.OK || !writer.restored {
		t.Fatalf("Google connection test failed: result=%+v restored=%v err=%v", result, writer.restored, err)
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
			ResetCredits: &RateLimitResetCredits{AvailableCount: 1, Credits: []RateLimitResetCredit{{
				ID: "credit-secret", Status: "available",
			}}},
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
	if err != nil || !result.OK || result.Code != "OK" || result.ResetCredits == nil || result.ResetCredits.AvailableCount != 1 {
		t.Fatalf("unexpected subscription test result: %+v err=%v", result, err)
	}
	if !proxySeen || !writer.restored || writer.updated.QuotaStatus != QuotaAvailable || len(writer.quotas) != 1 {
		t.Fatalf("subscription state was not refreshed: updated=%+v quotas=%+v", writer.updated.Resource, writer.quotas)
	}
	if string(writer.updated.Sealed.Ciphertext) != "refreshed-auth-cache" {
		t.Fatalf("refreshed credential was not persisted: %q", writer.updated.Sealed.Ciphertext)
	}
	auditedResult, ok := writer.audit.After.(map[string]any)["test"].(ConnectionResult)
	if !ok || auditedResult.ResetCredits == nil || auditedResult.ResetCredits.AvailableCount != 1 || len(auditedResult.ResetCredits.Credits) != 0 {
		t.Fatalf("reset credit IDs must not be written to audit: %+v", writer.audit.After)
	}
}

func TestConsumeResourceResetCreditRefreshesQuotaAndRestoresResource(t *testing.T) {
	updatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	reset := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	percent := 10.0
	expires := time.Now().In(time.FixedZone("CST", 8*60*60)).Add(24 * time.Hour).Truncate(time.Second)
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{
				ID: 48, ProviderID: 40, Name: "OpenAI 个人订阅",
				AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterOpenAICodex,
				RuntimeStatus: "BLOCKED", UpdatedAt: updatedAt,
			},
			Sealed: catalog.SealedCredential{Ciphertext: []byte("imported-auth-cache"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: "openai-official"},
	}
	consumeSeen := false
	adapter := resetCreditAdapterStub{
		subscriptionAdapterStub: subscriptionAdapterStub{},
		consume: ResetCreditConsume{Outcome: "reset", Probe: SubscriptionProbe{
			Inspection: SubscriptionInspection{AccountRef: "account-1", PlanCode: "plus"},
			Credential: []byte("refreshed-auth-cache"),
			Quotas: []ResourceQuota{{
				Code: "codex.primary", Status: QuotaAvailable, UsedPercent: &percent, ResetsAt: &reset,
			}},
			ResetCredits: &RateLimitResetCredits{AvailableCount: 0, Credits: []RateLimitResetCredit{{
				ID: "credit-1", ResetType: "codexRateLimits", Status: "consumed",
				GrantedAt: expires.Add(-time.Hour), ExpiresAt: &expires,
			}}},
		}},
		onConsume: func(idempotencyKey, creditID string, proxy *catalog.OutboundProxy) {
			consumeSeen = idempotencyKey == "request-1" && creditID == "credit-1" && proxy == nil
		},
	}
	service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, nil, WithSubscriptionAdapter(adapter))

	result, err := service.ConsumeResourceResetCredit(
		context.Background(), admin.Identity{ID: 1}, 48, "request-1", "credit-1", appsec.RequestMeta{},
	)
	if err != nil || result.Outcome != "reset" || result.ResetCredits == nil || result.ResetCredits.AvailableCount != 0 {
		t.Fatalf("unexpected reset result: %+v err=%v", result, err)
	}
	if !consumeSeen || !writer.restored || writer.updated.QuotaStatus != QuotaAvailable || len(writer.quotas) != 1 ||
		string(writer.updated.Sealed.Ciphertext) != "refreshed-auth-cache" {
		t.Fatalf("reset state was not persisted: updated=%+v quotas=%+v", writer.updated.Resource, writer.quotas)
	}
	if result.ResetCredits.Credits[0].ExpiresAt == nil || result.ResetCredits.Credits[0].ExpiresAt.Location() != time.UTC {
		t.Fatalf("reset credit time was not normalized: %+v", result.ResetCredits.Credits[0])
	}
	if writer.audit.Event != "RESOURCE_RATE_LIMIT_RESET" {
		t.Fatalf("unexpected audit: %+v", writer.audit)
	}
}

func TestCreatePersonalSubscriptionPersistsWithoutOnlineProbe(t *testing.T) {
	local := time.FixedZone("CST", 8*60*60)
	effectiveAt := time.Date(2026, 9, 1, 8, 0, 0, 0, local)
	expiresAt := effectiveAt.Add(24 * time.Hour)
	writer := &resourceCreateWriter{}
	service := New(resourceCreateStore{writer: writer}, fixedMemberID{id: 48}, providerTestCipher{}, nil,
		WithSubscriptionAdapter(subscriptionAdapterStub{onProbe: func(*catalog.OutboundProxy) {
			t.Fatal("saving must not probe upstream")
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
	if created.SubscriptionType == nil || *created.SubscriptionType != SubscriptionPersonal || created.QuotaStatus != QuotaUnknown || created.QuotaCheckedAt != nil {
		t.Fatalf("unexpected subscription resource: %+v", created)
	}
	if created.EffectiveAt == nil || created.ExpiresAt == nil || created.EffectiveAt.Location() != time.UTC || created.ExpiresAt.Location() != time.UTC {
		t.Fatal("resource times were not normalized to UTC")
	}
	if string(writer.created.Sealed.Ciphertext) != "imported-auth-cache" || len(writer.quotas) != 0 {
		t.Fatal("original credential must be persisted without quotas")
	}
}

func TestUpdateSubscriptionCredentialClearsStaleQuotaWithoutOnlineProbe(t *testing.T) {
	for _, code := range []string{AuthAdapterOpenAICodex, AuthAdapterClaudeCode} {
		t.Run(code, func(t *testing.T) {
			stub := subscriptionAdapterStub{onProbe: func(*catalog.OutboundProxy) { t.Fatal("saving must not probe upstream") }}
			var adapter SubscriptionAdapter = stub
			provider := Provider{ID: 40, Code: catalog.OpenAIOfficialCode}
			if code == AuthAdapterClaudeCode {
				adapter = claudeSubscriptionAdapterStub{subscriptionAdapterStub: stub}
				provider.Code = catalog.AnthropicOfficialCode
			}
			now := time.Now().UTC()
			writer := &resourceTestWriter{provider: provider, resource: ResourceRecord{Resource: Resource{
				ID: 48, ProviderID: 40, AuthType: AuthTypeSubscription, AuthAdapter: code,
				ExternalAccountRef: stringPointer("old-account"), PlanCode: stringPointer("old-plan"),
				QuotaStatus: QuotaAvailable, QuotaCheckedAt: &now, QuotaResetsAt: &now,
				CredentialRefreshedAt: &now, CredentialExpiresAt: &now,
			}}, quotas: []ResourceQuota{{Code: "old-quota"}}}
			service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, nil, WithSubscriptionAdapter(adapter))
			if err := service.UpdateCredential(context.Background(), admin.Identity{ID: 1}, 48, "replacement-auth-cache", appsec.RequestMeta{}); err != nil {
				t.Fatal(err)
			}
			updated := writer.updated
			if string(updated.Sealed.Ciphertext) != "replacement-auth-cache" || updated.QuotaStatus != QuotaUnknown || updated.QuotaCheckedAt != nil || updated.QuotaResetsAt != nil || len(writer.quotas) != 0 {
				t.Fatal("replacement credential must be saved and stale quota cleared")
			}
			if updated.ExternalAccountRef != nil || updated.PlanCode != nil || updated.CredentialRefreshedAt != nil || updated.CredentialExpiresAt != nil {
				t.Fatal("stale credential metadata was retained")
			}
		})
	}
}

func TestCreateClaudeSubscriptionPersistsWithoutOnlineProbe(t *testing.T) {
	probeCalled := false
	writer := &resourceCreateWriter{provider: Provider{ID: 36, Code: catalog.AnthropicOfficialCode}}
	adapter := claudeSubscriptionAdapterStub{subscriptionAdapterStub: subscriptionAdapterStub{
		onProbe: func(*catalog.OutboundProxy) { probeCalled = true },
	}}
	service := New(resourceCreateStore{writer: writer}, fixedMemberID{id: 48}, providerTestCipher{}, nil,
		WithSubscriptionAdapter(adapter),
	)

	created, err := service.CreateAuthenticationResource(context.Background(), admin.Identity{ID: 1}, CreateResourceInput{
		ProviderID: 36, Name: "Anthropic 个人订阅", Credential: "9527",
		AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterClaudeCode, Priority: 100,
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if probeCalled {
		t.Fatal("Claude subscription was probed while saving")
	}
	if string(writer.created.Sealed.Ciphertext) != "9527" || created.QuotaStatus != QuotaUnknown || created.QuotaCheckedAt != nil {
		t.Fatalf("unexpected saved Claude subscription: created=%+v sealed=%q", created, writer.created.Sealed.Ciphertext)
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
