package modelcatalog

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropicclaude"
)

const (
	anthropicCatalogURL      = "https://api.anthropic.com/v1/models"
	anthropicCatalogPageSize = "1000"
	anthropicAPIVersion      = "2023-06-01"
)

var errInvalidAnthropicCatalog = errors.New("invalid Anthropic model catalog response")

// anthropicAdapter 读取 Anthropic 官方模型目录。API Key 使用 x-api-key；
// Claude Code 订阅 Token 使用 OAuth Bearer，目录接口不要求附带 Claude Code 身份。
type anthropicAdapter struct{}

func (anthropicAdapter) Name() string { return "anthropic" }

func (anthropicAdapter) Request(source mgmt.ModelDiscoverySource, page int, afterID string) (catalogRequest, error) {
	if page <= 0 || page > maxCatalogPages || (page == 1) != (afterID == "") || (afterID != "" && !validModelCode(afterID)) {
		return catalogRequest{}, errInvalidAnthropicCatalog
	}
	endpoint, err := url.Parse(anthropicCatalogURL)
	if err != nil {
		return catalogRequest{}, errInvalidAnthropicCatalog
	}
	query := endpoint.Query()
	query.Set("limit", anthropicCatalogPageSize)
	if afterID != "" {
		query.Set("after_id", afterID)
	}
	endpoint.RawQuery = query.Encode()
	request := catalogRequest{URL: endpoint.String(), Headers: map[string]string{"anthropic-version": anthropicAPIVersion}}
	if source.AuthType == mgmt.AuthTypeSubscription {
		request.Bearer = true
		request.Headers["anthropic-beta"] = anthropicclaude.OAuthBeta
	} else {
		request.APIKeyHeader = "x-api-key"
	}
	return request, nil
}

func (anthropicAdapter) SupportsSubscription(authAdapter string) bool {
	return authAdapter == anthropicclaude.AdapterCode
}

func (anthropicAdapter) SubscriptionCredential(raw []byte) ([]byte, error) {
	token, err := anthropicclaude.RequestCredential(raw)
	if err != nil {
		return nil, err
	}
	return []byte(token), nil
}

func (anthropicAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Data []struct {
			Type         string  `json:"type"`
			ID           string  `json:"id"`
			DisplayName  string  `json:"display_name"`
			Lifecycle    string  `json:"lifecycle"`
			DeprecatedAt *string `json:"deprecated_at"`
			Capabilities struct {
				ImageInput struct {
					Supported bool `json:"supported"`
				} `json:"image_input"`
			} `json:"capabilities"`
		} `json:"data"`
		HasMore *bool  `json:"has_more"`
		LastID  string `json:"last_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Data == nil || payload.HasMore == nil {
		return nil, "", errInvalidAnthropicCatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Type != "model" || !validModelCode(item.ID) {
			return nil, "", errInvalidAnthropicCatalog
		}
		// 已宣布弃用的模型仍可调用到退役日，但不再作为新模型同步进目录。
		if item.DeprecatedAt != nil || item.Lifecycle == "deprecated" || item.Lifecycle == "retired" {
			continue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		inputModalities := []string{"TEXT"}
		if item.Capabilities.ImageInput.Supported {
			inputModalities = append(inputModalities, "IMAGE")
		}
		models = append(models, mgmt.DiscoveredModel{
			Code:             item.ID,
			Name:             anthropicDisplayName(item.ID, item.DisplayName),
			InputModalities:  inputModalities,
			OutputModalities: []string{"TEXT"},
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	if !*payload.HasMore {
		return models, "", nil
	}
	if !validModelCode(payload.LastID) {
		return nil, "", errInvalidAnthropicCatalog
	}
	return models, payload.LastID, nil
}

func anthropicDisplayName(code, displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" && len(displayName) <= 128 && utf8.ValidString(displayName) && !strings.ContainsRune(displayName, 0) {
		return displayName
	}
	return deepSeekDisplayName(code)
}
