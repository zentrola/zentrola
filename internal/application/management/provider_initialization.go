package management

import (
	"context"
	"errors"
	"time"

	"github.com/zentrola/zentrola/internal/application/bootstrap"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

const providerInitializationReadLimit = 10000

// InitializeOfficialProviders 只创建缺失的内置厂商，不修改任何已有厂商配置。
func (s *Service) InitializeOfficialProviders(ctx context.Context, actor admin.Identity, meta appsec.RequestMeta) (ProviderInitializeResult, error) {
	templates := bootstrap.OfficialProviderTemplates()
	result := ProviderInitializeResult{Total: len(templates)}
	err := s.store.Write(ctx, actor, func(writer Writer) error {
		providers, err := readAllProviders(ctx, writer)
		if err != nil {
			return err
		}
		existing := make(map[string]struct{}, len(providers))
		for _, provider := range providers {
			existing[provider.Code] = struct{}{}
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		for _, template := range templates {
			if _, ok := existing[template.Code]; ok {
				result.Existing++
				continue
			}
			id, err := s.next()
			if err != nil {
				return err
			}
			provider := Provider{
				ID: id, Code: template.Code, Name: template.Name, Type: "OFFICIAL", Status: "ACTIVE",
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
			existing[provider.Code] = struct{}{}
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
		page, err := reader.Providers(ctx, Page{After: after, Limit: 100})
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
