package management

import (
	"context"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"strconv"
)

func idString(id int64) string { return strconv.FormatInt(id, 10) }
func validPage(p Page) bool    { return p.After >= 0 && p.Limit >= 1 && p.Limit <= 100 }
func read[T any](ctx context.Context, s *Service, a admin.Identity, fn func(Reader) (T, error)) (result T, err error) {
	err = s.store.Read(ctx, a, func(r Reader) error { var e error; result, e = fn(r); return e })
	return
}
func readPage[T any](ctx context.Context, s *Service, a admin.Identity, list func(Reader) ([]T, error), count func(Reader) (int64, error)) (PageData[T], error) {
	return read(ctx, s, a, func(r Reader) (PageData[T], error) {
		items, err := list(r)
		if err != nil {
			return PageData[T]{}, err
		}
		total, err := count(r)
		return PageData[T]{Items: items, Total: total}, err
	})
}
func (s *Service) Members(ctx context.Context, a admin.Identity, p Page) (PageData[Member], error) {
	if !validPage(p) {
		return PageData[Member]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Member, error) { return r.Members(ctx, p) }, func(r Reader) (int64, error) { return r.CountMembers(ctx) })
}
func (s *Service) MemberSuggestions(ctx context.Context, a admin.Identity, p Page, name string) (PageData[Member], error) {
	if !validPage(p) || !validText(name, 128) {
		return PageData[Member]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Member, error) { return r.MemberSuggestions(ctx, p, name) }, func(r Reader) (int64, error) { return r.CountMemberSuggestions(ctx, name) })
}
func (s *Service) GroupsByStatus(ctx context.Context, a admin.Identity, p Page, status string) (PageData[Group], error) {
	if !validPage(p) || (status != "" && !validStatus(status)) {
		return PageData[Group]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Group, error) { return r.Groups(ctx, p, status) }, func(r Reader) (int64, error) { return r.CountGroups(ctx, status) })
}
func (s *Service) Models(ctx context.Context, a admin.Identity, p Page, status string) (PageData[Model], error) {
	if !validPage(p) || (status != "" && !validStatus(status)) {
		return PageData[Model]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Model, error) { return r.Models(ctx, p, status) }, func(r Reader) (int64, error) { return r.CountModels(ctx, status) })
}
func (s *Service) Providers(ctx context.Context, a admin.Identity, p Page) (PageData[Provider], error) {
	if !validPage(p) {
		return PageData[Provider]{}, appsec.ErrInvalidArgument
	}
	result, err := readPage(ctx, s, a, func(r Reader) ([]Provider, error) { return r.Providers(ctx, p) }, func(r Reader) (int64, error) { return r.CountProviders(ctx) })
	if err != nil {
		return PageData[Provider]{}, err
	}
	for index := range result.Items {
		result.Items[index] = s.withProviderCapabilities(result.Items[index])
	}
	return result, nil
}
func (s *Service) Resources(ctx context.Context, a admin.Identity, p Page) (PageData[Resource], error) {
	if !validPage(p) {
		return PageData[Resource]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Resource, error) { return r.Resources(ctx, p) }, func(r Reader) (int64, error) { return r.CountResources(ctx) })
}
func (s *Service) Operations(ctx context.Context, a admin.Identity, p Page) (PageData[Operation], error) {
	if !validPage(p) {
		return PageData[Operation]{}, appsec.ErrInvalidArgument
	}
	return readPage(ctx, s, a, func(r Reader) ([]Operation, error) { return r.Operations(ctx, p) }, func(r Reader) (int64, error) { return r.CountOperations(ctx) })
}
func (s *Service) GroupMembers(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Member], error) {
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
func (s *Service) MemberGroups(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Group], error) {
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
func (s *Service) GroupModels(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Model], error) {
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
func (s *Service) Keys(ctx context.Context, a admin.Identity, id int64, p Page) (PageData[Key], error) {
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
func (s *Service) Member(ctx context.Context, a admin.Identity, id int64) (Member, error) {
	if id <= 0 {
		return Member{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (Member, error) { return r.Member(ctx, id) })
}
func (s *Service) Group(ctx context.Context, a admin.Identity, id int64) (Group, error) {
	if id <= 0 {
		return Group{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (Group, error) { return r.Group(ctx, id) })
}
func (s *Service) Resource(ctx context.Context, a admin.Identity, id int64) (Resource, error) {
	if id <= 0 {
		return Resource{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (Resource, error) { row, err := r.Resource(ctx, id); return row.Resource, err })
}
func (s *Service) ResourceQuotas(ctx context.Context, a admin.Identity, id int64) ([]ResourceQuota, error) {
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
func (s *Service) Provider(ctx context.Context, a admin.Identity, id int64) (ProviderDetail, error) {
	if id <= 0 {
		return ProviderDetail{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) (ProviderDetail, error) {
		provider, err := r.Provider(ctx, id)
		if err != nil {
			return ProviderDetail{}, err
		}
		mappings, err := r.ProviderMappings(ctx, id)
		return ProviderDetail{Provider: s.withProviderCapabilities(provider), Mappings: mappings}, err
	})
}
