package modelcatalog

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

const xAILanguageModelsURL = "https://api.x.ai/v1/language-models"

var errInvalidXAICatalog = errors.New("invalid xAI model catalog response")

type xAIAdapter struct{}

func (xAIAdapter) Name() string { return "xai" }

func (xAIAdapter) Request(mgmt.ModelDiscoverySource, int, string) (catalogRequest, error) {
	return catalogRequest{URL: xAILanguageModelsURL, Bearer: true}, nil
}

func (xAIAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Models []struct {
			ID               string   `json:"id"`
			Aliases          []string `json:"aliases"`
			Object           string   `json:"object"`
			OwnedBy          string   `json:"owned_by"`
			InputModalities  []string `json:"input_modalities"`
			OutputModalities []string `json:"output_modalities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Models == nil {
		return nil, "", errInvalidXAICatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))
	for _, item := range payload.Models {
		if item.Object != "model" || item.OwnedBy != "xai" || !validModelCode(item.ID) || item.Aliases == nil {
			return nil, "", errInvalidXAICatalog
		}
		for _, alias := range item.Aliases {
			if !validModelCode(alias) {
				return nil, "", errInvalidXAICatalog
			}
		}
		inputModalities, inputOK := xAIModalities(item.InputModalities)
		outputModalities, outputOK := xAIModalities(item.OutputModalities)
		if !inputOK || !outputOK {
			return nil, "", errInvalidXAICatalog
		}
		code := xAIModelCode(item.ID, item.Aliases)
		if code == "" {
			continue
		}
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: code, Name: xAIDisplayName(code),
			InputModalities: inputModalities, OutputModalities: outputModalities,
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, "", nil
}

func xAIModelCode(id string, aliases []string) string {
	if strings.HasPrefix(strings.ToLower(id), "grok-") {
		return id
	}
	for _, alias := range aliases {
		if strings.HasPrefix(strings.ToLower(alias), "grok-") {
			return alias
		}
	}
	return ""
}

func xAIModalities(values []string) ([]string, bool) {
	modalities := make([]string, len(values))
	for index, value := range values {
		modalities[index] = strings.ToUpper(value)
	}
	return modalities, catalog.ValidModalities(modalities)
}

func xAIDisplayName(code string) string {
	if len(code) > len("grok-") && strings.EqualFold(code[:len("grok-")], "grok-") {
		return "Grok " + strings.ReplaceAll(code[len("grok-"):], "-", " ")
	}
	return deepSeekDisplayName(code)
}
