package management

import (
	"context"
	"strings"
	"unicode/utf8"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type Service struct {
	MemberService
	GroupService
	ModelService
	ProviderService
	QueryService
	ResourceService
	store         Store
	ids           shared.IDGenerator
	cipher        Cipher
	tester        ConnectionTester
	discoverer    ModelDiscoverer
	subscriptions []SubscriptionAdapter
}

type Option func(*Service)

func WithModelDiscoverer(discoverer ModelDiscoverer) Option {
	return func(service *Service) { service.discoverer = discoverer }
}

func WithSubscriptionAdapter(adapter SubscriptionAdapter) Option {
	return func(service *Service) {
		if adapter != nil {
			service.subscriptions = append(service.subscriptions, adapter)
		}
	}
}

func New(store Store, ids shared.IDGenerator, cipher Cipher, tester ConnectionTester, options ...Option) *Service {
	service := &Service{store: store, ids: ids, cipher: cipher, tester: tester}
	service.MemberService = MemberService{store: store, ids: ids}
	service.GroupService = GroupService{store: store, ids: ids}
	service.ModelService = ModelService{store: modelStoreAdapter{Store: store}, ids: ids}
	for _, option := range options {
		option(service)
	}
	service.ProviderService = ProviderService{
		store: service.store, ids: service.ids, cipher: service.cipher,
		discoverer: service.discoverer, subscriptions: append([]SubscriptionAdapter(nil), service.subscriptions...),
	}
	service.ResourceService = ResourceService{
		store: service.store, ids: service.ids, cipher: service.cipher, tester: service.tester,
		subscriptions: append([]SubscriptionAdapter(nil), service.subscriptions...),
	}
	service.QueryService = QueryService{
		store: service.store, discoverer: service.discoverer,
		subscriptions: append([]SubscriptionAdapter(nil), service.subscriptions...),
	}
	return service
}

func validText(s string, max int) bool {
	return utf8.ValidString(s) && s == strings.TrimSpace(s) && s != "" && len(s) <= max && !strings.ContainsRune(s, 0)
}
func validRemark(s string) bool { return s == "" || validText(s, 2000) }
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
	id, err := ids.NextID(ctx)
	if err != nil {
		return 0, appsec.ErrUnavailable
	}
	return id, nil
}
