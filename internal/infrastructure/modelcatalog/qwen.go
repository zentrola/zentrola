package modelcatalog

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

const qwenCatalogPageSize = 100

var errInvalidQwenCatalog = errors.New("invalid Qwen model catalog response")

type qwenAdapter struct{}

func (qwenAdapter) Name() string { return "qwen" }

func (qwenAdapter) Request(source mgmt.ModelDiscoverySource, page int) (catalogRequest, error) {
	baseURL := qwenCatalogBaseURL(source.Endpoints)
	baseURL, ok := provider.BaseURL(baseURL)
	if !ok || page <= 0 || page > maxCatalogPages {
		return catalogRequest{}, errInvalidQwenCatalog
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return catalogRequest{}, errInvalidQwenCatalog
	}
	endpoint.Path = "/api/v1/models"
	endpoint.RawPath = ""
	query := endpoint.Query()
	query.Set("capabilities", "TG")
	query.Set("features", "model-experience")
	query.Set("language", "en-US")
	query.Set("page_no", strconv.Itoa(page))
	query.Set("page_size", strconv.Itoa(qwenCatalogPageSize))
	query.Set("providers", "qwen")
	query.Set("supports", "inference")
	endpoint.RawQuery = query.Encode()
	return catalogRequest{URL: endpoint.String(), Bearer: true}, nil
}

func (qwenAdapter) Decode(data []byte, requestedPage int) ([]mgmt.DiscoveredModel, bool, error) {
	var payload struct {
		Success bool `json:"success"`
		Output  struct {
			Total    int `json:"total"`
			PageNo   int `json:"page_no"`
			PageSize int `json:"page_size"`
			Models   []struct {
				Model             string `json:"model"`
				Name              string `json:"name"`
				InferenceMetadata struct {
					RequestModality  []string `json:"request_modality"`
					ResponseModality []string `json:"response_modality"`
				} `json:"inference_metadata"`
			} `json:"models"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || !payload.Success || payload.Output.Models == nil ||
		payload.Output.Total < 0 || payload.Output.Total > maxCatalogModels || payload.Output.PageNo != requestedPage ||
		payload.Output.PageSize != qwenCatalogPageSize || len(payload.Output.Models) > payload.Output.PageSize {
		return nil, false, errInvalidQwenCatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Output.Models))
	seen := make(map[string]struct{}, len(payload.Output.Models))
	for _, item := range payload.Output.Models {
		if !isQwenModelCode(item.Model) {
			continue
		}
		if !validModelCode(item.Model) {
			return nil, false, errInvalidQwenCatalog
		}
		if _, duplicate := seen[item.Model]; duplicate {
			continue
		}
		seen[item.Model] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code:             item.Model,
			Name:             qwenDisplayName(item.Model, item.Name),
			InputModalities:  qwenModalities(item.InferenceMetadata.RequestModality),
			OutputModalities: qwenModalities(item.InferenceMetadata.ResponseModality),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	hasNext := payload.Output.PageNo*payload.Output.PageSize < payload.Output.Total
	if hasNext && len(payload.Output.Models) == 0 {
		return nil, false, errInvalidQwenCatalog
	}
	return models, hasNext, nil
}

func qwenCatalogBaseURL(endpoints []mgmt.ProviderEndpoint) string {
	for _, endpoint := range endpoints {
		parsed, err := url.Parse(endpoint.BaseURL)
		if err == nil && strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".maas.aliyuncs.com") {
			return endpoint.BaseURL
		}
	}
	for _, endpoint := range endpoints {
		if endpoint.ProtocolType == "OPENAI" {
			return endpoint.BaseURL
		}
	}
	if len(endpoints) > 0 {
		return endpoints[0].BaseURL
	}
	return ""
}

func isQwenModelCode(code string) bool {
	return strings.HasPrefix(strings.ToLower(code), "qwen")
}

func qwenDisplayName(code, name string) string {
	name = strings.TrimSpace(name)
	if name != "" && len(name) <= 128 && utf8.ValidString(name) && !strings.ContainsRune(name, 0) {
		return name
	}
	parts := strings.Split(code, "-")
	for index := range parts {
		if index == 0 {
			parts[index] = "Qwen" + parts[index][len("qwen"):]
			continue
		}
		runes := []rune(parts[index])
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[index] = string(runes)
		}
	}
	return strings.Join(parts, "-")
}

func qwenModalities(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		switch value {
		case "TEXT", "IMAGE", "AUDIO", "VIDEO":
		default:
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
