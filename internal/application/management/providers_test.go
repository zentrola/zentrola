package management

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type mappingWriter struct {
	Writer
	mappings []ProviderMapping
	updated  []ProviderMapping
	deleted  []int64
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
		Name:    "阿里云百炼",
		Website: "  https://www.deepseek.com/  ",
		Endpoints: []ProviderEndpoint{{
			ProtocolType: "OPENAI_CHAT",
			BaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1/",
		}},
	})
	if !ok || got.Website == nil || *got.Website != "https://www.deepseek.com" || len(got.Endpoints) != 1 || got.Endpoints[0].BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("unexpected provider: %+v", got)
	}

	for _, input := range []ProviderInput{
		{Name: "没有接口"},
		{Name: "不安全协议", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI_CHAT", BaseURL: "http://api.example.com"}}},
		{Name: "包含凭证", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI_CHAT", BaseURL: "https://key@api.example.com"}}},
		{Name: "包含查询参数", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI_CHAT", BaseURL: "https://api.example.com/v1?token=secret"}}},
		{Name: "未知协议", Endpoints: []ProviderEndpoint{{ProtocolType: "GEMINI_NATIVE", BaseURL: "https://api.example.com"}}},
		{Name: "重复协议", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI_CHAT", BaseURL: "https://one.example.com"}, {ProtocolType: "OPENAI_CHAT", BaseURL: "https://two.example.com"}}},
	} {
		if _, ok := providerFromInput(current, input); ok {
			t.Fatalf("invalid provider accepted: %+v", input)
		}
	}
}

func TestProviderMappingValidation(t *testing.T) {
	provider := Provider{}
	valid := []ProviderMappingInput{
		{ModelID: 1, UpstreamModelCode: "vendor-model-pro"},
		{ModelID: 2, UpstreamModelCode: "vendor-model-flash", Priority: 200},
	}
	if !validProviderMappings(provider, valid) {
		t.Fatal("valid mappings rejected")
	}

	invalid := [][]ProviderMappingInput{
		nil,
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

func TestReplaceProviderMappingsLogicallyDeletesUncheckedModels(t *testing.T) {
	writer := &mappingWriter{mappings: []ProviderMapping{
		{ID: 10, ProviderID: 8, ModelID: 1, UpstreamModelCode: "old-one", Priority: 100},
		{ID: 11, ProviderID: 8, ModelID: 2, UpstreamModelCode: "old-two", Priority: 100},
	}}
	service := &Service{}
	result, err := service.replaceProviderMappings(context.Background(), writer, Provider{ID: 8}, []ProviderMappingInput{{
		ModelID: 1, UpstreamModelCode: "new-one",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].ID != 10 || result[0].Priority != defaultProviderMappingPriority {
		t.Fatalf("unexpected retained mappings: %+v", result)
	}
	if len(writer.updated) != 1 || writer.updated[0].UpstreamModelCode != "new-one" {
		t.Fatalf("checked mapping was not updated: %+v", writer.updated)
	}
	if len(writer.deleted) != 1 || writer.deleted[0] != 11 {
		t.Fatalf("unchecked mapping was not logically deleted: %+v", writer.deleted)
	}
}

func TestNormalizeProxyURLMasksPassword(t *testing.T) {
	raw, display, ok := normalizeProxyURL(" http://user:password@proxy.example.com:8080/ ")
	if !ok || raw != "http://user:password@proxy.example.com:8080" {
		t.Fatalf("unexpected normalized proxy URL: %q", raw)
	}
	if strings.Contains(display, "password") || strings.Contains(display, "user") || !strings.Contains(display, "proxy.example.com:8080") {
		t.Fatalf("proxy display was not masked: %q", display)
	}
	for _, invalid := range []string{
		"socks5://proxy.example.com:1080",
		"http://proxy.example.com:8080/path",
		"http://proxy.example.com:8080?token=secret",
	} {
		if _, _, ok := normalizeProxyURL(invalid); ok {
			t.Fatalf("invalid proxy URL accepted: %s", invalid)
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
