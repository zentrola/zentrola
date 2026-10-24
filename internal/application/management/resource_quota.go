package management

import (
	"context"
	"strconv"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

// quotaProbeModelFamily 是刷新额度时优先选择的模型系列关键字。额度只从推理响应头读取，
// 与所用模型无关，因此选择订阅中单价最低的 Haiku 系列；按系列关键字匹配映射而不写死模型编码，
// 避免模型版本更新后失效。没有 Haiku 映射时回退连接测试的默认选择。
const quotaProbeModelFamily = "haiku"

// QuotaRefreshResult 是手动刷新订阅额度的结果。OK 表示已从上游响应读取并保存额度；
// 上游以 429 拒绝但返回了统一限额时同样视为刷新成功，额度会显示为耗尽。
type QuotaRefreshResult struct {
	OK                     bool            `json:"ok"`
	Code                   string          `json:"code"`
	HTTPStatus             int             `json:"httpStatus,omitempty"`
	LatencyMS              int64           `json:"latencyMs"`
	ProviderModelMappingID int64           `json:"providerModelMappingId,string,omitempty"`
	TestedModelID          int64           `json:"testedModelId,string,omitempty"`
	TestedModelCode        string          `json:"testedModelCode,omitempty"`
	Quotas                 []ResourceQuota `json:"quotas"`
}

// RefreshSubscriptionQuota 供获后台管理授权的成员 Key 按需刷新订阅额度，返回刷新结果码（成功为 OK）。
// 以系统身份执行刷新，审计操作人名称记录触发的 Key。
func (s *ResourceService) RefreshSubscriptionQuota(ctx context.Context, identity appsec.PrincipalIdentity, id int64, meta appsec.RequestMeta) (string, error) {
	if identity.ID <= 0 || identity.AccessKeyID <= 0 {
		return "", appsec.ErrUnauthenticated
	}
	actor := admin.Identity{System: true, DisplayName: "Member Key #" + strconv.FormatInt(identity.AccessKeyID, 10)}
	result, err := s.RefreshResourceQuota(ctx, actor, id, meta)
	return result.Code, err
}

// RefreshResourceQuota 主动读取订阅额度并保存：Claude 订阅发送一次最小推理请求，只从响应头读取统一限额；
// 其他订阅（如 Codex）由认证适配器只读查询额度。它只更新额度与凭据元数据，不恢复或封禁凭据：
// 凭据可用性由连接测试负责（会复核模型映射），刷新额度失败时只返回错误码，避免一次读取额度的请求改变路由状态。
func (s *ResourceService) RefreshResourceQuota(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (QuotaRefreshResult, error) {
	if id <= 0 {
		return QuotaRefreshResult{}, appsec.ErrInvalidArgument
	}
	var resource ResourceRecord
	var provider Provider
	var mapping ProviderMapping
	var model Model
	adapterQuery := false
	err := s.store.ReadResource(ctx, actor, func(r ResourceReadSession) error {
		var err error
		resource, err = r.Resource(ctx, id)
		if err != nil {
			return err
		}
		if resource.AuthType != AuthTypeSubscription {
			return ErrQuotaRefreshUnsupported
		}
		if resource.AuthAdapter != AuthAdapterClaudeCode {
			adapterQuery = true
			return nil
		}
		provider, err = r.Provider(ctx, resource.ProviderID)
		if err != nil {
			return err
		}
		mappings, err := r.ProviderMappings(ctx, resource.ProviderID)
		if err != nil || len(mappings) == 0 {
			return err
		}
		models, err := readAllModels(ctx, r)
		if err != nil {
			return err
		}
		mapping, model = quotaProbeMapping(provider, mappings, models)
		return nil
	})
	if err != nil {
		return QuotaRefreshResult{}, err
	}
	if adapterQuery {
		return s.refreshAdapterQuota(ctx, actor, id, meta)
	}
	upstreamModelCode := testedMappingCode(mapping, model)
	result := QuotaRefreshResult{Code: "PROVIDER_MODEL_MAPPING_REQUIRED"}
	if mapping.ID > 0 {
		result.ProviderModelMappingID, result.TestedModelID, result.TestedModelCode = mapping.ID, model.ID, upstreamModelCode
	}
	var observed []ResourceQuota
	plain, decryptErr := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
	if decryptErr != nil {
		result.Code = "CREDENTIAL_UNRECOVERABLE"
	} else {
		defer clear(plain)
		subscription := s.subscriptionAdapter(resource.AuthAdapter)
		if subscription == nil || !subscription.SupportsProvider(provider) {
			result.Code = "SUBSCRIPTION_ADAPTER_UNAVAILABLE"
		} else if mapping.ID > 0 {
			if s.tester == nil {
				return QuotaRefreshResult{}, appsec.ErrUnavailable
			}
			// 订阅 Token 固定发往 Anthropic 官方主机，端点仅用于确认服务商声明了 Anthropic 协议。
			selectedProtocol, baseURL, networkScope := providerEndpoint(provider, "ANTHROPIC")
			proxy, proxyErr := s.decryptedProviderProxy(provider)
			if baseURL == "" {
				result.Code = "PROVIDER_UNAVAILABLE"
			} else if proxyErr != nil {
				result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
			} else {
				probe := s.tester.Test(ctx, ConnectionTarget{
					ProviderCode: provider.Code, Protocol: selectedProtocol, BaseURL: baseURL, NetworkScope: networkScope,
					UpstreamModelCode: upstreamModelCode, AuthType: resource.AuthType, AuthAdapter: resource.AuthAdapter,
					QuotaProbe: true,
				}, plain, proxy)
				result.Code, result.HTTPStatus, result.LatencyMS = probe.Code, probe.HTTPStatus, probe.LatencyMS
				observed = probe.ObservedQuotas
				if len(observed) > 0 {
					result.OK, result.Code = true, "OK"
				} else if probe.OK {
					result.Code = "UPSTREAM_QUOTA_UNAVAILABLE"
				}
			}
		}
	}
	served := result.HTTPStatus >= 200 && result.HTTPStatus < 300
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = s.store.WriteResource(auditCtx, actor, func(w ResourceSession) error {
		current, err := w.Resource(auditCtx, id)
		if err != nil {
			return err
		}
		if current.Version != resource.Version {
			result.OK, result.Code = false, "RESOURCE_CHANGED"
		} else if result.OK {
			checkedAt := businessTime(s.now)
			current.QuotaStatus, current.QuotaResetsAt = ObservedQuotaStatus(observed, served)
			current.QuotaCheckedAt = &checkedAt
			current.UpdatedAt = checkedAt
			if err := w.UpdateResource(auditCtx, current); err != nil {
				return err
			}
			if err := w.ReplaceResourceQuotas(auditCtx, id, observed); err != nil {
				return err
			}
			result.Quotas = observed
		}
		code := ""
		if !result.OK {
			code = result.Code
		}
		auditResult := result
		auditResult.Quotas = nil
		return w.Audit(auditCtx, Audit{
			Event: operation.ResourceQuotaRefresh, Target: "RESOURCE", ID: id, Name: resource.Name,
			After: map[string]any{"refresh": auditResult, "quotaStatus": current.QuotaStatus}, ErrorCode: code,
		}, meta)
	})
	return result, err
}

// refreshAdapterQuota 通过认证适配器只读查询订阅额度（如 Codex App Server 的 account/rateLimits/read）。
// 查询可能轮换 refresh token：与连接测试一样持有网关刷新锁直到结果落盘，并在加锁后重新读取最新凭据。
func (s *ResourceService) refreshAdapterQuota(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (QuotaRefreshResult, error) {
	release, err := s.lockSubscriptionRefresh(ctx, id)
	if err != nil {
		return QuotaRefreshResult{}, err
	}
	defer release()
	var resource ResourceRecord
	var provider Provider
	err = s.store.ReadResource(ctx, actor, func(r ResourceReadSession) error {
		var err error
		if resource, err = r.Resource(ctx, id); err != nil {
			return err
		}
		if resource.AuthType != AuthTypeSubscription || resource.AuthAdapter == AuthAdapterClaudeCode {
			return ErrQuotaRefreshUnsupported
		}
		provider, err = r.Provider(ctx, resource.ProviderID)
		return err
	})
	if err != nil {
		return QuotaRefreshResult{}, err
	}
	result := QuotaRefreshResult{Code: "SUBSCRIPTION_ADAPTER_UNAVAILABLE"}
	var probe SubscriptionProbe
	var refreshedSealed catalog.SealedCredential
	startedAt := time.Now()
	plain, decryptErr := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
	if decryptErr == nil {
		defer clear(plain)
	}
	subscription := s.subscriptionAdapter(resource.AuthAdapter)
	if decryptErr != nil {
		result.Code = "CREDENTIAL_UNRECOVERABLE"
	} else if subscription != nil && subscription.SupportsProvider(provider) {
		proxy, proxyErr := s.decryptedProviderProxy(provider)
		if proxyErr != nil {
			result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
		} else if probed, probeErr := subscription.Probe(ctx, plain, proxy); probeErr != nil {
			result.Code = subscriptionConnectionCode(probeErr)
			if sealed, inspection, ok := s.sealRotatedCredential(actor, resource.Resource, subscription, probeErr); ok {
				refreshedSealed = sealed
				resource.CredentialRefreshedAt = inspection.CredentialRefreshedAt
				resource.CredentialExpiresAt = inspection.CredentialExpiresAt
			}
		} else {
			probe = probed
			normalizeSubscriptionProbe(&probe)
			result.OK, result.Code = len(probe.Quotas) > 0, "OK"
			if !result.OK {
				result.Code = "UPSTREAM_QUOTA_UNAVAILABLE"
			}
			if len(probe.Credential) > 0 {
				sealed, encryptErr := s.cipher.Encrypt(probe.Credential, owner(actor, resource.Resource))
				if encryptErr != nil {
					result.OK, result.Code = false, "CREDENTIAL_UNRECOVERABLE"
				} else {
					refreshedSealed = sealed
					resource.CredentialRefreshedAt = probe.Inspection.CredentialRefreshedAt
					resource.CredentialExpiresAt = probe.Inspection.CredentialExpiresAt
				}
			}
			clear(probe.Credential)
		}
	}
	result.LatencyMS = time.Since(startedAt).Milliseconds()
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = s.store.WriteResource(auditCtx, actor, func(w ResourceSession) error {
		current, err := w.Resource(auditCtx, id)
		if err != nil {
			return err
		}
		if current.Version != resource.Version {
			result.OK, result.Code = false, "RESOURCE_CHANGED"
		}
		now := businessTime(s.now)
		if !result.OK {
			// 未整体更新资源时，上游可能已轮换凭据，仍须单独保存新凭据，否则旧 refresh token 已失效。
			if err := persistRotatedCredential(auditCtx, w, current, resource.Sealed, refreshedSealed,
				resource.CredentialRefreshedAt, resource.CredentialExpiresAt, now); err != nil {
				return err
			}
		} else {
			current.PlanCode = stringPointer(probe.Inspection.PlanCode)
			current.ExternalAccountRef = stringPointer(probe.Inspection.AccountRef)
			if probe.Inspection.ExpiresAt != nil {
				current.ExpiresAt = probe.Inspection.ExpiresAt
			}
			if refreshedSealed.KeyVersion != 0 {
				current.Sealed = refreshedSealed
				current.CredentialRefreshedAt = resource.CredentialRefreshedAt
				current.CredentialExpiresAt = resource.CredentialExpiresAt
			}
			current.QuotaStatus, current.QuotaResetsAt = AggregateQuota(probe.Quotas)
			current.QuotaCheckedAt = &now
			current.UpdatedAt = now
			if err := w.UpdateResource(auditCtx, current); err != nil {
				return err
			}
			if err := w.ReplaceResourceQuotas(auditCtx, id, probe.Quotas); err != nil {
				return err
			}
			result.Quotas = probe.Quotas
		}
		code := ""
		if !result.OK {
			code = result.Code
		}
		auditResult := result
		auditResult.Quotas = nil
		return w.Audit(auditCtx, Audit{
			Event: operation.ResourceQuotaRefresh, Target: "RESOURCE", ID: id, Name: resource.Name,
			After: map[string]any{"refresh": auditResult, "quotaStatus": current.QuotaStatus}, ErrorCode: code,
		}, meta)
	})
	return result, err
}

// quotaProbeMapping 选择刷新额度使用的模型映射：优先 Haiku 系列，其次沿用连接测试的默认排序。
func quotaProbeMapping(provider Provider, mappings []ProviderMapping, models []Model) (ProviderMapping, Model) {
	modelsByID := make(map[int64]Model, len(models))
	for _, model := range models {
		modelsByID[model.ID] = model
	}
	var selected ProviderMapping
	var selectedModel Model
	bestRank := int(^uint(0) >> 1)
	for _, mapping := range mappings {
		model, exists := modelsByID[mapping.ModelID]
		if mapping.ProviderID != provider.ID || !exists || model.Status != "ACTIVE" {
			continue
		}
		code := testedMappingCode(mapping, model)
		rank := 1 + connectionProbeModelRank(provider.Code, code)
		if strings.Contains(strings.ToLower(code), quotaProbeModelFamily) {
			rank = 0
		}
		if rank < bestRank {
			selected, selectedModel, bestRank = mapping, model, rank
		}
	}
	return selected, selectedModel
}
