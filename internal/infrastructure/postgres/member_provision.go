package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/netip"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type MemberProvisionStore struct {
	pool *pgxpool.Pool
	ids  interface {
		NextID(context.Context) (int64, error)
	}
	log *slog.Logger
}

func NewMemberProvisionStore(pool *pgxpool.Pool, ids interface {
	NextID(context.Context) (int64, error)
}, log *slog.Logger) *MemberProvisionStore {
	return &MemberProvisionStore{pool: pool, ids: ids, log: log}
}

func (s *MemberProvisionStore) CreateMemberAndKey(ctx context.Context, actor appsec.PrincipalIdentity, memberID int64, name, remark string, groupIDs []int64, key appsec.KeyRecord, meta appsec.RequestMeta) error {
	return s.write(ctx, actor, func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx, operator string) error {
		now := time.Now().UTC()
		if err := q.ManageCreateMember(ctx, dbgen.ManageCreateMemberParams{ID: memberID, Name: name, Remark: optional(remark, 2000), CreatedBy: operator, CreatedAt: pgTime(now)}); err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			var active bool
			if err := tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM principal_group WHERE id=$1 AND NOT is_deleted FOR SHARE`, groupID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
				return appsec.ErrNotFound
			} else if err != nil {
				return err
			}
			if !active {
				return appsec.ErrInvalidArgument
			}
			id, err := s.ids.NextID(ctx)
			if err != nil {
				return err
			}
			if err := q.ManageAddMember(ctx, dbgen.ManageAddMemberParams{ID: id, GroupID: groupID, PrincipalID: memberID, CreatedBy: operator, CreatedAt: pgTime(now)}); err != nil {
				return err
			}
			if err := s.audit(ctx, q, actor, "GROUP", operation.GroupMemberAdd, "GROUP", groupID, "", map[string]any{"memberId": strconv.FormatInt(memberID, 10), "included": true}, meta); err != nil {
				return err
			}
		}
		if err := s.insertKey(ctx, q, key, operator); err != nil {
			return err
		}
		if err := q.ManageMemberStatus(ctx, dbgen.ManageMemberStatusParams{ID: memberID, Status: "ACTIVE", UpdatedBy: operator, UpdatedAt: pgTime(now)}); err != nil {
			return err
		}
		if err := s.audit(ctx, q, actor, "MEMBER", operation.MemberCreate, "PRINCIPAL", memberID, name, map[string]any{"status": "ACTIVE", "principalType": "MEMBER"}, meta); err != nil {
			return err
		}
		return s.audit(ctx, q, actor, "ACCESS_KEY", operation.AccessKeyCreate, "ACCESS_KEY", key.ID, key.Name, map[string]any{"status": "ACTIVE", "principalId": strconv.FormatInt(memberID, 10), "maskedKey": key.MaskedKey, "canManageMembers": false, "canViewUpstreamQuota": false}, meta)
	})
}

func (s *MemberProvisionStore) IssueMemberKey(ctx context.Context, actor appsec.PrincipalIdentity, key appsec.KeyRecord, meta appsec.RequestMeta) error {
	return s.write(ctx, actor, func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx, operator string) error {
		// 签发 Key 不改变成员状态；停用成员需先启用再签发。
		_, status, err := lockManagedMember(ctx, tx, key.PrincipalID, actor.ID)
		if err != nil {
			return err
		}
		if status != "ACTIVE" {
			return appsec.ErrMemberDisabled
		}
		if err := s.insertKey(ctx, q, key, operator); err != nil {
			return err
		}
		return s.audit(ctx, q, actor, "ACCESS_KEY", operation.AccessKeyCreate, "ACCESS_KEY", key.ID, key.Name, map[string]any{"status": "ACTIVE", "principalId": strconv.FormatInt(key.PrincipalID, 10), "maskedKey": key.MaskedKey, "canManageMembers": false, "canViewUpstreamQuota": false}, meta)
	})
}

// SetMemberStatus 启停用管理范围内的普通成员；状态未变化时不写审计并返回 false。
func (s *MemberProvisionStore) SetMemberStatus(ctx context.Context, actor appsec.PrincipalIdentity, memberID int64, status string, meta appsec.RequestMeta) (bool, error) {
	changed := false
	err := s.write(ctx, actor, func(ctx context.Context, q *dbgen.Queries, tx pgx.Tx, operator string) error {
		name, current, err := lockManagedMember(ctx, tx, memberID, actor.ID)
		if err != nil {
			return err
		}
		if current == status {
			return nil
		}
		if status == "ACTIVE" {
			// 与管理员启用规则一致：成员至少持有一把有效 Key 才能启用。
			var hasKey bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM principal_access_key WHERE principal_id=$1 AND NOT is_deleted
				AND status='ACTIVE' AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()))`, memberID).Scan(&hasKey); err != nil {
				return err
			}
			if !hasKey {
				return appsec.ErrMemberAccessKeyRequired
			}
		}
		if err := q.ManageMemberStatus(ctx, dbgen.ManageMemberStatusParams{ID: memberID, Status: status, UpdatedBy: operator, UpdatedAt: pgTime(time.Now().UTC())}); err != nil {
			return err
		}
		changed = true
		return s.auditChange(ctx, q, actor, "MEMBER", operation.MemberStatusChange, "PRINCIPAL", memberID, name, map[string]string{"status": current}, map[string]string{"status": status}, meta)
	})
	return changed, err
}

// lockManagedMember 锁定管理 Key 可操作的普通成员：系统内任意未删除成员，
// 但不包括操作者本人和持有有效管理授权 Key 的成员，避免管理 Key 之间相互操作。
func lockManagedMember(ctx context.Context, tx pgx.Tx, memberID, actorID int64) (name, status string, err error) {
	err = tx.QueryRow(ctx, `SELECT p.name,p.status FROM principal p
		WHERE p.id=$1 AND p.id<>$2 AND p.principal_type='MEMBER' AND NOT p.is_deleted
		AND NOT EXISTS (SELECT 1 FROM principal_access_key m WHERE m.principal_id=p.id AND m.can_manage_members
			AND NOT m.is_deleted AND m.status='ACTIVE' AND m.revoked_at IS NULL AND (m.expires_at IS NULL OR m.expires_at>now()))
		FOR UPDATE OF p`, memberID, actorID).Scan(&name, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", appsec.ErrNotFound
	}
	return name, status, err
}

func (s *MemberProvisionStore) write(ctx context.Context, actor appsec.PrincipalIdentity, fn func(context.Context, *dbgen.Queries, pgx.Tx, string) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return diagnosePostgresError(ctx, s.log, "member_provision", "begin", err, appsec.ErrUnavailable)
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(829314004)`); err != nil {
		return diagnosePostgresError(ctx, s.log, "member_provision", "lock", err, appsec.ErrUnavailable)
	}
	if err := authorizeManager(ctx, tx, actor, true); err != nil {
		if errors.Is(err, appsec.ErrForbidden) {
			return err
		}
		return diagnosePostgresError(ctx, s.log, "member_provision", "authorize", err, appsec.ErrUnavailable)
	}
	operator := "member-key:" + strconv.FormatInt(actor.AccessKeyID, 10)
	if err := fn(ctx, dbgen.New(tx), tx, operator); err != nil {
		if isMemberNameConflict(err) {
			return appsec.ErrMemberNameExists
		}
		for _, known := range []error{appsec.ErrNotFound, appsec.ErrInvalidArgument, appsec.ErrForbidden, appsec.ErrMemberDisabled, appsec.ErrMemberAccessKeyRequired} {
			if errors.Is(err, known) {
				return known
			}
		}
		return diagnosePostgresError(ctx, s.log, "member_provision", "write", err, appsec.ErrUnavailable)
	}
	return diagnosePostgresError(ctx, s.log, "member_provision", "commit", tx.Commit(ctx), appsec.ErrUnavailable)
}

// ActiveGroups 返回可在创建成员时选择的启用分组，按 ID 倒序分页。
func (s *MemberProvisionStore) ActiveGroups(ctx context.Context, actor appsec.PrincipalIdentity, after int64, limit int32) ([]appsec.ManagedGroup, int64, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, 0, diagnosePostgresError(ctx, s.log, "member_provision", "begin_read", err, appsec.ErrUnavailable)
	}
	defer tx.Rollback(context.Background())
	if err := authorizeManager(ctx, tx, actor, false); err != nil {
		if errors.Is(err, appsec.ErrForbidden) {
			return nil, 0, err
		}
		return nil, 0, diagnosePostgresError(ctx, s.log, "member_provision", "authorize", err, appsec.ErrUnavailable)
	}
	q := dbgen.New(tx)
	rows, err := q.ManageGroups(ctx, dbgen.ManageGroupsParams{Status: "ACTIVE", AfterID: after, PageLimit: limit})
	if err != nil {
		return nil, 0, diagnosePostgresError(ctx, s.log, "member_provision", "groups", err, appsec.ErrUnavailable)
	}
	total, err := q.CountManageGroups(ctx, "ACTIVE")
	if err != nil {
		return nil, 0, diagnosePostgresError(ctx, s.log, "member_provision", "count_groups", err, appsec.ErrUnavailable)
	}
	groups := make([]appsec.ManagedGroup, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, appsec.ManagedGroup{ID: row.ID, Code: row.GroupCode, Name: row.GroupName, Remark: row.Remark, Status: row.Status})
	}
	return groups, total, nil
}

// authorizeManager 在数据库中复核管理 Key 仍然有效，避免只依赖可能过期的认证缓存。
// 写事务加共享锁，防止授权在本次写入期间被撤销；只读事务不能加锁。
func authorizeManager(ctx context.Context, tx pgx.Tx, actor appsec.PrincipalIdentity, lock bool) error {
	query := `SELECT true FROM principal_access_key k JOIN principal p ON p.id=k.principal_id
		WHERE k.id=$1 AND k.principal_id=$2 AND k.can_manage_members AND NOT k.is_deleted AND k.status='ACTIVE'
		AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())
		AND p.principal_type='MEMBER' AND p.status='ACTIVE' AND NOT p.is_deleted`
	if lock {
		query += ` FOR SHARE OF k,p`
	}
	var authorized bool
	err := tx.QueryRow(ctx, query, actor.AccessKeyID, actor.ID).Scan(&authorized)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrForbidden
	}
	return err
}

func (s *MemberProvisionStore) insertKey(ctx context.Context, q *dbgen.Queries, key appsec.KeyRecord, operator string) error {
	return q.CreateAccessKey(ctx, dbgen.CreateAccessKeyParams{ID: key.ID, PrincipalID: key.PrincipalID, KeyHash: key.Hash, MaskedKey: key.MaskedKey, Name: key.Name, ExpiresAt: nullableTime(key.ExpiresAt), CanViewUpstreamQuota: false, CreatedBy: operator, CreatedAt: pgTime(key.CreatedAt)})
}

func (s *MemberProvisionStore) audit(ctx context.Context, q *dbgen.Queries, actor appsec.PrincipalIdentity, module string, event operation.Type, target string, targetID int64, targetName string, after any, meta appsec.RequestMeta) error {
	return s.auditChange(ctx, q, actor, module, event, target, targetID, targetName, nil, after, meta)
}

func (s *MemberProvisionStore) auditChange(ctx context.Context, q *dbgen.Queries, actor appsec.PrincipalIdentity, module string, event operation.Type, target string, targetID int64, targetName string, before, after any, meta appsec.RequestMeta) error {
	id, err := s.ids.NextID(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(after)
	if err != nil {
		return err
	}
	var beforeData []byte
	if before != nil {
		if beforeData, err = json.Marshal(before); err != nil {
			return err
		}
	}
	var ip *netip.Addr
	if parsed, err := netip.ParseAddr(meta.IP); err == nil {
		ip = &parsed
	}
	return q.AppendSecurityOperation(ctx, dbgen.AppendSecurityOperationParams{
		ID: id, OperatorType: string(operation.MemberKey), OperatorID: &actor.AccessKeyID, OperatorName: "Member Key #" + strconv.FormatInt(actor.AccessKeyID, 10),
		Module: module, OperationType: string(event), TargetType: target, TargetID: &targetID, TargetName: optional(targetName, 128),
		RequestID: optional(meta.RequestID, 64), RequestMethod: optional(meta.Method, 16), RequestPath: optional(meta.Path, 2048),
		IpAddress: ip, UserAgent: optional(meta.UserAgent, 1024), Result: "SUCCESS", BeforeData: beforeData, AfterData: data, CreatedAt: pgTime(time.Now().UTC()),
	})
}
