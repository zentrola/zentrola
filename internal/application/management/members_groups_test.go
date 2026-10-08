package management

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

type memberSessionStub struct {
	MemberSession
	member       Member
	groups       map[int64]Group
	memberGroups []Group
	created      *Member
	updated      *Member
	relations    [][3]int64
	audits       []Audit
	auditErr     error
	deleted      bool
	quotaAdds    int
}

func (s *memberSessionStub) Member(context.Context, int64) (Member, error) { return s.member, nil }
func (s *memberSessionStub) Group(_ context.Context, id int64) (Group, error) {
	group, ok := s.groups[id]
	if !ok {
		return Group{}, appsec.ErrNotFound
	}
	return group, nil
}
func (s *memberSessionStub) MemberGroups(context.Context, int64, Page) ([]Group, error) {
	return slices.Clone(s.memberGroups), nil
}
func (s *memberSessionStub) CreateMember(_ context.Context, member Member) error {
	s.created = &member
	return nil
}
func (s *memberSessionStub) UpdateMember(_ context.Context, member Member) error {
	s.updated = &member
	return nil
}
func (s *memberSessionStub) DeleteMember(context.Context, int64) error {
	s.deleted = true
	return nil
}
func (s *memberSessionStub) AddPrincipalTokenQuota(_ context.Context, _ int64, amount int64) (int64, error) {
	limit := amount
	if s.member.MonthlyTokenLimit != nil {
		limit += *s.member.MonthlyTokenLimit
	}
	s.member.MonthlyTokenLimit = &limit
	s.quotaAdds++
	return limit, nil
}
func (s *memberSessionStub) SetGroupMember(_ context.Context, groupID, memberID int64, add bool) (bool, error) {
	value := int64(0)
	if add {
		value = 1
	}
	s.relations = append(s.relations, [3]int64{groupID, memberID, value})
	return true, nil
}
func (s *memberSessionStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return s.auditErr
}

type memberStoreStub struct{ session *memberSessionStub }

func (s memberStoreStub) WriteMember(_ context.Context, _ admin.Identity, fn func(MemberSession) error) error {
	return fn(s.session)
}

func TestCreateMemberUsesInjectedUTCMicrosecondClock(t *testing.T) {
	local := time.Date(2026, 10, 2, 9, 8, 7, 654321987, time.FixedZone("CST", 8*60*60))
	session := &memberSessionStub{groups: map[int64]Group{}}
	service := MemberService{
		store: memberStoreStub{session: session}, ids: fixedMemberID{id: 42}, now: func() time.Time { return local },
	}
	created, err := service.CreateMember(context.Background(), admin.Identity{ID: 1}, "成员", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	want := local.UTC().Truncate(time.Microsecond)
	if !created.CreatedAt.Equal(want) || created.CreatedAt.Location() != time.UTC {
		t.Fatalf("createdAt=%s; want %s in UTC", created.CreatedAt, want)
	}
	if session.created == nil || !session.created.CreatedAt.Equal(want) || len(session.audits) != 1 {
		t.Fatalf("unexpected persistence: created=%+v audits=%d", session.created, len(session.audits))
	}
}

func TestUpdateMemberWithGroupsSynchronizesRelations(t *testing.T) {
	session := &memberSessionStub{
		member: Member{ID: 7, Name: "旧名称", Status: "ACTIVE"},
		groups: map[int64]Group{
			10: {ID: 10, Name: "保留", Status: "ACTIVE"},
			12: {ID: 12, Name: "新增", Status: "ACTIVE"},
		},
		memberGroups: []Group{{ID: 10, Name: "保留"}, {ID: 11, Name: "移除"}},
	}
	service := MemberService{store: memberStoreStub{session: session}}
	updated, err := service.UpdateMemberWithGroups(
		context.Background(), admin.Identity{ID: 1}, 7, "新名称", "备注", []int64{10, 12}, appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "新名称" || session.updated == nil || session.updated.Name != "新名称" {
		t.Fatalf("member was not updated: %+v", session.updated)
	}
	wantRelations := [][3]int64{{11, 7, 0}, {12, 7, 1}}
	if !slices.Equal(session.relations, wantRelations) || len(session.audits) != 3 {
		t.Fatalf("relations=%v audits=%d", session.relations, len(session.audits))
	}
}

func TestDeleteMemberPropagatesAuditFailure(t *testing.T) {
	want := errors.New("audit failed")
	session := &memberSessionStub{member: Member{ID: 7, Name: "成员"}, auditErr: want}
	service := MemberService{store: memberStoreStub{session: session}}
	err := service.DeleteMember(context.Background(), admin.Identity{ID: 1}, 7, appsec.RequestMeta{})
	if !errors.Is(err, want) || !session.deleted {
		t.Fatalf("err=%v deleted=%v", err, session.deleted)
	}
}

func TestAddMemberTokenQuotaInitializesLimitAndAuditsReason(t *testing.T) {
	session := &memberSessionStub{member: Member{ID: 7, Name: "成员"}}
	service := MemberService{store: memberStoreStub{session: session}}
	updated, err := service.AddMemberTokenQuota(
		context.Background(), admin.Identity{ID: 1}, 7, 500, "项目扩容", appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MonthlyTokenLimit == nil || *updated.MonthlyTokenLimit != 500 || session.quotaAdds != 1 {
		t.Fatalf("updated=%+v quotaAdds=%d", updated, session.quotaAdds)
	}
	if len(session.audits) != 1 || session.audits[0].Event != operation.PrincipalTokenQuotaAdd {
		t.Fatalf("audit missing: %+v", session.audits)
	}
	before := session.audits[0].Before.(map[string]any)
	after := session.audits[0].After.(map[string]any)
	if before["monthlyTokenLimit"] != nil || after["monthlyTokenLimit"] != "500" || after["reason"] != "项目扩容" {
		t.Fatalf("unexpected audit snapshots: before=%+v after=%+v", before, after)
	}
}

func TestAddMemberTokenQuotaRejectsOverflowBeforeWrite(t *testing.T) {
	limit := int64(^uint64(0) >> 1)
	session := &memberSessionStub{member: Member{ID: 7, Name: "成员", MonthlyTokenLimit: &limit}}
	service := MemberService{store: memberStoreStub{session: session}}
	_, err := service.AddMemberTokenQuota(
		context.Background(), admin.Identity{ID: 1}, 7, 1, "项目扩容", appsec.RequestMeta{},
	)
	if !errors.Is(err, appsec.ErrInvalidArgument) || session.quotaAdds != 0 || len(session.audits) != 0 {
		t.Fatalf("err=%v quotaAdds=%d audits=%d", err, session.quotaAdds, len(session.audits))
	}
}

type groupSessionStub struct {
	GroupSession
	group         Group
	models        map[int64]Model
	currentModels []Model
	created       *Group
	updated       *Group
	deleted       bool
	relations     [][3]int64
	audits        []Audit
	quotaAdds     int
}

func (s *groupSessionStub) Group(context.Context, int64) (Group, error) { return s.group, nil }
func (s *groupSessionStub) Model(_ context.Context, id int64) (Model, error) {
	model, ok := s.models[id]
	if !ok {
		return Model{}, appsec.ErrNotFound
	}
	return model, nil
}
func (s *groupSessionStub) GroupModels(context.Context, int64, Page) ([]Model, error) {
	return slices.Clone(s.currentModels), nil
}
func (s *groupSessionStub) CreateGroup(_ context.Context, group Group) error {
	s.created = &group
	return nil
}
func (s *groupSessionStub) UpdateGroup(_ context.Context, group Group) error {
	s.updated = &group
	return nil
}
func (s *groupSessionStub) DeleteGroup(context.Context, int64) error {
	s.deleted = true
	return nil
}
func (s *groupSessionStub) AddGroupTokenQuota(_ context.Context, _ int64, amount int64) (int64, error) {
	limit := amount
	if s.group.MonthlyTokenLimit != nil {
		limit += *s.group.MonthlyTokenLimit
	}
	s.group.MonthlyTokenLimit = &limit
	s.quotaAdds++
	return limit, nil
}
func (s *groupSessionStub) SetGroupModel(_ context.Context, groupID, modelID int64, grant bool) (bool, error) {
	value := int64(0)
	if grant {
		value = 1
	}
	s.relations = append(s.relations, [3]int64{groupID, modelID, value})
	return true, nil
}
func (s *groupSessionStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

type groupStoreStub struct{ session *groupSessionStub }

func (s groupStoreStub) WriteGroup(_ context.Context, _ admin.Identity, fn func(GroupSession) error) error {
	return fn(s.session)
}

func TestCreateGroupWithModelsRejectsInactiveModelBeforeWrite(t *testing.T) {
	session := &groupSessionStub{models: map[int64]Model{9: {ID: 9, Status: "DISABLED"}}}
	service := GroupService{store: groupStoreStub{session: session}, ids: fixedMemberID{id: 22}}
	_, err := service.CreateGroupWithModels(
		context.Background(), admin.Identity{ID: 1}, "", "分组", "", []int64{9}, appsec.RequestMeta{},
	)
	if !errors.Is(err, ErrConflict) || session.created != nil || len(session.audits) != 0 {
		t.Fatalf("err=%v created=%+v audits=%d", err, session.created, len(session.audits))
	}
}

func TestCreateGroupWithModelsPersistsRelationsAndAudits(t *testing.T) {
	session := &groupSessionStub{models: map[int64]Model{9: {ID: 9, Status: "ACTIVE"}}}
	service := GroupService{store: groupStoreStub{session: session}, ids: fixedMemberID{id: 22}}
	created, err := service.CreateGroupWithModels(
		context.Background(), admin.Identity{ID: 1}, "", "分组", "", []int64{9}, appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Code != "group-22" || session.created == nil ||
		!slices.Equal(session.relations, [][3]int64{{22, 9, 1}}) || len(session.audits) != 2 {
		t.Fatalf("created=%+v relations=%v audits=%d", session.created, session.relations, len(session.audits))
	}
}

func TestUpdateGroupWithModelsSynchronizesRelations(t *testing.T) {
	session := &groupSessionStub{
		group: Group{ID: 22, Code: "group-22", Name: "旧分组", Status: "ACTIVE"},
		models: map[int64]Model{
			9:  {ID: 9, Status: "ACTIVE"},
			11: {ID: 11, Status: "ACTIVE"},
		},
		currentModels: []Model{{ID: 9}, {ID: 10}},
	}
	service := GroupService{store: groupStoreStub{session: session}}
	updated, err := service.UpdateGroupWithModels(
		context.Background(), admin.Identity{ID: 1}, 22, "新分组", "", []int64{9, 11}, appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantRelations := [][3]int64{{22, 10, 0}, {22, 11, 1}}
	if updated.Name != "新分组" || session.updated == nil ||
		!slices.Equal(session.relations, wantRelations) || len(session.audits) != 3 {
		t.Fatalf("updated=%+v relations=%v audits=%d", session.updated, session.relations, len(session.audits))
	}
}

func TestDeleteGroupAuditsDeletion(t *testing.T) {
	session := &groupSessionStub{group: Group{ID: 22, Name: "分组"}}
	service := GroupService{store: groupStoreStub{session: session}}
	if err := service.DeleteGroup(context.Background(), admin.Identity{ID: 1}, 22, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if !session.deleted || len(session.audits) != 1 {
		t.Fatalf("deleted=%v audits=%d", session.deleted, len(session.audits))
	}
}

func TestAddGroupTokenQuotaPreservesPreviousLimitInAudit(t *testing.T) {
	limit := int64(1000)
	session := &groupSessionStub{group: Group{ID: 22, Name: "分组", MonthlyTokenLimit: &limit}}
	service := GroupService{store: groupStoreStub{session: session}}
	updated, err := service.AddGroupTokenQuota(
		context.Background(), admin.Identity{ID: 1}, 22, 250, "季度扩容", appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MonthlyTokenLimit == nil || *updated.MonthlyTokenLimit != 1250 || session.quotaAdds != 1 {
		t.Fatalf("updated=%+v quotaAdds=%d", updated, session.quotaAdds)
	}
	if len(session.audits) != 1 || session.audits[0].Event != operation.GroupTokenQuotaAdd {
		t.Fatalf("audit missing: %+v", session.audits)
	}
	before := session.audits[0].Before.(map[string]any)
	if before["monthlyTokenLimit"] != "1000" {
		t.Fatalf("unexpected previous quota: %+v", before)
	}
}
