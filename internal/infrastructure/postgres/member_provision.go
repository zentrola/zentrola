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
			// 只能分配操作者本人所在的分组，避免借新成员获得操作者没有的模型权限。
			var active, joined bool
			if err := tx.QueryRow(ctx, `SELECT g.status='ACTIVE',EXISTS (SELECT 1 FROM principal_group_membership m
				WHERE m.group_id=g.id AND m.principal_id=$2 AND NOT m.is_deleted)
				FROM principal_group g WHERE g.id=$1 AND NOT g.is_deleted FOR SHARE OF g`, groupID, actor.ID).Scan(&active, &joined); errors.Is(err, pgx.ErrNoRows) {
				return appsec.ErrNotFound
			} else if err != nil {
				return err
			}
			if !joined {
				return appsec.ErrForbidden
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
		// 只能为本成员的 Key 创建、且未持有管理权限的成员签发 Key；不改变成员状态，
		// 管理员停用的成员不能借签发 Key 恢复启用。
		var status string
		err := tx.QueryRow(ctx, `SELECT p.status FROM principal p
			WHERE p.id=$1 AND p.id<>$2 AND p.principal_type='MEMBER' AND NOT p.is_deleted
			AND p.created_by IN (SELECT 'member-key:' || k.id FROM principal_access_key k WHERE k.principal_id=$2)
			AND NOT EXISTS (SELECT 1 FROM principal_access_key m WHERE m.principal_id=p.id AND m.can_manage_members
				AND NOT m.is_deleted AND m.status='ACTIVE' AND m.revoked_at IS NULL)
			FOR UPDATE OF p`, key.PrincipalID, actor.ID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return appsec.ErrNotFound
		}
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
	var authorized bool
	err = tx.QueryRow(ctx, `SELECT true FROM principal_access_key k JOIN principal p ON p.id=k.principal_id
		WHERE k.id=$1 AND k.principal_id=$2 AND k.can_manage_members AND NOT k.is_deleted AND k.status='ACTIVE'
		AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())
		AND p.principal_type='MEMBER' AND p.status='ACTIVE' AND NOT p.is_deleted FOR SHARE OF k,p`, actor.AccessKeyID, actor.ID).Scan(&authorized)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrForbidden
	}
	if err != nil {
		return diagnosePostgresError(ctx, s.log, "member_provision", "authorize", err, appsec.ErrUnavailable)
	}
	operator := "member-key:" + strconv.FormatInt(actor.AccessKeyID, 10)
	if err := fn(ctx, dbgen.New(tx), tx, operator); err != nil {
		if isMemberNameConflict(err) {
			return appsec.ErrMemberNameExists
		}
		for _, known := range []error{appsec.ErrNotFound, appsec.ErrInvalidArgument, appsec.ErrForbidden, appsec.ErrMemberDisabled} {
			if errors.Is(err, known) {
				return known
			}
		}
		return diagnosePostgresError(ctx, s.log, "member_provision", "write", err, appsec.ErrUnavailable)
	}
	return diagnosePostgresError(ctx, s.log, "member_provision", "commit", tx.Commit(ctx), appsec.ErrUnavailable)
}

func (s *MemberProvisionStore) insertKey(ctx context.Context, q *dbgen.Queries, key appsec.KeyRecord, operator string) error {
	return q.CreateAccessKey(ctx, dbgen.CreateAccessKeyParams{ID: key.ID, PrincipalID: key.PrincipalID, KeyHash: key.Hash, MaskedKey: key.MaskedKey, Name: key.Name, ExpiresAt: nullableTime(key.ExpiresAt), CanViewUpstreamQuota: false, CreatedBy: operator, CreatedAt: pgTime(key.CreatedAt)})
}

func (s *MemberProvisionStore) audit(ctx context.Context, q *dbgen.Queries, actor appsec.PrincipalIdentity, module string, event operation.Type, target string, targetID int64, targetName string, after any, meta appsec.RequestMeta) error {
	id, err := s.ids.NextID(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(after)
	if err != nil {
		return err
	}
	var ip *netip.Addr
	if parsed, err := netip.ParseAddr(meta.IP); err == nil {
		ip = &parsed
	}
	return q.AppendSecurityOperation(ctx, dbgen.AppendSecurityOperationParams{
		ID: id, OperatorType: string(operation.MemberKey), OperatorID: &actor.AccessKeyID, OperatorName: "Member Key #" + strconv.FormatInt(actor.AccessKeyID, 10),
		Module: module, OperationType: string(event), TargetType: target, TargetID: &targetID, TargetName: optional(targetName, 128),
		RequestID: optional(meta.RequestID, 64), RequestMethod: optional(meta.Method, 16), RequestPath: optional(meta.Path, 2048),
		IpAddress: ip, UserAgent: optional(meta.UserAgent, 1024), Result: "SUCCESS", AfterData: data, CreatedAt: pgTime(time.Now().UTC()),
	})
}
