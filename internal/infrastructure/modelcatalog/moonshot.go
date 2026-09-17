package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

var errInvalidMoonshotCatalog = errors.New("invalid Moonshot model catalog response")

type moonshotAdapter struct{}

func (moonshotAdapter) Name() string { return "moonshot" }

func (moonshotAdapter) Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error) {
	return catalogRequest{URL: "https://api.moonshot.cn/v1/models", Bearer: true}, nil
}

func (moonshotAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, "", errInvalidMoonshotCatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Object != "model" || item.OwnedBy != "moonshot" || !validModelCode(item.ID) {
			return nil, "", errInvalidMoonshotCatalog
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: item.ID,
			Name: moonshotDisplayName(item.ID),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, "", nil
}

func moonshotDisplayName(code string) string {
	switch {
	case strings.HasPrefix(code, "kimi-"):
		return "Kimi " + capitalizeMoonshotSuffix(code[len("kimi-"):])
	case strings.HasPrefix(code, "moonshot-"):
		return "Moonshot " + capitalizeMoonshotSuffix(code[len("moonshot-"):])
	default:
		return deepSeekDisplayName(code)
	}
}

func capitalizeMoonshotSuffix(suffix string) string {
	runes := []rune(strings.ReplaceAll(suffix, "-", " "))
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}
