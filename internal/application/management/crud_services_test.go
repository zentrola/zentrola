package management

import (
	"context"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type providerSessionStub struct {
	ProviderSession
	provider *Provider
	deleted  *time.Time
	audits   []Audit
}

func (s *providerSessionStub) Provider(context.Context, int64) (Provider, error) {
	return *s.provider, nil
}
func (s *providerSessionStub) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return nil, nil
}
func (s *providerSessionStub) CreateProvider(_ context.Context, provider Provider) error {
	s.provider = &provider
	return nil
}
func (s *providerSessionStub) DeleteProvider(_ context.Context, _ int64, at time.Time) error {
	s.deleted = &at
	return nil
}
func (s *providerSessionStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

type providerStoreStub struct{ session *providerSessionStub }

func (s providerStoreStub) ReadProvider(_ context.Context, _ admin.Identity, fn func(ProviderReadSession) error) error {
	return fn(s.session)
}
func (s providerStoreStub) WriteProvider(_ context.Context, _ admin.Identity, fn func(ProviderSession) error) error {
	return fn(s.session)
}

func TestProviderCreateAndDeleteUseNarrowStore(t *testing.T) {
	fixed := time.Date(2026, 10, 2, 3, 4, 5, 123456789, time.UTC)
	session := &providerSessionStub{}
	service := ProviderService{
		store: providerStoreStub{session: session}, ids: fixedMemberID{id: 31},
		cipher: providerTestCipher{}, now: func() time.Time { return fixed },
	}
	created, err := service.CreateProvider(context.Background(), admin.Identity{ID: 1}, ProviderInput{
		Name: " 服务商 ", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"}},
	}, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	wantTime := fixed.Truncate(time.Microsecond)
	if session.provider == nil || created.Code != "provider-31" || created.Name != "服务商" || !created.CreatedAt.Equal(wantTime) {
		t.Fatalf("unexpected provider: %+v", created)
	}
	if err := service.DeleteProvider(context.Background(), admin.Identity{ID: 1}, created.ID, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if session.deleted == nil || !session.deleted.Equal(wantTime) || len(session.audits) != 2 {
		t.Fatalf("deletedAt=%v audits=%d", session.deleted, len(session.audits))
	}
}

type resourceSessionStub struct {
	ResourceSession
	provider Provider
	record   *ResourceRecord
	deleted  bool
	audits   []Audit
}

func (s *resourceSessionStub) Provider(context.Context, int64) (Provider, error) {
	return s.provider, nil
}
func (s *resourceSessionStub) Resource(context.Context, int64) (ResourceRecord, error) {
	return *s.record, nil
}
func (s *resourceSessionStub) CreateResource(_ context.Context, record ResourceRecord) error {
	s.record = &record
	return nil
}
func (s *resourceSessionStub) DeleteResource(context.Context, int64, time.Time) (bool, error) {
	s.deleted = true
	return true, nil
}
func (s *resourceSessionStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

type resourceStoreStub struct{ session *resourceSessionStub }

func (s resourceStoreStub) ReadResource(_ context.Context, _ admin.Identity, fn func(ResourceReadSession) error) error {
	return fn(s.session)
}
func (s resourceStoreStub) WriteResource(_ context.Context, _ admin.Identity, fn func(ResourceSession) error) error {
	return fn(s.session)
}

func TestResourceCreateAndDeleteUseNarrowStore(t *testing.T) {
	fixed := time.Date(2026, 10, 2, 3, 4, 5, 123456789, time.UTC)
	session := &resourceSessionStub{provider: Provider{ID: 4, Status: "ACTIVE"}}
	service := ResourceService{
		store: resourceStoreStub{session: session}, ids: fixedMemberID{id: 41},
		cipher: providerTestCipher{}, now: func() time.Time { return fixed },
	}
	created, err := service.CreateResource(
		context.Background(), admin.Identity{ID: 1}, 4, "资源", "sk-valid-key", appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantTime := fixed.Truncate(time.Microsecond)
	if session.record == nil || created.ID != 41 || !created.CreatedAt.Equal(wantTime) || len(session.record.Sealed.Ciphertext) == 0 {
		t.Fatalf("unexpected resource: %+v record=%+v", created, session.record)
	}
	if err := service.DeleteResource(context.Background(), admin.Identity{ID: 1}, created.ID, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if !session.deleted || len(session.audits) != 2 {
		t.Fatalf("deleted=%v audits=%d", session.deleted, len(session.audits))
	}
}
