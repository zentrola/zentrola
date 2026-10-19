package management

import (
	"bytes"
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
	store         ResourceStore
	ids           shared.IDGenerator
	cipher        Cipher
	tester        ConnectionTester
	now           func() time.Time
	subscriptions []SubscriptionAdapter
	// refreshCoordinator 是网关使用的跨实例订阅刷新锁；注入后管理端改用它与网关互斥，
	// 未注入时回退到存储适配器的 SubscriptionRefreshLocker。
	refreshCoordinator SubscriptionRefreshCoordinator
}

type ResourceReadSession interface {
	ProviderReader
	ResourceReader
	modelLister
}

type ResourceSession interface {
	ResourceReadSession
	ResourceWriter
	AuditWriter
}

type ResourceStore interface {
	ReadResource(context.Context, admin.Identity, func(ResourceReadSession) error) error
	WriteResource(context.Context, admin.Identity, func(ResourceSession) error) error
}

// SubscriptionRefreshLocker 由存储适配器实现：在订阅探测或兑换重置卡期间持有与网关相同的
// per-resource 刷新锁，避免两边并发向上游轮换同一凭据导致旧 refresh token 失效。
type SubscriptionRefreshLocker interface {
	LockSubscriptionRefresh(context.Context, int64) (func(), error)
}

// SubscriptionRefreshCoordinator 与网关的 SubscriptionRefreshCoordinator 是同一把 per-resource 锁：
// 网关注入协调器后只持有它而不持有存储层的刷新锁，管理端必须获取同一把锁才能与网关互斥。
// 获取是非阻塞的：acquired 为 false 表示锁由其他持有者占用。
type SubscriptionRefreshCoordinator interface {
	AcquireSubscriptionRefreshResource(context.Context, int64) (func(), bool, error)
}

const (
	// 管理端等待网关释放刷新锁的上限与轮询间隔；超时后返回冲突，由管理员稍后重试。
	subscriptionRefreshLockWait = 10 * time.Second
	subscriptionRefreshLockPoll = 200 * time.Millisecond
)

// SubscriptionCredentialReplacer 由写事务实现：仅当库中密文仍等于 expected 时保存新凭据，
// 用于资源其他字段发生版本冲突时单独落盘已经轮换的订阅凭据。
type SubscriptionCredentialReplacer interface {
	ReplaceSubscriptionCredential(ctx context.Context, id int64, expected, replacement catalog.SealedCredential,
		refreshedAt, expiresAt *time.Time, at time.Time) (bool, error)
}

type resourceStoreAdapter struct{ Store }

// LockSubscriptionRefresh 在底层存储支持时持有订阅刷新锁；不支持时（如测试替身）返回空操作。
func (s resourceStoreAdapter) LockSubscriptionRefresh(ctx context.Context, id int64) (func(), error) {
	if locker, ok := s.Store.(SubscriptionRefreshLocker); ok {
		return locker.LockSubscriptionRefresh(ctx, id)
	}
	return func() {}, nil
}

func (s resourceStoreAdapter) ReadResource(ctx context.Context, actor admin.Identity, fn func(ResourceReadSession) error) error {
	if dependencyMissing(s.Store) {
		return appsec.ErrUnavailable
	}
	return s.Read(ctx, actor, func(reader Reader) error { return fn(reader) })
}

func (s resourceStoreAdapter) WriteResource(ctx context.Context, actor admin.Identity, fn func(ResourceSession) error) error {
	if dependencyMissing(s.Store) {
		return appsec.ErrUnavailable
	}
	return s.Write(ctx, actor, func(writer Writer) error { return fn(writer) })
}

func (s *ResourceService) lockSubscriptionRefresh(ctx context.Context, id int64) (func(), error) {
	if s.refreshCoordinator != nil {
		return s.acquireSubscriptionRefresh(ctx, id)
	}
	if locker, ok := s.store.(SubscriptionRefreshLocker); ok {
		return locker.LockSubscriptionRefresh(ctx, id)
	}
	return func() {}, nil
}

// acquireSubscriptionRefresh 在有限时间内轮询协调器的刷新锁。协调后端不可用时失败关闭：
// 无法确认网关未在刷新，就不能向上游轮换同一凭据。
func (s *ResourceService) acquireSubscriptionRefresh(ctx context.Context, id int64) (func(), error) {
	waitCtx, cancel := context.WithTimeout(ctx, subscriptionRefreshLockWait)
	defer cancel()
	var ticker *time.Ticker
	for {
		release, acquired, err := s.refreshCoordinator.AcquireSubscriptionRefreshResource(waitCtx, id)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if waitCtx.Err() != nil {
				return nil, ErrConflict
			}
			return nil, appsec.ErrUnavailable
		}
		if acquired && release != nil {
			return release, nil
		}
		if ticker == nil {
			ticker = time.NewTicker(subscriptionRefreshLockPoll)
			defer ticker.Stop()
		}
		select {
		case <-waitCtx.Done():
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, ErrConflict
		case <-ticker.C:
		}
	}
}

// persistRotatedCredential 在资源版本已变化时单独保存探测轮换出的新凭据：只有库中密文仍是探测前
// 读取的版本才更新，既不覆盖网关的并发刷新结果，也不会因其他字段冲突丢弃已经生效的新凭据。
func persistRotatedCredential(ctx context.Context, w ResourceSession, current ResourceRecord,
	read, replacement catalog.SealedCredential, refreshedAt, expiresAt *time.Time, at time.Time,
) error {
	if replacement.KeyVersion == 0 || !sameSealedCredential(current.Sealed, read) {
		return nil
	}
	replacer, ok := w.(SubscriptionCredentialReplacer)
	if !ok {
		return nil
	}
	_, err := replacer.ReplaceSubscriptionCredential(ctx, current.ID, read, replacement, refreshedAt, expiresAt, at)
	return err
}

func sameSealedCredential(left, right catalog.SealedCredential) bool {
	return left.KeyVersion == right.KeyVersion && bytes.Equal(left.Ciphertext, right.Ciphertext) &&
		bytes.Equal(left.Nonce, right.Nonce)
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
		if inspection.AccountRef != "" {
			input.Name = subscriptionResourceName(input.Name, inspection.AccountRef)
			if !validText(input.Name, 128) {
				return Resource{}, appsec.ErrInvalidArgument
			}
		}
		var provider Provider
		if err := s.store.ReadResource(ctx, actor, func(r ResourceReadSession) error {
			var err error
			provider, err = r.Provider(ctx, input.ProviderID)
			return err
		}); err != nil {
			return Resource{}, err
		}
		if !subscription.SupportsProvider(provider) {
			return Resource{}, appsec.ErrInvalidArgument
		}
		// 保存只做本地校验；在线认证和额度读取由独立的测试连接负责。
	default:
		return Resource{}, appsec.ErrInvalidArgument
	}

	id, err := s.next(ctx)
	if err != nil {
		return Resource{}, err
	}
	now := businessTime(s.now)
	resource := Resource{
		ID: id, Version: 1, ProviderID: input.ProviderID, Name: input.Name,
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
	err = s.store.WriteResource(ctx, actor, func(w ResourceSession) error {
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
	if err := s.store.ReadResource(ctx, actor, func(r ResourceReadSession) error {
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
	return s.store.WriteResource(ctx, actor, func(w ResourceSession) error {
		record, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		if record.Version != original.Version {
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
		record.UpdatedAt = businessTime(s.now)
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
	err := s.store.WriteResource(ctx, actor, func(w ResourceSession) error {
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
	return s.store.WriteResource(ctx, actor, func(w ResourceSession) error {
		record, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		deleted, err := w.DeleteResource(ctx, id, businessTime(s.now))
		if err != nil {
			return err
		}
		if !deleted {
			return ErrConflict
		}
		return w.Audit(ctx, Audit{Event: operation.ResourceDelete, Target: "RESOURCE", ID: id, Name: record.Name, Before: record.Resource, After: map[string]bool{"deleted": true}}, meta)
	})
}
