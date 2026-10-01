package management

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type ResourceService struct {
	store         Store
	ids           shared.IDGenerator
	cipher        Cipher
	tester        ConnectionTester
	subscriptions []SubscriptionAdapter
}

func (s *ResourceService) next(ctx context.Context) (int64, error) {
	return nextID(ctx, s.ids)
}

func (s *ResourceService) subscriptionAdapter(code string) SubscriptionAdapter {
	for _, adapter := range s.subscriptions {
		if adapter.Supports(code) {
			return adapter
		}
	}
	return nil
}

func (s *ResourceService) decryptedProviderProxy(provider Provider) (*catalog.OutboundProxy, error) {
	return decryptProviderProxy(s.cipher, provider)
}

func decryptProviderProxy(cipher Cipher, provider Provider) (*catalog.OutboundProxy, error) {
	if !provider.ProxyEnabled {
		return nil, nil
	}
	urlBytes, err := cipher.DecryptProviderProxy(provider.ProxyURLSealed, proxyOwner(provider.ID, "url"))
	if err != nil {
		return nil, err
	}
	proxy := &catalog.OutboundProxy{URL: string(urlBytes), Headers: map[string]string{}}
	clear(urlBytes)
	if provider.ProxyHeadersSealed.KeyVersion == 0 {
		return proxy, nil
	}
	headerBytes, err := cipher.DecryptProviderProxy(provider.ProxyHeadersSealed, proxyOwner(provider.ID, "headers"))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(headerBytes, &proxy.Headers)
	clear(headerBytes)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}
func (s *ResourceService) CreateResource(ctx context.Context, actor admin.Identity, providerID int64, name, credential string, meta appsec.RequestMeta) (Resource, error) {
	return s.CreateAuthenticationResource(ctx, actor, CreateResourceInput{
		ProviderID: providerID, Name: name, Credential: credential,
		AuthType: AuthTypeAPIKey, AuthAdapter: AuthAdapterAPIKey, Priority: 100,
	}, meta)
}

func (s *ResourceService) CreateAuthenticationResource(ctx context.Context, actor admin.Identity, input CreateResourceInput, meta appsec.RequestMeta) (Resource, error) {
	input.EffectiveAt = utcTimePointer(input.EffectiveAt)
	input.ExpiresAt = utcTimePointer(input.ExpiresAt)
	if input.ProviderID <= 0 || !validText(input.Name, 128) || input.Priority < 0 {
		return Resource{}, appsec.ErrInvalidArgument
	}
	if input.AuthType == "" {
		input.AuthType = AuthTypeAPIKey
	}
	if input.AuthAdapter == "" && input.AuthType == AuthTypeAPIKey {
		input.AuthAdapter = AuthAdapterAPIKey
	}
	if input.EffectiveAt != nil && input.ExpiresAt != nil && !input.ExpiresAt.After(*input.EffectiveAt) {
		return Resource{}, appsec.ErrInvalidArgument
	}

	plain := []byte(input.Credential)
	defer func() { clear(plain) }()
	var inspection SubscriptionInspection
	var subscription SubscriptionAdapter
	switch input.AuthType {
	case AuthTypeAPIKey:
		if input.AuthAdapter != AuthAdapterAPIKey || !validCredential(input.Credential) {
			return Resource{}, appsec.ErrInvalidArgument
		}
	case AuthTypeSubscription:
		subscription = s.subscriptionAdapter(input.AuthAdapter)
		if !validSubscriptionCredential(input.Credential) || subscription == nil {
			return Resource{}, appsec.ErrInvalidArgument
		}
		var err error
		inspection, err = subscription.Inspect(plain)
		if err != nil {
			return Resource{}, appsec.ErrInvalidArgument
		}
		var provider Provider
		if err := s.store.Read(ctx, actor, func(r Reader) error {
			var err error
			provider, err = r.Provider(ctx, input.ProviderID)
			return err
		}); err != nil {
			return Resource{}, err
		}
		if !subscription.SupportsProvider(provider) {
			return Resource{}, appsec.ErrInvalidArgument
		}
		if inspection.AccountRef != "" {
			input.Name = subscriptionResourceName(input.Name, inspection.AccountRef)
			if !validText(input.Name, 128) {
				return Resource{}, appsec.ErrInvalidArgument
			}
		}
		// 保存只做本地校验；在线认证和额度读取由独立的测试连接负责。
	default:
		return Resource{}, appsec.ErrInvalidArgument
	}

	id, err := s.next(ctx)
	if err != nil {
		return Resource{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	resource := Resource{
		ID: id, ProviderID: input.ProviderID, Name: input.Name,
		AuthType: input.AuthType, AuthAdapter: input.AuthAdapter, Priority: input.Priority,
		EffectiveAt: input.EffectiveAt, ExpiresAt: input.ExpiresAt, QuotaStatus: QuotaUnknown,
		RuntimeStatus: "HEALTHY", CredentialConfigured: true, CreatedAt: now, UpdatedAt: now,
	}
	if input.AuthType == AuthTypeSubscription {
		subscriptionType := SubscriptionPersonal
		resource.SubscriptionType = &subscriptionType
		resource.ExternalAccountRef = stringPointer(inspection.AccountRef)
		resource.PlanCode = stringPointer(inspection.PlanCode)
		resource.CredentialRefreshedAt = utcTimePointer(inspection.CredentialRefreshedAt)
		resource.CredentialExpiresAt = utcTimePointer(inspection.CredentialExpiresAt)
		if resource.ExpiresAt == nil {
			resource.ExpiresAt = utcTimePointer(inspection.ExpiresAt)
		}
	}
	sealed, err := s.cipher.Encrypt(plain, owner(actor, resource))
	if err != nil {
		return Resource{}, appsec.ErrUnavailable
	}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		if _, err := w.Provider(ctx, input.ProviderID); err != nil {
			return err
		}
		if err := w.CreateResource(ctx, ResourceRecord{Resource: resource, Sealed: sealed}); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ResourceCreate, Target: "RESOURCE", ID: id, Name: input.Name, After: resource}, meta)
	})
	return resource, err
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func accountRefSuffix(accountRef string) string {
	characters := []rune(accountRef)
	if len(characters) <= 6 {
		return accountRef
	}
	return string(characters[len(characters)-6:])
}

func subscriptionResourceName(name, accountRef string) string {
	return name + " · " + accountRefSuffix(accountRef)
}

func replaceSubscriptionResourceName(name string, previous *string, accountRef string) string {
	if previous != nil && *previous != "" {
		name = strings.TrimSuffix(name, " · "+accountRefSuffix(*previous))
	}
	return subscriptionResourceName(name, accountRef)
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func normalizeSubscriptionProbe(probe *SubscriptionProbe) {
	probe.Inspection.ExpiresAt = utcTimePointer(probe.Inspection.ExpiresAt)
	probe.Inspection.CredentialRefreshedAt = utcTimePointer(probe.Inspection.CredentialRefreshedAt)
	probe.Inspection.CredentialExpiresAt = utcTimePointer(probe.Inspection.CredentialExpiresAt)
	for index := range probe.Quotas {
		probe.Quotas[index].ResetsAt = utcTimePointer(probe.Quotas[index].ResetsAt)
		probe.Quotas[index].ObservedAt = probe.Quotas[index].ObservedAt.UTC()
	}
	if probe.ResetCredits != nil {
		for index := range probe.ResetCredits.Credits {
			probe.ResetCredits.Credits[index].GrantedAt = probe.ResetCredits.Credits[index].GrantedAt.UTC()
			probe.ResetCredits.Credits[index].ExpiresAt = utcTimePointer(probe.ResetCredits.Credits[index].ExpiresAt)
		}
	}
}

func (s *ResourceService) UpdateCredential(ctx context.Context, actor admin.Identity, id int64, credential string, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	var original ResourceRecord
	var provider Provider
	if err := s.store.Read(ctx, actor, func(r Reader) error {
		var err error
		original, err = r.Resource(ctx, id)
		if err != nil {
			return err
		}
		if original.AuthType == AuthTypeSubscription {
			provider, err = r.Provider(ctx, original.ProviderID)
		}
		return err
	}); err != nil {
		return err
	}
	plain := []byte(credential)
	defer func() { clear(plain) }()
	var inspection SubscriptionInspection
	if original.AuthType == AuthTypeAPIKey {
		if !validCredential(credential) {
			return appsec.ErrInvalidArgument
		}
	} else {
		subscription := s.subscriptionAdapter(original.AuthAdapter)
		if !validSubscriptionCredential(credential) || subscription == nil || !subscription.SupportsProvider(provider) {
			return appsec.ErrInvalidArgument
		}
		var err error
		inspection, err = subscription.Inspect(plain)
		if err != nil {
			return appsec.ErrInvalidArgument
		}
	}
	sealed, err := s.cipher.Encrypt(plain, owner(actor, original.Resource))
	if err != nil {
		return appsec.ErrUnavailable
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		record, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		if !record.UpdatedAt.Equal(original.UpdatedAt) {
			return ErrConflict
		}
		record.Sealed = sealed
		if record.AuthType == AuthTypeSubscription {
			if inspection.AccountRef != "" {
				record.Name = replaceSubscriptionResourceName(record.Name, record.ExternalAccountRef, inspection.AccountRef)
				if !validText(record.Name, 128) {
					return appsec.ErrInvalidArgument
				}
			}
			record.ExternalAccountRef = stringPointer(inspection.AccountRef)
			record.PlanCode = stringPointer(inspection.PlanCode)
			record.CredentialRefreshedAt = utcTimePointer(inspection.CredentialRefreshedAt)
			record.CredentialExpiresAt = utcTimePointer(inspection.CredentialExpiresAt)
			if inspection.ExpiresAt != nil {
				record.ExpiresAt = utcTimePointer(inspection.ExpiresAt)
			}
			record.QuotaStatus = QuotaUnknown
			record.QuotaResetsAt = nil
			record.QuotaCheckedAt = nil
		}
		record.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if err := w.UpdateResource(ctx, record); err != nil {
			return err
		}
		if record.AuthType == AuthTypeSubscription {
			if err := w.ReplaceResourceQuotas(ctx, id, nil); err != nil {
				return err
			}
		}
		return w.Audit(ctx, Audit{
			Event: operation.ResourceCredentialUpdate, Target: "RESOURCE", ID: id, Name: record.Name,
			After: map[string]any{"credentialConfigured": true, "authType": record.AuthType},
		}, meta)
	})
}

// ExportSubscriptionCredential 解密可导出的个人订阅凭据；返回值含敏感信息，调用方使用后必须清零。
func (s *ResourceService) ExportSubscriptionCredential(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) ([]byte, error) {
	if id <= 0 {
		return nil, appsec.ErrInvalidArgument
	}
	var credential []byte
	err := s.store.Write(ctx, actor, func(w Writer) error {
		resource, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		if resource.AuthType != AuthTypeSubscription || resource.AuthAdapter != AuthAdapterOpenAICodex ||
			resource.SubscriptionType == nil || *resource.SubscriptionType != SubscriptionPersonal {
			return ErrCredentialExportUnsupported
		}
		credential, err = s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
		if err != nil {
			return ErrCredential
		}
		if exporter, ok := s.subscriptionAdapter(resource.AuthAdapter).(SubscriptionCredentialExporter); ok {
			exported, exportErr := exporter.ExportCredential(credential)
			if exportErr != nil {
				return ErrCredential
			}
			clear(credential)
			credential = exported
		}
		return w.Audit(ctx, Audit{
			Event:  operation.ResourceCredentialExport,
			Target: "RESOURCE",
			ID:     id,
			Name:   resource.Name,
			After: map[string]string{
				"authType":    resource.AuthType,
				"authAdapter": resource.AuthAdapter,
			},
		}, meta)
	})
	if err != nil {
		clear(credential)
		return nil, err
	}
	return credential, nil
}

func (s *ResourceService) DeleteResource(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		record, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		deleted, err := w.DeleteResource(ctx, id, time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			return err
		}
		if !deleted {
			return ErrConflict
		}
		return w.Audit(ctx, Audit{Event: operation.ResourceDelete, Target: "RESOURCE", ID: id, Name: record.Name, Before: record.Resource, After: map[string]bool{"deleted": true}}, meta)
	})
}
