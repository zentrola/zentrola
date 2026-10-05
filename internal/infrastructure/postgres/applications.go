package postgres

import (
	"context"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func (s *ManagementStore) ReadApplication(ctx context.Context, actor admin.Identity, fn func(mgmt.ApplicationReader) error) error {
	return s.run(ctx, actor, false, func(session *managementSession) error { return fn(session) })
}

func (s *ManagementStore) WriteApplication(ctx context.Context, actor admin.Identity, fn func(mgmt.ApplicationSession) error) error {
	return s.run(ctx, actor, true, func(session *managementSession) error { return fn(session) })
}

func (s *managementSession) Applications(ctx context.Context, page mgmt.Page) ([]mgmt.Application, error) {
	rows, err := s.q.ManageApplications(ctx, dbgen.ManageApplicationsParams{ID: page.After, Limit: managementPageLimit(page)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Application, 0, len(rows))
	for _, row := range rows {
		result = append(result, mgmt.Application(memberView(row)))
	}
	return result, nil
}

func (s *managementSession) CountApplications(ctx context.Context) (int64, error) {
	return s.q.CountManageApplications(ctx)
}

func (s *managementSession) ApplicationSuggestions(ctx context.Context, page mgmt.Page, name string) ([]mgmt.Application, error) {
	rows, err := s.q.ManageApplicationSuggestions(ctx, dbgen.ManageApplicationSuggestionsParams{ApplicationName: name,
		AfterID: page.After, PageLimit: managementPageLimit(page)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Application, 0, len(rows))
	for _, row := range rows {
		result = append(result, mgmt.Application(memberView(row)))
	}
	return result, nil
}

func (s *managementSession) CountApplicationSuggestions(ctx context.Context, name string) (int64, error) {
	return s.q.CountManageApplicationSuggestions(ctx, name)
}

func (s *managementSession) Application(ctx context.Context, id int64) (mgmt.Application, error) {
	row, err := s.q.ManageApplication(ctx, id)
	return mgmt.Application(memberView(row)), err
}

func (s *managementSession) ApplicationGroups(ctx context.Context, id int64, page mgmt.Page) ([]mgmt.Group, error) {
	return s.MemberGroups(ctx, id, page)
}

func (s *managementSession) ApplicationKeys(ctx context.Context, id int64, page mgmt.Page) ([]mgmt.Key, error) {
	return s.Keys(ctx, id, page)
}

func (s *managementSession) CountApplicationKeys(ctx context.Context, id int64) (int64, error) {
	return s.CountKeys(ctx, id)
}

func (s *managementSession) HasUsableApplicationKey(ctx context.Context, id int64, now time.Time) (bool, error) {
	return s.q.ManageHasUsableApplicationKey(ctx, dbgen.ManageHasUsableApplicationKeyParams{
		PrincipalID: id, Now: pgTime(now),
	})
}

func (s *managementSession) ApplicationOperations(ctx context.Context, id int64, page mgmt.Page) ([]mgmt.Operation, error) {
	rows, err := s.q.ManageApplicationOperations(ctx, dbgen.ManageApplicationOperationsParams{ApplicationID: &id,
		AfterID: page.After, PageLimit: managementPageLimit(page)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Operation, 0, len(rows))
	for _, row := range rows {
		result = append(result, mgmt.Operation{ID: row.ID, OperatorName: row.OperatorName, Type: row.OperationType,
			TargetType: row.TargetType, TargetID: row.TargetID, TargetName: row.TargetName, RequestID: row.RequestID,
			Result: row.Result, ErrorCode: row.ErrorCode, Before: row.BeforeData, After: row.AfterData,
			CreatedAt: row.CreatedAt.Time.UTC()})
	}
	return result, nil
}

func (s *managementSession) CountApplicationOperations(ctx context.Context, id int64) (int64, error) {
	return s.q.CountManageApplicationOperations(ctx, &id)
}

func (s *managementSession) CreateApplication(ctx context.Context, value mgmt.Application) error {
	return s.q.ManageCreateApplication(ctx, dbgen.ManageCreateApplicationParams{ID: value.ID, Name: value.Name,
		Remark: value.Remark, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(value.CreatedAt)})
}

func (s *managementSession) UpdateApplication(ctx context.Context, value mgmt.Application) error {
	return s.q.ManageUpdateApplication(ctx, dbgen.ManageUpdateApplicationParams{ID: value.ID, Name: value.Name,
		Remark: value.Remark, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}

func (s *managementSession) SetApplicationStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageApplicationStatus(ctx, dbgen.ManageApplicationStatusParams{ID: id, Status: status,
		UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}

func (s *managementSession) DeleteApplication(ctx context.Context, id int64) error {
	now, actor := pgTime(time.Now().UTC()), actorRef(s.actor.ID)
	if err := s.q.ManageDeleteApplication(ctx, dbgen.ManageDeleteApplicationParams{ID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteMemberGroups(ctx, dbgen.ManageDeleteMemberGroupsParams{PrincipalID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	return s.q.ManageRevokeMemberKeys(ctx, dbgen.ManageRevokeMemberKeysParams{PrincipalID: id, UpdatedBy: actor, RevokedAt: now})
}

func (s *managementSession) SetGroupApplication(ctx context.Context, groupID, applicationID int64, add bool) (bool, error) {
	return s.SetGroupMember(ctx, groupID, applicationID, add)
}
