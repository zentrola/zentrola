package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

var errInvalidZhipuCatalog = errors.New("invalid Zhipu model catalog response")

type zhipuAdapter struct{}

func (zhipuAdapter) Name() string { return "zhipu" }

func (zhipuAdapter) Request() catalogRequest {
	return catalogRequest{URL: "https://open.bigmodel.cn/api/paas/v4/models", Bearer: true}
}

func (zhipuAdapter) Decode(data []byte) ([]mgmt.DiscoveredModel, error) {
	var payload struct {
		Object string `json:"object"`
		Data   []struct {
			ID     string `json:"id"`
			Object string `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Object != "list" || payload.Data == nil {
		return nil, errInvalidZhipuCatalog
	}
	models := make([]mgmt.DiscoveredModel, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		if item.Object != "model" || !validModelCode(item.ID) {
			return nil, errInvalidZhipuCatalog
		}
		if _, duplicate := seen[item.ID]; duplicate {
			continue
		}
		seen[item.ID] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: item.ID,
			Name: zhipuDisplayName(item.ID),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, nil
}

func zhipuDisplayName(code string) string {
	if len(code) >= 4 && strings.EqualFold(code[:4], "glm-") {
		return "GLM-" + code[4:]
	}
	return deepSeekDisplayName(code)
}
