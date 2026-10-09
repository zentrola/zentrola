package http

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	app "github.com/zentrola/zentrola/internal/application/billing"
	"github.com/zentrola/zentrola/internal/domain/admin"
	domain "github.com/zentrola/zentrola/internal/domain/billing"
)

type usageCostStoreStub struct {
	page app.UsageCostPage
}

func (s *usageCostStoreStub) UsageCostSummary(context.Context, admin.Identity, app.UsageCostFilter) (app.UsageCostSummary, error) {
	return app.UsageCostSummary{}, nil
}

func (s *usageCostStoreStub) UsageCosts(context.Context, admin.Identity, app.UsageCostFilter) (app.UsageCostPage, error) {
	return s.page, nil
}

func (s *usageCostStoreStub) UsageCost(context.Context, admin.Identity, int64) (app.UsageCostDetail, error) {
	return app.UsageCostDetail{}, nil
}

func (*usageCostStoreStub) SubscriptionPrices(context.Context) ([]app.SubscriptionPrice, error) {
	return nil, nil
}

func (*usageCostStoreStub) SubscriptionUsage(context.Context, int64, time.Time, time.Time) ([]domain.UsageShare, error) {
	return nil, nil
}

func (*usageCostStoreStub) PrincipalIdentities(context.Context, []int64) ([]app.PrincipalIdentity, error) {
	return nil, nil
}

func (*usageCostStoreStub) CurrentAPIKeyAttribution(context.Context, time.Time, time.Time) ([]app.PrincipalTotal, error) {
	return nil, nil
}

func usageCostTestRouter(store app.UsageCostQueryStore) http.Handler {
	router := chiRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), adminIdentityKey{}, admin.Identity{ID: 1})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	(&SecurityHandlers{BillingCosts: app.NewUsageCostQuery(store)}).mountBillingCosts(router)
	return router
}

func chiRouter() *chi.Mux {
	return chi.NewRouter()
}

func TestUsageCostFilterRequiresStrictUTCAndKnownParameters(t *testing.T) {
	valid := "from=2026-09-01T00%3A00%3A00Z&to=2026-10-01T00%3A00%3A00Z"
	tests := []struct {
		name  string
		query string
	}{
		{name: "missing range", query: ""},
		{name: "offset timezone", query: "from=2026-09-01T08%3A00%3A00%2B08%3A00&to=2026-10-01T00%3A00%3A00Z"},
		{name: "duplicate", query: valid + "&currency=USD&currency=CNY"},
		{name: "unknown", query: valid + "&unexpected=1"},
		{name: "bad enum", query: valid + "&currency=EUR"},
		{name: "export pagination", query: valid + "&limit=10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "/billing/usage-costs"
			if test.name == "export pagination" {
				path += "/export"
			}
			recorder := httptest.NewRecorder()
			usageCostTestRouter(&usageCostStoreStub{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path+"?"+test.query, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestCurrentAttributionFilterRequiresStrictUTCAndKnownParameters(t *testing.T) {
	valid := "from=2026-10-01T00%3A00%3A00Z&to=2026-11-01T00%3A00%3A00Z&limit=100"
	for _, rawQuery := range []string{
		"",
		"from=2026-10-01T08%3A00%3A00%2B08%3A00&to=2026-11-01T00%3A00%3A00Z",
		valid + "&limit=10",
		valid + "&billingType=SUBSCRIPTION",
	} {
		req := httptest.NewRequest(http.MethodGet, "/billing/current-attribution?"+rawQuery, nil)
		filter, err := currentAttributionFilter(req)
		if rawQuery == "" {
			if err != nil || !filter.From.IsZero() || !filter.To.IsZero() {
				t.Fatalf("empty filter parse failed: filter=%+v err=%v", filter, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("invalid query accepted: %s", rawQuery)
		}
	}
	filter, err := currentAttributionFilter(httptest.NewRequest(http.MethodGet, "/billing/current-attribution?"+valid, nil))
	if err != nil || filter.Limit != 100 || filter.From.Location() != time.UTC || filter.To.Location() != time.UTC {
		t.Fatalf("filter=%+v err=%v", filter, err)
	}
}

func TestUsageCostListReturnsOffsetCursor(t *testing.T) {
	store := &usageCostStoreStub{page: app.UsageCostPage{Items: []app.UsageCostRow{{ID: 1}, {ID: 2}, {ID: 3}}, Total: 9}}
	recorder := httptest.NewRecorder()
	path := "/billing/usage-costs?from=2026-09-01T00%3A00%3A00Z&to=2026-10-01T00%3A00%3A00Z&after=4&limit=2"
	usageCostTestRouter(store).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"nextCursor":"6"`) || !strings.Contains(recorder.Body.String(), `"total":9`) {
		t.Fatalf("unexpected page response: %s", recorder.Body.String())
	}
}

func TestUsageCostDetailRejectsQueryString(t *testing.T) {
	recorder := httptest.NewRecorder()
	usageCostTestRouter(&usageCostStoreStub{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/billing/usage-costs/1?expand=true", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestUsageCostCSVUsesBOMUTCAndFormulaProtection(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	started := time.Date(2026, 9, 6, 7, 30, 0, 123000000, time.UTC)
	recorder := httptest.NewRecorder()
	writeUsageCostCSV(recorder, app.UsageCostFilter{From: from, To: from.AddDate(0, 1, 0)}, []app.UsageCostRow{{
		ID: 1, RequestID: "=cmd", AttemptNo: 1, PrincipalID: 2, PrincipalName: "+member",
		ModelID: 3, ModelName: "-model", ProviderID: 4, ProviderName: "@provider",
		ResourceID: 5, ResourceName: "safe", StartedAt: started,
		PrincipalType: "MEMBER", ClientProtocol: "OPENAI_CHAT", Status: "SUCCESS",
		BillingType: "API_KEY", RatingStatus: "RATED",
	}}, "zh-CN")
	body := recorder.Body.Bytes()
	if len(body) < 3 || string(body[:3]) != "\xef\xbb\xbf" {
		t.Fatal("CSV must start with an UTF-8 BOM")
	}
	rows, err := csv.NewReader(strings.NewReader(string(body[3:]))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := rows[1][1]; got != "'=cmd" {
		t.Fatalf("request ID formula protection = %q", got)
	}
	for index, want := range map[int]string{6: "'+member", 8: "'-model", 10: "'@provider"} {
		if rows[1][index] != want {
			t.Fatalf("column %d = %q, want %q", index, rows[1][index], want)
		}
	}
	if rows[1][3] != "2026-09-06T07:30:00.123Z" {
		t.Fatalf("started_at = %q", rows[1][3])
	}
	for index, want := range map[int]string{
		4: "用户", 13: "OpenAI Chat", 14: "成功", 18: "按 Token 调用计费", 19: "已核算",
	} {
		if rows[1][index] != want {
			t.Fatalf("column %d = %q, want %q", index, rows[1][index], want)
		}
	}
	if recorder.Header().Get("Content-Language") != "zh-CN" {
		t.Fatalf("Content-Language = %q", recorder.Header().Get("Content-Language"))
	}
	contentDisposition, err := url.QueryUnescape(recorder.Header().Get("Content-Disposition"))
	if err != nil || !strings.Contains(contentDisposition, "20260901-20261001") {
		t.Fatalf("Content-Disposition = %q", recorder.Header().Get("Content-Disposition"))
	}
}

func TestUsageCostCSVUsesEnglishLabels(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	recorder := httptest.NewRecorder()
	writeUsageCostCSV(recorder, app.UsageCostFilter{From: from, To: from.AddDate(0, 1, 0)}, []app.UsageCostRow{{
		PrincipalType: "APPLICATION", ClientProtocol: "ANTHROPIC_MESSAGES", Status: "FAILED",
		BillingType: "SUBSCRIPTION", RatingStatus: "SUBSCRIPTION_SHARED",
	}}, "en-US")
	rows, err := csv.NewReader(strings.NewReader(string(recorder.Body.Bytes()[3:]))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range map[int]string{
		4: "Application", 13: "Anthropic Messages", 14: "Failed",
		18: "Personal subscription allocation", 19: "Subscription period allocation",
	} {
		if rows[1][index] != want {
			t.Fatalf("column %d = %q, want %q", index, rows[1][index], want)
		}
	}
	if recorder.Header().Get("Content-Language") != "en-US" {
		t.Fatalf("Content-Language = %q", recorder.Header().Get("Content-Language"))
	}
}

func TestUsageCostCSVWithoutLanguageKeepsEnumCodes(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	recorder := httptest.NewRecorder()
	writeUsageCostCSV(recorder, app.UsageCostFilter{From: from, To: from.AddDate(0, 1, 0)}, []app.UsageCostRow{{
		PrincipalType: "MEMBER", Status: "SUCCESS", BillingType: "API_KEY", RatingStatus: "RATED",
	}}, "")
	rows, err := csv.NewReader(strings.NewReader(string(recorder.Body.Bytes()[3:]))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range map[int]string{4: "MEMBER", 14: "SUCCESS", 18: "API_KEY", 19: "RATED"} {
		if rows[1][index] != want {
			t.Fatalf("column %d = %q, want %q", index, rows[1][index], want)
		}
	}
	if recorder.Header().Get("Content-Language") != "" {
		t.Fatalf("Content-Language = %q", recorder.Header().Get("Content-Language"))
	}
}
