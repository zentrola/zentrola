package bootstrap

import (
	"context"
	"errors"
	"testing"
)

type sequentialIDs struct {
	next int64
	fail int64
}

func (g *sequentialIDs) NextID(context.Context) (int64, error) {
	g.next++
	if g.next == g.fail {
		return 0, errors.New("id failed")
	}
	return g.next, nil
}

type captureStore struct{ seed Seed }

func (s *captureStore) InitializeOnce(_ context.Context, seed Seed) error { s.seed = seed; return nil }
func (*captureStore) Initialized(context.Context) (bool, error)           { return true, nil }

func TestInitializeSeedsOnlyOrganization(t *testing.T) {
	store := &captureStore{}
	if err := New(store, &sequentialIDs{}).Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.seed.OrganizationID != 1 || store.seed.CreatedAt.IsZero() {
		t.Fatalf("unexpected seed: %+v", store.seed)
	}
}

func TestOfficialProviderTemplatesRemainAvailableForExplicitInitialization(t *testing.T) {
	providers := OfficialProviderTemplates()
	if len(providers) != 7 {
		t.Fatalf("unexpected provider template count: %d", len(providers))
	}
	want := map[string][]Endpoint{
		"openai-official":        {{ProtocolType: "OPENAI", BaseURL: "https://api.openai.com/v1"}},
		"anthropic-official":     {{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"}},
		"google-gemini-official": {{ProtocolType: "OPENAI", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"}},
		"deepseek-official":      {{ProtocolType: "OPENAI", BaseURL: "https://api.deepseek.com"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://api.deepseek.com/anthropic"}},
		"zhipu-official":         {{ProtocolType: "OPENAI", BaseURL: "https://open.bigmodel.cn/api/paas/v4"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://open.bigmodel.cn/api/anthropic"}},
		"kimi-official":          {{ProtocolType: "OPENAI", BaseURL: "https://api.moonshot.cn/v1"}},
		"qwen-official":          {{ProtocolType: "OPENAI", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://dashscope.aliyuncs.com/apps/anthropic"}},
	}
	wantNames := map[string][2]string{
		"openai-official":        {"OpenAI", "OpenAI"},
		"anthropic-official":     {"Anthropic", "Anthropic"},
		"google-gemini-official": {"Google", "Google"},
		"deepseek-official":      {"深度求索", "DeepSeek"},
		"zhipu-official":         {"智谱 AI", "Zhipu AI"},
		"kimi-official":          {"月之暗面", "Moonshot AI"},
		"qwen-official":          {"阿里云", "Alibaba Cloud"},
	}
	for _, provider := range providers {
		protocols, ok := want[provider.Code]
		names := wantNames[provider.Code]
		if !ok || provider.LocalizedName("zh-CN") != names[0] || provider.LocalizedName("en-US") != names[1] || provider.Website == "" || len(provider.Endpoints) != len(protocols) {
			t.Fatalf("unexpected provider: %+v", provider)
		}
		for index, endpoint := range protocols {
			if provider.Endpoints[index] != endpoint {
				t.Fatalf("unexpected endpoint: %+v", provider.Endpoints[index])
			}
		}
	}
}

func TestInitializeStopsWhenOrganizationIDGenerationFails(t *testing.T) {
	store := &captureStore{}
	if err := New(store, &sequentialIDs{fail: 1}).Initialize(context.Background()); err == nil {
		t.Fatal("ID failure was ignored")
	}
	if store.seed.OrganizationID != 0 {
		t.Fatal("partial seed reached store")
	}
}
