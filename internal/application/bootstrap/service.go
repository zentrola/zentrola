// Package bootstrap 保存管理员可显式安装的官方服务商和模型目录。
package bootstrap

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type Provider struct {
	Code      string     `json:"code"`
	NameZH    string     `json:"nameZH"`
	NameEN    string     `json:"nameEN"`
	Website   string     `json:"website"`
	Endpoints []Endpoint `json:"endpoints"`
	Models    []Model    `json:"models"`
}

func (p Provider) LocalizedName(locale string) string {
	if locale == "zh-CN" {
		return p.NameZH
	}
	return p.NameEN
}

type Endpoint struct {
	ProtocolType string `json:"protocolType"`
	BaseURL      string `json:"baseUrl"`
}

type Model struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	ModelType        string   `json:"modelType"`
	InputModalities  []string `json:"inputModalities"`
	OutputModalities []string `json:"outputModalities"`
}

type providerCatalog struct {
	SchemaVersion  int        `json:"schemaVersion"`
	CatalogVersion string     `json:"catalogVersion"`
	Providers      []Provider `json:"providers"`
}

//go:embed catalog/providers.json
var providerCatalogJSON []byte

var providerCatalogVersion, providerTemplates = mustLoadProviderCatalog(providerCatalogJSON)

func mustLoadProviderCatalog(data []byte) (string, []Provider) {
	var catalog providerCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		panic(fmt.Errorf("decode built-in provider catalog: %w", err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		panic("built-in provider catalog contains trailing JSON values")
	}
	if catalog.SchemaVersion != 1 || len(catalog.Providers) == 0 {
		panic("invalid built-in provider catalog header")
	}
	parsedCatalogVersion, err := time.Parse("2006-01-02", catalog.CatalogVersion)
	if err != nil || parsedCatalogVersion.Format("2006-01-02") != catalog.CatalogVersion {
		panic(fmt.Sprintf("invalid built-in provider catalog version %q", catalog.CatalogVersion))
	}
	seen := make(map[string]struct{}, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		if provider.Code == "" || provider.NameZH == "" || provider.NameEN == "" || provider.Website == "" || len(provider.Endpoints) == 0 {
			panic(fmt.Sprintf("invalid built-in provider %q", provider.Code))
		}
		if _, duplicate := seen[provider.Code]; duplicate {
			panic(fmt.Sprintf("duplicate built-in provider %q", provider.Code))
		}
		seen[provider.Code] = struct{}{}
		for _, endpoint := range provider.Endpoints {
			if (endpoint.ProtocolType != "OPENAI" && endpoint.ProtocolType != "ANTHROPIC") || endpoint.BaseURL == "" {
				panic(fmt.Sprintf("invalid endpoint for built-in provider %q", provider.Code))
			}
		}
		modelCodes := make(map[string]struct{}, len(provider.Models))
		for _, model := range provider.Models {
			if model.Code == "" || model.Name == "" || model.ModelType != "CHAT" ||
				!validCatalogModalities(model.InputModalities) || !validCatalogModalities(model.OutputModalities) {
				panic(fmt.Sprintf("invalid model for built-in provider %q", provider.Code))
			}
			if _, duplicate := modelCodes[model.Code]; duplicate {
				panic(fmt.Sprintf("duplicate model %q for built-in provider %q", model.Code, provider.Code))
			}
			modelCodes[model.Code] = struct{}{}
		}
	}
	return catalog.CatalogVersion, catalog.Providers
}

func validCatalogModalities(values []string) bool {
	if len(values) == 0 || len(values) > 4 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		switch value {
		case "TEXT", "IMAGE", "AUDIO", "VIDEO":
		default:
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

// OfficialProviderModels 返回厂商内置模型目录的独立副本。
func OfficialProviderModels(providerCode string) []Model {
	for _, provider := range providerTemplates {
		if provider.Code != providerCode {
			continue
		}
		result := append([]Model(nil), provider.Models...)
		for index := range result {
			result[index].InputModalities = append([]string(nil), provider.Models[index].InputModalities...)
			result[index].OutputModalities = append([]string(nil), provider.Models[index].OutputModalities...)
		}
		return result
	}
	return nil
}

// OfficialProviderCatalogVersion 返回应用内置厂商和模型目录的内容版本。
func OfficialProviderCatalogVersion() string {
	return providerCatalogVersion
}

// OfficialProviderTemplates 返回独立副本，供管理员显式初始化或补齐预置厂商使用。
func OfficialProviderTemplates() []Provider {
	result := make([]Provider, len(providerTemplates))
	for index, template := range providerTemplates {
		result[index] = template
		result[index].Endpoints = append([]Endpoint(nil), template.Endpoints...)
		result[index].Models = append([]Model(nil), template.Models...)
		for modelIndex := range result[index].Models {
			result[index].Models[modelIndex].InputModalities = append([]string(nil), template.Models[modelIndex].InputModalities...)
			result[index].Models[modelIndex].OutputModalities = append([]string(nil), template.Models[modelIndex].OutputModalities...)
		}
	}
	return result
}
