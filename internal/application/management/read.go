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
func (s *Service) Members(ctx context.Context, a admin.Identity, p Page) ([]Member, error) {
	if !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Member, error) { return r.Members(ctx, p) })
}
func (s *Service) Groups(ctx context.Context, a admin.Identity, p Page) ([]Group, error) {
	return s.GroupsByStatus(ctx, a, p, "")
}
func (s *Service) GroupsByStatus(ctx context.Context, a admin.Identity, p Page, status string) ([]Group, error) {
	if !validPage(p) || (status != "" && !validStatus(status)) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Group, error) { return r.Groups(ctx, p, status) })
}
func (s *Service) Models(ctx context.Context, a admin.Identity, p Page, status string) ([]Model, error) {
	if !validPage(p) || (status != "" && !validStatus(status)) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Model, error) { return r.Models(ctx, p, status) })
}
func (s *Service) Providers(ctx context.Context, a admin.Identity, p Page) ([]Provider, error) {
	if !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Provider, error) { return r.Providers(ctx, p) })
}
func (s *Service) Resources(ctx context.Context, a admin.Identity, p Page) ([]Resource, error) {
	if !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Resource, error) { return r.Resources(ctx, p) })
}
func (s *Service) Operations(ctx context.Context, a admin.Identity, p Page) ([]Operation, error) {
	if !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Operation, error) { return r.Operations(ctx, p) })
}
func (s *Service) GroupMembers(ctx context.Context, a admin.Identity, id int64, p Page) ([]Member, error) {
	if id <= 0 || !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Member, error) {
		if _, err := r.Group(ctx, id); err != nil {
			return nil, err
		}
		return r.GroupMembers(ctx, id, p)
	})
}
func (s *Service) MemberGroups(ctx context.Context, a admin.Identity, id int64, p Page) ([]Group, error) {
	if id <= 0 || !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Group, error) {
		if _, err := r.Member(ctx, id); err != nil {
			return nil, err
		}
		return r.MemberGroups(ctx, id, p)
	})
}
func (s *Service) GroupModels(ctx context.Context, a admin.Identity, id int64, p Page) ([]Model, error) {
	if id <= 0 || !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Model, error) {
		if _, err := r.Group(ctx, id); err != nil {
			return nil, err
		}
		return r.GroupModels(ctx, id, p)
	})
}
func (s *Service) Keys(ctx context.Context, a admin.Identity, id int64, p Page) ([]Key, error) {
	if id <= 0 || !validPage(p) {
		return nil, appsec.ErrInvalidArgument
	}
	return read(ctx, s, a, func(r Reader) ([]Key, error) {
		if _, err := r.Member(ctx, id); err != nil {
			return nil, err
		}
		return r.Keys(ctx, id, p)
	})
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
		return ProviderDetail{Provider: provider, Mappings: mappings}, err
	})
}
