package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type queryStoreStub struct {
	dashboard Dashboard
	from      time.Time
	to        time.Time
}

func (s *queryStoreStub) Query(context.Context, admin.Identity, Filter) (Page, error) {
	return Page{}, nil
}

func (s *queryStoreStub) Dashboard(_ context.Context, _ admin.Identity, from, to time.Time) (Dashboard, error) {
	s.from, s.to = from, to
	return s.dashboard, nil
}

func TestDashboardValidatesIdentityAndRange(t *testing.T) {
	from := time.Date(2026, 9, 8, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	to := from.Add(24 * time.Hour)
	store := &queryStoreStub{dashboard: Dashboard{ActiveMemberCount: 3, ModelCount: 2, ProviderCount: 1, TotalTokens: 42}}
	service := NewQuery(store)

	result, err := service.Dashboard(context.Background(), admin.Identity{ID: 1, OrganizationID: 2}, from, to)
	if err != nil || result.ActiveMemberCount != 3 || result.TotalTokens != 42 || !store.from.Equal(from) || !store.to.Equal(to) {
		t.Fatalf("unexpected dashboard result: result=%+v err=%v", result, err)
	}

	for _, tc := range []struct {
		name  string
		actor admin.Identity
		from  time.Time
		to    time.Time
		want  error
	}{
		{"missing identity", admin.Identity{}, from, to, appsec.ErrUnauthenticated},
		{"empty range", admin.Identity{ID: 1, OrganizationID: 2}, time.Time{}, to, appsec.ErrInvalidArgument},
		{"reversed range", admin.Identity{ID: 1, OrganizationID: 2}, to, from, appsec.ErrInvalidArgument},
		{"range too long", admin.Identity{ID: 1, OrganizationID: 2}, from, from.Add(367 * 24 * time.Hour), appsec.ErrInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Dashboard(context.Background(), tc.actor, tc.from, tc.to)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
