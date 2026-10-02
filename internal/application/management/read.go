package management

import (
	"context"
	"strconv"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// QueryService 承载管理面的只读查询，不持有写入、加密或上游连接依赖。
type QueryService struct {
	store         QueryStore
	discoverer    ModelDiscoverer
	subscriptions []SubscriptionAdapter
}

type QueryStore interface {
	Read(context.Context, admin.Identity, func(Reader) error) error
}

func (s *QueryService) withProviderCapabilities(provider Provider) Provider {
	return addProviderCapabilities(provider, s.discoverer, s.subscriptions)
}

func idString(id int64) string { return strconv.FormatInt(id, 10) }
func validPage(p Page) bool    { return p.After >= 0 && p.Limit >= 1 && p.Limit <= 100 }
func read[T any](ctx context.Context, s *QueryService, a admin.Identity, fn func(Reader) (T, error)) (result T, err error) {
	if s == nil || dependencyMissing(s.store) {
		return result, appsec.ErrUnavailable
	}
	err = s.store.Read(ctx, a, func(r Reader) error { var e error; result, e = fn(r); return e })
	return
}
func readPage[T any](ctx context.Context, s *QueryService, a admin.Identity, list func(Reader) ([]T, error), count func(Reader) (int64, error)) (PageData[T], error) {
	return read(ctx, s, a, func(r Reader) (PageData[T], error) {
		items, err := list(r)
		if err != nil {
			return PageData[T]{}, err
		}
		total, err := count(r)
		return PageData[T]{Items: items, Total: total}, err
	})
}
func (s *QueryService) Members(ctx context.Context, a admin.Identity, p Page) (PageData[Member], error) {
	if !validPage(p) {
		return PageData[Member]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Member, error) { return r.Members(ctx, p) }, func(r Reader) (int64, error) { return r.CountMembers(ctx) })
}
func (s *QueryService) MemberSuggestions(ctx context.Context, a admin.Identity, p Page, name string) (PageData[Member], error) {
	if !validPage(p) || !validText(name, 128) {
		return PageData[Member]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Member, error) { return r.MemberSuggestions(ctx, p, name) }, func(r Reader) (int64, error) { return r.CountMemberSuggestions(ctx, name) })
}
func (s *QueryService) GroupsByStatus(ctx context.Context, a admin.Identity, p Page, status string) (PageData[Group], error) {
	if !validPage(p) || (status != "" && !validStatus(status)) {
		return PageData[Group]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Group, error) { return r.Groups(ctx, p, status) }, func(r Reader) (int64, error) { return r.CountGroups(ctx, status) })
}
func (s *QueryService) Models(ctx context.Context, a admin.Identity, p Page, status string) (PageData[Model], error) {
	if !validPage(p) || (status != "" && !validStatus(status)) {
		return PageData[Model]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Model, error) { return r.Models(ctx, p, status) }, func(r Reader) (int64, error) { return r.CountModels(ctx, status) })
}
func validProviderType(value string) bool {
	switch catalog.ProviderType(value) {
	case catalog.Official, catalog.Platform, catalog.Partner, catalog.Custom:
		return true
	default:
		return false
	}
}
func (s *QueryService) Providers(ctx context.Context, a admin.Identity, p Page, providerType string) (PageData[Provider], error) {
	if !validPage(p) || (providerType != "" && !validProviderType(providerType)) {
		return PageData[Provider]{}, appsec.ErrInvalidArgument
	}
	result, err := readPage(ctx, s, a, func(r Reader) ([]Provider, error) { return r.Providers(ctx, p, providerType) }, func(r Reader) (int64, error) { return r.CountProviders(ctx, providerType) })
	if err != nil {
		return PageData[Provider]{}, err
	}
	for index := range result.Items {
		result.Items[index] = s.withProviderCapabilities(result.Items[index])
	}
	return result, nil
}
func (s *QueryService) Resources(ctx context.Context, a admin.Identity, p Page) (PageData[Resource], error) {
	if !validPage(p) {
		return PageData[Resource]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Resource, error) { return r.Resources(ctx, p) }, func(r Reader) (int64, error) { return r.CountResources(ctx) })
}
func (s *QueryService) Operations(ctx context.Context, a admin.Identity, p Page) (PageData[Operation], error) {
	if !validPage(p) {
		return PageData[Operation]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Operation, error) { return r.Operations(ctx, p) }, func(r Reader) (int64, error) { return r.CountOperations(ctx) })
}
func (s *QueryService) GroupMembers(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Member], error) {
	if id <= 0 || !validPage(p) {
		return PageData[Member]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Member, error) {
		if _, err := r.Group(ctx, id); err != nil {
			return nil, err
		}
		return r.GroupMembers(ctx, id, p)
	}, func(r Reader) (int64, error) { return r.CountGroupMembers(ctx, id) })
}
func (s *QueryService) MemberGroups(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Group], error) {
	if id <= 0 || !validPage(p) {
		return PageData[Group]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Group, error) {
		if _, err := r.Member(ctx, id); err != nil {
			return nil, err
		}
		return r.MemberGroups(ctx, id, p)
	}, func(r Reader) (int64, error) { return r.CountMemberGroups(ctx, id) })
}
func (s *QueryService) GroupModels(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Model], error) {
	if id <= 0 || !validPage(p) {
		return PageData[Model]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Model, error) {
		if _, err := r.Group(ctx, id); err != nil {
			return nil, err
		}
		return r.GroupModels(ctx, id, p)
	}, func(r Reader) (int64, error) { return r.CountGroupModels(ctx, id) })
}
func (s *QueryService) Keys(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Key], error) {
	if id <= 0 || !validPage(p) {
		return PageData[Key]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Key, error) {
		if _, err := r.Member(ctx, id); err != nil {
			return nil, err
		}
		return r.Keys(ctx, id, p)
	}, func(r Reader) (int64, error) { return r.CountKeys(ctx, id) })
}
func (s *QueryService) Member(ctx context.Context, a admin.Identity, id int64) (Member, error) {
	if id <= 0 {
		return Member{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (Member, error) { return r.Member(ctx, id) })
}
func (s *QueryService) Group(ctx context.Context, a admin.Identity, id int64) (Group, error) {
	if id <= 0 {
		return Group{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (Group, error) { return r.Group(ctx, id) })
}
func (s *QueryService) Resource(ctx context.Context, a admin.Identity, id int64) (Resource, error) {
	if id <= 0 {
		return Resource{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (Resource, error) { row, err := r.Resource(ctx, id); return row.Resource, err })
}
func (s *QueryService) ResourceQuotas(ctx context.Context, a admin.Identity, id int64) ([]ResourceQuota, error) {
	if id <= 0 {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]ResourceQuota, error) {
		if _, err := r.Resource(ctx, id); err != nil {
			return nil, err
		}
		return r.ResourceQuotas(ctx, id)
	})
}
func (s *QueryService) Provider(ctx context.Context, a admin.Identity, id int64) (ProviderDetail, error) {
	if id <= 0 {
		return ProviderDetail{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (ProviderDetail, error) {
		provider, err := r.Provider(ctx, id)
		if err != nil {
			return ProviderDetail{}, err
		}
		mappings, err := r.ProviderMappings(ctx, id)
		if err != nil {
			return ProviderDetail{}, err
		}
		models, err := readAllModels(ctx, r)
		if err != nil {
			return ProviderDetail{}, err
		}
		liveMappings := liveProviderMappings(mappings, models)
		provider.ModelCount = activeProviderMappingCount(liveMappings, models)
		selectableModels := providerSelectableModels(provider, models)
		return ProviderDetail{
			Provider: s.withProviderCapabilities(provider),
			Mappings: liveMappings,
			Models:   selectableModels,
		}, nil
	})
}

func providerSelectableModels(provider Provider, models []Model) []Model {
	selectable := make([]Model, 0, len(models))
	for _, model := range models {
		if provider.Type == string(catalog.Official) {
			if model.PublisherProviderID != nil && *model.PublisherProviderID == provider.ID {
				selectable = append(selectable, model)
			}
			continue
		}
		if model.Status == "ACTIVE" {
			selectable = append(selectable, model)
		}
	}
	return selectable
}

func liveProviderMappings(mappings []ProviderMapping, models []Model) []ProviderMapping {
	liveModelIDs := make(map[int64]struct{}, len(models))
	for _, model := range models {
		liveModelIDs[model.ID] = struct{}{}
	}
	liveMappings := make([]ProviderMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if _, exists := liveModelIDs[mapping.ModelID]; exists {
			liveMappings = append(liveMappings, mapping)
		}
	}
	return liveMappings
}

func activeProviderMappingCount(mappings []ProviderMapping, models []Model) int64 {
	activeModelIDs := make(map[int64]struct{}, len(models))
	for _, model := range models {
		if model.Status == "ACTIVE" {
			activeModelIDs[model.ID] = struct{}{}
		}
	}
	var count int64
	for _, mapping := range mappings {
		if _, active := activeModelIDs[mapping.ModelID]; active {
			count++
		}
	}
	return count
}
