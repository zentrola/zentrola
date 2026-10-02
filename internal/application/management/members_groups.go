package management

import (
	"context"
	"strconv"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type MemberService struct {
	store MemberStore
	ids   shared.IDGenerator
	now   func() time.Time
}

type MemberSession interface {
	Member(context.Context, int64) (Member, error)
	Group(context.Context, int64) (Group, error)
	MemberGroups(context.Context, int64, Page) ([]Group, error)
	Keys(context.Context, int64, Page) ([]Key, error)
	CreateMember(context.Context, Member) error
	UpdateMember(context.Context, Member) error
	SetMemberStatus(context.Context, int64, string) error
	DeleteMember(context.Context, int64) error
	SetGroupMember(context.Context, int64, int64, bool) (bool, error)
	Audit(context.Context, Audit, appsec.RequestMeta) error
}

type MemberStore interface {
	WriteMember(context.Context, admin.Identity, func(MemberSession) error) error
}

type memberStoreAdapter struct{ Store }

func (s memberStoreAdapter) WriteMember(ctx context.Context, actor admin.Identity, fn func(MemberSession) error) error {
	if dependencyMissing(s.Store) {
		return appsec.ErrUnavailable
	}
	return s.Write(ctx, actor, func(writer Writer) error { return fn(writer) })
}

func (s *MemberService) next(ctx context.Context) (int64, error) {
	return nextID(ctx, s.ids)
}

type GroupService struct {
	store GroupStore
	ids   shared.IDGenerator
	now   func() time.Time
}

type GroupSession interface {
	Group(context.Context, int64) (Group, error)
	Member(context.Context, int64) (Member, error)
	Model(context.Context, int64) (Model, error)
	GroupModels(context.Context, int64, Page) ([]Model, error)
	CreateGroup(context.Context, Group) error
	UpdateGroup(context.Context, Group) error
	SetGroupStatus(context.Context, int64, string) error
	DeleteGroup(context.Context, int64) error
	SetGroupMember(context.Context, int64, int64, bool) (bool, error)
	SetGroupModel(context.Context, int64, int64, bool) (bool, error)
	Audit(context.Context, Audit, appsec.RequestMeta) error
}

type GroupStore interface {
	WriteGroup(context.Context, admin.Identity, func(GroupSession) error) error
}

type groupStoreAdapter struct{ Store }

func (s groupStoreAdapter) WriteGroup(ctx context.Context, actor admin.Identity, fn func(GroupSession) error) error {
	if dependencyMissing(s.Store) {
		return appsec.ErrUnavailable
	}
	return s.Write(ctx, actor, func(writer Writer) error { return fn(writer) })
}

func (s *GroupService) next(ctx context.Context) (int64, error) {
	return nextID(ctx, s.ids)
}

func (s *MemberService) CreateMember(ctx context.Context, actor admin.Identity, name, note string, meta appsec.RequestMeta) (Member, error) {
	return s.CreateMemberWithGroups(ctx, actor, name, note, nil, meta)
}
func (s *MemberService) CreateMemberWithGroups(ctx context.Context, actor admin.Identity, name, note string, groupIDs []int64, meta appsec.RequestMeta) (Member, error) {
	if !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(groupIDs) {
		return Member{}, appsec.ErrInvalidArgument
	}
	id, err := s.next(ctx)
	if err != nil {
		return Member{}, err
	}
	m := Member{ID: id, Name: name, Remark: remark(note), Status: "DISABLED", CreatedAt: businessTime(s.now)}
	err = s.store.WriteMember(ctx, actor, func(w MemberSession) error {
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
		if err := w.CreateMember(ctx, m); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.MemberCreate, Target: "PRINCIPAL", ID: id, Name: name, After: m}, meta); err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			changed, err := w.SetGroupMember(ctx, groupID, id, true)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			group := groups[groupID]
			if err := w.Audit(ctx, Audit{Event: operation.GroupMemberAdd, Target: "GROUP", ID: groupID, Name: group.Name, Before: map[string]any{"memberId": idString(id), "included": false}, After: map[string]any{"memberId": idString(id), "included": true}}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return m, err
}
func (s *MemberService) UpdateMemberWithGroups(ctx context.Context, actor admin.Identity, id int64, name, note string, groupIDs []int64, meta appsec.RequestMeta) (Member, error) {
	if id <= 0 || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(groupIDs) {
		return Member{}, appsec.ErrInvalidArgument
	}
	var updated Member
	err := s.store.WriteMember(ctx, actor, func(w MemberSession) error {
		current, err := w.Member(ctx, id)
		if err != nil {
			return err
		}
		currentGroups, err := w.MemberGroups(ctx, id, Page{Limit: 1<<31 - 1})
		if err != nil {
			return err
		}
		currentIDs := make(map[int64]Group, len(currentGroups))
		for _, group := range currentGroups {
			currentIDs[group.ID] = group
		}
		selectedGroups := make(map[int64]Group, len(groupIDs))
		for _, groupID := range groupIDs {
			group, err := w.Group(ctx, groupID)
			if err != nil {
				return err
			}
			if _, alreadyIncluded := currentIDs[groupID]; !alreadyIncluded && group.Status != "ACTIVE" {
				return ErrConflict
			}
			selectedGroups[groupID] = group
		}

		updated = current
		updated.Name = name
		updated.Remark = remark(note)
		if err := w.UpdateMember(ctx, updated); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.MemberUpdate, Target: "PRINCIPAL", ID: id, Name: name, Before: current, After: updated}, meta); err != nil {
			return err
		}
		for _, group := range currentGroups {
			if _, keep := selectedGroups[group.ID]; keep {
				continue
			}
			changed, err := w.SetGroupMember(ctx, group.ID, id, false)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			if err := w.Audit(ctx, Audit{Event: operation.GroupMemberRemove, Target: "GROUP", ID: group.ID, Name: group.Name, Before: map[string]any{"memberId": idString(id), "included": true}, After: map[string]any{"memberId": idString(id), "included": false}}, meta); err != nil {
				return err
			}
		}
		for _, groupID := range groupIDs {
			if _, exists := currentIDs[groupID]; exists {
				continue
			}
			changed, err := w.SetGroupMember(ctx, groupID, id, true)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			group := selectedGroups[groupID]
			if err := w.Audit(ctx, Audit{Event: operation.GroupMemberAdd, Target: "GROUP", ID: groupID, Name: group.Name, Before: map[string]any{"memberId": idString(id), "included": false}, After: map[string]any{"memberId": idString(id), "included": true}}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}
func (s *MemberService) SetMemberStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteMember(ctx, actor, func(w MemberSession) error {
		m, err := w.Member(ctx, id)
		if err != nil {
			return err
		}
		if m.Status == status {
			return nil
		}
		if status == "ACTIVE" {
			keys, err := w.Keys(ctx, id, Page{Limit: 1})
			if err != nil {
				return err
			}
			if len(keys) == 0 {
				return ErrMemberAccessKeyRequired
			}
		}
		if err := w.SetMemberStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.MemberStatusChange, Target: "PRINCIPAL", ID: id, Name: m.Name, Before: map[string]string{"status": m.Status}, After: map[string]string{"status": status}}, meta)
	})
}
func (s *MemberService) DeleteMember(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteMember(ctx, actor, func(w MemberSession) error {
		m, err := w.Member(ctx, id)
		if err != nil {
			return err
		}
		if err := w.DeleteMember(ctx, id); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.MemberDelete, Target: "PRINCIPAL", ID: id, Name: m.Name, Before: m, After: map[string]bool{"deleted": true}}, meta)
	})
}
func (s *GroupService) CreateGroup(ctx context.Context, actor admin.Identity, code, name, note string, meta appsec.RequestMeta) (Group, error) {
	return s.createGroup(ctx, actor, code, name, note, nil, meta)
}
func (s *GroupService) CreateGroupWithModels(ctx context.Context, actor admin.Identity, code, name, note string, modelIDs []int64, meta appsec.RequestMeta) (Group, error) {
	return s.createGroup(ctx, actor, code, name, note, modelIDs, meta)
}
func (s *GroupService) createGroup(ctx context.Context, actor admin.Identity, code, name, note string, modelIDs []int64, meta appsec.RequestMeta) (Group, error) {
	if (code != "" && !validText(code, 64)) || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(modelIDs) {
		return Group{}, appsec.ErrInvalidArgument
	}
	id, err := s.next(ctx)
	if err != nil {
		return Group{}, err
	}
	if code == "" {
		code = "group-" + strconv.FormatInt(id, 10)
	}
	g := Group{ID: id, Code: code, Name: name, Remark: remark(note), Status: "ACTIVE", CreatedAt: businessTime(s.now)}
	err = s.store.WriteGroup(ctx, actor, func(w GroupSession) error {
		for _, modelID := range modelIDs {
			model, err := w.Model(ctx, modelID)
			if err != nil {
				return err
			}
			if model.Status != "ACTIVE" {
				return ErrConflict
			}
		}
		if err := w.CreateGroup(ctx, g); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.GroupCreate, Target: "GROUP", ID: id, Name: name, After: g}, meta); err != nil {
			return err
		}
		for _, modelID := range modelIDs {
			changed, err := w.SetGroupModel(ctx, id, modelID, true)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			if err := w.Audit(ctx, Audit{Event: operation.GroupModelGrant, Target: "GROUP", ID: id, Name: name, Before: map[string]any{"modelId": idString(modelID), "allowed": false}, After: map[string]any{"modelId": idString(modelID), "allowed": true}}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return g, err
}
func uniquePositiveIDs(ids []int64) bool {
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}
func (s *GroupService) UpdateGroupWithModels(ctx context.Context, actor admin.Identity, id int64, name, note string, modelIDs []int64, meta appsec.RequestMeta) (Group, error) {
	if id <= 0 || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(modelIDs) {
		return Group{}, appsec.ErrInvalidArgument
	}
	var updated Group
	err := s.store.WriteGroup(ctx, actor, func(w GroupSession) error {
		current, err := w.Group(ctx, id)
		if err != nil {
			return err
		}
		currentModels, err := w.GroupModels(ctx, id, Page{Limit: 1<<31 - 1})
		if err != nil {
			return err
		}
		currentIDs := make(map[int64]struct{}, len(currentModels))
		for _, model := range currentModels {
			currentIDs[model.ID] = struct{}{}
		}
		selectedIDs := make(map[int64]struct{}, len(modelIDs))
		for _, modelID := range modelIDs {
			model, err := w.Model(ctx, modelID)
			if err != nil {
				return err
			}
			_, alreadyGranted := currentIDs[modelID]
			if !alreadyGranted && (model.Status != "ACTIVE" || current.Status != "ACTIVE") {
				return ErrConflict
			}
			selectedIDs[modelID] = struct{}{}
		}

		updated = current
		updated.Name = name
		updated.Remark = remark(note)
		if err := w.UpdateGroup(ctx, updated); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.GroupUpdate, Target: "GROUP", ID: id, Name: name, Before: current, After: updated}, meta); err != nil {
			return err
		}
		for _, model := range currentModels {
			if _, keep := selectedIDs[model.ID]; keep {
				continue
			}
			if changed, err := w.SetGroupModel(ctx, id, model.ID, false); err != nil {
				return err
			} else if changed {
				if err := w.Audit(ctx, Audit{Event: operation.GroupModelRevoke, Target: "GROUP", ID: id, Name: name, Before: map[string]any{"modelId": idString(model.ID), "allowed": true}, After: map[string]any{"modelId": idString(model.ID), "allowed": false}}, meta); err != nil {
					return err
				}
			}
		}
		for _, modelID := range modelIDs {
			if _, exists := currentIDs[modelID]; exists {
				continue
			}
			if changed, err := w.SetGroupModel(ctx, id, modelID, true); err != nil {
				return err
			} else if changed {
				if err := w.Audit(ctx, Audit{Event: operation.GroupModelGrant, Target: "GROUP", ID: id, Name: name, Before: map[string]any{"modelId": idString(modelID), "allowed": false}, After: map[string]any{"modelId": idString(modelID), "allowed": true}}, meta); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return updated, err
}
func (s *GroupService) SetGroupStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteGroup(ctx, actor, func(w GroupSession) error {
		group, err := w.Group(ctx, id)
		if err != nil {
			return err
		}
		if group.Status == status {
			return nil
		}
		if err := w.SetGroupStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.GroupStatusChange, Target: "GROUP", ID: id, Name: group.Name, Before: map[string]string{"status": group.Status}, After: map[string]string{"status": status}}, meta)
	})
}
func (s *GroupService) DeleteGroup(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteGroup(ctx, actor, func(w GroupSession) error {
		group, err := w.Group(ctx, id)
		if err != nil {
			return err
		}
		if err := w.DeleteGroup(ctx, id); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.GroupDelete, Target: "GROUP", ID: id, Name: group.Name, Before: group, After: map[string]bool{"deleted": true}}, meta)
	})
}
func (s *GroupService) SetGroupMember(ctx context.Context, actor admin.Identity, groupID, memberID int64, add bool, meta appsec.RequestMeta) error {
	if groupID <= 0 || memberID <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteGroup(ctx, actor, func(w GroupSession) error {
		g, err := w.Group(ctx, groupID)
		if err != nil {
			return err
		}
		if _, err := w.Member(ctx, memberID); err != nil {
			return err
		}
		if add && g.Status != "ACTIVE" {
			return ErrConflict
		}
		changed, err := w.SetGroupMember(ctx, groupID, memberID, add)
		if err != nil || !changed {
			return err
		}
		event := operation.GroupMemberRemove
		if add {
			event = operation.GroupMemberAdd
		}
		return w.Audit(ctx, Audit{Event: event, Target: "GROUP", ID: groupID, Name: g.Name, Before: map[string]any{"memberId": idString(memberID), "included": !add}, After: map[string]any{"memberId": idString(memberID), "included": add}}, meta)
	})
}
func (s *GroupService) SetGroupModel(ctx context.Context, actor admin.Identity, groupID, modelID int64, grant bool, meta appsec.RequestMeta) error {
	if groupID <= 0 || modelID <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteGroup(ctx, actor, func(w GroupSession) error {
		g, err := w.Group(ctx, groupID)
		if err != nil {
			return err
		}
		m, err := w.Model(ctx, modelID)
		if err != nil {
			return err
		}
		if grant && (g.Status != "ACTIVE" || m.Status != "ACTIVE") {
			return ErrConflict
		}
		changed, err := w.SetGroupModel(ctx, groupID, modelID, grant)
		if err != nil || !changed {
			return err
		}
		event := operation.GroupModelRevoke
		if grant {
			event = operation.GroupModelGrant
		}
		return w.Audit(ctx, Audit{Event: event, Target: "GROUP", ID: groupID, Name: g.Name, Before: map[string]any{"modelId": idString(modelID), "allowed": !grant}, After: map[string]any{"modelId": idString(modelID), "allowed": grant}}, meta)
	})
}
