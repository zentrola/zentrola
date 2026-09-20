package bootstrap

import (
	"bytes"
	"testing"
)

func TestOfficialProviderTemplatesRemainAvailableForExplicitInitialization(t *testing.T) {
	if version := OfficialProviderCatalogVersion(); version != "2026-09-20" {
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
		"qwen-official":            {"通义千问", "Qwen"},
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

func TestProviderCatalogRejectsInvalidContentVersion(t *testing.T) {
	invalid := bytes.Replace(
		providerCatalogJSON,
		[]byte(`"catalogVersion": "2026-09-20"`),
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
