package modelcatalog

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
)

const (
	googleCatalogURL      = "https://generativelanguage.googleapis.com/v1beta/models"
	googleCatalogPageSize = "100"
)

var errInvalidGoogleCatalog = errors.New("invalid Google model catalog response")

type googleAdapter struct{}

func (googleAdapter) Name() string { return "google" }

func (googleAdapter) Request(_ mgmt.ModelDiscoverySource, page int, pageToken string) (catalogRequest, error) {
	if page <= 0 || page > maxCatalogPages || (page == 1) != (pageToken == "") || !validGooglePageToken(pageToken) {
		return catalogRequest{}, errInvalidGoogleCatalog
	}
	endpoint, err := url.Parse(googleCatalogURL)
	if err != nil {
		return catalogRequest{}, errInvalidGoogleCatalog
	}
	query := endpoint.Query()
	query.Set("pageSize", googleCatalogPageSize)
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	endpoint.RawQuery = query.Encode()
	return catalogRequest{URL: endpoint.String(), APIKeyHeader: "X-Goog-Api-Key"}, nil
}

func (googleAdapter) Decode(data []byte, _ int) ([]mgmt.DiscoveredModel, string, error) {
	var payload struct {
		Models []struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || payload.Models == nil ||
		!validGooglePageToken(payload.NextPageToken) {
		return nil, "", errInvalidGoogleCatalog
	}

	models := make([]mgmt.DiscoveredModel, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))
	for _, item := range payload.Models {
		if !supportsGoogleGeneration(item.SupportedGenerationMethods) {
			continue
		}
		code, ok := strings.CutPrefix(item.Name, "models/")
		if !ok || !validModelCode(code) || strings.Contains(code, "/") {
			return nil, "", errInvalidGoogleCatalog
		}
		if !isGoogleStableGeneralModel(code) {
			continue
		}
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		models = append(models, mgmt.DiscoveredModel{
			Code: code,
			Name: googleDisplayName(code, item.DisplayName),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, payload.NextPageToken, nil
}

func validGooglePageToken(token string) bool {
	if len(token) > 4096 || token != strings.TrimSpace(token) || !utf8.ValidString(token) {
		return false
	}
	for _, character := range token {
		if character < 33 || character == 127 {
			return false
		}
	}
	return true
}

func supportsGoogleGeneration(methods []string) bool {
	for _, method := range methods {
		if method == "generateContent" {
			return true
		}
	}
	return false
}

func isGoogleStableGeneralModel(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	if !strings.HasPrefix(code, "gemini-") {
		return false
	}
	for _, unstable := range []string{"-exp", "experimental", "latest", "preview"} {
		if strings.Contains(code, unstable) {
			return false
		}
	}
	for _, specialized := range []string{"audio", "computer-use", "image", "live", "robotics", "transcribe", "tts"} {
		if strings.Contains(code, specialized) {
			return false
		}
	}
	return true
}

func googleDisplayName(code, displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" && len(displayName) <= 128 && utf8.ValidString(displayName) && !strings.ContainsRune(displayName, 0) {
		return displayName
	}
	if strings.HasPrefix(code, "gemini-") {
		return "Gemini " + strings.ReplaceAll(code[len("gemini-"):], "-", " ")
	}
	return deepSeekDisplayName(code)
}
