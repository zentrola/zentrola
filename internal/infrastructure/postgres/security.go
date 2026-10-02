package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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
	pool   *pgxpool.Pool
	ids    shared.IDGenerator
	logger *slog.Logger
}

func NewSecurityStore(pool *pgxpool.Pool, ids shared.IDGenerator, loggers ...*slog.Logger) *SecurityStore {
	return &SecurityStore{pool: pool, ids: ids, logger: optionalLogger(loggers)}
}
func (s *SecurityStore) HasAdmin(ctx context.Context) (bool, error) {
	value, err := dbgen.New(s.pool).HasActiveAdmin(ctx)
	return value, s.securityError(ctx, "has_active_admin", err)
}
func (s *SecurityStore) HasAnyAdmin(ctx context.Context) (bool, error) {
	value, err := dbgen.New(s.pool).HasAnyAdmin(ctx)
	return value, s.securityError(ctx, "has_any_admin", err)
}

func (s *SecurityStore) securityError(ctx context.Context, operation string, err error) error {
	return diagnosePostgresError(ctx, s.logger, "security", operation, err, appsec.ErrUnavailable)
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
		return s.securityError(ctx, "begin_create_initial", err)
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829314003)"); err != nil {
		return s.securityError(ctx, "lock_create_initial", err)
	}
	q := dbgen.New(tx)
	exists, err := q.HasAnyAdmin(ctx)
	if err != nil {
		return s.securityError(ctx, "check_existing_admin", err)
	}
	if exists {
		if !idempotent {
			return appsec.ErrAlreadyInitialized
		}
		if err := tx.Commit(ctx); err != nil {
			return s.securityError(ctx, "commit_existing_admin", err)
		}
		return nil
	}
	passwordHash, err := hash()
	if err != nil {
		return err
	}
	id, err := s.ids.NextID(ctx)
	if err != nil {
		return s.securityError(ctx, "generate_initial_admin_id", err)
	}
	if err := q.CreateInitialAdmin(ctx, dbgen.CreateInitialAdminParams{ID: id, Username: username, PasswordHash: passwordHash, CreatedAt: pgTime(time.Now().UTC())}); err != nil {
		return s.securityError(ctx, "create_initial_admin", err)
	}
	if !idempotent {
		actor := admin.Identity{DisplayName: "system"}
		after, _ := json.Marshal(map[string]string{"username": username, "status": "ACTIVE"})
		if err := s.appendLog(ctx, q, actor, "AUTH", operation.AdminInitialize, "ADMIN_USER", id, username, "SUCCESS", "", meta, nil, after, ""); err != nil {
			return s.securityError(ctx, "audit_initial_admin", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return s.securityError(ctx, "commit_create_initial", err)
	}
	return nil
}
func (s *SecurityStore) GetActive(ctx context.Context, id int64) (admin.Identity, error) {
	row, err := dbgen.New(s.pool).GetActiveAdmin(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return admin.Identity{}, appsec.ErrUnauthenticated
	}
	if err != nil {
		return admin.Identity{}, s.securityError(ctx, "get_active_admin", err)
	}
	return admin.Identity{ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, CredentialVersion: row.CredentialVersion}, nil
}
func (s *SecurityStore) Attempt(ctx context.Context, username string, meta appsec.RequestMeta, evaluate func(*admin.Account) admin.LoginDecision) (admin.Identity, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return admin.Identity{}, s.securityError(ctx, "begin_login_attempt", err)
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
		return admin.Identity{}, s.securityError(ctx, "get_admin_for_login", err)
	}
	account := admin.Account{Identity: admin.Identity{ID: row.ID, Username: row.Username, DisplayName: row.DisplayName, CredentialVersion: row.CredentialVersion}, PasswordHash: row.PasswordHash,
		Active: row.Status == "ACTIVE", FailedLogins: row.FailedLoginCount, LockedUntil: timePointer(row.LockedUntil), LastLoginAt: timePointer(row.LastLoginAt)}
	decision := evaluate(&account)
	state := decision.State
	now := time.Now().UTC()
	if err := q.SaveAdminLoginState(ctx, dbgen.SaveAdminLoginStateParams{ID: row.ID, FailedLoginCount: state.FailedLogins, LockedUntil: nullableTime(state.LockedUntil), LastLoginAt: nullableTime(state.LastLoginAt), UpdatedBy: actorRef(row.ID), UpdatedAt: pgTime(now)}); err != nil {
		return admin.Identity{}, s.securityError(ctx, "save_admin_login_state", err)
	}
	before, _ := json.Marshal(map[string]any{"failedLoginCount": account.FailedLogins, "lockedUntil": account.LockedUntil})
	after, _ := json.Marshal(map[string]any{"failedLoginCount": state.FailedLogins, "lockedUntil": state.LockedUntil})
	result, code := "FAILED", "UNAUTHENTICATED"
	if decision.Allowed {
		result = "SUCCESS"
		code = ""
	}
	if err := s.appendLog(ctx, q, account.Identity, "AUTH", decision.Event, "ADMIN_USER", row.ID, row.Username, result, code, meta, before, after, ""); err != nil {
		return admin.Identity{}, s.securityError(ctx, "audit_login_attempt", err)
	}
	// 拒绝登录也必须提交失败计数和操作日志，不能因返回认证错误而回滚。
	if err := tx.Commit(ctx); err != nil {
		return admin.Identity{}, s.securityError(ctx, "commit_login_attempt", err)
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
		return s.securityError(ctx, "begin_create_access_key", err)
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		if errors.Is(err, appsec.ErrUnauthenticated) {
			return err
		}
		return s.securityError(ctx, "validate_access_key_actor", err)
	}
	if _, err := q.GetMemberForKey(ctx, key.PrincipalID); errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	} else if err != nil {
		return s.securityError(ctx, "get_member_for_key", err)
	}
	if err := q.CreateAccessKey(ctx, dbgen.CreateAccessKeyParams{ID: key.ID, PrincipalID: key.PrincipalID, KeyHash: key.Hash, MaskedKey: key.MaskedKey, Name: key.Name, ExpiresAt: nullableTime(key.ExpiresAt), CreatedBy: actorRef(actor.ID), CreatedAt: pgTime(key.CreatedAt)}); err != nil {
		return s.securityError(ctx, "create_access_key", err)
	}
	after, _ := json.Marshal(map[string]any{"status": "ACTIVE", "principalId": strconv.FormatInt(key.PrincipalID, 10), "maskedKey": key.MaskedKey})
	if err := s.appendLog(ctx, q, actor, "ACCESS_KEY", operation.AccessKeyCreate, "ACCESS_KEY", key.ID, key.Name, "SUCCESS", "", meta, nil, after, ""); err != nil {
		return s.securityError(ctx, "audit_create_access_key", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return s.securityError(ctx, "commit_create_access_key", err)
	}
	return nil
}
func (s *SecurityStore) Revoke(ctx context.Context, actor admin.Identity, keyID int64, meta appsec.RequestMeta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return s.securityError(ctx, "begin_revoke_access_key", err)
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	if err := validateActor(ctx, q, actor); err != nil {
		if errors.Is(err, appsec.ErrUnauthenticated) {
			return err
		}
		return s.securityError(ctx, "validate_revoke_actor", err)
	}
	row, err := q.GetKeyForRevoke(ctx, keyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	}
	if err != nil {
		return s.securityError(ctx, "get_key_for_revoke", err)
	}
	if row.Status == "REVOKED" {
		if err := tx.Commit(ctx); err != nil {
			return s.securityError(ctx, "commit_already_revoked_key", err)
		}
		return nil
	}
	if err := q.RevokeAccessKey(ctx, dbgen.RevokeAccessKeyParams{ID: keyID, RevokedAt: pgTime(time.Now().UTC()), UpdatedBy: actorRef(actor.ID)}); err != nil {
		return s.securityError(ctx, "revoke_access_key", err)
	}
	before, _ := json.Marshal(map[string]string{"status": row.Status})
	if err := s.appendLog(ctx, q, actor, "ACCESS_KEY", operation.AccessKeyRevoke, "ACCESS_KEY", keyID, row.Name, "SUCCESS", "", meta, before, []byte(`{"status":"REVOKED"}`), ""); err != nil {
		return s.securityError(ctx, "audit_revoke_access_key", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return s.securityError(ctx, "commit_revoke_access_key", err)
	}
	return nil
}
func (s *SecurityStore) Authenticate(ctx context.Context, hash []byte, now time.Time) (appsec.PrincipalIdentity, error) {
	row, err := dbgen.New(s.pool).AuthenticateAccessKey(ctx, dbgen.AuthenticateAccessKeyParams{KeyHash: hash, Now: pgTime(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.PrincipalIdentity{}, appsec.ErrUnauthenticated
	}
	if err != nil {
		return appsec.PrincipalIdentity{}, s.securityError(ctx, "authenticate_access_key", err)
	}
	return appsec.PrincipalIdentity{ID: row.PrincipalID, AccessKeyID: row.ID, ExpiresAt: timePointer(row.ExpiresAt)}, nil
}

// ValidateCredentials 确认 MASTER_KEY 可以解密全部现有凭据，失败时不修改数据。
func (s *SecurityStore) ValidateCredentials(ctx context.Context, cipher *cryptosec.Credentials) error {
	resources, err := dbgen.New(s.pool).ListResourcesForCredentialCheck(ctx)
	if err != nil {
		return s.securityError(ctx, "list_credentials_for_validation", err)
	}
	for _, r := range resources {
		plain, err := cipher.Decrypt(cryptosec.SealedCredential{Ciphertext: r.CredentialCiphertext, Nonce: r.CredentialNonce, KeyVersion: r.KeyVersion}, cryptosec.CredentialOwner{ProviderID: r.ProviderID, ResourceID: r.ID})
		clear(plain)
		if err != nil {
			return errors.New("MASTER_KEY cannot decrypt existing provider credentials")
		}
	}
	return nil
}

func validateActor(ctx context.Context, q *dbgen.Queries, actor admin.Identity) error {
	_, err := q.GetActiveAdmin(ctx, actor.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrUnauthenticated
	}
	if err != nil {
		return err
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
