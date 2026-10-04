package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zentrola/zentrola/internal/application/health"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

type selfUsageKeyStore struct {
	identity appsec.PrincipalIdentity
}

func (*selfUsageKeyStore) Create(context.Context, admin.Identity, appsec.KeyRecord, appsec.RequestMeta) error {
	return nil
}
func (*selfUsageKeyStore) Revoke(context.Context, admin.Identity, int64, appsec.RequestMeta) error {
	return nil
}
func (s *selfUsageKeyStore) Authenticate(context.Context, []byte, time.Time) (appsec.PrincipalIdentity, error) {
	return s.identity, nil
}

type selfUsageQueryStore struct {
	tokens      int64
	principalID int64
	from        time.Time
	to          time.Time
}

func (*selfUsageQueryStore) Query(context.Context, admin.Identity, usageapp.Filter) (usageapp.Page, error) {
	return usageapp.Page{}, nil
}
func (*selfUsageQueryStore) Statistics(context.Context, admin.Identity, usageapp.StatisticFilter) (usageapp.StatisticPage, error) {
	return usageapp.StatisticPage{}, nil
}
func (*selfUsageQueryStore) Dashboard(context.Context, admin.Identity, time.Time, time.Time) (usageapp.Dashboard, error) {
	return usageapp.Dashboard{}, nil
}
func (s *selfUsageQueryStore) TokenUsage(_ context.Context, principalID int64, from, to time.Time) (int64, error) {
	s.principalID, s.from, s.to = principalID, from, to
	return s.tokens, nil
}

func TestSelfUsageAuthenticatesAccessKeyAndReturnsOwnTokens(t *testing.T) {
	store := &selfUsageQueryStore{tokens: 156000}
	keys := appsec.NewKeys(&selfUsageKeyStore{identity: appsec.PrincipalIdentity{ID: 42, AccessKeyID: 7}}, nil)
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		health.New(),
		CORSOptions{},
		time.Second,
		"prod",
		&SecurityHandlers{Keys: keys, Usage: usageapp.NewQuery(store)},
	)
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	for _, test := range []struct {
		name   string
		header string
	}{
		{name: "bearer", header: "Authorization"},
		{name: "api key", header: "X-Api-Key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/me/usage", nil)
			if test.header == "Authorization" {
				request.Header.Set(test.header, "Bearer "+key)
			} else {
				request.Header.Set(test.header, key)
			}
			before := time.Now().UTC()
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			after := time.Now().UTC()

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Code string             `json:"code"`
				Data usageapp.SelfUsage `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			reportedAt := body.Data.To.UTC()
			wantFrom := time.Date(reportedAt.Year(), reportedAt.Month(), 1, 0, 0, 0, 0, time.UTC)
			if body.Code != "OK" || body.Data.Tokens != 156000 || !body.Data.From.Equal(wantFrom) || body.Data.To.Before(before) || body.Data.To.After(after) {
				t.Fatalf("unexpected response: %+v", body)
			}
			if store.principalID != 42 || !store.from.Equal(body.Data.From) || !store.to.Equal(body.Data.To) {
				t.Fatalf("query escaped authenticated member: principal=%d from=%s to=%s", store.principalID, store.from, store.to)
			}
		})
	}
}

func TestSelfUsageAcceptsExplicitRange(t *testing.T) {
	store := &selfUsageQueryStore{tokens: 42000}
	keys := appsec.NewKeys(&selfUsageKeyStore{identity: appsec.PrincipalIdentity{ID: 42}}, nil)
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(), CORSOptions{}, time.Second, "prod",
		&SecurityHandlers{Keys: keys, Usage: usageapp.NewQuery(store)},
	)
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	from := "2026-08-31T16:00:00Z"
	to := "2026-09-15T03:20:00Z"
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me/usage?from="+from+"&to="+to, nil)
	request.Header.Set("Authorization", "Bearer "+key)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if store.principalID != 42 || store.from.Format(time.RFC3339) != from || store.to.Format(time.RFC3339) != to {
		t.Fatalf("unexpected store query: principal=%d from=%s to=%s", store.principalID, store.from, store.to)
	}
}

func TestSelfUsageRejectsMissingAmbiguousOrInvalidInput(t *testing.T) {
	store := &selfUsageQueryStore{}
	keys := appsec.NewKeys(&selfUsageKeyStore{identity: appsec.PrincipalIdentity{ID: 42}}, nil)
	router := NewRouter(
		slog.New(slog.NewTextHandler(io.Discard, nil)), health.New(), CORSOptions{}, time.Second, "prod",
		&SecurityHandlers{Keys: keys, Usage: usageapp.NewQuery(store)},
	)
	key := "vk-" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	for _, test := range []struct {
		name    string
		path    string
		headers map[string]string
		status  int
	}{
		{name: "missing key", path: "/api/v1/me/usage", status: http.StatusUnauthorized},
		{name: "both key forms", path: "/api/v1/me/usage", headers: map[string]string{"Authorization": "Bearer " + key, "X-Api-Key": key}, status: http.StatusUnauthorized},
		{name: "missing to", path: "/api/v1/me/usage?from=2026-09-01T00:00:00Z", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
		{name: "invalid time", path: "/api/v1/me/usage?from=2026-09-01&to=2026-09-02", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
		{name: "non UTC range", path: "/api/v1/me/usage?from=2026-09-01T00:00:00%2B08:00&to=2026-09-02T00:00:00%2B08:00", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
		{name: "reversed range", path: "/api/v1/me/usage?from=2026-09-02T00:00:00Z&to=2026-09-01T00:00:00Z", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
		{name: "unknown parameter", path: "/api/v1/me/usage?from=2026-09-01T00:00:00Z&to=2026-09-02T00:00:00Z&timezone=UTC", headers: map[string]string{"Authorization": "Bearer " + key}, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
}
