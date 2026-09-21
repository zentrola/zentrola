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
	updated  []ProviderMapping
	deleted  []int64
}

type providerStatusWriter struct {
	Writer
	provider             Provider
	mappings             []ProviderMapping
	models               []Model
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
func (w *providerStatusWriter) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return append([]ProviderMapping(nil), w.mappings...), nil
}
func (w *providerStatusWriter) Models(context.Context, Page, string) ([]Model, error) {
	return append([]Model(nil), w.models...), nil
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

func (s providerStatusStore) Read(context.Context, admin.Identity, func(Reader) error) error {
	return nil
}
func (s providerStatusStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

func (w *mappingWriter) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return append([]ProviderMapping(nil), w.mappings...), nil
}
func (w *mappingWriter) Model(_ context.Context, id int64) (Model, error) {
	return Model{ID: id}, nil
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
	service := &Service{}
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
	service := &Service{}
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
	if err := service.SetProviderStatus(context.Background(), admin.Identity{}, 8, "ACTIVE", appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if len(writer.statusChanges) != 1 || writer.statusChanges[0] != "ACTIVE" || len(writer.audits) != 1 {
		t.Fatalf("configured provider was not enabled and audited: statuses=%v audits=%v", writer.statusChanges, writer.audits)
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
	service := &Service{cipher: providerTestCipher{}}
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
	service := &Service{cipher: providerTestCipher{}}
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
		ProxyURL:     *created.ProxyURL,
		ProxyHeaders: []ProviderProxyHeaderInput{{Key: "X-Proxy-Token", Value: ""}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.ProxyURLSealed.Ciphertext) != "http://user:password@proxy.example.com:8080" {
		t.Fatal("masked URL did not preserve the encrypted value")
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
