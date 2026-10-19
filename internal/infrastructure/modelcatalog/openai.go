package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/openaicodex"
)

var errInvalidOpenAICatalog = errors.New("invalid OpenAI model catalog response")

const (
	openAICatalogURL         = "https://api.openai.com/v1/models"
	codexAutoReviewModelCode = "codex-auto-review"
	// ChatGPT 订阅 Token 缺少 api.model.read 权限，无法读取公共模型目录，只能读取 Codex 客户端目录。
	// 该目录要求 client_version，并按客户端版本返回可用模型；版本号取已验证可用的 Codex 版本。
	codexCatalogURL = "https://chatgpt.com/backend-api/codex/models?client_version=0.162.0"
)

// openAIAdapter 读取 OpenAI 官方模型目录。API Key 使用公共 /v1/models；
// Codex 订阅使用 ChatGPT Codex 目录，并与网关转发保持相同的账号请求头。
type openAIAdapter struct{}

func (openAIAdapter) Name() string { return "openai" }

func (openAIAdapter) Request(source mgmt.ModelDiscoverySource, _ int, _ string) (catalogRequest, error) {
	if source.AuthType == mgmt.AuthTypeSubscription {
		return catalogRequest{
			URL: codexCatalogURL, Bearer: true, AccountIDHeader: "ChatGPT-Account-Id",
			Headers: map[string]string{"Originator": "zentrola"},
		}, nil
	}
	return catalogRequest{URL: openAICatalogURL, Bearer: true}, nil
}

func (openAIAdapter) SupportsSubscription(authAdapter string) bool {
	return authAdapter == openaicodex.AdapterCode
}

// SubscriptionCredential 只使用存储中的 access token，不在目录同步时刷新凭据；
// 临近过期的 Token 交由后台刷新或连接测试更新，避免目录同步与刷新任务争用 refresh token。
func (openAIAdapter) SubscriptionCredential(raw []byte) (subscriptionToken, error) {
	accessToken, accountID, expiresAt, err := openaicodex.RequestCredential(raw)
	if err != nil {
		return subscriptionToken{}, err
	}
	if expiresAt != nil && !expiresAt.After(time.Now().UTC().Add(time.Minute)) {
		return subscriptionToken{}, errSubscriptionExpired
	}
	return subscriptionToken{Token: []byte(accessToken), AccountID: accountID}, nil
}

func (openAIAdapter) Decode(source mgmt.ModelDiscoverySource, data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	if source.AuthType == mgmt.AuthTypeSubscription {
		return decodeCodexCatalog(data)
	}
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
	return withCodexAutoReview(models, seen), "", nil
}

// decodeCodexCatalog 解析 Codex 订阅目录。visibility 为 hide 的是保留或内部模型，Codex 不在模型选择器中展示，
// 同步时同样跳过，只保留网关必须转发的自动审批模型。名称与 API Key 目录使用同一规则生成，
// 避免两类凭据交替同步时反复改写模型名称。
func decodeCodexCatalog(data []byte) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Models []struct {
			Slug            string   `json:"slug"`
			Visibility      string   `json:"visibility"`
			InputModalities []string `json:"input_modalities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Models == nil {
		return nil, "", errInvalidOpenAICatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))
	for _, item := range payload.Models {
		if !validModelCode(item.Slug) {
			return nil, "", errInvalidOpenAICatalog
		}
		if item.Visibility != "list" && item.Slug != codexAutoReviewModelCode {
			continue
		}
		if _, duplicate := seen[item.Slug]; duplicate {
			continue
		}
		seen[item.Slug] = struct{}{}
		model := mgmt.DiscoveredModel{Code: item.Slug, Name: openAIDisplayName(item.Slug)}
		if inputModalities := codexModalities(item.InputModalities); inputModalities != nil {
			model.InputModalities, model.OutputModalities = inputModalities, []string{"TEXT"}
		}
		models = append(models, model)
	}
	return withCodexAutoReview(models, seen), "", nil
}

// codexModalities 把 Codex 目录的小写模态转换为平台模态；出现未知值时返回 nil，由同步流程回退为纯文本。
func codexModalities(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		switch value {
		case "text":
			result = append(result, "TEXT")
		case "image":
			result = append(result, "IMAGE")
		default:
			return nil
		}
	}
	if !catalog.ValidModalities(result) {
		return nil
	}
	return result
}

// Codex 的自动审批使用内部模型编码，OpenAI 公共模型目录不会返回它。
// 同步 OpenAI 目录时必须补齐该逻辑模型及映射，确保自定义网关可以转发审核请求。
func withCodexAutoReview(models []mgmt.DiscoveredModel, seen map[string]struct{}) []mgmt.DiscoveredModel {
	if _, exists := seen[codexAutoReviewModelCode]; !exists {
		models = append(models, mgmt.DiscoveredModel{
			Code: codexAutoReviewModelCode,
			Name: "Codex Auto Review",
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models
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
