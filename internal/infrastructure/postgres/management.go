package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/shared"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type ManagementStore struct {
	pool   *pgxpool.Pool
	ids    shared.IDGenerator
	audit  *SecurityStore
	logger *slog.Logger
}

func NewManagementStore(pool *pgxpool.Pool, ids shared.IDGenerator, loggers ...*slog.Logger) *ManagementStore {
	logger := optionalLogger(loggers)
	return &ManagementStore{pool: pool, ids: ids, audit: NewSecurityStore(pool, ids, logger), logger: logger}
}

type managementSession struct {
	q     *dbgen.Queries
	actor admin.Identity
	store *ManagementStore
}

func (s *ManagementStore) Read(ctx context.Context, a admin.Identity, fn func(mgmt.Reader) error) error {
	return s.run(ctx, a, false, func(session *managementSession) error { return fn(session) })
}
func (s *ManagementStore) Write(ctx context.Context, a admin.Identity, fn func(mgmt.Writer) error) error {
	return s.run(ctx, a, true, func(session *managementSession) error { return fn(session) })
}
func (s *ManagementStore) run(ctx context.Context, a admin.Identity, write bool, fn func(*managementSession) error) error {
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if write {
		opts = pgx.TxOptions{}
	}
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return s.managementError(ctx, "begin_transaction", err)
	}
	defer tx.Rollback(context.Background())
	ctx = idgen.WithQuerier(ctx, tx)
	q := dbgen.New(tx)
	if write {
		// 单一私有部署内的管理写入串行化；不在此事务中执行上游网络请求。
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829314004)"); err != nil {
			return s.managementError(ctx, "acquire_management_lock", err)
		}
	}
	err = validateActor(ctx, q, a)
	if err != nil {
		return s.managementError(ctx, "validate_actor", err)
	}
	if err = fn(&managementSession{q: q, actor: a, store: s}); err != nil {
		return s.managementError(ctx, "execute_callback", err)
	}
	return s.managementError(ctx, "commit_transaction", tx.Commit(ctx))
}

func (s *ManagementStore) managementError(ctx context.Context, operation string, err error) error {
	mapped := managementError(err)
	if err != nil && errors.Is(mapped, appsec.ErrUnavailable) && !errors.Is(err, appsec.ErrUnavailable) {
		return diagnosePostgresError(ctx, s.logger, "management", operation, err, mapped)
	}
	return mapped
}
func managementError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return appsec.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" && pgErr.ConstraintName == "uk_provider_credential_subscription_account" {
			return mgmt.ErrSubscriptionAccountExists
		}
		switch pgErr.Code {
		case "23505", "40001", "40P01":
			return mgmt.ErrConflict
		case "22001":
			return appsec.ErrInvalidArgument
		}
	}
	for _, known := range []error{appsec.ErrInvalidArgument, appsec.ErrUnauthenticated, appsec.ErrNotFound, appsec.ErrUnavailable} {
		if errors.Is(err, known) {
			return known
		}
	}
	if public, ok := mgmt.PublicError(err); ok {
		return public
	}
	return appsec.ErrUnavailable
}
func (s *managementSession) Audit(ctx context.Context, a mgmt.Audit, meta appsec.RequestMeta) error {
	var before, after []byte
	var err error
	if a.Before != nil {
		before, err = json.Marshal(a.Before)
		if err != nil {
			return err
		}
	}
	if a.After != nil {
		after, err = json.Marshal(a.After)
		if err != nil {
			return err
		}
	}
	result := "SUCCESS"
	if a.ErrorCode != "" {
		result = "FAILED"
	}
	module := a.Target
	if a.Target == "PRINCIPAL" {
		module = "MEMBER"
	}
	return s.store.audit.appendLog(ctx, s.q, s.actor, module, a.Event, a.Target, a.ID, a.Name, result, a.ErrorCode, meta, before, after, "")
}
func (s *managementSession) providerEndpoints(ctx context.Context, providerID int64) ([]mgmt.ProviderEndpoint, error) {
	rows, err := s.q.ManageProviderEndpoints(ctx, providerID)
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.ProviderEndpoint, 0, len(rows))
	for _, row := range rows {
		result = append(result, endpointView(row))
	}
	return result, nil
}
func (s *managementSession) Members(ctx context.Context, p mgmt.Page) ([]mgmt.Member, error) {
	rows, err := s.q.ManageMembers(ctx, dbgen.ManageMembersParams{ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Member, 0, len(rows))
	for _, row := range rows {
		result = append(result, memberView(row))
	}
	return result, nil
}
func (s *managementSession) MemberSuggestions(ctx context.Context, p mgmt.Page, name string) ([]mgmt.Member, error) {
	rows, err := s.q.ManageMemberSuggestions(ctx, dbgen.ManageMemberSuggestionsParams{MemberName: name, AfterID: p.After, PageLimit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Member, 0, len(rows))
	for _, row := range rows {
		result = append(result, memberView(row))
	}
	return result, nil
}
func (s *managementSession) Groups(ctx context.Context, p mgmt.Page, status string) ([]mgmt.Group, error) {
	rows, err := s.q.ManageGroups(ctx, dbgen.ManageGroupsParams{Status: status, AfterID: p.After, PageLimit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Group, 0, len(rows))
	for _, row := range rows {
		result = append(result, groupView(row))
	}
	return result, nil
}
func (s *managementSession) Models(ctx context.Context, p mgmt.Page, status string) ([]mgmt.Model, error) {
	rows, err := s.q.ManageModels(ctx, dbgen.ManageModelsParams{Status: status, AfterID: p.After, PageLimit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Model, 0, len(rows))
	for _, row := range rows {
		result = append(result, modelView(row.ID, row.ModelCode, row.DisplayName, row.Status,
			row.InputModalities, row.OutputModalities, row.Remark, row.PublisherProviderID,
			row.PublisherProviderName, row.CreatedAt.Time, row.UpdatedAt.Time))
	}
	return result, nil
}
func (s *managementSession) Providers(ctx context.Context, p mgmt.Page, providerType string) ([]mgmt.Provider, error) {
	rows, err := s.q.ManageProviders(ctx, dbgen.ManageProvidersParams{ProviderType: providerType, AfterID: p.After, PageLimit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	providerIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		providerIDs = append(providerIDs, row.ID)
	}
	modelCounts := make(map[int64]int64, len(providerIDs))
	endpoints := make(map[int64][]mgmt.ProviderEndpoint, len(providerIDs))
	if len(providerIDs) > 0 {
		countRows, err := s.q.ManageProviderModelCounts(ctx, providerIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range countRows {
			modelCounts[row.ProviderID] = row.ModelCount
		}
		endpointRows, err := s.q.ManageProviderEndpointsByProviders(ctx, providerIDs)
		if err != nil {
			return nil, err
		}
		for _, row := range endpointRows {
			endpoints[row.ProviderID] = append(endpoints[row.ProviderID], endpointView(row))
		}
	}
	result := make([]mgmt.Provider, 0, len(rows))
	for _, row := range rows {
		provider := providerView(row)
		provider.ModelCount = modelCounts[provider.ID]
		if values := endpoints[provider.ID]; values != nil {
			provider.Endpoints = values
		}
		result = append(result, provider)
	}
	return result, nil
}
func (s *managementSession) ProviderMappings(ctx context.Context, providerID int64) ([]mgmt.ProviderMapping, error) {
	rows, err := s.q.ManageProviderMappings(ctx, providerID)
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.ProviderMapping, 0, len(rows))
	for _, row := range rows {
		result = append(result, providerMappingView(row))
	}
	return result, nil
}
func (s *managementSession) ProviderCredentialConfigured(ctx context.Context, providerID int64) (bool, error) {
	return s.q.ManageProviderCredentialConfigured(ctx, providerID)
}
func (s *managementSession) Resources(ctx context.Context, p mgmt.Page) ([]mgmt.Resource, error) {
	rows, err := s.q.ManageResources(ctx, dbgen.ManageResourcesParams{ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Resource, 0, len(rows))
	for _, row := range rows {
		result = append(result, resourceView(row))
	}
	return result, nil
}
func (s *managementSession) Operations(ctx context.Context, p mgmt.Page) ([]mgmt.Operation, error) {
	rows, err := s.q.ManageOperations(ctx, dbgen.ManageOperationsParams{ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Operation, 0, len(rows))
	for _, row := range rows {
		result = append(result, operationView(row))
	}
	return result, nil
}
func (s *managementSession) GroupMembers(ctx context.Context, id int64, p mgmt.Page) ([]mgmt.Member, error) {
	rows, err := s.q.ManageGroupMembers(ctx, dbgen.ManageGroupMembersParams{GroupID: id, ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Member, 0, len(rows))
	for _, row := range rows {
		result = append(result, memberView(row))
	}
	return result, nil
}
func (s *managementSession) MemberGroups(ctx context.Context, id int64, p mgmt.Page) ([]mgmt.Group, error) {
	rows, err := s.q.ManageMemberGroups(ctx, dbgen.ManageMemberGroupsParams{PrincipalID: id, ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Group, 0, len(rows))
	for _, row := range rows {
		result = append(result, groupView(row))
	}
	return result, nil
}
func (s *managementSession) GroupModels(ctx context.Context, id int64, p mgmt.Page) ([]mgmt.Model, error) {
	rows, err := s.q.ManageGroupModels(ctx, dbgen.ManageGroupModelsParams{GroupID: id, ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Model, 0, len(rows))
	for _, row := range rows {
		result = append(result, modelView(row.ID, row.ModelCode, row.DisplayName, row.Status,
			row.InputModalities, row.OutputModalities, row.Remark, row.PublisherProviderID,
			row.PublisherProviderName, row.CreatedAt.Time, row.UpdatedAt.Time))
	}
	return result, nil
}
func (s *managementSession) Keys(ctx context.Context, id int64, p mgmt.Page) ([]mgmt.Key, error) {
	rows, err := s.q.ManageKeys(ctx, dbgen.ManageKeysParams{PrincipalID: id, ID: p.After, Limit: managementPageLimit(p)})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Key, 0, len(rows))
	for _, row := range rows {
		result = append(result, keyView(row))
	}
	return result, nil
}
func (s *managementSession) CountMembers(ctx context.Context) (int64, error) {
	return s.q.CountManageMembers(ctx)
}
func (s *managementSession) CountMemberSuggestions(ctx context.Context, name string) (int64, error) {
	return s.q.CountManageMemberSuggestions(ctx, name)
}
func (s *managementSession) CountGroups(ctx context.Context, status string) (int64, error) {
	return s.q.CountManageGroups(ctx, status)
}
func (s *managementSession) CountModels(ctx context.Context, status string) (int64, error) {
	return s.q.CountManageModels(ctx, status)
}
func (s *managementSession) CountProviders(ctx context.Context, providerType string) (int64, error) {
	return s.q.CountManageProviders(ctx, providerType)
}
func (s *managementSession) CountResources(ctx context.Context) (int64, error) {
	return s.q.CountManageResources(ctx)
}
func (s *managementSession) CountOperations(ctx context.Context) (int64, error) {
	return s.q.CountManageOperations(ctx)
}
func (s *managementSession) CountGroupMembers(ctx context.Context, id int64) (int64, error) {
	return s.q.CountManageGroupMembers(ctx, id)
}
func (s *managementSession) CountMemberGroups(ctx context.Context, id int64) (int64, error) {
	return s.q.CountManageMemberGroups(ctx, id)
}
func (s *managementSession) CountGroupModels(ctx context.Context, id int64) (int64, error) {
	return s.q.CountManageGroupModels(ctx, id)
}
func (s *managementSession) CountKeys(ctx context.Context, id int64) (int64, error) {
	return s.q.CountManageKeys(ctx, id)
}
func (s *managementSession) Member(ctx context.Context, id int64) (mgmt.Member, error) {
	row, err := s.q.ManageMember(ctx, id)
	return memberView(row), err
}
func (s *managementSession) Group(ctx context.Context, id int64) (mgmt.Group, error) {
	row, err := s.q.ManageGroup(ctx, id)
	return groupView(row), err
}
func (s *managementSession) Model(ctx context.Context, id int64) (mgmt.Model, error) {
	row, err := s.q.ManageModel(ctx, id)
	return modelView(row.ID, row.ModelCode, row.DisplayName, row.Status,
		row.InputModalities, row.OutputModalities, row.Remark, row.PublisherProviderID,
		row.PublisherProviderName, row.CreatedAt.Time, row.UpdatedAt.Time), err
}
func (s *managementSession) Provider(ctx context.Context, id int64) (mgmt.Provider, error) {
	row, err := s.q.ManageProvider(ctx, id)
	if err != nil {
		return mgmt.Provider{}, err
	}
	provider := providerView(row)
	provider.Endpoints, err = s.providerEndpoints(ctx, id)
	return provider, err
}

func (s *managementSession) Resource(ctx context.Context, id int64) (mgmt.ResourceRecord, error) {
	r, err := s.q.ManageResource(ctx, id)
	return mgmt.ResourceRecord{Resource: mgmt.Resource{
		ID: r.ID, ProviderID: r.ProviderID, Name: r.ResourceName,
		AuthType: r.AuthType, AuthAdapter: r.AuthAdapter, SubscriptionType: r.SubscriptionType,
		PlanCode: r.PlanCode, ExternalAccountRef: r.ExternalAccountRef, Priority: r.Priority,
		EffectiveAt: timePointer(r.EffectiveAt), ExpiresAt: timePointer(r.ExpiresAt),
		QuotaStatus: r.QuotaStatus, QuotaCheckedAt: timePointer(r.QuotaCheckedAt), QuotaResetsAt: timePointer(r.QuotaResetsAt),
		CredentialRefreshedAt: timePointer(r.CredentialRefreshedAt), CredentialExpiresAt: timePointer(r.CredentialExpiresAt),
		RuntimeStatus: r.RuntimeStatus, BlockedReason: r.BlockedReason,
		BlockedAt: timePointer(r.BlockedAt), LastErrorAt: timePointer(r.LastErrorAt),
		LastHTTPStatus: r.LastHttpStatus, LastErrorCode: r.LastErrorCode,
		CredentialConfigured: true, Version: r.Version,
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}, Sealed: catalog.SealedCredential{Ciphertext: r.CredentialCiphertext, Nonce: r.CredentialNonce, KeyVersion: r.KeyVersion}}, err
}
func (s *managementSession) ResourceQuotas(ctx context.Context, id int64) ([]mgmt.ResourceQuota, error) {
	rows, err := s.q.ManageResourceQuotas(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.ResourceQuota, 0, len(rows))
	for _, row := range rows {
		result = append(result, mgmt.ResourceQuota{
			Code: row.QuotaCode, Name: row.QuotaName, Status: row.QuotaStatus, Unit: row.QuotaUnit,
			LimitValue: numericString(row.LimitValue), UsedValue: numericString(row.UsedValue),
			RemainingValue: numericString(row.RemainingValue), UsedPercent: numericFloat(row.UsedPercent),
			WindowDurationSeconds: row.WindowDurationSeconds, ResetsAt: timePointer(row.ResetsAt),
			ReachedType: row.ReachedType, ObservedAt: row.ObservedAt.Time.UTC(),
		})
	}
	return result, nil
}
func (s *managementSession) CreateMember(ctx context.Context, m mgmt.Member) error {
	return s.q.ManageCreateMember(ctx, dbgen.ManageCreateMemberParams{ID: m.ID, Name: m.Name, Remark: m.Remark, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(m.CreatedAt)})
}
func (s *managementSession) UpdateMember(ctx context.Context, m mgmt.Member) error {
	return s.q.ManageUpdateMember(ctx, dbgen.ManageUpdateMemberParams{ID: m.ID, Name: m.Name, Remark: m.Remark, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}
func (s *managementSession) SetMemberStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageMemberStatus(ctx, dbgen.ManageMemberStatusParams{ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}
func (s *managementSession) DeleteMember(ctx context.Context, id int64) error {
	now, actor := pgTime(time.Now().UTC()), actorRef(s.actor.ID)
	// 先锁定并删除成员，和签发 Key 的成员行锁串行化；所有更改与审计同事务提交。
	if err := s.q.ManageDeleteMember(ctx, dbgen.ManageDeleteMemberParams{ID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteMemberGroups(ctx, dbgen.ManageDeleteMemberGroupsParams{PrincipalID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	return s.q.ManageRevokeMemberKeys(ctx, dbgen.ManageRevokeMemberKeysParams{PrincipalID: id, UpdatedBy: actor, RevokedAt: now})
}
func (s *managementSession) CreateGroup(ctx context.Context, g mgmt.Group) error {
	return s.q.ManageCreateGroup(ctx, dbgen.ManageCreateGroupParams{ID: g.ID, GroupCode: g.Code, GroupName: g.Name, Remark: g.Remark, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(g.CreatedAt)})
}
func (s *managementSession) UpdateGroup(ctx context.Context, g mgmt.Group) error {
	return s.q.ManageUpdateGroup(ctx, dbgen.ManageUpdateGroupParams{ID: g.ID, GroupName: g.Name, Remark: g.Remark, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}
func (s *managementSession) SetGroupStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageGroupStatus(ctx, dbgen.ManageGroupStatusParams{ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}
func (s *managementSession) DeleteGroup(ctx context.Context, id int64) error {
	now, actor := pgTime(time.Now().UTC()), actorRef(s.actor.ID)
	if err := s.q.ManageDeleteGroup(ctx, dbgen.ManageDeleteGroupParams{ID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteGroupMembers(ctx, dbgen.ManageDeleteGroupMembersParams{GroupID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	return s.q.ManageDeleteGroupModels(ctx, dbgen.ManageDeleteGroupModelsParams{GroupID: id, UpdatedBy: actor, UpdatedAt: now})
}
func (s *managementSession) SetGroupMember(ctx context.Context, groupID, memberID int64, add bool) (bool, error) {
	exists, err := s.q.ManageMembershipExists(ctx, dbgen.ManageMembershipExistsParams{GroupID: groupID, PrincipalID: memberID})
	if err != nil || exists == add {
		return false, err
	}
	if add {
		id, err := s.store.ids.NextID(ctx)
		if err != nil {
			return false, err
		}
		err = s.q.ManageAddMember(ctx, dbgen.ManageAddMemberParams{ID: id, GroupID: groupID, PrincipalID: memberID, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(time.Now().UTC())})
		return err == nil, err
	}
	err = s.q.ManageRemoveMember(ctx, dbgen.ManageRemoveMemberParams{GroupID: groupID, PrincipalID: memberID, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
	return err == nil, err
}
func (s *managementSession) SetGroupModel(ctx context.Context, groupID, modelID int64, grant bool) (bool, error) {
	exists, err := s.q.ManagePermissionExists(ctx, dbgen.ManagePermissionExistsParams{GroupID: groupID, ModelID: modelID})
	if err != nil || exists == grant {
		return false, err
	}
	if grant {
		id, err := s.store.ids.NextID(ctx)
		if err != nil {
			return false, err
		}
		err = s.q.ManageGrantModel(ctx, dbgen.ManageGrantModelParams{ID: id, GroupID: groupID, ModelID: modelID, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(time.Now().UTC())})
		return err == nil, err
	}
	err = s.q.ManageRevokeModel(ctx, dbgen.ManageRevokeModelParams{GroupID: groupID, ModelID: modelID, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
	return err == nil, err
}
func (s *managementSession) SetModelStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageModelStatus(ctx, dbgen.ManageModelStatusParams{ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}

func (s *managementSession) CreateProvider(ctx context.Context, p mgmt.Provider) error {
	headerNames, err := providerHeaderNames(p)
	if err != nil {
		return err
	}
	err = s.q.ManageCreateProvider(ctx, dbgen.ManageCreateProviderParams{
		ID: p.ID, ProviderCode: p.Code, ProviderName: p.Name, ProviderType: p.Type,
		OfficialWebsite: p.Website,
		ProxyEnabled:    p.ProxyEnabled, ProxyUrlDisplay: p.ProxyURL,
		ProxyUrlCiphertext: p.ProxyURLSealed.Ciphertext, ProxyUrlNonce: p.ProxyURLSealed.Nonce, ProxyUrlKeyVersion: sealedVersion(p.ProxyURLSealed),
		ProxyHeaderNames: headerNames, ProxyHeadersCiphertext: p.ProxyHeadersSealed.Ciphertext,
		ProxyHeadersNonce: p.ProxyHeadersSealed.Nonce, ProxyHeadersKeyVersion: sealedVersion(p.ProxyHeadersSealed),
		Status: p.Status, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(p.CreatedAt),
	})
	if err != nil {
		return err
	}
	return s.syncProviderEndpoints(ctx, p)
}
func (s *managementSession) UpdateProvider(ctx context.Context, p mgmt.Provider) error {
	headerNames, err := providerHeaderNames(p)
	if err != nil {
		return err
	}
	err = s.q.ManageUpdateProvider(ctx, dbgen.ManageUpdateProviderParams{
		ID: p.ID, ProviderName: p.Name, OfficialWebsite: p.Website,
		ProxyEnabled: p.ProxyEnabled, ProxyUrlDisplay: p.ProxyURL,
		ProxyUrlCiphertext: p.ProxyURLSealed.Ciphertext, ProxyUrlNonce: p.ProxyURLSealed.Nonce, ProxyUrlKeyVersion: sealedVersion(p.ProxyURLSealed),
		ProxyHeaderNames: headerNames, ProxyHeadersCiphertext: p.ProxyHeadersSealed.Ciphertext,
		ProxyHeadersNonce: p.ProxyHeadersSealed.Nonce, ProxyHeadersKeyVersion: sealedVersion(p.ProxyHeadersSealed),
		UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(p.UpdatedAt),
	})
	if err != nil {
		return err
	}
	return s.syncProviderEndpoints(ctx, p)
}

func (s *managementSession) syncProviderEndpoints(ctx context.Context, p mgmt.Provider) error {
	desired := make(map[string]struct{}, len(p.Endpoints))
	for _, endpoint := range p.Endpoints {
		desired[endpoint.ProtocolType] = struct{}{}
		if err := s.q.ManageUpsertProviderEndpoint(ctx, dbgen.ManageUpsertProviderEndpointParams{
			ProviderID: p.ID, ProtocolType: endpoint.ProtocolType, BaseUrl: endpoint.BaseURL, NetworkScope: endpoint.NetworkScope,
			CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(p.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	for _, protocol := range []string{"OPENAI", "ANTHROPIC"} {
		if _, keep := desired[protocol]; keep {
			continue
		}
		if err := s.q.ManageDeleteProviderEndpoint(ctx, dbgen.ManageDeleteProviderEndpointParams{ProviderID: p.ID, ProtocolType: protocol}); err != nil {
			return err
		}
	}
	return nil
}
func (s *managementSession) DeleteProvider(ctx context.Context, id int64, at time.Time) error {
	actor, updatedAt := actorRef(s.actor.ID), pgTime(at)
	if err := s.q.ManageDeleteProviderMappings(ctx, dbgen.ManageDeleteProviderMappingsParams{ProviderID: id, UpdatedBy: actor, UpdatedAt: updatedAt}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteProviderResources(ctx, dbgen.ManageDeleteProviderResourcesParams{ProviderID: id, UpdatedBy: actor, UpdatedAt: updatedAt}); err != nil {
		return err
	}
	return s.q.ManageDeleteProvider(ctx, dbgen.ManageDeleteProviderParams{ID: id, UpdatedBy: actor, UpdatedAt: updatedAt})
}
func (s *managementSession) SetProviderStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageProviderStatus(ctx, dbgen.ManageProviderStatusParams{ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now().UTC())})
}
func (s *managementSession) CreateProviderMapping(ctx context.Context, mapping mgmt.ProviderMapping) error {
	return s.q.ManageCreateProviderMapping(ctx, dbgen.ManageCreateProviderMappingParams{ID: mapping.ID, ProviderID: mapping.ProviderID, ModelID: mapping.ModelID, UpstreamModelCode: mapping.UpstreamModelCode, Priority: mapping.Priority, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(mapping.CreatedAt)})
}
func (s *managementSession) UpdateProviderMapping(ctx context.Context, mapping mgmt.ProviderMapping) error {
	return s.q.ManageUpdateProviderMapping(ctx, dbgen.ManageUpdateProviderMappingParams{ID: mapping.ID, ProviderID: mapping.ProviderID, UpstreamModelCode: mapping.UpstreamModelCode, Priority: mapping.Priority, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(mapping.UpdatedAt)})
}
func (s *managementSession) DeleteProviderMapping(ctx context.Context, providerID, mappingID int64, at time.Time) error {
	return s.q.ManageDeleteProviderMapping(ctx, dbgen.ManageDeleteProviderMappingParams{ID: mappingID, ProviderID: providerID, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(at)})
}
func (s *managementSession) CreateModel(ctx context.Context, m mgmt.Model) error {
	input, err := json.Marshal(m.InputModalities)
	if err != nil {
		return err
	}
	output, err := json.Marshal(m.OutputModalities)
	if err != nil {
		return err
	}
	return s.q.ManageCreateModel(ctx, dbgen.ManageCreateModelParams{ID: m.ID, ModelCode: m.Code, DisplayName: m.Name, InputModalities: input, OutputModalities: output, Remark: m.Remark, Status: m.Status, PublisherProviderID: m.PublisherProviderID, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(m.CreatedAt)})
}
func (s *managementSession) UpdateModel(ctx context.Context, m mgmt.Model) error {
	input, err := json.Marshal(m.InputModalities)
	if err != nil {
		return err
	}
	output, err := json.Marshal(m.OutputModalities)
	if err != nil {
		return err
	}
	return s.q.ManageUpdateModel(ctx, dbgen.ManageUpdateModelParams{ID: m.ID, ModelCode: m.Code, DisplayName: m.Name, InputModalities: input, OutputModalities: output, Remark: m.Remark, PublisherProviderID: m.PublisherProviderID, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(m.UpdatedAt)})
}
func (s *managementSession) DeleteModel(ctx context.Context, id int64, at time.Time) error {
	actor, updatedAt := actorRef(s.actor.ID), pgTime(at)
	if err := s.q.ManageDeleteModelMappings(ctx, dbgen.ManageDeleteModelMappingsParams{ModelID: id, UpdatedBy: actor, UpdatedAt: updatedAt}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteModelPermissions(ctx, dbgen.ManageDeleteModelPermissionsParams{ModelID: id, UpdatedBy: actor, UpdatedAt: updatedAt}); err != nil {
		return err
	}
	return s.q.ManageDeleteModel(ctx, dbgen.ManageDeleteModelParams{ID: id, UpdatedBy: actor, UpdatedAt: updatedAt})
}
func (s *managementSession) CreateResource(ctx context.Context, r mgmt.ResourceRecord) error {
	return s.q.ManageCreateResource(ctx, dbgen.ManageCreateResourceParams{
		ID: r.ID, ProviderID: r.ProviderID, ResourceName: r.Name, AuthType: r.AuthType,
		AuthAdapter: r.AuthAdapter, SubscriptionType: r.SubscriptionType, PlanCode: r.PlanCode,
		ExternalAccountRef: r.ExternalAccountRef, Priority: r.Priority,
		EffectiveAt: nullableTime(r.EffectiveAt), ExpiresAt: nullableTime(r.ExpiresAt),
		QuotaStatus: r.QuotaStatus, QuotaCheckedAt: nullableTime(r.QuotaCheckedAt), QuotaResetsAt: nullableTime(r.QuotaResetsAt),
		CredentialRefreshedAt: nullableTime(r.CredentialRefreshedAt), CredentialExpiresAt: nullableTime(r.CredentialExpiresAt),
		CredentialCiphertext: r.Sealed.Ciphertext, CredentialNonce: r.Sealed.Nonce,
		KeyVersion: r.Sealed.KeyVersion, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(r.CreatedAt),
	})
}
func (s *managementSession) UpdateResource(ctx context.Context, r mgmt.ResourceRecord) error {
	updated, err := s.q.ManageUpdateResource(ctx, dbgen.ManageUpdateResourceParams{
		ID: r.ID, ResourceName: r.Name, AuthType: r.AuthType, AuthAdapter: r.AuthAdapter,
		SubscriptionType: r.SubscriptionType, PlanCode: r.PlanCode, ExternalAccountRef: r.ExternalAccountRef,
		Priority: r.Priority, EffectiveAt: nullableTime(r.EffectiveAt), ExpiresAt: nullableTime(r.ExpiresAt),
		QuotaStatus: r.QuotaStatus, QuotaCheckedAt: nullableTime(r.QuotaCheckedAt), QuotaResetsAt: nullableTime(r.QuotaResetsAt),
		CredentialRefreshedAt: nullableTime(r.CredentialRefreshedAt), CredentialExpiresAt: nullableTime(r.CredentialExpiresAt),
		CredentialCiphertext: r.Sealed.Ciphertext, CredentialNonce: r.Sealed.Nonce,
		KeyVersion: r.Sealed.KeyVersion, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(r.UpdatedAt),
		Version: r.Version,
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return mgmt.ErrConflict
	}
	return nil
}
func (s *managementSession) DeleteResource(ctx context.Context, id int64, at time.Time) (bool, error) {
	rows, err := s.q.ManageDeleteResource(ctx, dbgen.ManageDeleteResourceParams{ID: id, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(at)})
	return rows > 0, err
}
func (s *managementSession) ReplaceResourceQuotas(ctx context.Context, id int64, quotas []mgmt.ResourceQuota) error {
	if err := s.q.ManageDeleteResourceQuotas(ctx, id); err != nil {
		return err
	}
	for _, quota := range quotas {
		if err := s.q.ManageCreateResourceQuota(ctx, dbgen.ManageCreateResourceQuotaParams{
			ProviderCredentialID: id, QuotaCode: quota.Code, QuotaName: quota.Name,
			QuotaStatus: quota.Status, QuotaUnit: quota.Unit, LimitValue: pgNumeric(quota.LimitValue),
			UsedValue: pgNumeric(quota.UsedValue), RemainingValue: pgNumeric(quota.RemainingValue),
			UsedPercent: pgFloat(quota.UsedPercent), WindowDurationSeconds: quota.WindowDurationSeconds,
			ResetsAt: nullableTime(quota.ResetsAt), ReachedType: quota.ReachedType, ObservedAt: pgTime(quota.ObservedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}
func (s *managementSession) RestoreResourceRuntime(ctx context.Context, id, expectedVersion int64, at time.Time) error {
	updated, err := s.q.ManageRestoreResourceRuntime(ctx, dbgen.ManageRestoreResourceRuntimeParams{
		ID: id, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(at), ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return mgmt.ErrConflict
	}
	return nil
}
func (s *managementSession) BlockResourceRuntime(ctx context.Context, id, expectedVersion int64, reason, code string, status *int32, at time.Time) error {
	updated, err := s.q.ManageBlockResourceRuntime(ctx, dbgen.ManageBlockResourceRuntimeParams{
		ID: id, BlockedReason: &reason, BlockedAt: pgTime(at), LastHttpStatus: status,
		LastErrorCode: &code, UpdatedBy: actorRef(s.actor.ID), ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return mgmt.ErrConflict
	}
	return nil
}
