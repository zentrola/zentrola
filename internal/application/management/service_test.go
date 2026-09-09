package management

import (
	"context"
	"errors"
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

type memberSuggestionReader struct {
	Reader
	page   Page
	name   string
	result []Member
	called bool
}

func (r *memberSuggestionReader) MemberSuggestions(_ context.Context, page Page, name string) ([]Member, error) {
	r.called = true
	r.page = page
	r.name = name
	return r.result, nil
}
func (r *memberSuggestionReader) CountMemberSuggestions(context.Context, string) (int64, error) {
	return int64(len(r.result)), nil
}

type memberSuggestionStore struct{ reader *memberSuggestionReader }

func (s memberSuggestionStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.reader)
}

func (s memberSuggestionStore) Write(_ context.Context, _ admin.Identity, _ func(Writer) error) error {
	return nil
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

func TestMemberSuggestionsQueriesByName(t *testing.T) {
	reader := &memberSuggestionReader{result: []Member{{ID: 42, Name: "林知远"}}}
	service := New(memberSuggestionStore{reader: reader}, nil, nil, nil)
	page := Page{Limit: 8}

	result, err := service.MemberSuggestions(context.Background(), admin.Identity{}, page, "林知")
	if err != nil {
		t.Fatal(err)
	}
	if !reader.called || reader.page != page || reader.name != "林知" {
		t.Fatalf("query page=%#v name=%q called=%v", reader.page, reader.name, reader.called)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "林知远" || result.Total != 1 {
		t.Fatalf("result=%#v; want matching member", result)
	}
}

func TestMemberSuggestionsRejectsEmptyName(t *testing.T) {
	reader := &memberSuggestionReader{}
	service := New(memberSuggestionStore{reader: reader}, nil, nil, nil)

	_, err := service.MemberSuggestions(context.Background(), admin.Identity{}, Page{Limit: 8}, "")
	if !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("error=%v; want invalid argument", err)
	}
	if reader.called {
		t.Fatal("reader should not be called for an empty name")
	}
}
