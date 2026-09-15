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
	tokens    int64
	principal int64
}

func (s *queryStoreStub) Query(context.Context, admin.Identity, Filter) (Page, error) {
	return Page{}, nil
}

func (s *queryStoreStub) Statistics(context.Context, admin.Identity, StatisticFilter) (StatisticPage, error) {
	return StatisticPage{}, nil
}

func (s *queryStoreStub) Dashboard(_ context.Context, _ admin.Identity, from, to time.Time) (Dashboard, error) {
	s.from, s.to = from, to
	return s.dashboard, nil
}

func (s *queryStoreStub) TokenUsage(_ context.Context, principalID int64, from, to time.Time) (int64, error) {
	s.principal, s.from, s.to = principalID, from, to
	return s.tokens, nil
}

func TestStatisticsValidatesDimensionAndPagination(t *testing.T) {
	from := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	valid := StatisticFilter{Dimension: StatisticMember, From: from, To: from.Add(24 * time.Hour), Limit: 20}
	service := NewQuery(&queryStoreStub{})
	if _, err := service.Statistics(context.Background(), admin.Identity{ID: 1}, valid); err != nil {
		t.Fatalf("valid statistics filter failed: %v", err)
	}

	for _, tc := range []struct {
		name   string
		actor  admin.Identity
		filter StatisticFilter
		want   error
	}{
		{"missing identity", admin.Identity{}, valid, appsec.ErrUnauthenticated},
		{"invalid dimension", admin.Identity{ID: 1}, StatisticFilter{Dimension: "resource", From: from, To: from.Add(time.Hour), Limit: 20}, appsec.ErrInvalidArgument},
		{"negative offset", admin.Identity{ID: 1}, StatisticFilter{Dimension: StatisticModel, From: from, To: from.Add(time.Hour), After: -1, Limit: 20}, appsec.ErrInvalidArgument},
		{"oversized page", admin.Identity{ID: 1}, StatisticFilter{Dimension: StatisticProvider, From: from, To: from.Add(time.Hour), Limit: 101}, appsec.ErrInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Statistics(context.Background(), tc.actor, tc.filter)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDashboardValidatesIdentityAndRange(t *testing.T) {
	from := time.Date(2026, 9, 8, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	to := from.Add(24 * time.Hour)
	store := &queryStoreStub{dashboard: Dashboard{ActiveMemberCount: 3, ModelCount: 2, ProviderCount: 1, TotalTokens: 42}}
	service := NewQuery(store)

	result, err := service.Dashboard(context.Background(), admin.Identity{ID: 1}, from, to)
	if err != nil || result.ActiveMemberCount != 3 || result.TotalTokens != 42 || !store.from.Equal(from) || !store.to.Equal(to) ||
		store.from.Location() != time.UTC || store.to.Location() != time.UTC {
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
		{"empty range", admin.Identity{ID: 1}, time.Time{}, to, appsec.ErrInvalidArgument},
		{"reversed range", admin.Identity{ID: 1}, to, from, appsec.ErrInvalidArgument},
		{"range too long", admin.Identity{ID: 1}, from, from.Add(367 * 24 * time.Hour), appsec.ErrInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Dashboard(context.Background(), tc.actor, tc.from, tc.to)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSelfReturnsUsageForRange(t *testing.T) {
	store := &queryStoreStub{tokens: 156000}
	service := NewQuery(store)
	local := time.FixedZone("CST", 8*60*60)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, local)
	to := time.Date(2026, 9, 15, 11, 20, 30, 0, local)
	wantFrom, wantTo := from.UTC(), to.UTC()

	result, err := service.Self(context.Background(), appsec.PrincipalIdentity{ID: 42}, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tokens != 156000 || !result.From.Equal(wantFrom) || !result.To.Equal(wantTo) ||
		result.From.Location() != time.UTC || result.To.Location() != time.UTC {
		t.Fatalf("unexpected usage result: %+v", result)
	}
	if store.principal != 42 || !store.from.Equal(wantFrom) || !store.to.Equal(wantTo) ||
		store.from.Location() != time.UTC || store.to.Location() != time.UTC {
		t.Fatalf("unexpected store query: principal=%d from=%s to=%s", store.principal, store.from, store.to)
	}
}

func TestSelfRejectsMissingIdentityAndInvalidRange(t *testing.T) {
	service := NewQuery(&queryStoreStub{})
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	for _, tc := range []struct {
		name     string
		identity appsec.PrincipalIdentity
		from     time.Time
		to       time.Time
		want     error
	}{
		{"missing identity", appsec.PrincipalIdentity{}, from, to, appsec.ErrUnauthenticated},
		{"empty range", appsec.PrincipalIdentity{ID: 1}, time.Time{}, to, appsec.ErrInvalidArgument},
		{"reversed range", appsec.PrincipalIdentity{ID: 1}, to, from, appsec.ErrInvalidArgument},
		{"range too long", appsec.PrincipalIdentity{ID: 1}, from, from.Add(367 * 24 * time.Hour), appsec.ErrInvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Self(context.Background(), tc.identity, tc.from, tc.to); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
