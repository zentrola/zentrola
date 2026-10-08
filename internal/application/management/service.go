package management

import (
	"context"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type Service struct {
	MemberService
	GroupService
	ModelService
	ProviderService
	QueryService
	ResourceService
	PriceService
}

type serviceOptions struct {
	discoverer    ModelDiscoverer
	subscriptions []SubscriptionAdapter
	now           func() time.Time
	priceStore    PriceStore
}

type Option func(*serviceOptions)

func WithModelDiscoverer(discoverer ModelDiscoverer) Option {
	return func(options *serviceOptions) { options.discoverer = discoverer }
}

func WithSubscriptionAdapter(adapter SubscriptionAdapter) Option {
	return func(options *serviceOptions) {
		if adapter != nil {
			options.subscriptions = append(options.subscriptions, adapter)
		}
	}
}

func WithPriceStore(store PriceStore) Option {
	return func(options *serviceOptions) { options.priceStore = store }
}

// WithClock 注入业务时间，测试可使用固定时钟；所有结果统一规范化为 UTC 微秒精度。
func WithClock(now func() time.Time) Option {
	return func(options *serviceOptions) {
		if now != nil {
			options.now = now
		}
	}
}

func businessTime(now func() time.Time) time.Time {
	if now == nil {
		now = time.Now
	}
	return now().UTC().Truncate(time.Microsecond)
}

func New(store Store, ids shared.IDGenerator, cipher Cipher, tester ConnectionTester, options ...Option) *Service {
	configuration := serviceOptions{now: time.Now}
	for _, option := range options {
		if option != nil {
			option(&configuration)
		}
	}
	if dependencyMissing(cipher) {
		cipher = unavailableCipher{}
	}
	now := func() time.Time { return businessTime(configuration.now) }
	subscriptions := append([]SubscriptionAdapter(nil), configuration.subscriptions...)
	resourceService := ResourceService{
		store: resourceStoreAdapter{Store: store}, ids: ids, cipher: cipher, tester: tester, now: now,
		subscriptions: subscriptions,
	}
	return &Service{
		MemberService: MemberService{store: memberStoreAdapter{Store: store}, ids: ids, now: now},
		GroupService:  GroupService{store: groupStoreAdapter{Store: store}, ids: ids, now: now},
		ModelService:  ModelService{store: modelStoreAdapter{Store: store}, ids: ids, now: now},
		ProviderService: ProviderService{
			store: providerStoreAdapter{Store: store}, ids: ids, cipher: cipher, now: now,
			connectionProbe: &resourceService, discoverer: configuration.discoverer, subscriptions: subscriptions,
		},
		ResourceService: resourceService,
		PriceService:    PriceService{store: configuration.priceStore, ids: ids, now: now},
		QueryService: QueryService{
			store: store, discoverer: configuration.discoverer, subscriptions: subscriptions,
		},
	}
}

func dependencyMissing(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type unavailableCipher struct{}

func (unavailableCipher) Encrypt([]byte, catalog.CredentialOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{}, appsec.ErrUnavailable
}
func (unavailableCipher) Decrypt(catalog.SealedCredential, catalog.CredentialOwner) ([]byte, error) {
	return nil, appsec.ErrUnavailable
}
func (unavailableCipher) EncryptProviderProxy([]byte, catalog.ProviderProxyOwner) (catalog.SealedCredential, error) {
	return catalog.SealedCredential{}, appsec.ErrUnavailable
}
func (unavailableCipher) DecryptProviderProxy(catalog.SealedCredential, catalog.ProviderProxyOwner) ([]byte, error) {
	return nil, appsec.ErrUnavailable
}

func validText(s string, max int) bool {
	return utf8.ValidString(s) && s == strings.TrimSpace(s) && s != "" && len(s) <= max && !strings.ContainsRune(s, 0)
}
func validRemark(s string) bool { return s == "" || validText(s, 2000) }
func validTokenQuotaAddition(current *int64, amount int64) bool {
	return amount > 0 && (current == nil || amount <= int64(^uint64(0)>>1)-*current)
}
func validTokenQuotaReason(reason string) bool { return reason == "" || validText(reason, 500) }
func tokenQuotaBeforeValue(limit, amount int64) any {
	before := limit - amount
	if before == 0 {
		return nil
	}
	return idString(before)
}
func tokenQuotaAfterValue(limit, amount int64, reason string) map[string]any {
	after := map[string]any{"monthlyTokenLimit": idString(limit), "amount": idString(amount)}
	if reason != "" {
		after["reason"] = reason
	}
	return after
}
func remark(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func validStatus(s string) bool { return s == "ACTIVE" || s == "DISABLED" }
func validCredential(s string) bool {
	if len(s) == 0 || len(s) > 4096 {
		return false
	}
	for _, ch := range s {
		if ch < 33 || ch > 126 {
			return false
		}
	}
	return true
}

func validSubscriptionCredential(s string) bool {
	return len(s) > 0 && len(s) <= 64<<10 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func nextID(ctx context.Context, ids shared.IDGenerator) (int64, error) {
	if ids == nil {
		return 0, appsec.ErrUnavailable
	}
	id, err := ids.NextID(ctx)
	if err != nil {
		return 0, appsec.ErrUnavailable
	}
	return id, nil
}
