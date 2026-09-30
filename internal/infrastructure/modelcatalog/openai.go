package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

var errInvalidOpenAICatalog = errors.New("invalid OpenAI model catalog response")

const codexAutoReviewModelCode = "codex-auto-review"

type openAIAdapter struct{}

func (openAIAdapter) Name() string { return "openai" }

func (openAIAdapter) Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error) {
	return catalogRequest{URL: "https://api.openai.com/v1/models", Bearer: true}, nil
}

func (openAIAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, "", errInvalidOpenAICatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Object != "model" || !validModelCode(item.ID) || item.OwnedBy == "" || item.OwnedBy != strings.TrimSpace(item.OwnedBy) {
			return nil, "", errInvalidOpenAICatalog
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: item.ID,
			Name: openAIDisplayName(item.ID),
		})
	}
	// Codex 的自动审批使用内部模型编码，OpenAI 公共模型目录不会返回它。
	// 同步 OpenAI 目录时必须补齐该逻辑模型及映射，确保自定义网关可以转发审核请求。
	if _, exists := seen[codexAutoReviewModelCode]; !exists {
		models = append(models, mgmt.DiscoveredModel{
			Code: codexAutoReviewModelCode,
			Name: "Codex Auto Review",
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, "", nil
}

func openAIDisplayName(code string) string {
	switch {
	case code == codexAutoReviewModelCode:
		return "Codex Auto Review"
	case strings.HasPrefix(code, "gpt-"):
		return "GPT-" + code[len("gpt-"):]
	case strings.HasPrefix(code, "chatgpt-"):
		return "ChatGPT-" + code[len("chatgpt-"):]
	case strings.HasPrefix(code, "tts-"):
		return "TTS-" + code[len("tts-"):]
	case strings.HasPrefix(code, "dall-e-"):
		return "DALL-E " + code[len("dall-e-"):]
	default:
		return deepSeekDisplayName(code)
	}
}
