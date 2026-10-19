package modelcatalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

var codexSubscriptionSource = mgmt.ModelDiscoverySource{
	ProviderCode: catalog.OpenAIOfficialCode,
	AuthType:     mgmt.AuthTypeSubscription, AuthAdapter: mgmt.AuthAdapterOpenAICodex,
}

func codexAccessToken(expiresAt time.Time) string {
	payload := `{"exp":` + strconv.FormatInt(expiresAt.Unix(), 10) + `}`
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
}

func codexAuthFile(accessToken, accountID string) []byte {
	return []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id","access_token":"` + accessToken +
		`","refresh_token":"refresh-secret","account_id":"` + accountID + `"}}`)
}

func TestOpenAICatalogRequestByCredentialType(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source mgmt.ModelDiscoverySource
		want   catalogRequest
	}{
		{"API Key", mgmt.ModelDiscoverySource{ProviderCode: catalog.OpenAIOfficialCode, AuthType: mgmt.AuthTypeAPIKey},
			catalogRequest{URL: openAICatalogURL, Bearer: true}},
		{"legacy empty auth type", mgmt.ModelDiscoverySource{ProviderCode: catalog.OpenAIOfficialCode},
			catalogRequest{URL: openAICatalogURL, Bearer: true}},
		{"Codex subscription", codexSubscriptionSource, catalogRequest{
			URL: codexCatalogURL, Bearer: true, AccountIDHeader: "ChatGPT-Account-Id",
			Headers: map[string]string{"Originator": "zentrola"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (openAIAdapter{}).Request(tt.source, 1, "")
			if err != nil || got.URL != tt.want.URL || got.Bearer != tt.want.Bearer || got.APIKeyHeader != "" ||
				got.AccountIDHeader != tt.want.AccountIDHeader || len(got.Headers) != len(tt.want.Headers) ||
				got.Headers["Originator"] != tt.want.Headers["Originator"] {
				t.Fatalf("Request() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestDiscoverOpenAIModelsWithCodexSubscription(t *testing.T) {
	var logs bytes.Buffer
	accessToken := codexAccessToken(time.Now().UTC().Add(time.Hour))
	discoverer := NewDiscoverer(slog.New(slog.NewJSONHandler(&logs, nil)))
	discoverer.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != codexCatalogURL ||
			request.Header.Get("Authorization") != "Bearer "+accessToken ||
			request.Header.Get("ChatGPT-Account-Id") != "account-1" || request.Header.Get("Originator") != "zentrola" ||
			request.Header.Get("x-api-key") != "" {
			t.Fatalf("invalid Codex catalog request: %s %s %+v", request.Method, request.URL.String(), request.Header)
		}
		body := `{"models":[` +
			`{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","visibility":"list","input_modalities":["text","image"]},` +
			`{"slug":"gpt-6-luna","display_name":"GPT-6-Luna","visibility":"list","input_modalities":["text"]},` +
			`{"slug":"gpt-reserve","display_name":"GPT-Reserve","visibility":"hide","input_modalities":["text","image"]},` +
			`{"slug":"codex-auto-review","display_name":"Codex Auto Review","visibility":"hide","input_modalities":["text","image"]},` +
			`{"slug":"gpt-6.1-sol","display_name":"Duplicate","visibility":"list"}` +
			`]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})

	models, result := discoverer.Discover(context.Background(), codexSubscriptionSource, codexAuthFile(accessToken, "account-1"), nil)
	if !result.OK || result.Code != "OK" || len(models) != 3 ||
		models[0].Code != "codex-auto-review" || models[0].Name != "Codex Auto Review" ||
		models[1].Code != "gpt-6-luna" || models[1].Name != "GPT-6-luna" ||
		!slices.Equal(models[1].InputModalities, []string{"TEXT"}) ||
		models[2].Code != "gpt-6.1-sol" || models[2].Name != "GPT-6.1-sol" ||
		!slices.Equal(models[2].InputModalities, []string{"TEXT", "IMAGE"}) ||
		!slices.Equal(models[2].OutputModalities, []string{"TEXT"}) {
		t.Fatalf("unexpected Codex subscription discovery: %+v %+v", models, result)
	}
	output := logs.String()
	if !strings.Contains(output, `"catalog_adapter":"openai"`) || strings.Contains(output, accessToken) ||
		strings.Contains(output, "refresh-secret") || strings.Contains(output, "account-1") {
		t.Fatalf("unexpected Codex catalog log: %s", output)
	}
}

func TestDiscoverCodexSubscriptionRejectsCredentialWithoutRequest(t *testing.T) {
	fresh := codexAccessToken(time.Now().UTC().Add(time.Hour))
	for _, tt := range []struct {
		name       string
		credential []byte
		code       string
	}{
		{"expired access token", codexAuthFile(codexAccessToken(time.Now().UTC().Add(-time.Hour)), "account-1"), "SUBSCRIPTION_CREDENTIAL_EXPIRED"},
		{"access token expiring within a minute", codexAuthFile(codexAccessToken(time.Now().UTC().Add(30*time.Second)), "account-1"), "SUBSCRIPTION_CREDENTIAL_EXPIRED"},
		{"missing account", codexAuthFile(fresh, ""), "CREDENTIAL_INVALID"},
		{"unsafe account header", codexAuthFile(fresh, "account 1"), "CREDENTIAL_INVALID"},
		{"API key auth mode", []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test"}`), "CREDENTIAL_INVALID"},
		{"malformed auth file", []byte(`{`), "CREDENTIAL_INVALID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			discoverer := NewDiscoverer(nil)
			discoverer.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("rejected Codex subscription reached transport")
				return nil, nil
			})
			models, result := discoverer.Discover(context.Background(), codexSubscriptionSource, tt.credential, nil)
			if result.OK || result.Code != tt.code || models != nil {
				t.Fatalf("Discover() = %+v %+v, want code %s", models, result, tt.code)
			}
		})
	}
}

func TestDecodeCodexCatalog(t *testing.T) {
	for _, tt := range []struct {
		name  string
		body  string
		codes []string
		// inputs 按 codes 顺序给出期望的输入模态，nil 表示交由同步流程回退为纯文本。
		inputs [][]string
	}{
		{"empty catalog keeps auto review", `{"models":[]}`,
			[]string{"codex-auto-review"}, [][]string{nil}},
		{"listed model keeps modalities", `{"models":[{"slug":"gpt-6-sol","visibility":"list","input_modalities":["text","image"]}]}`,
			[]string{"codex-auto-review", "gpt-6-sol"}, [][]string{nil, {"TEXT", "IMAGE"}}},
		{"hidden and unknown visibility are skipped", `{"models":[{"slug":"gpt-reserve","visibility":"hide"},{"slug":"gpt-legacy","visibility":"none"},{"slug":"gpt-blank"}]}`,
			[]string{"codex-auto-review"}, [][]string{nil}},
		{"hidden auto review is kept once", `{"models":[{"slug":"codex-auto-review","visibility":"hide","input_modalities":["text"]}]}`,
			[]string{"codex-auto-review"}, [][]string{{"TEXT"}}},
		{"unknown modality falls back", `{"models":[{"slug":"gpt-6-sol","visibility":"list","input_modalities":["text","audio_stream"]}]}`,
			[]string{"codex-auto-review", "gpt-6-sol"}, [][]string{nil, nil}},
		{"duplicate modality falls back", `{"models":[{"slug":"gpt-6-sol","visibility":"list","input_modalities":["text","text"]}]}`,
			[]string{"codex-auto-review", "gpt-6-sol"}, [][]string{nil, nil}},
		{"missing modalities fall back", `{"models":[{"slug":"gpt-6-sol","visibility":"list"}]}`,
			[]string{"codex-auto-review", "gpt-6-sol"}, [][]string{nil, nil}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			models, cursor, err := (openAIAdapter{}).Decode(codexSubscriptionSource, []byte(tt.body), 1)
			if err != nil || cursor != "" || len(models) != len(tt.codes) {
				t.Fatalf("Decode() = %+v, %q, %v", models, cursor, err)
			}
			for i, model := range models {
				if model.Code != tt.codes[i] || !slices.Equal(model.InputModalities, tt.inputs[i]) ||
					(tt.inputs[i] == nil) != (model.OutputModalities == nil) {
					t.Fatalf("model %d = %+v, want code %s input %v", i, model, tt.codes[i], tt.inputs[i])
				}
			}
		})
	}
}

func TestDecodeCodexCatalogRejectsInvalidResponse(t *testing.T) {
	for _, body := range []string{
		`{`,
		`{}`,
		`{"models":null}`,
		`{"object":"list","data":[{"id":"gpt-6-sol","object":"model","owned_by":"openai"}]}`,
		`{"models":[{"slug":"","visibility":"list"}]}`,
		`{"models":[{"slug":"bad model\n","visibility":"hide"}]}`,
	} {
		if models, _, err := (openAIAdapter{}).Decode(codexSubscriptionSource, []byte(body), 1); err == nil {
			t.Fatalf("invalid Codex catalog accepted: %q %+v", body, models)
		}
	}
}

func TestOpenAICatalogNamesMatchAcrossCredentialTypes(t *testing.T) {
	apiModels, _, err := (openAIAdapter{}).Decode(mgmt.ModelDiscoverySource{ProviderCode: catalog.OpenAIOfficialCode},
		[]byte(`{"object":"list","data":[{"id":"gpt-6.1-sol","object":"model","owned_by":"openai"}]}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionModels, _, err := (openAIAdapter{}).Decode(codexSubscriptionSource,
		[]byte(`{"models":[{"slug":"gpt-6.1-sol","display_name":"GPT-6.1-Sol","visibility":"list"}]}`), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(apiModels) != len(subscriptionModels) {
		t.Fatalf("catalog sizes differ: %+v %+v", apiModels, subscriptionModels)
	}
	for i := range apiModels {
		if apiModels[i].Code != subscriptionModels[i].Code || apiModels[i].Name != subscriptionModels[i].Name {
			t.Fatalf("model names differ between credential types: %+v %+v", apiModels[i], subscriptionModels[i])
		}
	}
}
