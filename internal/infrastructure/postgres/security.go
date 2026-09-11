package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
)

type SecurityStore struct {
	pool *pgxpool.Pool
	ids  shared.IDGenerator
}

func NewSecurityStore(pool *pgxpool.Pool, ids shared.IDGenerator) *SecurityStore {
	return &SecurityStore{pool: pool, ids: ids}
}
func (s *SecurityStore) HasAdmin(ctx context.Context) (bool, error) {
	return dbgen.New(s.pool).HasActiveAdmin(ctx)
}
func (s *SecurityStore) HasAnyAdmin(ctx context.Context) (bool, error) {
	return dbgen.New(s.pool).HasAnyAdmin(ctx)
}
func (s *SecurityStore) EnsureInitial(ctx context.Context, username string, hash func() (string, error)) error {
	return s.createInitial(ctx, username, hash, appsec.RequestMeta{}, true)
}
func (s *SecurityStore) CreateFirstAdmin(ctx context.Context, username string, hash func() (string, error), meta appsec.RequestMeta) error {
	return s.createInitial(ctx, username, hash, meta, false)
}
func (s *SecurityStore) createInitial(ctx context.Context, username string, hash func() (string, error), meta appsec.RequestMeta, idempotent bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829314003)"); err != nil {
		return appsec.ErrUnavailable
	}
	q := dbgen.New(tx)
	exists, err := q.HasAnyAdmin(ctx)
	if err != nil {
		return appsec.ErrUnavailable
	}
	if exists {
		if !idempotent {
			return appsec.ErrAlreadyInitialized
		}
		if err := tx.Commit(ctx); err != nil {
			return appsec.ErrUnavailable
		}
		return nil
	}
	passwordHash, err := hash()
	if err != nil {
		return err
	}
	id, err := s.ids.NextID(ctx)
	if err != nil {
		return appsec.ErrUnavailable
	}
	if err := q.CreateInitialAdmin(ctx, dbgen.CreateInitialAdminParams{ID: id, Username: username, PasswordHash: passwordHash, CreatedAt: pgTime(time.Now().UTC())}); err != nil {
		return appsec.ErrUnavailable
	}
	if !idempotent {
		actor := admin.Identity{DisplayName: "system"}
		after, _ := json.Marshal(map[string]string{"username": username, "status": "ACTIVE"})
		if err := s.appendLog(ctx, q, actor, "AUTH", operation.AdminInitialize, "ADMIN_USER", id, username, "SUCCESS", "", meta, nil, after, ""); err != nil {
			return appsec.ErrUnavailable
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return appsec.ErrUnavailable
	}
	return nil
}
func (s *SecurityStore) GetActive(ctx context.Context, id int64) (admin.Identity, error) {
	row, err := dbgen.New(s.pool).GetActiveAdmin(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return admin.Identity{}, appsec.ErrUnauthenticated
	}
	if err != nil {
		return admin.Identity{}, appsec.ErrUnavailable
	}
	return admin.Identity{ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, CredentialVersion: row.CredentialVersion}, nil
}
func (s *SecurityStore) Attempt(ctx context.Context, username string, meta appsec.RequestMeta, evaluate func(*admin.Account) admin.LoginDecision) (admin.Identity, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return admin.Identity{}, appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	row, err := q.GetAdminForLogin(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		evaluate(nil)
		return admin.Identity{}, appsec.ErrUnauthenticated
	}
	if err != nil {
		return admin.Identity{}, appsec.ErrUnavailable
	}
	account := admin.Account{Identity: admin.Identity{ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, CredentialVersion: row.CredentialVersion}, PasswordHash: row.PasswordHash,
		Active: row.Status == "ACTIVE", FailedLogins: row.FailedLoginCount, LockedUntil: timePointer(row.LockedUntil), LastLoginAt: timePointer(row.LastLoginAt)}
	decision := evaluate(&account)
	state := decision.State
	now := time.Now().UTC()
	if err := q.SaveAdminLoginState(ctx, dbgen.SaveAdminLoginStateParams{ID: row.ID, FailedLoginCount: state.FailedLogins, LockedUntil: nullableTime(state.LockedUntil), LastLoginAt: nullableTime(state.LastLoginAt), UpdatedBy: actorRef(row.ID), UpdatedAt: pgTime(now)}); err != nil {
		return admin.Identity{}, appsec.ErrUnavailable
	}
	before, _ := json.Marshal(map[string]any{"failedLoginCount": account.FailedLogins, "lockedUntil": account.LockedUntil})
	after, _ := json.Marshal(map[string]any{"failedLoginCount": state.FailedLogins, "lockedUntil": state.LockedUntil})
	result, code := "FAILED", "UNAUTHENTICATED"
	if decision.Allowed {
		result = "SUCCESS"
		code = ""
	}
	if err := s.appendLog(ctx, q, account.Identity, "AUTH", decision.Event, "ADMIN_USER", row.ID, row.Username, result, code, meta, before, after, ""); err != nil {
		return admin.Identity{}, appsec.ErrUnavailable
	}
	// 拒绝登录也必须提交失败计数和操作日志，不能因返回认证错误而回滚。
	if err := tx.Commit(ctx); err != nil {
		return admin.Identity{}, appsec.ErrUnavailable
	}
	if !decision.Allowed {
		if decision.Event == operation.LoginLocked && state.LockedUntil != nil {
			// PostgreSQL 时间精度为微秒，首次响应与后续从数据库读取的截止时间保持一致。
			return admin.Identity{}, &appsec.AccountLockedError{LockedUntil: state.LockedUntil.Truncate(time.Microsecond)}
		}
		return admin.Identity{}, appsec.ErrUnauthenticated
	}
	return account.Identity, nil
}

func (s *SecurityStore) Create(ctx context.Context, actor admin.Identity, key appsec.KeyRecord, meta appsec.RequestMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		return err
	}
	if _, err := q.GetMemberForKey(ctx, key.PrincipalID); errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	} else if err != nil {
		return appsec.ErrUnavailable
	}
	if err := q.CreateAccessKey(ctx, dbgen.CreateAccessKeyParams{ID: key.ID, PrincipalID: key.PrincipalID, KeyHash: key.Hash, MaskedKey: key.MaskedKey, Name: key.Name, ExpiresAt: nullableTime(key.ExpiresAt), CreatedBy: actorRef(actor.ID), CreatedAt: pgTime(key.CreatedAt)}); err != nil {
		return appsec.ErrUnavailable
	}
	after, _ := json.Marshal(map[string]any{"status": "ACTIVE", "principalId": strconv.FormatInt(key.PrincipalID, 10), "maskedKey": key.MaskedKey})
	if err := s.appendLog(ctx, q, actor, "ACCESS_KEY", operation.AccessKeyCreate, "ACCESS_KEY", key.ID, key.Name, "SUCCESS", "", meta, nil, after, ""); err != nil {
		return appsec.ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return appsec.ErrUnavailable
	}
	return nil
}
func (s *SecurityStore) Revoke(ctx context.Context, actor admin.Identity, keyID int64, meta appsec.RequestMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		return err
	}
	row, err := q.GetKeyForRevoke(ctx, keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	}
	if err != nil {
		return appsec.ErrUnavailable
	}
	if row.Status == "REVOKED" {
		if err := tx.Commit(ctx); err != nil {
			return appsec.ErrUnavailable
		}
		return nil
	}
	if err := q.RevokeAccessKey(ctx, dbgen.RevokeAccessKeyParams{ID: keyID, RevokedAt: pgTime(time.Now().UTC()), UpdatedBy: actorRef(actor.ID)}); err != nil {
		return appsec.ErrUnavailable
	}
	before, _ := json.Marshal(map[string]string{"status": row.Status})
	if err := s.appendLog(ctx, q, actor, "ACCESS_KEY", operation.AccessKeyRevoke, "ACCESS_KEY", keyID, row.Name, "SUCCESS", "", meta, before, []byte(`{"status":"REVOKED"}`), ""); err != nil {
		return appsec.ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return appsec.ErrUnavailable
	}
	return nil
}
func (s *SecurityStore) Authenticate(ctx context.Context, hash []byte, now time.Time) (appsec.PrincipalIdentity, error) {
	row, err := dbgen.New(s.pool).AuthenticateAccessKey(ctx, dbgen.AuthenticateAccessKeyParams{KeyHash: hash, Now: pgTime(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.PrincipalIdentity{}, appsec.ErrUnauthenticated
	}
	if err != nil {
		return appsec.PrincipalIdentity{}, appsec.ErrUnavailable
	}
	return appsec.PrincipalIdentity{ID: row.PrincipalID, AccessKeyID: row.ID}, nil
}

// RecoverCredentials 只处理不可解密的现有资源。根密钥丢失后新生成 Key 时不尝试解密旧密文。
func (s *SecurityStore) RecoverCredentials(ctx context.Context, cipher *cryptosec.Credentials, newMaster bool) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	resources, err := q.ListResourcesForCredentialCheck(ctx)
	if err != nil {
		return 0, appsec.ErrUnavailable
	}
	deleted := 0
	for _, r := range resources {
		if !newMaster {
			plain, err := cipher.Decrypt(cryptosec.SealedCredential{Ciphertext: r.CredentialCiphertext, Nonce: r.CredentialNonce, KeyVersion: r.KeyVersion}, cryptosec.CredentialOwner{ProviderID: r.ProviderID, ResourceID: r.ID})
			clear(plain)
			if err == nil {
				continue
			}
		}
		if err := q.DeleteUnrecoverableResource(ctx, dbgen.DeleteUnrecoverableResourceParams{ID: r.ID, UpdatedAt: pgTime(time.Now().UTC())}); err != nil {
			return 0, appsec.ErrUnavailable
		}
		actor := admin.Identity{DisplayName: "system"}
		if err := s.appendLog(ctx, q, actor, "RESOURCE", operation.ResourceDelete, "RESOURCE", r.ID, r.ResourceName, "SUCCESS", "", appsec.RequestMeta{}, nil, []byte(`{"deleted":true}`), "CREDENTIAL_UNRECOVERABLE"); err != nil {
			return 0, appsec.ErrUnavailable
		}
		deleted++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, appsec.ErrUnavailable
	}
	return deleted, nil
}

func validateActor(ctx context.Context, q *dbgen.Queries, actor admin.Identity) error {
	_, err := q.GetActiveAdmin(ctx, actor.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrUnauthenticated
	}
	if err != nil {
		return appsec.ErrUnavailable
	}
	return nil
}
func (s *SecurityStore) appendLog(ctx context.Context, q *dbgen.Queries, actor admin.Identity, module string, event operation.Type, target string, targetID int64, targetName, result, code string, meta appsec.RequestMeta, before, after []byte, remark string) error {
	id, err := s.ids.NextID(ctx)
	if err != nil {
		return err
	}
	operatorType := "ADMIN"
	operatorID := &actor.ID
	if actor.ID == 0 {
		operatorType = "SYSTEM"
		operatorID = nil
	}
	var ip *netip.Addr
	if parsed, err := netip.ParseAddr(meta.IP); err == nil {
		ip = &parsed
	}
	return q.AppendSecurityOperation(ctx, dbgen.AppendSecurityOperationParams{ID: id, OperatorType: operatorType, OperatorID: operatorID, OperatorName: actor.DisplayName,
		Module: module, OperationType: string(event), TargetType: target, TargetID: &targetID, TargetName: optional(targetName, 128), RequestID: optional(meta.RequestID, 64), RequestMethod: optional(meta.Method, 16), RequestPath: optional(meta.Path, 2048), IpAddress: ip, UserAgent: optional(meta.UserAgent, 1024), Result: result, ErrorCode: optional(code, 64), BeforeData: before, AfterData: after, Remark: optional(remark, 2000), CreatedAt: pgTime(time.Now().UTC())})
}
func actorRef(id int64) string              { return "admin:" + strconv.FormatInt(id, 10) }
func pgTime(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }
func nullableTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgTime(*t)
}
func timePointer(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	value := t.Time.UTC()
	return &value
}
func optional(s string, max int) *string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ToValidUTF8(s, ""), "\x00", ""))
	if len(s) > max {
		s = s[:max]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	if s == "" {
		return nil
	}
	return &s
}
