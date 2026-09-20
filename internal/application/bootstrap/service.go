// Package bootstrap 保存管理员可显式安装的官方服务商元数据。
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
	}
	return catalog.CatalogVersion, catalog.Providers
}

// OfficialProviderCatalogVersion 返回应用内置厂商目录的内容版本。
func OfficialProviderCatalogVersion() string {
	return providerCatalogVersion
}

// OfficialProviderTemplates 返回独立副本，供管理员显式初始化或补齐预置厂商使用。
func OfficialProviderTemplates() []Provider {
	result := make([]Provider, len(providerTemplates))
	for index, template := range providerTemplates {
		result[index] = template
		result[index].Endpoints = append([]Endpoint(nil), template.Endpoints...)
	}
	return result
}
