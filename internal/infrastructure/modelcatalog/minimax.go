package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

const miniMaxCatalogURL = "https://api.minimax.cn/v1/models"

var errInvalidMiniMaxCatalog = errors.New("invalid MiniMax model catalog response")

type miniMaxAdapter struct{}

func (miniMaxAdapter) Name() string { return "minimax" }

func (miniMaxAdapter) Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error) {
	return catalogRequest{URL: miniMaxCatalogURL, Bearer: true}, nil
}

func (miniMaxAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, "", errInvalidMiniMaxCatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Object != "model" || !validModelCode(item.ID) ||
			item.OwnedBy == "" || item.OwnedBy != strings.TrimSpace(item.OwnedBy) {
			return nil, "", errInvalidMiniMaxCatalog
		}
		if !isMiniMaxStableModel(item.ID) {
			continue
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: item.ID,
			Name: miniMaxDisplayName(item.ID),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, "", nil
}

func isMiniMaxStableModel(code string) bool {
	if !validModelCode(code) {
		return false
	}
	parts := strings.FieldsFunc(strings.ToLower(code), func(character rune) bool {
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

func miniMaxDisplayName(code string) string {
	if len(code) >= len("minimax-") && strings.EqualFold(code[:len("minimax-")], "minimax-") {
		return "MiniMax " + strings.ReplaceAll(code[len("minimax-"):], "-", " ")
	}
	return deepSeekDisplayName(code)
}
