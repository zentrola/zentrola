package management

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

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
		Name:          "阿里云百炼",
		Website:       "  https://www.deepseek.com/  ",
		OpenAIBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/",
	})
	if !ok || got.Website == nil || *got.Website != "https://www.deepseek.com" || got.BaseURL != nil || got.OpenAIBaseURL == nil || *got.OpenAIBaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("unexpected provider: %+v", got)
	}

	for _, input := range []ProviderInput{
		{Name: "没有接口"},
		{Name: "不安全协议", BaseURL: "http://api.example.com"},
		{Name: "包含凭证", BaseURL: "https://key@api.example.com"},
		{Name: "包含查询参数", OpenAIBaseURL: "https://api.example.com/v1?token=secret"},
	} {
		if _, ok := providerFromInput(current, input); ok {
			t.Fatalf("invalid provider accepted: %+v", input)
		}
	}
}

func TestProviderMappingValidation(t *testing.T) {
	anthropicURL := "https://api.example.com/anthropic"
	openAIURL := "https://api.example.com/v1"
	provider := Provider{BaseURL: &anthropicURL, OpenAIBaseURL: &openAIURL}
	valid := []ProviderMappingInput{
		{ModelID: 1, UpstreamModelCode: "vendor-model-pro", ProtocolType: "ANTHROPIC", Status: "ACTIVE"},
		{ModelID: 1, UpstreamModelCode: "vendor-model-pro", ProtocolType: "OPENAI", Status: "DISABLED"},
	}
	if !validProviderMappings(provider, valid) {
		t.Fatal("valid mappings rejected")
	}

	invalid := [][]ProviderMappingInput{
		nil,
		{{ModelID: 0, UpstreamModelCode: "vendor-model", ProtocolType: "ANTHROPIC", Status: "ACTIVE"}},
		{{ModelID: 1, UpstreamModelCode: " vendor-model ", ProtocolType: "ANTHROPIC", Status: "ACTIVE"}},
		{{ModelID: 1, UpstreamModelCode: "vendor-model", ProtocolType: "UNKNOWN", Status: "ACTIVE"}},
		{{ModelID: 1, UpstreamModelCode: "vendor-model", ProtocolType: "ANTHROPIC", Status: "UNKNOWN"}},
		{valid[0], valid[0]},
	}
	for _, mappings := range invalid {
		if validProviderMappings(provider, mappings) {
			t.Fatalf("invalid mappings accepted: %+v", mappings)
		}
	}

	if validProviderMappings(Provider{OpenAIBaseURL: &openAIURL}, valid[:1]) {
		t.Fatal("Anthropic mapping accepted without Anthropic endpoint")
	}
	if validProviderMappings(Provider{BaseURL: &anthropicURL}, valid[1:]) {
		t.Fatal("OpenAI mapping accepted without OpenAI endpoint")
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
