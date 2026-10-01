package management

import (
	"context"
	"errors"
	"strings"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

const providerInitializationReadLimit = 10000

func (input *ProviderInitializeInput) Normalize() {
	input.Locale = strings.TrimSpace(input.Locale)
	for index := range input.ProviderCodes {
		input.ProviderCodes[index] = strings.TrimSpace(input.ProviderCodes[index])
	}
}

func (input ProviderInitializeInput) Valid() bool {
	if (input.Locale != "zh-CN" && input.Locale != "en-US") || len(input.ProviderCodes) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(input.ProviderCodes))
	for _, code := range input.ProviderCodes {
		if code == "" {
			return false
		}
		if _, duplicate := seen[code]; duplicate {
			return false
		}
		seen[code] = struct{}{}
	}
	return true
}

// OfficialProviderInitializationOptions 返回可供管理员选择的内置厂商。
func (s *ProviderService) OfficialProviderInitializationOptions(locale string) ([]ProviderInitializeOption, error) {
	locale = strings.TrimSpace(locale)
	if locale != "zh-CN" && locale != "en-US" {
		return nil, appsec.ErrInvalidArgument
	}
	templates := bootstrap.OfficialProviderTemplates()
	options := make([]ProviderInitializeOption, 0, len(templates))
	for _, template := range templates {
		options = append(options, ProviderInitializeOption{
			Code: template.Code, Name: template.LocalizedName(locale), Website: template.Website,
		})
	}
	return options, nil
}

// InitializeOfficialProviders 创建选定且缺失的内置厂商，并同步已有内置厂商的本地化名称和官方网站。
func (s *ProviderService) InitializeOfficialProviders(ctx context.Context, actor admin.Identity, input ProviderInitializeInput, meta appsec.RequestMeta) (ProviderInitializeResult, error) {
	input.Normalize()
	if !input.Valid() {
		return ProviderInitializeResult{}, appsec.ErrInvalidArgument
	}
	selected := make(map[string]struct{}, len(input.ProviderCodes))
	for _, code := range input.ProviderCodes {
		selected[code] = struct{}{}
	}
	templates := make([]bootstrap.Provider, 0, len(selected))
	for _, template := range bootstrap.OfficialProviderTemplates() {
		if _, ok := selected[template.Code]; !ok {
			continue
		}
		delete(selected, template.Code)
		templates = append(templates, template)
	}
	if len(selected) != 0 {
		return ProviderInitializeResult{}, appsec.ErrInvalidArgument
	}
	result := ProviderInitializeResult{Total: len(templates)}
	err := s.store.WriteProvider(ctx, actor, func(writer ProviderSession) error {
		providers, err := readAllProviders(ctx, writer)
		if err != nil {
			return err
		}
		existing := make(map[string]Provider, len(providers))
		for _, provider := range providers {
			existing[provider.Code] = provider
		}

		now := businessTime(s.now)
		for _, template := range templates {
			name := template.LocalizedName(input.Locale)
			if provider, ok := existing[template.Code]; ok {
				result.Existing++
				if provider.Name == name && provider.Website != nil && *provider.Website == template.Website {
					continue
				}
				before := provider
				provider.Name = name
				website := template.Website
				provider.Website = &website
				provider.UpdatedAt = now
				if err := writer.UpdateProvider(ctx, provider); err != nil {
					return err
				}
				if err := writer.Audit(ctx, Audit{Event: operation.ProviderUpdate, Target: "PROVIDER", ID: provider.ID, Name: provider.Name, Before: before, After: provider}, meta); err != nil {
					return err
				}
				result.Updated++
				continue
			}
			id, err := s.next(ctx)
			if err != nil {
				return err
			}
			website := template.Website
			provider := Provider{
				ID: id, Code: template.Code, Name: name, Type: "OFFICIAL", Status: "DISABLED",
				Website:   &website,
				Endpoints: make([]ProviderEndpoint, 0, len(template.Endpoints)),
				CreatedAt: now, UpdatedAt: now,
			}
			for _, endpoint := range template.Endpoints {
				provider.Endpoints = append(provider.Endpoints, ProviderEndpoint{ProtocolType: endpoint.ProtocolType, BaseURL: endpoint.BaseURL, NetworkScope: catalog.NetworkScopePublic})
			}
			if err := writer.CreateProvider(ctx, provider); err != nil {
				return err
			}
			if err := writer.Audit(ctx, Audit{Event: operation.ProviderCreate, Target: "PROVIDER", ID: id, Name: provider.Name, After: provider}, meta); err != nil {
				return err
			}
			existing[provider.Code] = provider
			result.Created++
		}
		return nil
	})
	return result, err
}

type providerLister interface {
	Providers(context.Context, Page, string) ([]Provider, error)
}

func readAllProviders(ctx context.Context, reader providerLister) ([]Provider, error) {
	providers := make([]Provider, 0)
	var after int64
	for {
		page, err := reader.Providers(ctx, Page{After: after, Limit: 100}, "")
		if err != nil {
			return nil, err
		}
		providers = append(providers, page...)
		if len(page) < 100 {
			return providers, nil
		}
		next := page[len(page)-1].ID
		if next <= 0 || next == after || len(providers) > providerInitializationReadLimit {
			return nil, errors.New("provider catalog pagination did not advance")
		}
		after = next
	}
}
