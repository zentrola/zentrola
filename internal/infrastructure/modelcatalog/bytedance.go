package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

const byteDanceCatalogURL = "https://ark.cn-beijing.volces.com/api/v3/models"

var errInvalidByteDanceCatalog = errors.New("invalid ByteDance model catalog response")

type byteDanceAdapter struct{}

func (byteDanceAdapter) Name() string { return "bytedance" }

func (byteDanceAdapter) Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error) {
	return catalogRequest{URL: byteDanceCatalogURL, Bearer: true}, nil
}

func (byteDanceAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, "", errInvalidByteDanceCatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Object != "model" || !validModelCode(item.ID) ||
			item.OwnedBy != strings.TrimSpace(item.OwnedBy) {
			return nil, "", errInvalidByteDanceCatalog
		}
		if !isByteDanceStableModel(item.ID, item.OwnedBy) {
			continue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: item.ID,
			Name: byteDanceDisplayName(item.ID),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, "", nil
}

func isByteDanceStableModel(code, owner string) bool {
	lowerCode := strings.ToLower(code)
	if !validModelCode(code) ||
		(!strings.HasPrefix(lowerCode, "doubao-seedance-") &&
			!strings.HasPrefix(lowerCode, "doubao-seedream-")) {
		return false
	}
	// 火山方舟当前的模型目录响应不返回 owned_by。模型编码的
	// doubao-seedance- / doubao-seedream- 前缀用于将同步范围限定为字节跳动的
	// 视频和图片生成模型；如果上游返回了归属字段，则继续按白名单校验。
	if owner != "" {
		switch strings.ToLower(owner) {
		case "bytedance", "byte-dance", "byte_dance", "doubao", "volcengine", "volc_engine":
		default:
			return false
		}
	}

	parts := strings.FieldsFunc(lowerCode, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
	for _, part := range parts {
		switch part {
		case "preview", "test", "testing", "beta", "alpha", "experimental", "exp", "dev", "canary", "rc", "latest", "trial":
			return false
		}
		for _, prefix := range []string{"preview", "test", "beta", "alpha", "experimental", "canary", "trial"} {
			if strings.HasPrefix(part, prefix) {
				return false
			}
		}
		if strings.HasPrefix(part, "rc") && len(part) > 2 {
			allDigits := true
			for _, character := range part[2:] {
				if !unicode.IsDigit(character) {
					allDigits = false
					break
				}
			}
			if allDigits {
				return false
			}
		}
	}
	return true
}

func byteDanceDisplayName(code string) string {
	parts := strings.Split(code, "-")
	for index := range parts {
		runes := []rune(parts[index])
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[index] = string(runes)
		}
	}
	return strings.Join(parts, " ")
}
