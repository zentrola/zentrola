package management

import (
	"context"
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

type applicationSessionStub struct {
	ApplicationSession
	application Application
	groups      map[int64]Group
	created     *Application
	status      string
	usable      bool
	checkedAt   time.Time
	keyError    error
	audits      []Audit
	quotaAdds   int
	quotaClears int
}

func (s *applicationSessionStub) Application(context.Context, int64) (Application, error) {
	return s.application, nil
}
func (s *applicationSessionStub) Group(_ context.Context, id int64) (Group, error) {
	group, ok := s.groups[id]
	if !ok {
		return Group{}, appsec.ErrNotFound
	}
	return group, nil
}
func (s *applicationSessionStub) CreateApplication(_ context.Context, value Application) error {
	s.created = &value
	return nil
}
func (s *applicationSessionStub) SetApplicationStatus(_ context.Context, _ int64, status string) error {
	s.status = status
	return nil
}
func (s *applicationSessionStub) HasUsableApplicationKey(_ context.Context, _ int64, now time.Time) (bool, error) {
	s.checkedAt = now
	return s.usable, s.keyError
}
func (s *applicationSessionStub) AddPrincipalTokenQuota(_ context.Context, _ int64, amount int64) (int64, error) {
	limit := amount
	if s.application.MonthlyTokenLimit != nil {
		limit += *s.application.MonthlyTokenLimit
	}
	s.application.MonthlyTokenLimit = &limit
	s.quotaAdds++
	return limit, nil
}
func (s *applicationSessionStub) ClearPrincipalTokenQuota(context.Context, int64) error {
	s.application.MonthlyTokenLimit = nil
	s.quotaClears++
	return nil
}
func (s *applicationSessionStub) Audit(_ context.Context, audit Audit, _ appsec.RequestMeta) error {
	s.audits = append(s.audits, audit)
	return nil
}

type applicationStoreStub struct{ session *applicationSessionStub }

func (s applicationStoreStub) ReadApplication(_ context.Context, _ admin.Identity, fn func(ApplicationReader) error) error {
	return fn(s.session)
}
func (s applicationStoreStub) WriteApplication(_ context.Context, _ admin.Identity, fn func(ApplicationSession) error) error {
	return fn(s.session)
}

func TestCreateApplicationDefaultsDisabledAndAudits(t *testing.T) {
	local := time.Date(2026, 10, 2, 9, 8, 7, 654321987, time.FixedZone("CST", 8*60*60))
	session := &applicationSessionStub{groups: map[int64]Group{}}
	service := ApplicationService{store: applicationStoreStub{session}, ids: fixedMemberID{id: 42}, now: func() time.Time { return local }}
	value, err := service.Create(context.Background(), admin.Identity{ID: 1}, "应用", "", nil, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if value.Status != "DISABLED" || value.ID != 42 || value.CreatedAt.Location() != time.UTC || !value.CreatedAt.Equal(local.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("unexpected application: %+v", value)
	}
	if session.created == nil || len(session.audits) != 1 || session.audits[0].Event != operation.ApplicationCreate || session.audits[0].Target != "APPLICATION" {
		t.Fatalf("create/audit missing: created=%+v audits=%+v", session.created, session.audits)
	}
}

func TestEnableApplicationRequiresUsableKey(t *testing.T) {
	local := time.Date(2026, 10, 2, 9, 8, 7, 654321987, time.FixedZone("CST", 8*60*60))
	for _, test := range []struct {
		name       string
		usable     bool
		keyError   error
		wantErr    error
		wantStatus string
	}{
		{name: "no active unexpired key", wantErr: ErrApplicationKeyRequired},
		{name: "usable key", usable: true, wantStatus: "ACTIVE"},
		{name: "lookup failed", keyError: appsec.ErrUnavailable, wantErr: appsec.ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &applicationSessionStub{application: Application{ID: 7, Name: "应用", Status: "DISABLED"}, usable: test.usable, keyError: test.keyError}
			service := ApplicationService{store: applicationStoreStub{session}, now: func() time.Time { return local }}
			err := service.SetStatus(context.Background(), admin.Identity{ID: 1}, 7, "ACTIVE", appsec.RequestMeta{})
			if !errors.Is(err, test.wantErr) || session.status != test.wantStatus {
				t.Fatalf("err=%v status=%q", err, session.status)
			}
			if !session.checkedAt.Equal(local.UTC().Truncate(time.Microsecond)) || session.checkedAt.Location() != time.UTC {
				t.Fatalf("key check did not use UTC business time: %s", session.checkedAt)
			}
			if test.wantErr == nil && (len(session.audits) != 1 || session.audits[0].Event != operation.ApplicationStatusChange) {
				t.Fatalf("status audit missing: %+v", session.audits)
			}
		})
	}
}

func TestAddApplicationTokenQuotaUsesPrincipalAuditType(t *testing.T) {
	session := &applicationSessionStub{application: Application{ID: 7, Name: "应用"}}
	service := ApplicationService{store: applicationStoreStub{session: session}}
	updated, err := service.AddTokenQuota(
		context.Background(), admin.Identity{ID: 1}, 7, 800, "新应用上线", appsec.RequestMeta{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MonthlyTokenLimit == nil || *updated.MonthlyTokenLimit != 800 || session.quotaAdds != 1 {
		t.Fatalf("updated=%+v quotaAdds=%d", updated, session.quotaAdds)
	}
	if len(session.audits) != 1 || session.audits[0].Event != operation.PrincipalTokenQuotaAdd || session.audits[0].Target != "APPLICATION" {
		t.Fatalf("unexpected audit: %+v", session.audits)
	}
}

func TestRemoveApplicationTokenQuotaClearsLimitAndAudits(t *testing.T) {
	limit := int64(800)
	session := &applicationSessionStub{application: Application{ID: 7, Name: "应用", MonthlyTokenLimit: &limit}}
	service := ApplicationService{store: applicationStoreStub{session: session}}
	updated, err := service.RemoveTokenQuota(context.Background(), admin.Identity{ID: 1}, 7, appsec.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MonthlyTokenLimit != nil || session.quotaClears != 1 {
		t.Fatalf("updated=%+v quotaClears=%d", updated, session.quotaClears)
	}
	if len(session.audits) != 1 || session.audits[0].Event != operation.PrincipalTokenQuotaRemove || session.audits[0].Target != "APPLICATION" {
		t.Fatalf("unexpected audit: %+v", session.audits)
	}
}

func TestKeyExpiryFilterRejectsUnknownValue(t *testing.T) {
	ctx, actor, page := context.Background(), admin.Identity{ID: 1}, Page{Limit: 20}
	for _, test := range []struct {
		name string
		read func() error
	}{
		{"member", func() error {
			_, err := (&QueryService{}).Keys(ctx, actor, 7, page, "expired")
			return err
		}},
		{"application", func() error {
			_, err := (&ApplicationService{}).Keys(ctx, actor, 7, page, "expired")
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.read(); !errors.Is(err, appsec.ErrInvalidArgument) {
				t.Fatalf("unknown expiry filter error = %v, want invalid argument", err)
			}
		})
	}
}
