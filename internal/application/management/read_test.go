package management

import (
	"context"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/admin"
)

type queryReaderStub struct{ Reader }

func (queryReaderStub) Members(context.Context, Page) ([]Member, error) {
	return []Member{{ID: 1, Name: "成员"}}, nil
}
func (queryReaderStub) CountMembers(context.Context) (int64, error) { return 1, nil }
func (queryReaderStub) Groups(context.Context, Page, string) ([]Group, error) {
	return []Group{{ID: 2, Name: "分组"}}, nil
}
func (queryReaderStub) CountGroups(context.Context, string) (int64, error) { return 1, nil }
func (queryReaderStub) Models(context.Context, Page, string) ([]Model, error) {
	return []Model{{ID: 3, Name: "模型", Status: "ACTIVE"}}, nil
}
func (queryReaderStub) CountModels(context.Context, string) (int64, error) { return 1, nil }
func (queryReaderStub) Providers(context.Context, Page, string) ([]Provider, error) {
	return []Provider{{ID: 4, Name: "服务商", Type: "CUSTOM"}}, nil
}
func (queryReaderStub) CountProviders(context.Context, string) (int64, error) { return 1, nil }
func (queryReaderStub) Resources(context.Context, Page) ([]Resource, error) {
	return []Resource{{ID: 5, Name: "资源"}}, nil
}
func (queryReaderStub) CountResources(context.Context) (int64, error) { return 1, nil }
func (queryReaderStub) Operations(context.Context, Page) ([]Operation, error) {
	return []Operation{{ID: 6}}, nil
}
func (queryReaderStub) CountOperations(context.Context) (int64, error) { return 1, nil }
func (queryReaderStub) GroupMembers(context.Context, int64, Page) ([]Member, error) {
	return []Member{{ID: 1}}, nil
}
func (queryReaderStub) CountGroupMembers(context.Context, int64) (int64, error) { return 1, nil }
func (queryReaderStub) MemberGroups(context.Context, int64, Page) ([]Group, error) {
	return []Group{{ID: 2}}, nil
}
func (queryReaderStub) CountMemberGroups(context.Context, int64) (int64, error) { return 1, nil }
func (queryReaderStub) GroupModels(context.Context, int64, Page) ([]Model, error) {
	return []Model{{ID: 3}}, nil
}
func (queryReaderStub) CountGroupModels(context.Context, int64) (int64, error) { return 1, nil }
func (queryReaderStub) Keys(context.Context, int64, Page, bool) ([]Key, error) {
	return []Key{{ID: 7}}, nil
}
func (queryReaderStub) CountKeys(context.Context, int64, bool) (int64, error) { return 1, nil }
func (queryReaderStub) Member(context.Context, int64) (Member, error) {
	return Member{ID: 1, Name: "成员"}, nil
}
func (queryReaderStub) Group(context.Context, int64) (Group, error) {
	return Group{ID: 2, Name: "分组"}, nil
}
func (queryReaderStub) Resource(context.Context, int64) (ResourceRecord, error) {
	return ResourceRecord{Resource: Resource{ID: 5, Name: "资源"}}, nil
}
func (queryReaderStub) ResourceQuotas(context.Context, int64) ([]ResourceQuota, error) {
	return []ResourceQuota{{}}, nil
}
func (queryReaderStub) Provider(context.Context, int64) (Provider, error) {
	return Provider{ID: 4, Name: "服务商", Type: "CUSTOM"}, nil
}
func (queryReaderStub) ProviderMappings(context.Context, int64) ([]ProviderMapping, error) {
	return []ProviderMapping{{ID: 8, ProviderID: 4, ModelID: 3}}, nil
}

type queryStoreStub struct{ reader Reader }

func (s queryStoreStub) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	return fn(s.reader)
}

func TestQueryServiceReadsAllManagementViews(t *testing.T) {
	service := QueryService{store: queryStoreStub{reader: queryReaderStub{}}}
	ctx, actor, page := context.Background(), admin.Identity{ID: 9}, Page{Limit: 10}
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "members", run: func() error { _, err := service.Members(ctx, actor, page); return err }},
		{name: "groups", run: func() error { _, err := service.GroupsByStatus(ctx, actor, page, "ACTIVE"); return err }},
		{name: "models", run: func() error { _, err := service.Models(ctx, actor, page, "ACTIVE"); return err }},
		{name: "providers", run: func() error { _, err := service.Providers(ctx, actor, page, "CUSTOM"); return err }},
		{name: "resources", run: func() error { _, err := service.Resources(ctx, actor, page); return err }},
		{name: "operations", run: func() error { _, err := service.Operations(ctx, actor, page); return err }},
		{name: "group members", run: func() error { _, err := service.GroupMembers(ctx, actor, 2, page); return err }},
		{name: "member groups", run: func() error { _, err := service.MemberGroups(ctx, actor, 1, page); return err }},
		{name: "group models", run: func() error { _, err := service.GroupModels(ctx, actor, 2, page); return err }},
		{name: "keys", run: func() error { _, err := service.Keys(ctx, actor, 1, page, ""); return err }},
		{name: "member", run: func() error { _, err := service.Member(ctx, actor, 1); return err }},
		{name: "group", run: func() error { _, err := service.Group(ctx, actor, 2); return err }},
		{name: "resource", run: func() error { _, err := service.Resource(ctx, actor, 5); return err }},
		{name: "resource quotas", run: func() error { _, err := service.ResourceQuotas(ctx, actor, 5); return err }},
		{name: "provider", run: func() error { _, err := service.Provider(ctx, actor, 4); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
