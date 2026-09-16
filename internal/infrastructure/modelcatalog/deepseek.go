package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

var errInvalidDeepSeekCatalog = errors.New("invalid DeepSeek model catalog response")

type deepSeekAdapter struct{}

func (deepSeekAdapter) Name() string { return "deepseek" }

func (deepSeekAdapter) Request(mgmt.ModelDiscoverySource, int) (catalogRequest, error) {
	return catalogRequest{URL: "https://api.deepseek.com/models", Bearer: true}, nil
}

func (deepSeekAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, bool, error) {
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, false, errInvalidDeepSeekCatalog
	}
	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Object != "model" || item.OwnedBy != "deepseek" || !validModelCode(item.ID) {
			return nil, false, errInvalidDeepSeekCatalog
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: item.ID,
			Name: deepSeekDisplayName(item.ID),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, false, nil
}

func deepSeekDisplayName(code string) string {
	name := strings.ReplaceAll(code, "-", " ")
	runes := []rune(name)
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

func validModelCode(code string) bool {
	return code != "" && code == strings.TrimSpace(code) && len(code) <= 128 && utf8.ValidString(code) && !strings.ContainsRune(code, 0)
}
