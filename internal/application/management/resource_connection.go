package management

import (
	"context"
	"errors"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

func (s *ResourceService) TestResource(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (ConnectionResult, error) {
	return s.TestResourceSelection(ctx, actor, id, "", 0, meta)
}

// TestResourceProtocol 使用指定的上游协议测试 API Key 资源；protocol 为空时
// 优先 Anthropic，未配置 Anthropic 时回退 OpenAI。订阅资源仍由认证适配器探测。
func (s *ResourceService) TestResourceProtocol(ctx context.Context, actor admin.Identity, id int64, protocol string, meta appsec.RequestMeta) (ConnectionResult, error) {
	return s.TestResourceSelection(ctx, actor, id, protocol, 0, meta)
}

// TestResourceSelection 使用指定的上游协议和模型映射测试 API Key 资源。
// providerModelMappingID 为零时自动选择有效映射；指定后不会回退到其他模型。
func (s *ResourceService) TestResourceSelection(ctx context.Context, actor admin.Identity, id int64, protocol string, providerModelMappingID int64, meta appsec.RequestMeta) (ConnectionResult, error) {
	if id <= 0 || providerModelMappingID < 0 {
		return ConnectionResult{}, appsec.ErrInvalidArgument
	}
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	if protocol != "" && !validProviderProtocol(protocol) {
		return ConnectionResult{}, appsec.ErrInvalidArgument
	}
	var resource ResourceRecord
	var provider Provider
	var mappings []ProviderMapping
	selectedMappingID := int64(0)
	selectedModelID := int64(0)
	upstreamModelCode := ""
	err := s.store.Read(ctx, actor, func(r Reader) error {
		var err error
		resource, err = r.Resource(ctx, id)
		if err != nil {
			return err
		}
		provider, err = r.Provider(ctx, resource.ProviderID)
		if err != nil {
			return err
		}
		mappings, err = r.ProviderMappings(ctx, resource.ProviderID)
		if err != nil {
			return err
		}
		if resource.AuthType == AuthTypeSubscription {
			if providerModelMappingID > 0 {
				return appsec.ErrInvalidArgument
			}
			return nil
		}
		if len(mappings) == 0 {
			if providerModelMappingID > 0 {
				return appsec.ErrInvalidArgument
			}
			return nil
		}
		models, modelErr := readAllModels(ctx, r)
		if modelErr != nil {
			return modelErr
		}
		modelsByID := make(map[int64]Model, len(models))
		for _, model := range models {
			modelsByID[model.ID] = model
		}
		bestRank := int(^uint(0) >> 1)
		for _, mapping := range mappings {
			if mapping.ProviderID != resource.ProviderID {
				continue
			}
			if providerModelMappingID > 0 && mapping.ID != providerModelMappingID {
				continue
			}
			model, exists := modelsByID[mapping.ModelID]
			if !exists || model.Status != "ACTIVE" {
				continue
			}
			candidate := strings.TrimSpace(mapping.UpstreamModelCode)
			if candidate == "" {
				candidate = model.Code
			}
			rank := connectionProbeModelRank(provider.Code, candidate)
			if selectedMappingID == 0 || rank < bestRank {
				selectedMappingID = mapping.ID
				selectedModelID = model.ID
				upstreamModelCode = candidate
				bestRank = rank
			}
			if providerModelMappingID > 0 || bestRank == 0 {
				break
			}
		}
		if providerModelMappingID > 0 && selectedMappingID == 0 {
			return appsec.ErrInvalidArgument
		}
		return nil
	})
	if err != nil {
		return ConnectionResult{}, err
	}
	result := ConnectionResult{
		Code:                   "PROVIDER_UNAVAILABLE",
		ProviderModelMappingID: selectedMappingID,
		TestedModelID:          selectedModelID,
		TestedModelCode:        upstreamModelCode,
	}
	var subscriptionProbe *SubscriptionProbe
	var refreshedSealed catalog.SealedCredential
	plain, decryptErr := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
	if decryptErr != nil {
		result.Code = "CREDENTIAL_UNRECOVERABLE"
	} else if resource.AuthType == AuthTypeSubscription {
		defer clear(plain)
		startedAt := time.Now()
		subscription := s.subscriptionAdapter(resource.AuthAdapter)
		if subscription == nil || !subscription.SupportsProvider(provider) {
			result.Code = "SUBSCRIPTION_ADAPTER_UNAVAILABLE"
		} else {
			proxy, proxyErr := s.decryptedProviderProxy(provider)
			if proxyErr != nil {
				result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
			} else if probe, probeErr := subscription.Probe(ctx, plain, proxy); probeErr != nil {
				result.Code = subscriptionConnectionCode(probeErr)
			} else {
				normalizeSubscriptionProbe(&probe)
				subscriptionProbe = &probe
				result.ResetCredits = probe.ResetCredits
				resource.PlanCode = stringPointer(probe.Inspection.PlanCode)
				resource.ExternalAccountRef = stringPointer(probe.Inspection.AccountRef)
				resource.CredentialRefreshedAt = probe.Inspection.CredentialRefreshedAt
				resource.CredentialExpiresAt = probe.Inspection.CredentialExpiresAt
				if probe.Inspection.ExpiresAt != nil {
					resource.ExpiresAt = probe.Inspection.ExpiresAt
				}
				resource.QuotaStatus, resource.QuotaResetsAt = aggregateQuota(probe.Quotas)
				now := time.Now().UTC().Truncate(time.Microsecond)
				resource.QuotaCheckedAt = &now
				result.OK, result.Code = true, "OK"
				if len(probe.Credential) > 0 {
					refreshedSealed, probeErr = s.cipher.Encrypt(probe.Credential, owner(actor, resource.Resource))
					if probeErr != nil {
						result.OK, result.Code = false, "CREDENTIAL_UNRECOVERABLE"
						subscriptionProbe = nil
					}
				}
				clear(probe.Credential)
			}
		}
		result.LatencyMS = time.Since(startedAt).Milliseconds()
	} else if len(mappings) == 0 || strings.TrimSpace(upstreamModelCode) == "" {
		result.Code = "PROVIDER_MODEL_MAPPING_REQUIRED"
	} else {
		defer clear(plain)
		selectedProtocol, baseURL, networkScope := preferredProviderEndpoint(provider)
		if protocol != "" {
			selectedProtocol, baseURL, networkScope = providerEndpoint(provider, protocol)
			if baseURL == "" {
				return ConnectionResult{}, appsec.ErrInvalidArgument
			}
		}
		target := ConnectionTarget{
			ProviderCode: provider.Code, Protocol: selectedProtocol, BaseURL: baseURL, NetworkScope: networkScope,
			UpstreamModelCode: upstreamModelCode,
			AuthType:          resource.AuthType, AuthAdapter: resource.AuthAdapter,
		}
		if baseURL == "" {
			result.Code = "PROVIDER_UNAVAILABLE"
		} else {
			proxy, proxyErr := s.decryptedProviderProxy(provider)
			if proxyErr != nil {
				result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
			} else {
				result = s.tester.Test(ctx, target, plain, proxy)
				result.ProviderModelMappingID = selectedMappingID
				result.TestedModelID = selectedModelID
				result.TestedModelCode = upstreamModelCode
			}
		}
	}
	// 网络调用不占有数据库事务；即使调用方取消，仍尽力保存有界审计记录。
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = s.store.Write(auditCtx, actor, func(w Writer) error {
		current, err := w.Resource(auditCtx, id)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(resource.UpdatedAt) {
			result.OK = false
			result.Code = "RESOURCE_CHANGED"
		}
		if result.OK && subscriptionProbe != nil {
			current.PlanCode = resource.PlanCode
			current.ExternalAccountRef = resource.ExternalAccountRef
			current.CredentialRefreshedAt = resource.CredentialRefreshedAt
			current.CredentialExpiresAt = resource.CredentialExpiresAt
			current.ExpiresAt = resource.ExpiresAt
			current.QuotaStatus = resource.QuotaStatus
			current.QuotaCheckedAt = resource.QuotaCheckedAt
			current.QuotaResetsAt = resource.QuotaResetsAt
			if refreshedSealed.KeyVersion != 0 {
				current.Sealed = refreshedSealed
			}
			current.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
			if err := w.UpdateResource(auditCtx, current); err != nil {
				return err
			}
			if err := w.ReplaceResourceQuotas(auditCtx, id, subscriptionProbe.Quotas); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if result.OK {
			if err := w.RestoreResourceRuntime(auditCtx, id, now); err != nil {
				return err
			}
		} else if reason := connectionBlockReason(result.Code); reason != "" {
			var status *int32
			if result.HTTPStatus > 0 {
				value := int32(result.HTTPStatus)
				status = &value
			}
			if err := w.BlockResourceRuntime(auditCtx, id, reason, result.Code, status, now); err != nil {
				return err
			}
		}
		code := ""
		if !result.OK {
			code = result.Code
		}
		auditResult := result
		if result.ResetCredits != nil {
			// 重置卡 ID 是后续兑换使用的 opaque 值，不应进入审计日志。
			auditResult.ResetCredits = &RateLimitResetCredits{AvailableCount: result.ResetCredits.AvailableCount}
		}
		return w.Audit(auditCtx, Audit{Event: operation.ResourceConnectionTest, Target: "RESOURCE", ID: id, Name: resource.Name, After: map[string]any{"test": auditResult, "testedUpdatedAt": resource.UpdatedAt}, ErrorCode: code}, meta)
	})
	return result, err
}

func connectionProbeModelRank(providerCode, modelCode string) int {
	if providerCode != catalog.GoogleOfficialCode {
		return 0
	}
	code := strings.ToLower(strings.TrimSpace(modelCode))
	if code == "gemini-3.6-flash" {
		return 0
	}
	if !strings.HasPrefix(code, "gemini-") {
		return 30
	}
	for _, specialized := range []string{"audio", "computer-use", "image", "live", "robotics", "transcribe", "tts"} {
		if strings.Contains(code, specialized) {
			return 30
		}
	}
	if strings.Contains(code, "preview") || strings.Contains(code, "-exp") {
		return 20
	}
	if strings.HasPrefix(code, "gemini-2.5-") {
		return 25
	}
	return 10
}

func subscriptionConnectionCode(err error) string {
	var connectionError SubscriptionConnectionError
	if errors.As(err, &connectionError) {
		switch connectionError.ConnectionCode() {
		case "CREDENTIAL_INVALID", "CODEX_APP_SERVER_UNAVAILABLE", "UPSTREAM_AUTH_FAILED",
			"UPSTREAM_BILLING_BLOCKED", "UPSTREAM_RATE_LIMITED", "UPSTREAM_TIMEOUT", "PROXY_SERVER_UNAVAILABLE",
			"UPSTREAM_UNAVAILABLE", "UPSTREAM_INVALID_RESPONSE":
			return connectionError.ConnectionCode()
		}
	}
	return "SUBSCRIPTION_UNAVAILABLE"
}

// ConsumeResourceResetCredit 使用一张 ChatGPT 官方额度重置卡，并以官方返回的
// 最新额度快照更新资源。idempotencyKey 在同一次逻辑兑换重试时必须保持不变。
func (s *ResourceService) ConsumeResourceResetCredit(ctx context.Context, actor admin.Identity, id int64, idempotencyKey, creditID string, meta appsec.RequestMeta) (ResetCreditConsumeResult, error) {
	if id <= 0 || !validText(idempotencyKey, 128) || (creditID != "" && !validText(creditID, 256)) {
		return ResetCreditConsumeResult{}, appsec.ErrInvalidArgument
	}
	var resource ResourceRecord
	var provider Provider
	if err := s.store.Read(ctx, actor, func(r Reader) error {
		var err error
		resource, err = r.Resource(ctx, id)
		if err != nil {
			return err
		}
		provider, err = r.Provider(ctx, resource.ProviderID)
		return err
	}); err != nil {
		return ResetCreditConsumeResult{}, err
	}
	if resource.AuthType != AuthTypeSubscription || resource.AuthAdapter != AuthAdapterOpenAICodex {
		return ResetCreditConsumeResult{}, ErrResetCreditUnsupported
	}
	adapter := s.subscriptionAdapter(resource.AuthAdapter)
	consumer, ok := adapter.(SubscriptionResetCreditConsumer)
	if adapter == nil || !adapter.SupportsProvider(provider) || !ok {
		return ResetCreditConsumeResult{}, ErrResetCreditUnsupported
	}
	plain, err := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
	if err != nil {
		return ResetCreditConsumeResult{}, ErrCredential
	}
	defer clear(plain)
	proxy, err := s.decryptedProviderProxy(provider)
	if err != nil {
		return ResetCreditConsumeResult{}, appsec.ErrUnavailable
	}
	consumed, err := consumer.ConsumeResetCredit(ctx, plain, proxy, idempotencyKey, creditID)
	if err != nil {
		return ResetCreditConsumeResult{}, appsec.ErrUnavailable
	}
	defer clear(consumed.Probe.Credential)
	normalizeSubscriptionProbe(&consumed.Probe)
	if consumed.Outcome != "reset" && consumed.Outcome != "alreadyRedeemed" && consumed.Outcome != "nothingToReset" && consumed.Outcome != "noCredit" {
		return ResetCreditConsumeResult{}, appsec.ErrUnavailable
	}
	var refreshedSealed catalog.SealedCredential
	if len(consumed.Probe.Credential) > 0 {
		refreshedSealed, err = s.cipher.Encrypt(consumed.Probe.Credential, owner(actor, resource.Resource))
		if err != nil {
			return ResetCreditConsumeResult{}, ErrCredential
		}
	}
	resource.PlanCode = stringPointer(consumed.Probe.Inspection.PlanCode)
	resource.ExternalAccountRef = stringPointer(consumed.Probe.Inspection.AccountRef)
	resource.CredentialRefreshedAt = consumed.Probe.Inspection.CredentialRefreshedAt
	resource.CredentialExpiresAt = consumed.Probe.Inspection.CredentialExpiresAt
	if consumed.Probe.Inspection.ExpiresAt != nil {
		resource.ExpiresAt = consumed.Probe.Inspection.ExpiresAt
	}
	resource.QuotaStatus, resource.QuotaResetsAt = aggregateQuota(consumed.Probe.Quotas)
	checkedAt := time.Now().UTC().Truncate(time.Microsecond)
	resource.QuotaCheckedAt = &checkedAt
	if refreshedSealed.KeyVersion != 0 {
		resource.Sealed = refreshedSealed
	}

	result := ResetCreditConsumeResult{Outcome: consumed.Outcome, ResetCredits: consumed.Probe.ResetCredits}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = s.store.Write(auditCtx, actor, func(w Writer) error {
		current, err := w.Resource(auditCtx, id)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(resource.UpdatedAt) {
			return ErrConflict
		}
		resource.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if err := w.UpdateResource(auditCtx, resource); err != nil {
			return err
		}
		if err := w.ReplaceResourceQuotas(auditCtx, id, consumed.Probe.Quotas); err != nil {
			return err
		}
		if err := w.RestoreResourceRuntime(auditCtx, id, resource.UpdatedAt); err != nil {
			return err
		}
		available := 0
		if result.ResetCredits != nil {
			available = result.ResetCredits.AvailableCount
		}
		return w.Audit(auditCtx, Audit{
			Event: operation.ResourceRateLimitReset, Target: "RESOURCE", ID: id, Name: resource.Name,
			After: map[string]any{"outcome": result.Outcome, "availableResetCredits": available},
		}, meta)
	})
	return result, err
}

func connectionBlockReason(code string) string {
	switch code {
	case "UPSTREAM_BILLING_BLOCKED":
		return "BILLING"
	case "UPSTREAM_ACCOUNT_SUSPENDED":
		return "ACCOUNT_SUSPENDED"
	case "UPSTREAM_AUTH_FAILED":
		return "AUTHENTICATION"
	case "CREDENTIAL_UNRECOVERABLE":
		return "CREDENTIAL_UNRECOVERABLE"
	default:
		return ""
	}
}

func aggregateQuota(quotas []ResourceQuota) (string, *time.Time) {
	status := QuotaUnknown
	var resetsAt *time.Time
	exhaustedResetUnknown := false
	for _, quota := range quotas {
		switch quota.Status {
		case QuotaExhausted:
			if status != QuotaExhausted {
				resetsAt = nil
				exhaustedResetUnknown = false
			}
			status = QuotaExhausted
			if quota.ResetsAt == nil {
				exhaustedResetUnknown = true
				resetsAt = nil
			} else if !exhaustedResetUnknown && (resetsAt == nil || quota.ResetsAt.After(*resetsAt)) {
				value := *quota.ResetsAt
				resetsAt = &value
			}
		case QuotaNearLimit:
			if status != QuotaExhausted {
				status = QuotaNearLimit
				if quota.ResetsAt != nil && (resetsAt == nil || quota.ResetsAt.Before(*resetsAt)) {
					value := *quota.ResetsAt
					resetsAt = &value
				}
			}
		case QuotaAvailable:
			if status == QuotaUnknown {
				status = QuotaAvailable
				if quota.ResetsAt != nil {
					value := *quota.ResetsAt
					resetsAt = &value
				}
			} else if status == QuotaAvailable && quota.ResetsAt != nil && (resetsAt == nil || quota.ResetsAt.Before(*resetsAt)) {
				value := *quota.ResetsAt
				resetsAt = &value
			}
		}
	}
	return status, resetsAt
}
