package management

import (
	"context"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type ApplicationReader interface {
	Applications(context.Context, Page) ([]Application, error)
	CountApplications(context.Context) (int64, error)
	ApplicationSuggestions(context.Context, Page, string) ([]Application, error)
	CountApplicationSuggestions(context.Context, string) (int64, error)
	Application(context.Context, int64) (Application, error)
	ApplicationGroups(context.Context, int64, Page) ([]Group, error)
	ApplicationKeys(context.Context, int64, Page) ([]Key, error)
	CountApplicationKeys(context.Context, int64) (int64, error)
	HasUsableApplicationKey(context.Context, int64, time.Time) (bool, error)
	ApplicationOperations(context.Context, int64, Page) ([]Operation, error)
	CountApplicationOperations(context.Context, int64) (int64, error)
	Group(context.Context, int64) (Group, error)
}

type ApplicationSession interface {
	ApplicationReader
	CreateApplication(context.Context, Application) error
	UpdateApplication(context.Context, Application) error
	SetApplicationStatus(context.Context, int64, string) error
	DeleteApplication(context.Context, int64) error
	AddPrincipalTokenQuota(context.Context, int64, int64) (int64, error)
	SetGroupApplication(context.Context, int64, int64, bool) (bool, error)
	Audit(context.Context, Audit, appsec.RequestMeta) error
}

type ApplicationStore interface {
	ReadApplication(context.Context, admin.Identity, func(ApplicationReader) error) error
	WriteApplication(context.Context, admin.Identity, func(ApplicationSession) error) error
}

type ApplicationService struct {
	store ApplicationStore
	ids   shared.IDGenerator
	now   func() time.Time
}

func NewApplications(store ApplicationStore, ids shared.IDGenerator) *ApplicationService {
	return &ApplicationService{store: store, ids: ids, now: time.Now}
}

func (s *ApplicationService) Applications(ctx context.Context, actor admin.Identity, page Page) (PageData[Application], error) {
	if !validPage(page) {
		return PageData[Application]{}, appsec.ErrInvalidArgument
	}
	var result PageData[Application]
	err := s.store.ReadApplication(ctx, actor, func(r ApplicationReader) error {
		var err error
		result.Items, err = r.Applications(ctx, page)
		if err != nil {
			return err
		}
		result.Total, err = r.CountApplications(ctx)
		return err
	})
	return result, err
}

func (s *ApplicationService) Suggestions(ctx context.Context, actor admin.Identity, page Page, name string) (PageData[Application], error) {
	if !validPage(page) || !validText(name, 128) {
		return PageData[Application]{}, appsec.ErrInvalidArgument
	}
	var result PageData[Application]
	err := s.store.ReadApplication(ctx, actor, func(r ApplicationReader) error {
		var err error
		result.Items, err = r.ApplicationSuggestions(ctx, page, name)
		if err != nil {
			return err
		}
		result.Total, err = r.CountApplicationSuggestions(ctx, name)
		return err
	})
	return result, err
}

func (s *ApplicationService) Application(ctx context.Context, actor admin.Identity, id int64) (Application, error) {
	if id <= 0 {
		return Application{}, appsec.ErrInvalidArgument
	}
	var result Application
	err := s.store.ReadApplication(ctx, actor, func(r ApplicationReader) (err error) {
		result, err = r.Application(ctx, id)
		return
	})
	return result, err
}

func (s *ApplicationService) ApplicationGroups(ctx context.Context, actor admin.Identity, id int64, page Page) (PageData[Group], error) {
	if id <= 0 || !validPage(page) {
		return PageData[Group]{}, appsec.ErrInvalidArgument
	}
	var result PageData[Group]
	err := s.store.ReadApplication(ctx, actor, func(r ApplicationReader) error {
		if _, err := r.Application(ctx, id); err != nil {
			return err
		}
		var err error
		result.Items, err = r.ApplicationGroups(ctx, id, page)
		if err != nil {
			return err
		}
		// 分组数量通常很小；与成员管理沿用同一个不分页读取口径。
		all, err := r.ApplicationGroups(ctx, id, Page{Limit: 1<<31 - 1})
		result.Total = int64(len(all))
		return err
	})
	return result, err
}

func (s *ApplicationService) Keys(ctx context.Context, actor admin.Identity, id int64, page Page) (PageData[Key], error) {
	if id <= 0 || !validPage(page) {
		return PageData[Key]{}, appsec.ErrInvalidArgument
	}
	var result PageData[Key]
	err := s.store.ReadApplication(ctx, actor, func(r ApplicationReader) error {
		if _, err := r.Application(ctx, id); err != nil {
			return err
		}
		var err error
		result.Items, err = r.ApplicationKeys(ctx, id, page)
		if err != nil {
			return err
		}
		result.Total, err = r.CountApplicationKeys(ctx, id)
		return err
	})
	return result, err
}

func (s *ApplicationService) Operations(ctx context.Context, actor admin.Identity, id int64, page Page) (PageData[Operation], error) {
	if id <= 0 || !validPage(page) {
		return PageData[Operation]{}, appsec.ErrInvalidArgument
	}
	var result PageData[Operation]
	err := s.store.ReadApplication(ctx, actor, func(r ApplicationReader) error {
		var err error
		result.Items, err = r.ApplicationOperations(ctx, id, page)
		if err != nil {
			return err
		}
		result.Total, err = r.CountApplicationOperations(ctx, id)
		return err
	})
	return result, err
}

func (s *ApplicationService) Create(ctx context.Context, actor admin.Identity, name, note string, groupIDs []int64, meta appsec.RequestMeta) (Application, error) {
	if !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(groupIDs) {
		return Application{}, appsec.ErrInvalidArgument
	}
	id, err := nextID(ctx, s.ids)
	if err != nil {
		return Application{}, err
	}
	value := Application{ID: id, Name: name, Remark: remark(note), Status: "DISABLED", CreatedAt: businessTime(s.now)}
	err = s.store.WriteApplication(ctx, actor, func(w ApplicationSession) error {
		groups := make(map[int64]Group, len(groupIDs))
		for _, groupID := range groupIDs {
			group, err := w.Group(ctx, groupID)
			if err != nil {
				return err
			}
			if group.Status != "ACTIVE" {
				return ErrConflict
			}
			groups[groupID] = group
		}
		if err := w.CreateApplication(ctx, value); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.ApplicationCreate, Target: "APPLICATION", ID: id, Name: name, After: value}, meta); err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			if err := setApplicationGroup(ctx, w, groups[groupID], id, true, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return value, err
}

func (s *ApplicationService) Update(ctx context.Context, actor admin.Identity, id int64, name, note string, groupIDs []int64, meta appsec.RequestMeta) (Application, error) {
	if id <= 0 || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(groupIDs) {
		return Application{}, appsec.ErrInvalidArgument
	}
	var updated Application
	err := s.store.WriteApplication(ctx, actor, func(w ApplicationSession) error {
		current, err := w.Application(ctx, id)
		if err != nil {
			return err
		}
		currentGroups, err := w.ApplicationGroups(ctx, id, Page{Limit: 1<<31 - 1})
		if err != nil {
			return err
		}
		currentIDs := make(map[int64]Group, len(currentGroups))
		for _, group := range currentGroups {
			currentIDs[group.ID] = group
		}
		selected := make(map[int64]Group, len(groupIDs))
		for _, groupID := range groupIDs {
			group, err := w.Group(ctx, groupID)
			if err != nil {
				return err
			}
			if _, included := currentIDs[groupID]; !included && group.Status != "ACTIVE" {
				return ErrConflict
			}
			selected[groupID] = group
		}
		updated = current
		updated.Name, updated.Remark = name, remark(note)
		if err := w.UpdateApplication(ctx, updated); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.ApplicationUpdate, Target: "APPLICATION", ID: id, Name: name, Before: current, After: updated}, meta); err != nil {
			return err
		}
		for _, group := range currentGroups {
			if _, keep := selected[group.ID]; !keep {
				if err := setApplicationGroup(ctx, w, group, id, false, meta); err != nil {
					return err
				}
			}
		}
		for _, groupID := range groupIDs {
			if _, included := currentIDs[groupID]; !included {
				if err := setApplicationGroup(ctx, w, selected[groupID], id, true, meta); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return updated, err
}

func setApplicationGroup(ctx context.Context, w ApplicationSession, group Group, applicationID int64, add bool, meta appsec.RequestMeta) error {
	changed, err := w.SetGroupApplication(ctx, group.ID, applicationID, add)
	if err != nil {
		return err
	}
	if !changed {
		return ErrConflict
	}
	event := operation.GroupApplicationRemove
	if add {
		event = operation.GroupApplicationAdd
	}
	return w.Audit(ctx, Audit{Event: event, Target: "GROUP", ID: group.ID, Name: group.Name,
		Before: map[string]any{"applicationId": idString(applicationID), "included": !add},
		After:  map[string]any{"applicationId": idString(applicationID), "included": add}}, meta)
}

func (s *ApplicationService) SetStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteApplication(ctx, actor, func(w ApplicationSession) error {
		value, err := w.Application(ctx, id)
		if err != nil || value.Status == status {
			return err
		}
		if status == "ACTIVE" {
			usable, err := w.HasUsableApplicationKey(ctx, id, businessTime(s.now))
			if err != nil {
				return err
			}
			if !usable {
				return ErrApplicationKeyRequired
			}
		}
		if err := w.SetApplicationStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ApplicationStatusChange, Target: "APPLICATION", ID: id, Name: value.Name,
			Before: map[string]string{"status": value.Status}, After: map[string]string{"status": status}}, meta)
	})
}

func (s *ApplicationService) Delete(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteApplication(ctx, actor, func(w ApplicationSession) error {
		value, err := w.Application(ctx, id)
		if err != nil {
			return err
		}
		if err := w.DeleteApplication(ctx, id); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ApplicationDelete, Target: "APPLICATION", ID: id, Name: value.Name,
			Before: value, After: map[string]bool{"deleted": true}}, meta)
	})
}

func (s *ApplicationService) AddTokenQuota(ctx context.Context, actor admin.Identity, id, amount int64, reason string, meta appsec.RequestMeta) (Application, error) {
	if id <= 0 || amount <= 0 || !validText(reason, 500) {
		return Application{}, appsec.ErrInvalidArgument
	}
	var updated Application
	err := s.store.WriteApplication(ctx, actor, func(w ApplicationSession) error {
		current, err := w.Application(ctx, id)
		if err != nil {
			return err
		}
		if !validTokenQuotaAddition(current.MonthlyTokenLimit, amount) {
			return appsec.ErrInvalidArgument
		}
		limit, err := w.AddPrincipalTokenQuota(ctx, id, amount)
		if err != nil {
			return err
		}
		updated = current
		updated.MonthlyTokenLimit = &limit
		return w.Audit(ctx, Audit{Event: operation.PrincipalTokenQuotaAdd, Target: "APPLICATION", ID: id, Name: current.Name,
			Before: map[string]any{"monthlyTokenLimit": tokenQuotaBeforeValue(limit, amount)},
			After:  map[string]any{"monthlyTokenLimit": idString(limit), "amount": idString(amount), "reason": reason}}, meta)
	})
	return updated, err
}
