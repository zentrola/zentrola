package management

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

const providerInitializationReadLimit = 10000

func (input *ProviderInitializeInput) Normalize() {
	input.Locale = strings.TrimSpace(input.Locale)
}

func (input ProviderInitializeInput) Valid() bool {
	return input.Locale == "zh-CN" || input.Locale == "en-US"
}

// InitializeOfficialProviders 创建缺失的内置厂商，并同步已有内置厂商的本地化名称和官方网站。
func (s *Service) InitializeOfficialProviders(ctx context.Context, actor admin.Identity, input ProviderInitializeInput, meta appsec.RequestMeta) (ProviderInitializeResult, error) {
	input.Normalize()
	if !input.Valid() {
		return ProviderInitializeResult{}, appsec.ErrInvalidArgument
	}
	templates := bootstrap.OfficialProviderTemplates()
	result := ProviderInitializeResult{Total: len(templates)}
	err := s.store.Write(ctx, actor, func(writer Writer) error {
		providers, err := readAllProviders(ctx, writer)
		if err != nil {
			return err
		}
		existing := make(map[string]Provider, len(providers))
		for _, provider := range providers {
			existing[provider.Code] = provider
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
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
				provider.Endpoints = append(provider.Endpoints, ProviderEndpoint{ProtocolType: endpoint.ProtocolType, BaseURL: endpoint.BaseURL})
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

func readAllProviders(ctx context.Context, reader Reader) ([]Provider, error) {
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
