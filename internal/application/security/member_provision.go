package security

import (
	"context"
	"strings"

	"github.com/zentrola/zentrola/internal/domain/principal"
)

// MemberProvisionStore keeps member activation and first-key issuance atomic.
type MemberProvisionStore interface {
	CreateMemberAndKey(context.Context, PrincipalIdentity, int64, string, string, []int64, KeyRecord, RequestMeta) error
	IssueMemberKey(context.Context, PrincipalIdentity, KeyRecord, RequestMeta) error
}

type MemberProvisioning struct {
	keys  *Keys
	store MemberProvisionStore
	cache interface {
		Clear(context.Context, string) bool
	}
}

type ProvisionedMember struct {
	ID     int64      `json:"id,string"`
	Name   string     `json:"name"`
	Status string     `json:"status"`
	Key    CreatedKey `json:"key"`
}

func NewMemberProvisioning(keys *Keys, store MemberProvisionStore, cache interface {
	Clear(context.Context, string) bool
}) *MemberProvisioning {
	return &MemberProvisioning{keys: keys, store: store, cache: cache}
}

func (s *MemberProvisioning) Create(ctx context.Context, actor PrincipalIdentity, name, remark, keyName string, groupIDs []int64, meta RequestMeta) (ProvisionedMember, error) {
	if err := validateMemberManager(actor); err != nil {
		return ProvisionedMember{}, err
	}
	name, remark, keyName = strings.TrimSpace(name), strings.TrimSpace(remark), strings.TrimSpace(keyName)
	if !validProvisionText(name, 128, true) || !validProvisionText(remark, 2000, false) || !validProvisionText(keyName, 128, true) || !validGroupIDs(groupIDs) {
		return ProvisionedMember{}, ErrInvalidArgument
	}
	if s == nil || s.keys == nil || s.store == nil {
		return ProvisionedMember{}, ErrUnavailable
	}
	memberID, err := s.keys.ids.NextID(ctx)
	if err != nil {
		return ProvisionedMember{}, ErrUnavailable
	}
	row, created, err := s.keys.prepare(ctx, memberID, principal.Member, keyName, nil, false)
	if err != nil {
		return ProvisionedMember{}, err
	}
	if err := s.store.CreateMemberAndKey(ctx, actor, memberID, name, remark, groupIDs, row, meta); err != nil {
		return ProvisionedMember{}, err
	}
	if s.cache != nil {
		s.cache.Clear(ctx, "member_provisioned")
	}
	return ProvisionedMember{ID: memberID, Name: name, Status: "ACTIVE", Key: created}, nil
}

func (s *MemberProvisioning) IssueKey(ctx context.Context, actor PrincipalIdentity, memberID int64, keyName string, meta RequestMeta) (CreatedKey, error) {
	if err := validateMemberManager(actor); err != nil {
		return CreatedKey{}, err
	}
	if s == nil || s.keys == nil || s.store == nil {
		return CreatedKey{}, ErrUnavailable
	}
	row, created, err := s.keys.prepare(ctx, memberID, principal.Member, strings.TrimSpace(keyName), nil, false)
	if err != nil {
		return CreatedKey{}, err
	}
	if err := s.store.IssueMemberKey(ctx, actor, row, meta); err != nil {
		return CreatedKey{}, err
	}
	if s.cache != nil {
		s.cache.Clear(ctx, "member_key_issued")
	}
	return created, nil
}

func validateMemberManager(actor PrincipalIdentity) error {
	if actor.ID <= 0 || actor.AccessKeyID <= 0 || actor.Type != principal.Member {
		return ErrUnauthenticated
	}
	if !actor.CanManageMembers {
		return ErrForbidden
	}
	return nil
}

func validProvisionText(value string, limit int, required bool) bool {
	if required && value == "" {
		return false
	}
	return len(value) <= limit && !strings.ContainsRune(value, 0)
}

func validGroupIDs(ids []int64) bool {
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
