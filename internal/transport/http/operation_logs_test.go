package http

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
)

func TestOperationLogFilterParsesOptionalQuery(t *testing.T) {
	for _, tt := range []struct {
		name    string
		query   url.Values
		want    mgmt.OperationFilter
		wantErr bool
	}{
		{name: "no filters", query: url.Values{}},
		{name: "all filters", query: url.Values{"keyword": {"  Admin_%  "}, "type": {"RESOURCE_CREATE"}, "result": {"FAILED"}},
			want: mgmt.OperationFilter{Keyword: "Admin_%", Type: "RESOURCE_CREATE", Result: "FAILED"}},
		{name: "blank keyword", query: url.Values{"keyword": {"   "}}},
		{name: "keyword too long", query: url.Values{"keyword": {strings.Repeat("k", 101)}}, wantErr: true},
		{name: "invalid result", query: url.Values{"result": {"PENDING"}}, wantErr: true},
		{name: "repeated keyword", query: url.Values{"keyword": {"a", "b"}}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/v1/operation-logs?"+tt.query.Encode(), nil)
			got, err := operationLogFilter(request)
			if tt.wantErr {
				if !errors.Is(err, appsec.ErrInvalidArgument) {
					t.Fatalf("error=%v; want invalid argument", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("filter=%+v err=%v; want %+v", got, err, tt.want)
			}
		})
	}
}
