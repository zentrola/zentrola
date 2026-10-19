package management

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
)

func TestNewOperationFilter(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		keyword, kind, state string
		want                 OperationFilter
		wantErr              bool
	}{
		{name: "empty", want: OperationFilter{}},
		{name: "trims keyword", keyword: "  admin 50%_x  ", want: OperationFilter{Keyword: "admin 50%_x"}},
		{name: "keyword at limit", keyword: strings.Repeat("操", 100), want: OperationFilter{Keyword: strings.Repeat("操", 100)}},
		{name: "keyword too long", keyword: strings.Repeat("a", 101), wantErr: true},
		{name: "keyword with nul", keyword: "a\x00b", wantErr: true},
		{name: "type", kind: "RESOURCE_PRICE_UPDATE", want: OperationFilter{Type: "RESOURCE_PRICE_UPDATE"}},
		{name: "lowercase type", kind: "resource_create", wantErr: true},
		{name: "success", state: "SUCCESS", want: OperationFilter{Result: "SUCCESS"}},
		{name: "failed", state: "FAILED", want: OperationFilter{Result: "FAILED"}},
		{name: "unknown result", state: "ERROR", wantErr: true},
		{name: "lowercase result", state: "success", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewOperationFilter(tt.keyword, tt.kind, tt.state)
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

type operationFilterReader struct {
	queryReaderStub
	listed, counted OperationFilter
}

func (r *operationFilterReader) Operations(_ context.Context, _ Page, filter OperationFilter) ([]Operation, error) {
	r.listed = filter
	return []Operation{{ID: 6}}, nil
}

func (r *operationFilterReader) CountOperations(_ context.Context, filter OperationFilter) (int64, error) {
	r.counted = filter
	return 1, nil
}

func TestQueryServicePassesOperationFilterToListAndCount(t *testing.T) {
	reader := &operationFilterReader{}
	service := QueryService{store: queryStoreStub{reader: reader}}
	filter := OperationFilter{Keyword: "admin", Type: "MEMBER_CREATE", Result: "FAILED"}
	page, err := service.Operations(context.Background(), admin.Identity{ID: 9}, Page{Limit: 10}, filter)
	if err != nil || page.Total != 1 || reader.listed != filter || reader.counted != filter {
		t.Fatalf("page=%+v err=%v listed=%+v counted=%+v", page, err, reader.listed, reader.counted)
	}
	if _, err := service.Operations(context.Background(), admin.Identity{ID: 9}, Page{Limit: 10},
		OperationFilter{Result: "UNKNOWN"}); !errors.Is(err, appsec.ErrInvalidArgument) {
		t.Fatalf("invalid filter error=%v", err)
	}
}
