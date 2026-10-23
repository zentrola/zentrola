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
	SetMemberStatus(context.Context, PrincipalIdentity, int64, string, RequestMeta) (bool, error)
	ActiveGroups(context.Context, PrincipalIdentity, int64, int32) ([]ManagedGroup, int64, error)
}

// ManagedGroup 是管理 Key 创建成员时可选择的分组，只暴露选择所需字段。
type ManagedGroup struct {
	ID     int64   `json:"id,string"`
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Remark *string `json:"remark"`
	Status string  `json:"status"`
}

type MemberProvisioning struct {
	keys  *Keys
	store MemberProvisionStore
	cache interface {
		Clear(context.Context, string) bool
	}
}

type ManagedMemberStatus struct {
	ID     int64  `json:"id,string"`
	Status string `json:"status"`
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
	// 至少指定一个分组，否则新成员没有任何模型权限，与管理后台创建规则一致。
	if !validProvisionText(name, 128, true) || !validProvisionText(remark, 2000, false) || !validProvisionText(keyName, 128, true) || len(groupIDs) == 0 || !validGroupIDs(groupIDs) {
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

// SetStatus 启停用普通成员。停用只让该成员的 Key 无法认证，不注销 Key；重新启用后原 Key 恢复可用。
func (s *MemberProvisioning) SetStatus(ctx context.Context, actor PrincipalIdentity, memberID int64, status string, meta RequestMeta) (ManagedMemberStatus, error) {
	if err := validateMemberManager(actor); err != nil {
		return ManagedMemberStatus{}, err
	}
	if memberID <= 0 || (status != "ACTIVE" && status != "DISABLED") {
		return ManagedMemberStatus{}, ErrInvalidArgument
	}
	if s == nil || s.store == nil {
		return ManagedMemberStatus{}, ErrUnavailable
	}
	changed, err := s.store.SetMemberStatus(ctx, actor, memberID, status, meta)
	if err != nil {
		return ManagedMemberStatus{}, err
	}
	// 停用必须让已缓存的成员身份立即失效。
	if changed && s.cache != nil {
		s.cache.Clear(ctx, "member_status_changed")
	}
	return ManagedMemberStatus{ID: memberID, Status: status}, nil
}

// Groups 分页返回启用分组；limit 由调用方决定是否多取一条用于判断下一页。
func (s *MemberProvisioning) Groups(ctx context.Context, actor PrincipalIdentity, after int64, limit int32) ([]ManagedGroup, int64, error) {
	if err := validateMemberManager(actor); err != nil {
		return nil, 0, err
	}
	if after < 0 || limit < 1 || limit > 101 {
		return nil, 0, ErrInvalidArgument
	}
	if s == nil || s.store == nil {
		return nil, 0, ErrUnavailable
	}
	return s.store.ActiveGroups(ctx, actor, after, limit)
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
