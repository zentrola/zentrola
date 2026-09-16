package bootstrap

import (
	"bytes"
	"reflect"
	"testing"
)

func TestOfficialProviderTemplatesRemainAvailableForExplicitInitialization(t *testing.T) {
	if version := OfficialProviderCatalogVersion(); version != "2026-09-16" {
		t.Fatalf("unexpected provider catalog version: %q", version)
	}
	providers := OfficialProviderTemplates()
	if len(providers) != 13 {
		t.Fatalf("unexpected provider template count: %d", len(providers))
	}
	want := map[string][]Endpoint{
		"openai-official":          {{ProtocolType: "OPENAI", BaseURL: "https://api.openai.com/v1"}},
		"anthropic-official":       {{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"}},
		"google-gemini-official":   {{ProtocolType: "OPENAI", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"}},
		"deepseek-official":        {{ProtocolType: "OPENAI", BaseURL: "https://api.deepseek.com"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://api.deepseek.com/anthropic"}},
		"zhipu-official":           {{ProtocolType: "OPENAI", BaseURL: "https://open.bigmodel.cn/api/paas/v4"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://open.bigmodel.cn/api/anthropic"}},
		"kimi-official":            {{ProtocolType: "OPENAI", BaseURL: "https://api.moonshot.cn/v1"}},
		"qwen-official":            {{ProtocolType: "OPENAI", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://dashscope.aliyuncs.com/apps/anthropic"}},
		"xai-official":             {{ProtocolType: "OPENAI", BaseURL: "https://api.x.ai/v1"}},
		"mistral-official":         {{ProtocolType: "OPENAI", BaseURL: "https://api.mistral.ai/v1"}},
		"minimax-official":         {{ProtocolType: "OPENAI", BaseURL: "https://api.minimax.cn/v1"}, {ProtocolType: "ANTHROPIC", BaseURL: "https://api.minimax.cn/anthropic"}},
		"doubao-official":          {{ProtocolType: "OPENAI", BaseURL: "https://ark.cn-beijing.volces.com/api/v3"}},
		"baidu-qianfan-official":   {{ProtocolType: "OPENAI", BaseURL: "https://qianfan.baidubce.com/v2"}},
		"tencent-hunyuan-official": {{ProtocolType: "OPENAI", BaseURL: "https://api.hunyuan.cloud.tencent.com/v1"}},
	}
	wantNames := map[string][2]string{
		"openai-official":          {"OpenAI", "OpenAI"},
		"anthropic-official":       {"Anthropic", "Anthropic"},
		"google-gemini-official":   {"Google", "Google"},
		"deepseek-official":        {"深度求索", "DeepSeek"},
		"zhipu-official":           {"智谱 AI", "Zhipu AI"},
		"kimi-official":            {"月之暗面", "Moonshot AI"},
		"qwen-official":            {"阿里云百炼", "Alibaba Cloud"},
		"xai-official":             {"xAI", "xAI"},
		"mistral-official":         {"Mistral AI", "Mistral AI"},
		"minimax-official":         {"MiniMax", "MiniMax"},
		"doubao-official":          {"字节跳动", "ByteDance"},
		"baidu-qianfan-official":   {"百度", "Baidu"},
		"tencent-hunyuan-official": {"腾讯", "Tencent"},
	}
	for _, provider := range providers {
		protocols, ok := want[provider.Code]
		names := wantNames[provider.Code]
		if !ok || provider.LocalizedName("zh-CN") != names[0] || provider.LocalizedName("en-US") != names[1] || provider.Website == "" || len(provider.Endpoints) != len(protocols) || len(provider.Models) == 0 {
			t.Fatalf("unexpected provider: %+v", provider)
		}
		for index, endpoint := range protocols {
			if provider.Endpoints[index] != endpoint {
				t.Fatalf("unexpected endpoint: %+v", provider.Endpoints[index])
			}
		}
	}
}

func TestOfficialProviderModelsAreLoadedFromCatalogAndCloned(t *testing.T) {
	models := OfficialProviderModels("deepseek-official")
	if len(models) != 2 || models[0].Code != "deepseek-flash" || models[0].ModelType != "CHAT" ||
		!reflect.DeepEqual(models[0].InputModalities, []string{"TEXT", "IMAGE"}) {
		t.Fatalf("unexpected built-in models: %+v", models)
	}
	models[0].InputModalities[0] = "AUDIO"
	models[0].Code = "changed"
	again := OfficialProviderModels("deepseek-official")
	if again[0].Code != "deepseek-flash" || again[0].InputModalities[0] != "TEXT" {
		t.Fatalf("built-in model catalog was mutated: %+v", again)
	}
	if models := OfficialProviderModels("missing-provider"); models != nil {
		t.Fatalf("unknown provider returned models: %+v", models)
	}
	openAIModels := OfficialProviderModels("openai-official")
	if len(openAIModels) != 6 || openAIModels[0].Code != "gpt-6-astra" ||
		!reflect.DeepEqual(openAIModels[0].InputModalities, []string{"TEXT", "IMAGE"}) ||
		!reflect.DeepEqual(openAIModels[0].OutputModalities, []string{"TEXT"}) ||
		openAIModels[4].Code != "gpt-image-2.5-sunburst" ||
		!reflect.DeepEqual(openAIModels[4].InputModalities, []string{"TEXT", "IMAGE"}) ||
		!reflect.DeepEqual(openAIModels[4].OutputModalities, []string{"IMAGE"}) {
		t.Fatalf("unexpected OpenAI models copied from the curated catalog: %+v", openAIModels)
	}
}

func TestProviderCatalogRejectsInvalidContentVersion(t *testing.T) {
	invalid := bytes.Replace(
		providerCatalogJSON,
		[]byte(`"catalogVersion": "2026-09-16"`),
		[]byte(`"catalogVersion": "2026-02-30"`),
		1,
	)
	defer func() {
		if recover() == nil {
			t.Fatal("invalid catalog version was accepted")
		}
	}()
	_, _ = mustLoadProviderCatalog(invalid)
}
