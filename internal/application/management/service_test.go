package management

import (
	"context"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type fixedMemberID struct{ id int64 }

func (g fixedMemberID) NextID() (int64, error) { return g.id, nil }

type memberCreateWriter struct {
	Writer
	created Member
	audit   Audit
}

func (w *memberCreateWriter) CreateMember(_ context.Context, member Member) error {
	w.created = member
	return nil
}

func (w *memberCreateWriter) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	w.audit = audit
	return nil
}

type memberCreateStore struct{ writer *memberCreateWriter }

func (s memberCreateStore) Read(_ context.Context, _ admin.Identity, _ func(Reader) error) error {
	return nil
}

func (s memberCreateStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(s.writer)
}

func TestCreateMemberDefaultsToDisabled(t *testing.T) {
	writer := &memberCreateWriter{}
	service := New(memberCreateStore{writer: writer}, fixedMemberID{id: 42}, nil, nil)

	created, err := service.CreateMember(context.Background(), admin.Identity{}, "新用户", "", appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "DISABLED" || writer.created.Status != "DISABLED" {
		t.Fatalf("created status=%q, persisted status=%q; want DISABLED", created.Status, writer.created.Status)
	}
	after, ok := writer.audit.After.(Member)
	if !ok || after.Status != "DISABLED" {
		t.Fatalf("audit after=%#v; want disabled member", writer.audit.After)
	}
}
