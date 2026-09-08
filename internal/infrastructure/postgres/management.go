package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/shared"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
	"time"
)

type ManagementStore struct {
	pool  *pgxpool.Pool
	ids   shared.IDGenerator
	audit *SecurityStore
}

func NewManagementStore(pool *pgxpool.Pool, ids shared.IDGenerator) *ManagementStore {
	return &ManagementStore{pool: pool, ids: ids, audit: NewSecurityStore(pool, ids)}
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
		return appsec.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	if write {
		// P0 单组织管理写入串行化；不在此事务中执行上游网络请求。
		_, err = q.LockManagementOrganization(ctx, dbgen.LockManagementOrganizationParams{ID: a.OrganizationID, ID_2: a.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return appsec.ErrUnauthenticated
		}
	} else {
		err = validateActor(ctx, q, a)
	}
	if err != nil {
		return managementError(err)
	}
	if err = fn(&managementSession{q: q, actor: a, store: s}); err != nil {
		return managementError(err)
	}
	return managementError(tx.Commit(ctx))
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
		switch pgErr.Code {
		case "23505", "40001", "40P01":
			return mgmt.ErrConflict
		case "22001":
			return appsec.ErrInvalidArgument
		}
	}
	for _, known := range []error{appsec.ErrInvalidArgument, appsec.ErrUnauthenticated, appsec.ErrNotFound, appsec.ErrUnavailable, mgmt.ErrConflict, mgmt.ErrCredential, mgmt.ErrProvider} {
		if errors.Is(err, known) {
			return known
		}
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
func memberView(r dbgen.Principal) mgmt.Member {
	return mgmt.Member{ID: r.ID, Name: r.Name, Remark: r.Remark, Status: r.Status, CreatedAt: r.CreatedAt.Time}
}
func groupView(r dbgen.AiGroup) mgmt.Group {
	return mgmt.Group{ID: r.ID, Code: r.GroupCode, Name: r.GroupName, Remark: r.Remark, Status: r.Status, CreatedAt: r.CreatedAt.Time}
}
func modelView(r dbgen.AiModel) mgmt.Model {
	// 数组格式由数据库 CHECK 保证；响应只暴露业务字段。
	input, output := []string{}, []string{}
	_ = json.Unmarshal(r.InputModalities, &input)
	_ = json.Unmarshal(r.OutputModalities, &output)
	return mgmt.Model{ID: r.ID, Code: r.ModelCode, Name: r.DisplayName, Status: r.Status, InputModalities: input, OutputModalities: output, Remark: r.Remark, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}
func providerView(r dbgen.AiProvider) mgmt.Provider {
	names := []string{}
	_ = json.Unmarshal(r.ProxyHeaderNames, &names)
	headers := make([]mgmt.ProviderProxyHeader, 0, len(names))
	for _, name := range names {
		headers = append(headers, mgmt.ProviderProxyHeader{Key: name, Configured: true})
	}
	provider := mgmt.Provider{
		ID: r.ID, Code: r.ProviderCode, Name: r.ProviderName, Type: r.ProviderType,
		Website: r.OfficialWebsite, Endpoints: []mgmt.ProviderEndpoint{},
		ProxyEnabled: r.ProxyEnabled, ProxyURL: r.ProxyUrlDisplay, ProxyHeaders: headers,
		Status: r.Status, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ProxyUrlKeyVersion != nil {
		provider.ProxyURLSealed = catalog.SealedCredential{Ciphertext: r.ProxyUrlCiphertext, Nonce: r.ProxyUrlNonce, KeyVersion: *r.ProxyUrlKeyVersion}
	}
	if r.ProxyHeadersKeyVersion != nil {
		provider.ProxyHeadersSealed = catalog.SealedCredential{Ciphertext: r.ProxyHeadersCiphertext, Nonce: r.ProxyHeadersNonce, KeyVersion: *r.ProxyHeadersKeyVersion}
	}
	return provider
}

func providerHeaderNames(p mgmt.Provider) ([]byte, error) {
	names := make([]string, 0, len(p.ProxyHeaders))
	for _, header := range p.ProxyHeaders {
		names = append(names, header.Key)
	}
	return json.Marshal(names)
}

func sealedVersion(sealed catalog.SealedCredential) *int32 {
	if sealed.KeyVersion <= 0 {
		return nil
	}
	version := sealed.KeyVersion
	return &version
}
func providerMappingView(r dbgen.ProviderModel) mgmt.ProviderMapping {
	return mgmt.ProviderMapping{ID: r.ID, ProviderID: r.ProviderID, ModelID: r.ModelID, UpstreamModelCode: r.UpstreamModelCode, Priority: r.Priority, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}

func endpointView(r dbgen.ProviderEndpoint) mgmt.ProviderEndpoint {
	return mgmt.ProviderEndpoint{ProtocolType: r.ProtocolType, BaseURL: r.BaseUrl}
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
func resourceView(r dbgen.ManageResourcesRow) mgmt.Resource {
	return mgmt.Resource{ID: r.ID, ProviderID: r.ProviderID, Name: r.ResourceName, Status: r.Status, CredentialConfigured: true, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
}
func keyView(r dbgen.ManageKeysRow) mgmt.Key {
	return mgmt.Key{ID: r.ID, Name: r.Name, MaskedKey: r.MaskedKey, Status: r.Status, ExpiresAt: timePointer(r.ExpiresAt), RevokedAt: timePointer(r.RevokedAt), CreatedAt: r.CreatedAt.Time}
}
func operationView(r dbgen.ManageOperationsRow) mgmt.Operation {
	return mgmt.Operation{ID: r.ID, OperatorName: r.OperatorName, Type: r.OperationType, TargetType: r.TargetType, TargetID: r.TargetID, RequestID: r.RequestID, Result: r.Result, ErrorCode: r.ErrorCode, Before: r.BeforeData, After: r.AfterData, CreatedAt: r.CreatedAt.Time}
}
func (s *managementSession) Members(ctx context.Context, p mgmt.Page) ([]mgmt.Member, error) {
	rows, err := s.q.ManageMembers(ctx, dbgen.ManageMembersParams{OrganizationID: s.actor.OrganizationID, ID: p.After, Limit: p.Limit})
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
	rows, err := s.q.ManageGroups(ctx, dbgen.ManageGroupsParams{OrganizationID: s.actor.OrganizationID, Status: status, AfterID: p.After, PageLimit: p.Limit})
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
	rows, err := s.q.ManageModels(ctx, dbgen.ManageModelsParams{Status: status, AfterID: p.After, PageLimit: p.Limit})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Model, 0, len(rows))
	for _, row := range rows {
		result = append(result, modelView(row))
	}
	return result, nil
}
func (s *managementSession) Providers(ctx context.Context, p mgmt.Page) ([]mgmt.Provider, error) {
	rows, err := s.q.ManageProviders(ctx, dbgen.ManageProvidersParams{ID: p.After, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Provider, 0, len(rows))
	for _, row := range rows {
		provider := providerView(row)
		provider.Endpoints, err = s.providerEndpoints(ctx, provider.ID)
		if err != nil {
			return nil, err
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
func (s *managementSession) Resources(ctx context.Context, p mgmt.Page) ([]mgmt.Resource, error) {
	rows, err := s.q.ManageResources(ctx, dbgen.ManageResourcesParams{OrganizationID: s.actor.OrganizationID, ID: p.After, Limit: p.Limit})
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
	rows, err := s.q.ManageOperations(ctx, dbgen.ManageOperationsParams{OrganizationID: s.actor.OrganizationID, ID: p.After, Limit: p.Limit})
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
	rows, err := s.q.ManageGroupMembers(ctx, dbgen.ManageGroupMembersParams{OrganizationID: s.actor.OrganizationID, GroupID: id, ID: p.After, Limit: p.Limit})
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
	rows, err := s.q.ManageMemberGroups(ctx, dbgen.ManageMemberGroupsParams{OrganizationID: s.actor.OrganizationID, PrincipalID: id, ID: p.After, Limit: p.Limit})
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
	rows, err := s.q.ManageGroupModels(ctx, dbgen.ManageGroupModelsParams{OrganizationID: s.actor.OrganizationID, GroupID: id, ID: p.After, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Model, 0, len(rows))
	for _, row := range rows {
		result = append(result, modelView(row))
	}
	return result, nil
}
func (s *managementSession) Keys(ctx context.Context, id int64, p mgmt.Page) ([]mgmt.Key, error) {
	rows, err := s.q.ManageKeys(ctx, dbgen.ManageKeysParams{OrganizationID: s.actor.OrganizationID, PrincipalID: id, ID: p.After, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	result := make([]mgmt.Key, 0, len(rows))
	for _, row := range rows {
		result = append(result, keyView(row))
	}
	return result, nil
}
func (s *managementSession) Member(ctx context.Context, id int64) (mgmt.Member, error) {
	row, err := s.q.ManageMember(ctx, dbgen.ManageMemberParams{OrganizationID: s.actor.OrganizationID, ID: id})
	return memberView(row), err
}
func (s *managementSession) Group(ctx context.Context, id int64) (mgmt.Group, error) {
	row, err := s.q.ManageGroup(ctx, dbgen.ManageGroupParams{OrganizationID: s.actor.OrganizationID, ID: id})
	return groupView(row), err
}
func (s *managementSession) Model(ctx context.Context, id int64) (mgmt.Model, error) {
	row, err := s.q.ManageModel(ctx, id)
	return modelView(row), err
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
	r, err := s.q.ManageResource(ctx, dbgen.ManageResourceParams{OrganizationID: s.actor.OrganizationID, ID: id})
	return mgmt.ResourceRecord{Resource: mgmt.Resource{ID: r.ID, ProviderID: r.ProviderID, Name: r.ResourceName, Status: r.Status, CredentialConfigured: true, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}, Sealed: catalog.SealedCredential{Ciphertext: r.CredentialCiphertext, Nonce: r.CredentialNonce, KeyVersion: r.KeyVersion}}, err
}
func (s *managementSession) CreateMember(ctx context.Context, m mgmt.Member) error {
	return s.q.ManageCreateMember(ctx, dbgen.ManageCreateMemberParams{ID: m.ID, OrganizationID: s.actor.OrganizationID, Name: m.Name, Remark: m.Remark, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(m.CreatedAt)})
}
func (s *managementSession) UpdateMember(ctx context.Context, m mgmt.Member) error {
	return s.q.ManageUpdateMember(ctx, dbgen.ManageUpdateMemberParams{OrganizationID: s.actor.OrganizationID, ID: m.ID, Name: m.Name, Remark: m.Remark, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
}
func (s *managementSession) SetMemberStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageMemberStatus(ctx, dbgen.ManageMemberStatusParams{OrganizationID: s.actor.OrganizationID, ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
}
func (s *managementSession) DeleteMember(ctx context.Context, id int64) error {
	now, actor := pgTime(time.Now()), actorRef(s.actor.ID)
	// 先锁定并删除成员，和签发 Key 的成员行锁串行化；所有更改与审计同事务提交。
	if err := s.q.ManageDeleteMember(ctx, dbgen.ManageDeleteMemberParams{OrganizationID: s.actor.OrganizationID, ID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteMemberGroups(ctx, dbgen.ManageDeleteMemberGroupsParams{OrganizationID: s.actor.OrganizationID, PrincipalID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	return s.q.ManageRevokeMemberKeys(ctx, dbgen.ManageRevokeMemberKeysParams{OrganizationID: s.actor.OrganizationID, PrincipalID: id, UpdatedBy: actor, RevokedAt: now})
}
func (s *managementSession) CreateGroup(ctx context.Context, g mgmt.Group) error {
	return s.q.ManageCreateGroup(ctx, dbgen.ManageCreateGroupParams{ID: g.ID, OrganizationID: s.actor.OrganizationID, GroupCode: g.Code, GroupName: g.Name, Remark: g.Remark, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(g.CreatedAt)})
}
func (s *managementSession) UpdateGroup(ctx context.Context, g mgmt.Group) error {
	return s.q.ManageUpdateGroup(ctx, dbgen.ManageUpdateGroupParams{OrganizationID: s.actor.OrganizationID, ID: g.ID, GroupName: g.Name, Remark: g.Remark, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
}
func (s *managementSession) SetGroupStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageGroupStatus(ctx, dbgen.ManageGroupStatusParams{OrganizationID: s.actor.OrganizationID, ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
}
func (s *managementSession) DeleteGroup(ctx context.Context, id int64) error {
	now, actor := pgTime(time.Now()), actorRef(s.actor.ID)
	if err := s.q.ManageDeleteGroup(ctx, dbgen.ManageDeleteGroupParams{OrganizationID: s.actor.OrganizationID, ID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	if err := s.q.ManageDeleteGroupMembers(ctx, dbgen.ManageDeleteGroupMembersParams{OrganizationID: s.actor.OrganizationID, GroupID: id, UpdatedBy: actor, UpdatedAt: now}); err != nil {
		return err
	}
	return s.q.ManageDeleteGroupModels(ctx, dbgen.ManageDeleteGroupModelsParams{OrganizationID: s.actor.OrganizationID, GroupID: id, UpdatedBy: actor, UpdatedAt: now})
}
func (s *managementSession) SetGroupMember(ctx context.Context, groupID, memberID int64, add bool) (bool, error) {
	exists, err := s.q.ManageMembershipExists(ctx, dbgen.ManageMembershipExistsParams{OrganizationID: s.actor.OrganizationID, GroupID: groupID, PrincipalID: memberID})
	if err != nil || exists == add {
		return false, err
	}
	if add {
		id, err := s.store.ids.NextID()
		if err != nil {
			return false, err
		}
		err = s.q.ManageAddMember(ctx, dbgen.ManageAddMemberParams{ID: id, OrganizationID: s.actor.OrganizationID, GroupID: groupID, PrincipalID: memberID, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(time.Now())})
		return err == nil, err
	}
	err = s.q.ManageRemoveMember(ctx, dbgen.ManageRemoveMemberParams{OrganizationID: s.actor.OrganizationID, GroupID: groupID, PrincipalID: memberID, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
	return err == nil, err
}
func (s *managementSession) SetGroupModel(ctx context.Context, groupID, modelID int64, grant bool) (bool, error) {
	exists, err := s.q.ManagePermissionExists(ctx, dbgen.ManagePermissionExistsParams{OrganizationID: s.actor.OrganizationID, GroupID: groupID, ModelID: modelID})
	if err != nil || exists == grant {
		return false, err
	}
	if grant {
		id, err := s.store.ids.NextID()
		if err != nil {
			return false, err
		}
		err = s.q.ManageGrantModel(ctx, dbgen.ManageGrantModelParams{ID: id, OrganizationID: s.actor.OrganizationID, GroupID: groupID, ModelID: modelID, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(time.Now())})
		return err == nil, err
	}
	err = s.q.ManageRevokeModel(ctx, dbgen.ManageRevokeModelParams{OrganizationID: s.actor.OrganizationID, GroupID: groupID, ModelID: modelID, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
	return err == nil, err
}
func (s *managementSession) SetModelStatus(ctx context.Context, id int64, status string) error {
	return s.q.ManageModelStatus(ctx, dbgen.ManageModelStatusParams{ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
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
			ProviderID: p.ID, ProtocolType: endpoint.ProtocolType, BaseUrl: endpoint.BaseURL,
			CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(p.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	for _, protocol := range []string{"OPENAI_CHAT", "OPENAI_RESPONSES", "ANTHROPIC_MESSAGES"} {
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
	return s.q.ManageProviderStatus(ctx, dbgen.ManageProviderStatusParams{ID: id, Status: status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(time.Now())})
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
	return s.q.ManageCreateModel(ctx, dbgen.ManageCreateModelParams{ID: m.ID, ModelCode: m.Code, DisplayName: m.Name, InputModalities: input, OutputModalities: output, Remark: m.Remark, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(m.CreatedAt)})
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
	return s.q.ManageUpdateModel(ctx, dbgen.ManageUpdateModelParams{ID: m.ID, ModelCode: m.Code, DisplayName: m.Name, InputModalities: input, OutputModalities: output, Remark: m.Remark, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(m.UpdatedAt)})
}
func (s *managementSession) CreateResource(ctx context.Context, r mgmt.ResourceRecord) error {
	return s.q.ManageCreateResource(ctx, dbgen.ManageCreateResourceParams{ID: r.ID, OrganizationID: s.actor.OrganizationID, ProviderID: r.ProviderID, ResourceName: r.Name, CredentialCiphertext: r.Sealed.Ciphertext, CredentialNonce: r.Sealed.Nonce, KeyVersion: r.Sealed.KeyVersion, Status: r.Status, CreatedBy: actorRef(s.actor.ID), CreatedAt: pgTime(r.CreatedAt)})
}
func (s *managementSession) UpdateResource(ctx context.Context, r mgmt.ResourceRecord) error {
	return s.q.ManageUpdateResource(ctx, dbgen.ManageUpdateResourceParams{OrganizationID: s.actor.OrganizationID, ID: r.ID, ResourceName: r.Name, CredentialCiphertext: r.Sealed.Ciphertext, CredentialNonce: r.Sealed.Nonce, KeyVersion: r.Sealed.KeyVersion, Status: r.Status, UpdatedBy: actorRef(s.actor.ID), UpdatedAt: pgTime(r.UpdatedAt)})
}
