package postgres

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	gw "github.com/zentrola/zentrola/internal/application/gateway"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

type GatewayStore struct{ pool *pgxpool.Pool }

type subscriptionRefreshConnectionKey struct{}

func NewGatewayStore(pool *pgxpool.Pool) *GatewayStore { return &GatewayStore{pool: pool} }

func (s *GatewayStore) gatewayQueries(ctx context.Context) *dbgen.Queries {
	if conn, ok := ctx.Value(subscriptionRefreshConnectionKey{}).(*pgxpool.Conn); ok {
		return dbgen.New(conn)
	}
	return dbgen.New(s.pool)
}
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
	if protocol != gw.AnthropicProtocol && !gw.IsOpenAIProtocol(protocol) {
		return nil, gw.ErrInvalid
	}
	preferredProtocol := gatewayEndpointProtocols(protocol)[0]
	// 缓存未命中时使用一致性快照加载身份、授权与路由，不在网络转发期间占用连接。
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
		if protocol == gw.OpenAIImagesProtocol && row.ProtocolType != gw.OpenAIEndpoint {
			continue
		}
		if row.AuthType == mgmt.AuthTypeSubscription && protocol != gw.OpenAIResponsesProtocol {
			continue
		}
		key := candidateKey{providerModelID: row.ProviderModelID, resourceID: row.ResourceID}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		upstreamModel := row.UpstreamModelCode
		if upstreamModel == "" {
			upstreamModel = model
		}
		route := gw.Route{
			ModelID: m.ID, ProviderID: row.ProviderID, ProviderModelID: row.ProviderModelID, ResourceID: row.ResourceID,
			ModelCode: m.ModelCode, ModelName: m.DisplayName, ProviderName: row.ProviderName,
			UpstreamModel: upstreamModel, BaseURL: row.BaseUrl, EndpointProtocol: row.ProtocolType, NetworkScope: row.NetworkScope,
			AuthType: row.AuthType, AuthAdapter: row.AuthAdapter, ResourcePriority: row.ResourcePriority,
			QuotaStatus: row.QuotaStatus, ExpiresAt: timePointer(row.ExpiresAt),
			CredentialRefreshedAt: timePointer(row.CredentialRefreshedAt), CredentialExpiresAt: timePointer(row.CredentialExpiresAt),
			Credential:   catalog.SealedCredential{Ciphertext: row.CredentialCiphertext, Nonce: row.CredentialNonce, KeyVersion: row.KeyVersion},
			ProxyEnabled: row.ProxyEnabled,
		}
		if row.SubscriptionType != nil {
			route.SubscriptionType = *row.SubscriptionType
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
	if protocol == gw.OpenAIImagesProtocol {
		return []string{gw.OpenAIEndpoint}
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

func (s *GatewayStore) UpdateResourceCredential(ctx context.Context, route gw.Route, sealed catalog.SealedCredential) error {
	if route.ResourceID <= 0 || route.ProviderID <= 0 || sealed.KeyVersion <= 0 {
		return gw.ErrInvalid
	}
	updated, err := s.gatewayQueries(ctx).UpdateGatewayResourceCredential(ctx, dbgen.UpdateGatewayResourceCredentialParams{
		ResourceID: route.ResourceID, ProviderID: route.ProviderID,
		CredentialCiphertext: sealed.Ciphertext, CredentialNonce: sealed.Nonce, KeyVersion: sealed.KeyVersion,
		CredentialRefreshedAt: nullableTime(route.CredentialRefreshedAt), CredentialExpiresAt: nullableTime(route.CredentialExpiresAt),
		UpdatedAt: pgtype.Timestamptz{Time: time.Now().UTC().Truncate(time.Microsecond), Valid: true},
	})
	if err != nil || updated != 1 {
		return gw.ErrUnavailable
	}
	return nil
}

func (s *GatewayStore) UpdateResourceCredentialRefreshMetadata(ctx context.Context, route gw.Route) error {
	if route.ResourceID <= 0 || route.ProviderID <= 0 || route.CredentialExpiresAt == nil {
		return gw.ErrInvalid
	}
	updated, err := s.gatewayQueries(ctx).UpdateGatewayResourceCredentialRefreshMetadata(ctx, dbgen.UpdateGatewayResourceCredentialRefreshMetadataParams{
		ResourceID: route.ResourceID, ProviderID: route.ProviderID,
		CredentialRefreshedAt: nullableTime(route.CredentialRefreshedAt), CredentialExpiresAt: nullableTime(route.CredentialExpiresAt),
		UpdatedAt: pgtype.Timestamptz{Time: time.Now().UTC().Truncate(time.Microsecond), Valid: true},
	})
	if err != nil || updated > 1 {
		return gw.ErrUnavailable
	}
	return nil
}

func (s *GatewayStore) LoadResourceCredential(ctx context.Context, route gw.Route) (catalog.SealedCredential, error) {
	if route.ResourceID <= 0 || route.ProviderID <= 0 {
		return catalog.SealedCredential{}, gw.ErrInvalid
	}
	row, err := s.gatewayQueries(ctx).GatewayResourceCredential(ctx, dbgen.GatewayResourceCredentialParams{
		ResourceID: route.ResourceID,
		ProviderID: route.ProviderID,
	})
	if err != nil {
		return catalog.SealedCredential{}, gw.ErrUnavailable
	}
	return catalog.SealedCredential{
		Ciphertext: row.CredentialCiphertext,
		Nonce:      row.CredentialNonce,
		KeyVersion: row.KeyVersion,
	}, nil
}

func (s *GatewayStore) LockSubscriptionRefresh(ctx context.Context, resourceID int64) (context.Context, func(), error) {
	if resourceID <= 0 {
		return ctx, nil, gw.ErrInvalid
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return ctx, nil, gw.ErrUnavailable
	}
	if _, err := dbgen.New(conn).LockGatewaySubscriptionRefresh(ctx, resourceID); err != nil {
		conn.Release()
		return ctx, nil, gw.ErrUnavailable
	}
	var once sync.Once
	lockedCtx := context.WithValue(ctx, subscriptionRefreshConnectionKey{}, conn)
	return lockedCtx, func() {
		once.Do(func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			unlocked, unlockErr := dbgen.New(conn).UnlockGatewaySubscriptionRefresh(unlockCtx, resourceID)
			if unlockErr == nil && unlocked {
				conn.Release()
				return
			}
			// 不能把仍持有 session advisory lock 的连接放回连接池。
			hijacked := conn.Hijack()
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer closeCancel()
			_ = hijacked.Close(closeCtx)
		})
	}, nil
}

func (s *GatewayStore) ListSubscriptionCredentials(ctx context.Context, after int64, limit int32, refreshBefore time.Time) ([]gw.SubscriptionCredential, error) {
	if after < 0 || limit <= 0 {
		return nil, gw.ErrInvalid
	}
	rows, err := dbgen.New(s.pool).ListGatewaySubscriptionCredentials(ctx, dbgen.ListGatewaySubscriptionCredentialsParams{
		AfterID: after, PageLimit: limit, RefreshBefore: pgTime(refreshBefore),
	})
	if err != nil {
		return nil, gw.ErrUnavailable
	}
	result := make([]gw.SubscriptionCredential, 0, len(rows))
	for _, row := range rows {
		result = append(result, gw.SubscriptionCredential{
			ProviderID: row.ProviderID, ResourceID: row.ResourceID, AuthAdapter: row.AuthAdapter,
			CredentialRefreshedAt: timePointer(row.CredentialRefreshedAt), CredentialExpiresAt: timePointer(row.CredentialExpiresAt),
			Credential: catalog.SealedCredential{
				Ciphertext: row.CredentialCiphertext, Nonce: row.CredentialNonce, KeyVersion: row.KeyVersion,
			},
			ProxyEnabled: row.ProxyEnabled,
		})
		if row.ProxyUrlKeyVersion != nil {
			result[len(result)-1].ProxyURL = catalog.SealedCredential{
				Ciphertext: row.ProxyUrlCiphertext, Nonce: row.ProxyUrlNonce, KeyVersion: *row.ProxyUrlKeyVersion,
			}
		}
		if row.ProxyHeadersKeyVersion != nil {
			result[len(result)-1].ProxyHeaders = catalog.SealedCredential{
				Ciphertext: row.ProxyHeadersCiphertext, Nonce: row.ProxyHeadersNonce, KeyVersion: *row.ProxyHeadersKeyVersion,
			}
		}
	}
	return result, nil
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
