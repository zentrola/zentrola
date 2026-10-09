package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type mappingWriter struct {
	Writer
	mappings []ProviderMapping
	models   map[int64]Model
	updated  []ProviderMapping
	deleted  []int64
}

type providerStatusWriter struct {
	Writer
	provider             Provider
	mappings             []ProviderMapping
	models               []Model
	resources            []Resource
	credentialConfigured bool
	statusChanges        []string
	audits               []Audit
}

func (w *providerStatusWriter) Provider(context.Context, int64) (Provider, error) {
	return w.provider, nil
}
func (w *providerStatusWriter) ProviderCredentialConfigured(context.Context, int64) (bool, error) {
	return w.credentialConfigured, nil
}
func (w *providerStatusWriter) ProviderActivationResourceIDs(_ context.Context, providerID int64, at time.Time) ([]int64, error) {
	var ids []int64
	for _, resource := range w.resources {
		if resource.ProviderID == providerID && resource.CredentialConfigured &&
			(resource.EffectiveAt == nil || !resource.EffectiveAt.After(at)) &&
			(resource.ExpiresAt == nil || resource.ExpiresAt.After(at)) {
			ids = append(ids, resource.ID)
		}
	}
	return ids, nil
}
func (w *providerStatusWriter) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return append([]ProviderMapping(nil), w.mappings...), nil
}
func (w *providerStatusWriter) Models(context.Context, Page, string) ([]Model, error) {
	return append([]Model(nil), w.models...), nil
}
func (w *providerStatusWriter) Model(_ context.Context, id int64) (Model, error) {
	for _, model := range w.models {
		if model.ID == id {
			return model, nil
		}
	}
	return Model{}, appsec.ErrNotFound
}
func (w *providerStatusWriter) Resources(context.Context, Page) ([]Resource, error) {
	return append([]Resource(nil), w.resources...), nil
}
func (w *providerStatusWriter) Resource(_ context.Context, id int64) (ResourceRecord, error) {
	for _, resource := range w.resources {
		if resource.ID == id {
			return ResourceRecord{Resource: resource}, nil
		}
	}
	return ResourceRecord{}, errors.New("resource not found")
}
func (w *providerStatusWriter) SetProviderStatus(_ context.Context, _ int64, status string) error {
	w.statusChanges = append(w.statusChanges, status)
	return nil
}
func (w *providerStatusWriter) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	w.audits = append(w.audits, audit)
	return nil
}

type providerStatusStore struct{ writer *providerStatusWriter }

func (s providerStatusStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.writer)
}
func (s providerStatusStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

type providerConnectionProbeFunc func(context.Context, admin.Identity, int64, appsec.RequestMeta) (ConnectionResult, error)

func (f providerConnectionProbeFunc) TestResource(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (ConnectionResult, error) {
	return f(ctx, actor, id, meta)
}
func (f providerConnectionProbeFunc) TestResourceSelection(ctx context.Context, actor admin.Identity, id int64, _ string, _ int64, meta appsec.RequestMeta) (ConnectionResult, error) {
	return f(ctx, actor, id, meta)
}

type providerSelectionProbeFunc func(context.Context, admin.Identity, int64, string, int64, appsec.RequestMeta) (ConnectionResult, error)

func (f providerSelectionProbeFunc) TestResource(context.Context, admin.Identity, int64, appsec.RequestMeta) (ConnectionResult, error) {
	return ConnectionResult{}, errors.New("unexpected default connection test")
}
func (f providerSelectionProbeFunc) TestResourceSelection(ctx context.Context, actor admin.Identity, id int64, protocol string, mappingID int64, meta appsec.RequestMeta) (ConnectionResult, error) {
	return f(ctx, actor, id, protocol, mappingID, meta)
}

func (w *mappingWriter) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return append([]ProviderMapping(nil), w.mappings...), nil
}
func (w *mappingWriter) Model(_ context.Context, id int64) (Model, error) {
	if model, exists := w.models[id]; exists {
		return model, nil
	}
	return Model{ID: id, Status: "ACTIVE"}, nil
}
func (w *mappingWriter) UpdateProviderMapping(_ context.Context, mapping ProviderMapping) error {
	w.updated = append(w.updated, mapping)
	return nil
}
func (w *mappingWriter) DeleteProviderMapping(_ context.Context, _ int64, mappingID int64, _ time.Time) error {
	w.deleted = append(w.deleted, mappingID)
	return nil
}

type providerTestCipher struct{}

func (providerTestCipher) Encrypt(plain []byte, _ catalog.CredentialOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{Ciphertext: append([]byte(nil), plain...), Nonce: make([]byte, 12), KeyVersion: 1}, nil
}
func (providerTestCipher) Decrypt(sealed catalog.SealedCredential, _ catalog.CredentialOwner) ([]byte, error) {
	return append([]byte(nil), sealed.Ciphertext...), nil
}
func (providerTestCipher) EncryptProviderProxy(plain []byte, _ catalog.ProviderProxyOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{Ciphertext: append([]byte(nil), plain...), Nonce: make([]byte, 12), KeyVersion: 1}, nil
}
func (providerTestCipher) DecryptProviderProxy(sealed catalog.SealedCredential, _ catalog.ProviderProxyOwner) ([]byte, error) {
	return append([]byte(nil), sealed.Ciphertext...), nil
}

func TestProviderFromInput(t *testing.T) {
	current := Provider{ID: 1, Code: "provider-1", Type: "CUSTOM", Status: "DISABLED"}
	got, ok := providerFromInput(current, ProviderInput{
		Name:    "通义千问",
		Website: "  https://www.deepseek.com/  ",
		Endpoints: []ProviderEndpoint{{
			ProtocolType: "OPENAI",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1/",
		}},
	})
	if !ok || got.Website == nil || *got.Website != "https://www.deepseek.com" || len(got.Endpoints) != 1 || got.Endpoints[0].BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("unexpected provider: %+v", got)
	}
	private, ok := providerFromInput(current, ProviderInput{
		Name: "内网服务", Endpoints: []ProviderEndpoint{{
			ProtocolType: "OPENAI", BaseURL: "http://192.168.1.20:8080/v1", NetworkScope: catalog.NetworkScopePrivate,
		}},
	})
	if !ok || private.Endpoints[0].NetworkScope != catalog.NetworkScopePrivate {
		t.Fatalf("private endpoint rejected: %+v", private)
	}

	for _, input := range []ProviderInput{
		{Name: "没有接口"},
		{Name: "不安全协议", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "http://api.example.com"}}},
		{Name: "未知网络范围", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com", NetworkScope: "INTERNAL"}}},
		{Name: "元数据地址", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "http://169.254.169.254/latest/meta-data", NetworkScope: catalog.NetworkScopePrivate}}},
		{Name: "包含凭证", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://key@api.example.com"}}},
		{Name: "包含查询参数", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1?token=secret"}}},
		{Name: "未知协议", Endpoints: []ProviderEndpoint{{ProtocolType: "GEMINI_NATIVE", BaseURL: "https://api.example.com"}}},
		{Name: "重复协议", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://one.example.com"}, {ProtocolType: "OPENAI", BaseURL: "https://two.example.com"}}},
	} {
		if _, ok := providerFromInput(current, input); ok {
			t.Fatalf("invalid provider accepted: %+v", input)
		}
	}
}

func TestProviderInputNormalizationAndValidation(t *testing.T) {
	input := ProviderInput{
		Name:      "  服务商  ",
		Website:   "  https://example.com/  ",
		Endpoints: []ProviderEndpoint{{ProtocolType: " OPENAI ", BaseURL: " https://api.example.com/v1/ "}},
		Mappings:  []ProviderMappingInput{{ModelID: 1, UpstreamModelCode: " upstream-model "}},
	}
	input.Normalize()
	if !input.Valid() || input.Name != "服务商" || input.Website != "https://example.com/" ||
		input.Endpoints[0].ProtocolType != "OPENAI" || input.Endpoints[0].BaseURL != "https://api.example.com/v1/" ||
		input.Mappings[0].UpstreamModelCode != "upstream-model" {
		t.Fatalf("unexpected normalized provider input: %+v", input)
	}
	input.Mappings = nil
	if !input.Valid() {
		t.Fatal("provider input without mappings should be valid while disabled")
	}
}

func TestProviderMappingValidation(t *testing.T) {
	provider := Provider{}
	valid := []ProviderMappingInput{
		{ModelID: 1},
		{ModelID: 2, UpstreamModelCode: "vendor-model-flash", Priority: 200},
	}
	if !validProviderMappings(provider, valid) {
		t.Fatal("valid mappings rejected")
	}

	invalid := [][]ProviderMappingInput{
		{{ModelID: 0, UpstreamModelCode: "vendor-model"}},
		{{ModelID: 1, UpstreamModelCode: " vendor-model "}},
		{{ModelID: 1, UpstreamModelCode: "vendor-model", Priority: 10001}},
		{valid[0], valid[0]},
	}
	for _, mappings := range invalid {
		if validProviderMappings(provider, mappings) {
			t.Fatalf("invalid mappings accepted: %+v", mappings)
		}
	}
}

func TestReplaceProviderMappingsAllowsRemovingAllModels(t *testing.T) {
	writer := &mappingWriter{mappings: []ProviderMapping{
		{ID: 10, ProviderID: 8, ModelID: 1, Priority: 100},
		{ID: 11, ProviderID: 8, ModelID: 2, Priority: 100},
	}}
	service := &ProviderService{}
	result, err := service.replaceProviderMappings(context.Background(), writer, Provider{ID: 8}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 || len(writer.deleted) != 2 {
		t.Fatalf("all mappings were not removed: result=%+v deleted=%v", result, writer.deleted)
	}
}

func TestReplaceProviderMappingsLogicallyDeletesUncheckedModels(t *testing.T) {
	writer := &mappingWriter{mappings: []ProviderMapping{
		{ID: 10, ProviderID: 8, ModelID: 1, UpstreamModelCode: "old-one", Priority: 100},
		{ID: 11, ProviderID: 8, ModelID: 2, UpstreamModelCode: "old-two", Priority: 100},
	}}
	service := &ProviderService{}
	result, err := service.replaceProviderMappings(context.Background(), writer, Provider{ID: 8}, []ProviderMappingInput{{
		ModelID: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].ID != 10 || result[0].Priority != defaultProviderMappingPriority {
		t.Fatalf("unexpected retained mappings: %+v", result)
	}
	if len(writer.updated) != 1 || writer.updated[0].UpstreamModelCode != "" {
		t.Fatalf("checked mapping was not updated: %+v", writer.updated)
	}
	if len(writer.deleted) != 1 || writer.deleted[0] != 11 {
		t.Fatalf("unchecked mapping was not logically deleted: %+v", writer.deleted)
	}
}

func TestReplaceProviderMappingsRejectsDisabledModelWithoutChangingMappings(t *testing.T) {
	writer := &mappingWriter{
		mappings: []ProviderMapping{{ID: 10, ProviderID: 8, ModelID: 1}},
		models:   map[int64]Model{2: {ID: 2, Status: "DISABLED"}},
	}
	service := &ProviderService{}
	_, err := service.replaceProviderMappings(context.Background(), writer, Provider{ID: 8}, []ProviderMappingInput{
		{ModelID: 1}, {ModelID: 2},
	})
	if !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("error=%v; want invalid argument", err)
	}
	if len(writer.updated) != 0 || len(writer.deleted) != 0 {
		t.Fatalf("rejected mapping changed state: updated=%+v deleted=%v", writer.updated, writer.deleted)
	}
}

func TestSetProviderStatusRequiresConfiguredCredentialWhenEnabling(t *testing.T) {
	writer := &providerStatusWriter{
		provider: Provider{ID: 8, Name: "待配置服务商", Status: "DISABLED"},
		mappings: []ProviderMapping{{ID: 10, ProviderID: 8, ModelID: 1}},
		models:   []Model{{ID: 1, Status: "ACTIVE"}},
	}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil)

	err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{})
	if !errors.Is(err, ErrProviderCredentialRequired) {
		t.Fatalf("error=%v; want provider credential required", err)
	}
	if len(writer.statusChanges) != 0 || len(writer.audits) != 0 {
		t.Fatalf("rejected enable changed state: statuses=%v audits=%v", writer.statusChanges, writer.audits)
	}

	writer.credentialConfigured = true
	writer.resources = []Resource{{ID: 20, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true}}
	service.ProviderService.connectionProbe = providerConnectionProbeFunc(func(_ context.Context, _ admin.Identity, id int64, _ appsec.RequestMeta) (ConnectionResult, error) {
		if id != 20 {
			t.Fatalf("tested resource=%d; want 20", id)
		}
		return ConnectionResult{OK: true, Code: "OK", ProviderModelMappingID: 10, TestedModelID: 1, verifiedResourceVersion: 1}, nil
	})
	if err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if len(writer.statusChanges) != 1 || writer.statusChanges[0] != "ACTIVE" || len(writer.audits) != 1 {
		t.Fatalf("configured provider was not enabled and audited: statuses=%v audits=%v", writer.statusChanges, writer.audits)
	}
}

func TestSetProviderStatusRequiresSuccessfulConnectionTest(t *testing.T) {
	writer := &providerStatusWriter{
		provider:             Provider{ID: 8, Name: "待验证服务商", Status: "DISABLED"},
		mappings:             []ProviderMapping{{ID: 10, ProviderID: 8, ModelID: 1}},
		models:               []Model{{ID: 1, Status: "ACTIVE"}},
		credentialConfigured: true,
		resources: []Resource{
			{ID: 20, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true},
			{ID: 21, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true},
		},
	}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil)
	var tested []int64
	service.ProviderService.connectionProbe = providerConnectionProbeFunc(func(_ context.Context, _ admin.Identity, id int64, _ appsec.RequestMeta) (ConnectionResult, error) {
		tested = append(tested, id)
		return ConnectionResult{Code: "UPSTREAM_RATE_LIMITED"}, nil
	})
	err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{})
	if !errors.Is(err, ErrProviderConnectionTestFailed) {
		t.Fatalf("error=%v; want provider connection test failed", err)
	}
	if len(tested) != 2 || tested[0] != 20 || tested[1] != 21 || len(writer.statusChanges) != 0 || len(writer.audits) != 0 {
		t.Fatalf("failed tests enabled provider: tested=%v statuses=%v audits=%v", tested, writer.statusChanges, writer.audits)
	}
	service.ProviderService.connectionProbe = providerConnectionProbeFunc(func(_ context.Context, _ admin.Identity, id int64, _ appsec.RequestMeta) (ConnectionResult, error) {
		tested = append(tested, id)
		return ConnectionResult{OK: id == 21, Code: "OK", ProviderModelMappingID: 10, TestedModelID: 1, verifiedResourceVersion: 1}, nil
	})
	if err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if len(writer.statusChanges) != 1 || writer.statusChanges[0] != "ACTIVE" {
		t.Fatalf("successful second credential did not enable provider: %v", writer.statusChanges)
	}
}

func TestActivateProviderUsesSelectedConnection(t *testing.T) {
	writer := &providerStatusWriter{
		provider: Provider{ID: 8, Status: "DISABLED"},
		mappings: []ProviderMapping{
			{ID: 10, ProviderID: 8, ModelID: 1, UpstreamModelCode: "unavailable-model"},
			{ID: 11, ProviderID: 8, ModelID: 2, UpstreamModelCode: "working-model"},
		},
		models:               []Model{{ID: 1, Status: "ACTIVE"}, {ID: 2, Status: "ACTIVE"}},
		credentialConfigured: true,
		resources:            []Resource{{ID: 20, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true}},
	}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil)
	service.ProviderService.connectionProbe = providerSelectionProbeFunc(func(_ context.Context, _ admin.Identity, id int64, protocol string, mappingID int64, _ appsec.RequestMeta) (ConnectionResult, error) {
		if id != 20 || protocol != "OPENAI" || mappingID != 11 {
			t.Fatalf("selection = resource %d, protocol %s, mapping %d", id, protocol, mappingID)
		}
		return ConnectionResult{OK: true, Code: "OK", ProviderModelMappingID: 11, TestedModelID: 2, TestedModelCode: "working-model", verifiedResourceVersion: 1}, nil
	})
	result, err := service.ActivateProvider(context.Background(), admin.Identity{}, 8, ProviderActivationSelection{
		ResourceID: 20, Protocol: "OPENAI", ProviderModelMappingID: 11,
	}, appsec.RequestMeta{})
	if err != nil || !result.OK || len(writer.statusChanges) != 1 || writer.statusChanges[0] != "ACTIVE" {
		t.Fatalf("activation failed: result=%+v err=%v statuses=%v", result, err, writer.statusChanges)
	}
}

func TestActivateProviderLeavesDisabledOnFailedTestOrChangedMapping(t *testing.T) {
	for _, test := range []struct {
		name          string
		changeMapping bool
	}{
		{name: "failed test"},
		{name: "mapping changed after test", changeMapping: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &providerStatusWriter{
				provider:             Provider{ID: 8, Status: "DISABLED"},
				mappings:             []ProviderMapping{{ID: 11, ProviderID: 8, ModelID: 2, UpstreamModelCode: "working-model"}},
				models:               []Model{{ID: 2, Status: "ACTIVE"}},
				credentialConfigured: true,
				resources:            []Resource{{ID: 20, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true}},
			}
			service := New(providerStatusStore{writer: writer}, nil, nil, nil)
			service.ProviderService.connectionProbe = providerSelectionProbeFunc(func(context.Context, admin.Identity, int64, string, int64, appsec.RequestMeta) (ConnectionResult, error) {
				if test.changeMapping {
					writer.mappings[0].UpstreamModelCode = "changed-model"
					return ConnectionResult{OK: true, Code: "OK", ProviderModelMappingID: 11, TestedModelID: 2, TestedModelCode: "working-model", verifiedResourceVersion: 1}, nil
				}
				return ConnectionResult{Code: "UPSTREAM_AUTH_FAILED"}, nil
			})
			result, err := service.ActivateProvider(context.Background(), admin.Identity{}, 8, ProviderActivationSelection{
				ResourceID: 20, Protocol: "OPENAI", ProviderModelMappingID: 11,
			}, appsec.RequestMeta{})
			if test.changeMapping {
				if !errors.Is(err, ErrConflict) {
					t.Fatalf("changed mapping error=%v; want conflict", err)
				}
			} else if err != nil || result.OK || result.Code != "UPSTREAM_AUTH_FAILED" {
				t.Fatalf("failed test result=%+v err=%v", result, err)
			}
			if len(writer.statusChanges) != 0 || len(writer.audits) != 0 {
				t.Fatalf("provider enabled after failed verification: statuses=%v audits=%v", writer.statusChanges, writer.audits)
			}
		})
	}
}

func TestSetProviderStatusRejectsCredentialChangedAfterConnectionTest(t *testing.T) {
	writer := &providerStatusWriter{
		provider:             Provider{ID: 8, Name: "待验证服务商", Status: "DISABLED"},
		mappings:             []ProviderMapping{{ID: 10, ProviderID: 8, ModelID: 1}},
		models:               []Model{{ID: 1, Status: "ACTIVE"}},
		credentialConfigured: true,
		resources:            []Resource{{ID: 20, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true}},
	}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil)
	service.ProviderService.connectionProbe = providerConnectionProbeFunc(func(context.Context, admin.Identity, int64, appsec.RequestMeta) (ConnectionResult, error) {
		writer.resources[0].Version++
		return ConnectionResult{OK: true, Code: "OK", ProviderModelMappingID: 10, TestedModelID: 1, verifiedResourceVersion: 1}, nil
	})
	err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{})
	if !errors.Is(err, ErrConflict) || len(writer.statusChanges) != 0 {
		t.Fatalf("changed credential enabled provider: error=%v statuses=%v", err, writer.statusChanges)
	}
}

func TestSetProviderStatusRejectsTestedModelDisabledAfterConnectionTest(t *testing.T) {
	writer := &providerStatusWriter{
		provider: Provider{ID: 8, Name: "待验证服务商", Status: "DISABLED"},
		mappings: []ProviderMapping{
			{ID: 10, ProviderID: 8, ModelID: 1},
			{ID: 11, ProviderID: 8, ModelID: 2},
		},
		models:               []Model{{ID: 1, Status: "ACTIVE"}, {ID: 2, Status: "ACTIVE"}},
		credentialConfigured: true,
		resources:            []Resource{{ID: 20, ProviderID: 8, Version: 1, AuthType: AuthTypeAPIKey, CredentialConfigured: true}},
	}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil)
	service.ProviderService.connectionProbe = providerConnectionProbeFunc(func(context.Context, admin.Identity, int64, appsec.RequestMeta) (ConnectionResult, error) {
		writer.models[0].Status = "DISABLED"
		return ConnectionResult{OK: true, Code: "OK", ProviderModelMappingID: 10, TestedModelID: 1, verifiedResourceVersion: 1}, nil
	})
	err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{})
	if !errors.Is(err, ErrConflict) || len(writer.statusChanges) != 0 {
		t.Fatalf("disabled tested model enabled provider: error=%v statuses=%v", err, writer.statusChanges)
	}
}

func TestSetProviderStatusDoesNotTestExpiredCredential(t *testing.T) {
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	writer := &providerStatusWriter{
		provider:             Provider{ID: 8, Name: "待验证服务商", Status: "DISABLED"},
		mappings:             []ProviderMapping{{ID: 10, ProviderID: 8, ModelID: 1}},
		models:               []Model{{ID: 1, Status: "ACTIVE"}},
		credentialConfigured: true,
		resources:            []Resource{{ID: 20, ProviderID: 8, Version: 1, CredentialConfigured: true, ExpiresAt: &expiredAt}},
	}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil, WithClock(func() time.Time { return now }))
	service.ProviderService.connectionProbe = providerConnectionProbeFunc(func(context.Context, admin.Identity, int64, appsec.RequestMeta) (ConnectionResult, error) {
		t.Fatal("expired credential was tested")
		return ConnectionResult{}, nil
	})
	err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{})
	if !errors.Is(err, ErrProviderConnectionTestFailed) || len(writer.statusChanges) != 0 {
		t.Fatalf("expired credential enabled provider: error=%v statuses=%v", err, writer.statusChanges)
	}
}

func TestSetProviderStatusRequiresActiveModelMappingWhenEnabling(t *testing.T) {
	tests := []struct {
		name     string
		mappings []ProviderMapping
		models   []Model
	}{
		{name: "没有映射"},
		{
			name:     "映射模型已停用",
			mappings: []ProviderMapping{{ID: 10, ProviderID: 8, ModelID: 1}},
			models:   []Model{{ID: 1, Status: "DISABLED"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &providerStatusWriter{
				provider:             Provider{ID: 8, Name: "待配置服务商", Status: "DISABLED"},
				mappings:             test.mappings,
				models:               test.models,
				credentialConfigured: true,
			}
			service := New(providerStatusStore{writer: writer}, nil, nil, nil)

			err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{})
			if !errors.Is(err, ErrProviderModelMappingRequired) {
				t.Fatalf("error=%v; want provider model mapping required", err)
			}
			if len(writer.statusChanges) != 0 || len(writer.audits) != 0 {
				t.Fatalf("rejected enable changed state: statuses=%v audits=%v", writer.statusChanges, writer.audits)
			}
		})
	}
}

func TestSetProviderStatusAllowsDisablingWithoutCredential(t *testing.T) {
	writer := &providerStatusWriter{provider: Provider{ID: 8, Name: "已启用服务商", Status: "ACTIVE"}}
	service := New(providerStatusStore{writer: writer}, nil, nil, nil)

	if err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "DISABLED", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if len(writer.statusChanges) != 1 || writer.statusChanges[0] != "DISABLED" {
		t.Fatalf("provider was not disabled: %v", writer.statusChanges)
	}
}

func TestNormalizeProxyURLMasksPassword(t *testing.T) {
	for _, test := range []struct {
		input       string
		want        string
		wantDisplay string
	}{
		{
			input:       " http://user:password@proxy.example.com:8080/ ",
			want:        "http://user:password@proxy.example.com:8080",
			wantDisplay: "http://******:******@proxy.example.com:8080",
		},
		{
			input:       " socks5://user:password@proxy.example.com:1080/ ",
			want:        "socks5://user:password@proxy.example.com:1080",
			wantDisplay: "socks5://******:******@proxy.example.com:1080",
		},
		{
			input:       "socks5h://user@proxy.example.com:1080",
			want:        "socks5h://user@proxy.example.com:1080",
			wantDisplay: "socks5h://******@proxy.example.com:1080",
		},
	} {
		raw, display, ok := normalizeProxyURL(test.input)
		if !ok || raw != test.want {
			t.Fatalf("unexpected normalized proxy URL: %q", raw)
		}
		if display != test.wantDisplay {
			t.Fatalf("unexpected proxy display: got %q, want %q", display, test.wantDisplay)
		}
	}
	for _, invalid := range []string{
		"ftp://proxy.example.com:21",
		"http://proxy.example.com:8080/path",
		"http://proxy.example.com:8080?token=secret",
	} {
		if _, _, ok := normalizeProxyURL(invalid); ok {
			t.Fatalf("invalid proxy URL accepted: %s", invalid)
		}
	}
}

func TestApplyProviderProxyRejectsSOCKS5Headers(t *testing.T) {
	service := &ProviderService{cipher: providerTestCipher{}}
	for _, scheme := range []string{"socks5", "socks5h"} {
		if _, err := service.applyProviderProxy(Provider{ID: 81}, ProviderInput{
			ProxyEnabled: true,
			ProxyURL:     scheme + "://proxy.example.com:1080",
			ProxyHeaders: []ProviderProxyHeaderInput{{Key: "X-Proxy-Token", Value: "secret"}},
		}); !errors.Is(err, errInvalidProviderProxy) {
			t.Fatalf("%s proxy header was accepted: %v", scheme, err)
		}
	}
}

func TestApplyProviderProxyPreservesMaskedSecrets(t *testing.T) {
	service := &ProviderService{cipher: providerTestCipher{}}
	created, err := service.applyProviderProxy(Provider{ID: 81}, ProviderInput{
		ProxyEnabled: true,
		ProxyURL:     "http://user:password@proxy.example.com:8080",
		ProxyHeaders: []ProviderProxyHeaderInput{{Key: "X-Proxy-Token", Value: "header-secret"}},
	})
	if err != nil || created.ProxyURL == nil || strings.Contains(*created.ProxyURL, "password") {
		t.Fatalf("unexpected created proxy: %+v, %v", created, err)
	}
	updated, err := service.applyProviderProxy(created, ProviderInput{
		ProxyEnabled: true,
		ProxyURL:     "https://******:******@new-proxy.example.com:8443",
		ProxyHeaders: []ProviderProxyHeaderInput{{Key: "X-Proxy-Token", Value: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.ProxyURLSealed.Ciphertext) != "https://user:password@new-proxy.example.com:8443" {
		t.Fatalf("masked URL did not preserve credentials while updating the address: %q", updated.ProxyURLSealed.Ciphertext)
	}
	values := map[string]string{}
	if err := json.Unmarshal(updated.ProxyHeadersSealed.Ciphertext, &values); err != nil || values["X-Proxy-Token"] != "header-secret" {
		t.Fatalf("blank configured header did not preserve its value: %v, %v", values, err)
	}
	if _, err := service.applyProviderProxy(created, ProviderInput{
		ProxyEnabled: true,
		ProxyURL:     *created.ProxyURL,
		ProxyHeaders: []ProviderProxyHeaderInput{{Key: "X-Renamed-Token", Value: ""}},
	}); !errors.Is(err, errInvalidProviderProxy) {
		t.Fatal("renamed header without a value was accepted")
	}
}

func TestApplyProviderProxyUpdatesCredentialsOnlyWhenRequested(t *testing.T) {
	service := &ProviderService{cipher: providerTestCipher{}}
	created, err := service.applyProviderProxy(Provider{ID: 81}, ProviderInput{
		ProxyEnabled: true,
		ProxyURL:     "http://user:password@proxy.example.com:8080",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.applyProviderProxy(created, ProviderInput{
		ProxyEnabled:           true,
		ProxyURL:               "http://new-user:new-password@proxy.example.com:8080",
		UpdateProxyCredentials: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.ProxyURLSealed.Ciphertext) != "http://new-user:new-password@proxy.example.com:8080" {
		t.Fatalf("new proxy credentials were not saved: %q", updated.ProxyURLSealed.Ciphertext)
	}

	preserved, err := service.applyProviderProxy(updated, ProviderInput{
		ProxyEnabled: true,
		ProxyURL:     "http://proxy.example.com:9000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(preserved.ProxyURLSealed.Ciphertext) != "http://new-user:new-password@proxy.example.com:9000" {
		t.Fatalf("omitted proxy credentials were not preserved: %q", preserved.ProxyURLSealed.Ciphertext)
	}
}
