package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type GatewayStore struct{ pool *pgxpool.Pool }

func NewGatewayStore(pool *pgxpool.Pool) *GatewayStore { return &GatewayStore{pool: pool} }
func (s *GatewayStore) Resolve(ctx context.Context, identity appsec.PrincipalIdentity, model string, protocols ...string) (gw.Route, error) {
	protocol := gw.AnthropicProtocol
	if len(protocols) > 0 {
		protocol = protocols[0]
	}
	if protocol != gw.AnthropicProtocol && protocol != gw.OpenAIProtocol {
		return gw.Route{}, gw.ErrInvalid
	}
	// 使用每次请求的一致性快照，不缓存身份、授权或路由，也不在网络转发期间占用连接。
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return gw.Route{}, gw.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	active, err := q.GatewayIdentityActive(ctx, dbgen.GatewayIdentityActiveParams{AccessKeyID: identity.AccessKeyID, OrganizationID: identity.OrganizationID, PrincipalID: identity.ID})
	if err != nil {
		return gw.Route{}, gw.ErrUnavailable
	}
	if !active {
		return gw.Route{}, gw.ErrAuthentication
	}
	m, err := q.GatewayModel(ctx, model)
	if errors.Is(err, pgx.ErrNoRows) {
		return gw.Route{}, gw.ErrModelUnknown
	}
	if err != nil {
		return gw.Route{}, gw.ErrUnavailable
	}
	if m.Status != "ACTIVE" {
		return gw.Route{ModelID: m.ID}, gw.ErrModelDisabled
	}
	allowed, err := q.HasGroupModelPermission(ctx, dbgen.HasGroupModelPermissionParams{OrganizationID: identity.OrganizationID, PrincipalID: identity.ID, ModelID: m.ID})
	if err != nil {
		return gw.Route{ModelID: m.ID}, gw.ErrUnavailable
	}
	if !allowed {
		return gw.Route{ModelID: m.ID}, gw.ErrPermission
	}
	mappings, err := q.GatewayMappings(ctx, dbgen.GatewayMappingsParams{ModelID: m.ID, ProtocolType: protocol})
	if err != nil {
		return gw.Route{ModelID: m.ID}, gw.ErrUnavailable
	}
	if len(mappings) != 1 {
		return gw.Route{ModelID: m.ID}, gw.ErrRoute
	}
	mapping := mappings[0]
	baseURL := mapping.AnthropicBaseUrl
	if protocol == gw.OpenAIProtocol {
		if mapping.OpenaiBaseUrl == nil || *mapping.OpenaiBaseUrl != "https://api.deepseek.com" {
			return gw.Route{ModelID: m.ID}, gw.ErrRoute
		}
		baseURL = *mapping.OpenaiBaseUrl
	}
	resources, err := q.GatewayResources(ctx, dbgen.GatewayResourcesParams{OrganizationID: identity.OrganizationID, ProviderID: mapping.ProviderID})
	if err != nil {
		return gw.Route{ModelID: m.ID}, gw.ErrUnavailable
	}
	if len(resources) != 1 {
		return gw.Route{ModelID: m.ID}, gw.ErrResource
	}
	r := resources[0]
	if err := tx.Commit(ctx); err != nil {
		return gw.Route{ModelID: m.ID}, gw.ErrUnavailable
	}
	return gw.Route{ModelID: m.ID, ProviderID: mapping.ProviderID, ProviderModelID: mapping.ID, ResourceID: r.ID, UpstreamModel: mapping.UpstreamModelCode, BaseURL: baseURL, Credential: catalog.SealedCredential{Ciphertext: r.CredentialCiphertext, Nonce: r.CredentialNonce, KeyVersion: r.KeyVersion}}, nil
}

func (s *GatewayStore) Models(ctx context.Context, identity appsec.PrincipalIdentity) ([]gw.Model, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	active, err := q.GatewayIdentityActive(ctx, dbgen.GatewayIdentityActiveParams{AccessKeyID: identity.AccessKeyID, OrganizationID: identity.OrganizationID, PrincipalID: identity.ID})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	if !active {
		return nil, gw.ErrAuthentication
	}
	rows, err := q.OpenAIModels(ctx, dbgen.OpenAIModelsParams{OrganizationID: identity.OrganizationID, PrincipalID: identity.ID})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	result := make([]gw.Model, 0, len(rows))
	for _, r := range rows {
		result = append(result, gw.Model{ID: r.ModelCode, Object: "model", Created: r.CreatedAt.Time.Unix(), OwnedBy: r.ProviderCode})
	}
	if tx.Commit(ctx) != nil {
		return nil, gw.ErrUnavailable
	}
	return result, nil
}
