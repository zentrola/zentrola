package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type GatewayStore struct{ pool *pgxpool.Pool }

func NewGatewayStore(pool *pgxpool.Pool) *GatewayStore { return &GatewayStore{pool: pool} }
func (s *GatewayStore) Resolve(ctx context.Context, identity appsec.PrincipalIdentity, model string, protocols ...string) (gw.Route, error) {
	routes, err := s.ResolveCandidates(ctx, identity, model, protocols...)
	if err != nil {
		return gw.Route{}, err
	}
	return routes[0], nil
}

func (s *GatewayStore) ResolveCandidates(ctx context.Context, identity appsec.PrincipalIdentity, model string, protocols ...string) ([]gw.Route, error) {
	protocol := gw.AnthropicProtocol
	if len(protocols) > 0 {
		protocol = protocols[0]
	}
	if protocol != gw.AnthropicProtocol && protocol != gw.OpenAIProtocol && protocol != gw.OpenAIResponsesProtocol {
		return nil, gw.ErrInvalid
	}
	preferredProtocol := gatewayEndpointProtocols(protocol)[0]
	// 使用每次请求的一致性快照，不缓存身份、授权或路由，也不在网络转发期间占用连接。
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	active, err := q.GatewayIdentityActive(ctx, dbgen.GatewayIdentityActiveParams{AccessKeyID: identity.AccessKeyID, PrincipalID: identity.ID})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	if !active {
		return nil, gw.ErrAuthentication
	}
	m, err := q.GatewayModel(ctx, model)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, gw.ErrModelUnknown
	}
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	if m.Status != "ACTIVE" {
		return nil, gw.ErrModelDisabled
	}
	allowed, err := q.HasGroupModelPermission(ctx, dbgen.HasGroupModelPermissionParams{PrincipalID: identity.ID, ModelID: m.ID})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	if !allowed {
		return nil, gw.ErrPermission
	}
	rows, err := q.GatewayCandidates(ctx, dbgen.GatewayCandidatesParams{
		ModelID: m.ID, PreferredProtocol: preferredProtocol,
	})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	if len(rows) == 0 {
		routeExists, availabilityErr := q.GatewayRouteExists(ctx, m.ID)
		if availabilityErr != nil {
			return nil, gw.ErrUnavailable
		}
		if routeExists {
			return nil, gw.ErrResource
		}
		return nil, gw.ErrRoute
	}
	// 一个服务商可以同时配置两类 Endpoint；每个映射只保留排序靠前的首选协议。
	type candidateKey struct{ providerModelID, resourceID int64 }
	seen := make(map[candidateKey]struct{}, len(rows))
	routes := make([]gw.Route, 0, len(rows))
	for _, row := range rows {
		key := candidateKey{providerModelID: row.ProviderModelID, resourceID: row.ResourceID}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		route := gw.Route{
			ModelID: m.ID, ProviderID: row.ProviderID, ProviderModelID: row.ProviderModelID, ResourceID: row.ResourceID,
			UpstreamModel: row.UpstreamModelCode, BaseURL: row.BaseUrl, EndpointProtocol: row.ProtocolType,
			Credential:   catalog.SealedCredential{Ciphertext: row.CredentialCiphertext, Nonce: row.CredentialNonce, KeyVersion: row.KeyVersion},
			ProxyEnabled: row.ProxyEnabled,
		}
		if row.ProxyUrlKeyVersion != nil {
			route.ProxyURL = catalog.SealedCredential{Ciphertext: row.ProxyUrlCiphertext, Nonce: row.ProxyUrlNonce, KeyVersion: *row.ProxyUrlKeyVersion}
		}
		if row.ProxyHeadersKeyVersion != nil {
			route.ProxyHeaders = catalog.SealedCredential{Ciphertext: row.ProxyHeadersCiphertext, Nonce: row.ProxyHeadersNonce, KeyVersion: *row.ProxyHeadersKeyVersion}
		}
		routes = append(routes, route)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, gw.ErrUnavailable
	}
	if len(routes) == 0 {
		return nil, gw.ErrRoute
	}
	return routes, nil
}

func gatewayEndpointProtocols(protocol string) []string {
	if protocol == gw.AnthropicProtocol {
		return []string{gw.AnthropicEndpoint, gw.OpenAIEndpoint}
	}
	return []string{gw.OpenAIEndpoint, gw.AnthropicEndpoint}
}

func (s *GatewayStore) BlockResource(ctx context.Context, identity appsec.PrincipalIdentity, resourceID int64, block gw.ResourceBlock) error {
	if identity.ID <= 0 || resourceID <= 0 || block.Reason == "" || block.ErrorCode == "" {
		return gw.ErrInvalid
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	status := block.HTTPStatus
	var httpStatus *int32
	if status > 0 {
		httpStatus = &status
	}
	reason, code := block.Reason, block.ErrorCode
	_, err := dbgen.New(s.pool).BlockGatewayResource(ctx, dbgen.BlockGatewayResourceParams{
		ResourceID:    resourceID,
		BlockedReason: &reason, BlockedAt: pgtype.Timestamptz{Time: now, Valid: true},
		HttpStatus: httpStatus, ErrorCode: &code,
	})
	if err != nil {
		return gw.ErrUnavailable
	}
	return nil
}

func (s *GatewayStore) Models(ctx context.Context, identity appsec.PrincipalIdentity) ([]gw.Model, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	defer tx.Rollback(context.Background())
	q := dbgen.New(tx)
	active, err := q.GatewayIdentityActive(ctx, dbgen.GatewayIdentityActiveParams{AccessKeyID: identity.AccessKeyID, PrincipalID: identity.ID})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	if !active {
		return nil, gw.ErrAuthentication
	}
	rows, err := q.OpenAIModels(ctx, identity.ID)
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
