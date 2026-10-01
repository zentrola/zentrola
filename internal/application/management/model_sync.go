package management

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

const modelSyncLimit = 1000

func preferredProviderEndpoint(provider Provider) (string, string, string) {
	for _, preferred := range [...]string{"ANTHROPIC", "OPENAI"} {
		for _, endpoint := range provider.Endpoints {
			if endpoint.ProtocolType == preferred {
				return endpoint.ProtocolType, endpoint.BaseURL, endpoint.NetworkScope
			}
		}
	}
	return "", "", ""
}

func providerEndpoint(provider Provider, protocol string) (string, string, string) {
	for _, endpoint := range provider.Endpoints {
		if endpoint.ProtocolType == protocol {
			return endpoint.ProtocolType, endpoint.BaseURL, endpoint.NetworkScope
		}
	}
	return "", "", ""
}

func (s *ProviderService) SyncProviderModels(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (ModelSyncResult, error) {
	if id <= 0 {
		return ModelSyncResult{}, appsec.ErrInvalidArgument
	}

	var provider Provider
	var resourceID int64
	err := s.store.ReadProvider(ctx, actor, func(reader ProviderReadSession) error {
		var err error
		provider, err = reader.Provider(ctx, id)
		if err != nil {
			return err
		}
		if s.discoverer == nil || !s.discoverer.Supports(provider.Code) {
			return ErrProvider
		}

		var after int64
		var selected Resource
		for {
			resources, err := reader.Resources(ctx, Page{After: after, Limit: 100})
			if err != nil {
				return err
			}
			for _, resource := range resources {
				if resource.ProviderID != id ||
					(resource.AuthType != AuthTypeAPIKey && resource.AuthType != "") {
					continue
				}
				if resourceID == 0 || resource.Priority < selected.Priority {
					selected = resource
					resourceID = resource.ID
				}
			}
			if len(resources) < 100 {
				break
			}
			next := resources[len(resources)-1].ID
			if next <= 0 || next == after {
				return errors.New("resource pagination did not advance")
			}
			after = next
		}
		return nil
	})
	if err != nil {
		return ModelSyncResult{}, err
	}
	if resourceID != 0 {
		return s.SyncResourceModels(ctx, actor, resourceID, meta)
	}
	return ModelSyncResult{}, ErrModelSyncCredentialRequired
}

func (s *ProviderService) SyncResourceModels(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (ModelSyncResult, error) {
	if id <= 0 {
		return ModelSyncResult{}, appsec.ErrInvalidArgument
	}
	if s.discoverer == nil {
		return ModelSyncResult{}, ErrProvider
	}

	var resource ResourceRecord
	var provider Provider
	err := s.store.ReadProvider(ctx, actor, func(reader ProviderReadSession) error {
		var err error
		resource, err = reader.Resource(ctx, id)
		if err != nil {
			return err
		}
		provider, err = reader.Provider(ctx, resource.ProviderID)
		return err
	})
	if err != nil {
		return ModelSyncResult{}, err
	}
	if !s.discoverer.Supports(provider.Code) {
		return ModelSyncResult{}, ErrProvider
	}

	plain, err := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
	if err != nil {
		return ModelSyncResult{}, ErrCredential
	}
	defer clear(plain)
	proxy, err := s.decryptedProviderProxy(provider)
	if err != nil {
		return ModelSyncResult{}, ErrProvider
	}

	discovered, connection := s.discoverer.Discover(ctx, ModelDiscoverySource{
		ProviderCode: provider.Code,
		Endpoints:    slices.Clone(provider.Endpoints),
	}, plain, proxy)
	result := ModelSyncResult{ConnectionResult: connection, Discovered: len(discovered), Source: "PROVIDER"}
	if len(discovered) > modelSyncLimit {
		result.OK = false
		result.Code = "UPSTREAM_INVALID_RESPONSE"
		discovered = nil
	}
	if !result.OK {
		err = s.auditModelSync(ctx, actor, provider, resource, result, meta)
		return result, err
	}
	return s.persistDiscoveredModels(ctx, actor, provider, &resource, discovered, result, meta)
}

func (s *ProviderService) persistDiscoveredModels(ctx context.Context, actor admin.Identity, provider Provider, resource *ResourceRecord, discovered []DiscoveredModel, result ModelSyncResult, meta appsec.RequestMeta) (ModelSyncResult, error) {
	err := s.store.WriteProvider(ctx, actor, func(writer ProviderSession) error {
		if resource != nil {
			current, err := writer.Resource(ctx, resource.ID)
			if err != nil {
				return err
			}
			if !current.UpdatedAt.Equal(resource.UpdatedAt) {
				return ErrConflict
			}
		}

		models, err := readAllModels(ctx, writer)
		if err != nil {
			return err
		}
		modelsByCode := make(map[string]Model, len(models))
		for _, model := range models {
			modelsByCode[model.Code] = model
		}
		mappings, err := writer.ProviderMappings(ctx, provider.ID)
		if err != nil {
			return err
		}
		mappedModels := make(map[int64]struct{}, len(mappings))
		for _, mapping := range mappings {
			mappedModels[mapping.ModelID] = struct{}{}
		}

		now := businessTime(s.now)
		publisherProviderID := provider.ID
		publisherProviderName := provider.Name
		seen := make(map[string]struct{}, len(discovered))
		for _, candidate := range discovered {
			if !validText(candidate.Code, 128) {
				continue
			}
			if _, duplicate := seen[candidate.Code]; duplicate {
				continue
			}
			seen[candidate.Code] = struct{}{}
			model, exists := modelsByCode[candidate.Code]
			name := candidate.Name
			if !validText(name, 128) || name == candidate.Code {
				name = modelDisplayName(candidate.Code)
			}
			if !exists {
				modelID, err := s.next(ctx)
				if err != nil {
					return err
				}
				inputModalities, outputModalities := discoveredModalities(candidate)
				model = Model{
					ID: modelID, Code: candidate.Code, Name: name, Status: "ACTIVE",
					InputModalities: inputModalities, OutputModalities: outputModalities,
					PublisherProviderID: &publisherProviderID, PublisherProviderName: &publisherProviderName,
					CreatedAt: now, UpdatedAt: now,
				}
				if err := writer.CreateModel(ctx, model); err != nil {
					return err
				}
				modelsByCode[model.Code] = model
				result.Created++
			} else if model.Name != name || model.PublisherProviderID == nil || *model.PublisherProviderID != provider.ID {
				model.Name = name
				model.PublisherProviderID = &publisherProviderID
				model.PublisherProviderName = &publisherProviderName
				model.UpdatedAt = now
				if err := writer.UpdateModel(ctx, model); err != nil {
					return err
				}
				modelsByCode[model.Code] = model
				result.Updated++
			}
			if _, exists := mappedModels[model.ID]; exists {
				continue
			}
			mappingID, err := s.next(ctx)
			if err != nil {
				return err
			}
			if err := writer.CreateProviderMapping(ctx, ProviderMapping{
				ID: mappingID, ProviderID: provider.ID, ModelID: model.ID,
				UpstreamModelCode: "", Priority: defaultProviderMappingPriority,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				return err
			}
			mappedModels[model.ID] = struct{}{}
			result.Mapped++
		}
		after := map[string]any{"source": result.Source, "sync": result}
		if resource != nil {
			after["resourceId"] = idString(resource.ID)
		}
		return writer.Audit(ctx, Audit{
			Event: operation.ModelCatalogSync, Target: "PROVIDER", ID: provider.ID, Name: provider.Name,
			After: after,
		}, meta)
	})
	return result, err
}

func discoveredModalities(model DiscoveredModel) ([]string, []string) {
	if catalog.ValidModalities(model.InputModalities) && catalog.ValidModalities(model.OutputModalities) {
		return slices.Clone(model.InputModalities), slices.Clone(model.OutputModalities)
	}
	return []string{"TEXT"}, []string{"TEXT"}
}

func modelDisplayName(code string) string {
	name := strings.ReplaceAll(code, "-", " ")
	runes := []rune(name)
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

type modelLister interface {
	Models(context.Context, Page, string) ([]Model, error)
}

func readAllModels(ctx context.Context, reader modelLister) ([]Model, error) {
	models := make([]Model, 0)
	var after int64
	for {
		page, err := reader.Models(ctx, Page{After: after, Limit: 100}, "")
		if err != nil {
			return nil, err
		}
		models = append(models, page...)
		if len(page) < 100 {
			return models, nil
		}
		next := page[len(page)-1].ID
		if next <= 0 || next == after || len(models) > modelSyncLimit {
			return nil, errors.New("model catalog pagination did not advance")
		}
		after = next
	}
}

func (s *ProviderService) auditModelSync(ctx context.Context, actor admin.Identity, provider Provider, resource ResourceRecord, result ModelSyncResult, meta appsec.RequestMeta) error {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	return s.store.WriteProvider(auditCtx, actor, func(writer ProviderSession) error {
		current, err := writer.Resource(auditCtx, resource.ID)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(resource.UpdatedAt) {
			return ErrConflict
		}
		return writer.Audit(auditCtx, Audit{
			Event: operation.ModelCatalogSync, Target: "PROVIDER", ID: provider.ID, Name: provider.Name,
			After: map[string]any{"resourceId": idString(resource.ID), "sync": result}, ErrorCode: result.Code,
		}, meta)
	})
}
