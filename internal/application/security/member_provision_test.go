package security

import (
	"context"
	"errors"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/principal"
)

type provisionStoreStub struct {
	creates  int
	issues   int
	statuses int
	changed  bool
	key      KeyRecord
	groups   []int64
}
type provisionIDs struct{ next int64 }

func (s *provisionIDs) NextID(context.Context) (int64, error) { s.next++; return s.next, nil }
func (s *provisionStoreStub) CreateMemberAndKey(_ context.Context, _ PrincipalIdentity, memberID int64, _ string, _ string, groups []int64, key KeyRecord, _ RequestMeta) error {
	s.creates++
	if key.PrincipalID != memberID {
		return errors.New("key belongs to a different member")
	}
	s.groups = groups
	s.key = key
	return nil
}
func (s *provisionStoreStub) IssueMemberKey(_ context.Context, _ PrincipalIdentity, key KeyRecord, _ RequestMeta) error {
	s.issues++
	s.key = key
	return nil
}

func (s *provisionStoreStub) SetMemberStatus(context.Context, PrincipalIdentity, int64, string, RequestMeta) (bool, error) {
	s.statuses++
	return s.changed, nil
}

type provisionCacheStub struct{ clears int }

func (c *provisionCacheStub) Clear(context.Context, string) bool { c.clears++; return true }

func TestMemberProvisioningRequiresIndividualPermissionAndIssuesOrdinaryKey(t *testing.T) {
	store := &provisionStoreStub{}
	keys := NewKeys(&keyTestStore{}, &provisionIDs{next: 40})
	service := NewMemberProvisioning(keys, store, nil)
	actor := PrincipalIdentity{ID: 1, AccessKeyID: 2, Type: principal.Member}
	if _, err := service.Create(context.Background(), actor, "New", "", "First", nil, RequestMeta{}); !errors.Is(err, ErrForbidden) || store.creates != 0 {
		t.Fatalf("unauthorized create: err=%v writes=%d", err, store.creates)
	}
	actor.CanManageMembers = true
	created, err := service.Create(context.Background(), actor, "New", "", "First", []int64{7}, RequestMeta{})
	if err != nil || created.Status != "ACTIVE" || created.Key.Key == "" || store.creates != 1 || store.key.CanManageMembers || store.key.CanViewUpstreamQuota || store.key.PrincipalType != principal.Member {
		t.Fatalf("created=%+v record=%+v err=%v", created, store.key, err)
	}
	if _, err := service.IssueKey(context.Background(), actor, 99, "More", RequestMeta{}); err != nil || store.issues != 1 || store.key.CanManageMembers {
		t.Fatalf("issue err=%v writes=%d record=%+v", err, store.issues, store.key)
	}
}

func TestMemberProvisioningRejectsInvalidGroupsBeforeWriting(t *testing.T) {
	store := &provisionStoreStub{}
	service := NewMemberProvisioning(NewKeys(&keyTestStore{}, &provisionIDs{next: 40}), store, nil)
	actor := PrincipalIdentity{ID: 1, AccessKeyID: 2, Type: principal.Member, CanManageMembers: true}
	if _, err := service.Create(context.Background(), actor, "New", "", "First", []int64{7, 7}, RequestMeta{}); !errors.Is(err, ErrInvalidArgument) || store.creates != 0 {
		t.Fatalf("invalid groups: err=%v writes=%d", err, store.creates)
	}
}

func TestMemberProvisioningSetStatus(t *testing.T) {
	manager := PrincipalIdentity{ID: 1, AccessKeyID: 2, Type: principal.Member, CanManageMembers: true}
	for _, tc := range []struct {
		name       string
		actor      PrincipalIdentity
		memberID   int64
		status     string
		changed    bool
		wantErr    error
		wantWrites int
		wantClears int
	}{
		{name: "without permission", actor: PrincipalIdentity{ID: 1, AccessKeyID: 2, Type: principal.Member}, memberID: 9, status: "DISABLED", wantErr: ErrForbidden},
		{name: "application key", actor: PrincipalIdentity{ID: 1, AccessKeyID: 2, Type: principal.Application, CanManageMembers: true}, memberID: 9, status: "DISABLED", wantErr: ErrUnauthenticated},
		{name: "invalid member", actor: manager, memberID: 0, status: "DISABLED", wantErr: ErrInvalidArgument},
		{name: "invalid status", actor: manager, memberID: 9, status: "DELETED", wantErr: ErrInvalidArgument},
		{name: "disable clears identity cache", actor: manager, memberID: 9, status: "DISABLED", changed: true, wantWrites: 1, wantClears: 1},
		{name: "unchanged keeps cache", actor: manager, memberID: 9, status: "ACTIVE", wantWrites: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, cache := &provisionStoreStub{changed: tc.changed}, &provisionCacheStub{}
			service := NewMemberProvisioning(NewKeys(&keyTestStore{}, &provisionIDs{next: 40}), store, cache)
			result, err := service.SetStatus(context.Background(), tc.actor, tc.memberID, tc.status, RequestMeta{})
			if !errors.Is(err, tc.wantErr) || store.statuses != tc.wantWrites || cache.clears != tc.wantClears {
				t.Fatalf("err=%v writes=%d clears=%d", err, store.statuses, cache.clears)
			}
			if tc.wantErr == nil && (result.ID != tc.memberID || result.Status != tc.status) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}
