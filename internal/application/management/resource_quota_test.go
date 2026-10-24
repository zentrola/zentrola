package management

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

func TestQuotaProbeMappingPrefersHaiku(t *testing.T) {
	provider := Provider{ID: 40, Code: catalog.AnthropicOfficialCode}
	models := []Model{
		{ID: 1, Code: "claude-sonnet", Status: "ACTIVE"},
		{ID: 2, Code: "claude-haiku", Status: "ACTIVE"},
		{ID: 3, Code: "claude-haiku-old", Status: "DISABLED"},
		{ID: 4, Code: "claude-opus", Status: "ACTIVE"},
	}
	tests := []struct {
		name     string
		mappings []ProviderMapping
		want     int64
	}{
		{
			name: "Haiku mapping after other models",
			mappings: []ProviderMapping{
				{ID: 10, ProviderID: 40, ModelID: 1},
				{ID: 11, ProviderID: 40, ModelID: 2},
			},
			want: 11,
		},
		{
			name: "upstream code decides model family",
			mappings: []ProviderMapping{
				{ID: 10, ProviderID: 40, ModelID: 1},
				{ID: 12, ProviderID: 40, ModelID: 4, UpstreamModelCode: "Claude-Haiku-Latest"},
			},
			want: 12,
		},
		{
			name: "falls back to connection test default",
			mappings: []ProviderMapping{
				{ID: 13, ProviderID: 40, ModelID: 3},
				{ID: 10, ProviderID: 40, ModelID: 1},
				{ID: 14, ProviderID: 40, ModelID: 4},
			},
			want: 10,
		},
		{
			name: "no active mapping for provider",
			mappings: []ProviderMapping{
				{ID: 13, ProviderID: 40, ModelID: 3},
				{ID: 15, ProviderID: 41, ModelID: 2},
				{ID: 16, ProviderID: 40, ModelID: 99},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping, model := quotaProbeMapping(provider, test.mappings, models)
			if mapping.ID != test.want || (test.want > 0 && model.ID != mapping.ModelID) {
				t.Fatalf("mapping=%+v model=%+v, want mapping %d", mapping, model, test.want)
			}
		})
	}
}

func TestRefreshResourceQuota(t *testing.T) {
	reset := time.Date(2026, 10, 19, 5, 50, 0, 0, time.UTC)
	used := 1.0
	allowed := []ResourceQuota{
		{Code: "subscription", Status: QuotaAvailable},
		{Code: "five_hour", Status: QuotaAvailable, UsedPercent: &used, ResetsAt: &reset},
	}
	rejected := []ResourceQuota{
		{Code: "subscription", Status: QuotaExhausted, ResetsAt: &reset},
		{Code: "five_hour", Status: QuotaExhausted, ResetsAt: &reset},
	}
	claude := Resource{
		ID: 48, ProviderID: 40, Name: "Claude 订阅", Version: 3,
		AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterClaudeCode,
		QuotaStatus: QuotaUnknown, RuntimeStatus: "BLOCKED",
	}
	mappings := []ProviderMapping{
		{ID: 1, ProviderID: 40, ModelID: 90, UpstreamModelCode: "claude-sonnet"},
		{ID: 2, ProviderID: 40, ModelID: 91, UpstreamModelCode: "claude-haiku"},
	}
	tests := []struct {
		name           string
		resource       Resource
		mappings       []ProviderMapping
		probe          ConnectionResult
		changeResource bool
		wantErr        error
		wantCalls      int
		wantOK         bool
		wantCode       string
		wantStatus     string
	}{
		{
			name: "records quota from successful probe", resource: claude, mappings: mappings,
			probe:     ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200, ObservedQuotas: allowed},
			wantCalls: 1, wantOK: true, wantCode: "OK", wantStatus: QuotaAvailable,
		},
		{
			name: "records exhausted quota from rejected probe", resource: claude, mappings: mappings,
			probe:     ConnectionResult{Code: "UPSTREAM_RATE_LIMITED", HTTPStatus: 429, ObservedQuotas: rejected},
			wantCalls: 1, wantOK: true, wantCode: "OK", wantStatus: QuotaExhausted,
		},
		{
			name: "successful probe without unified headers", resource: claude, mappings: mappings,
			probe:     ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200},
			wantCalls: 1, wantCode: "UPSTREAM_QUOTA_UNAVAILABLE", wantStatus: QuotaUnknown,
		},
		{
			name: "failed probe keeps quota", resource: claude, mappings: mappings,
			probe:     ConnectionResult{Code: "UPSTREAM_AUTH_FAILED", HTTPStatus: 401},
			wantCalls: 1, wantCode: "UPSTREAM_AUTH_FAILED", wantStatus: QuotaUnknown,
		},
		{
			name: "resource changed during refresh", resource: claude, mappings: mappings, changeResource: true,
			probe:     ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200, ObservedQuotas: allowed},
			wantCalls: 1, wantCode: "RESOURCE_CHANGED", wantStatus: QuotaUnknown,
		},
		{
			name: "missing model mapping", resource: claude,
			wantCode: "PROVIDER_MODEL_MAPPING_REQUIRED", wantStatus: QuotaUnknown,
		},
		{
			name: "API key resource", mappings: mappings,
			resource: Resource{ID: 48, ProviderID: 40, Version: 3, AuthType: AuthTypeAPIKey},
			wantErr:  ErrQuotaRefreshUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &resourceTestWriter{
				resource: ResourceRecord{Resource: test.resource, Sealed: catalog.SealedCredential{Ciphertext: []byte("setup-token"), KeyVersion: 1}},
				provider: Provider{
					ID: 40, Code: catalog.AnthropicOfficialCode,
					Endpoints: []ProviderEndpoint{{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"}},
				},
				models: []Model{
					{ID: 90, Code: "claude-sonnet", Status: "ACTIVE"},
					{ID: 91, Code: "claude-haiku", Status: "ACTIVE"},
				},
				mappings: test.mappings,
			}
			calls := 0
			checkedAt := time.Date(2026, 10, 19, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
			service := New(resourceTestStore{writer: writer}, nil, providerTestCipher{}, connectionTesterFunc(
				func(_ context.Context, target ConnectionTarget, credential []byte, _ *catalog.OutboundProxy) ConnectionResult {
					calls++
					if !target.QuotaProbe || target.Protocol != "ANTHROPIC" || target.UpstreamModelCode != "claude-haiku" ||
						target.AuthAdapter != AuthAdapterClaudeCode || string(credential) != "setup-token" {
						t.Fatalf("unexpected quota probe target: %+v", target)
					}
					if test.changeResource {
						writer.resource.Version++
					}
					return test.probe
				},
			), WithSubscriptionAdapter(claudeSubscriptionAdapterStub{}), WithClock(func() time.Time { return checkedAt }))

			result, err := service.RefreshResourceQuota(context.Background(), admin.Identity{ID: 1}, 48, appsec.RequestMeta{})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || calls != 0 {
					t.Fatalf("err=%v calls=%d, want %v", err, calls, test.wantErr)
				}
				return
			}
			if err != nil || calls != test.wantCalls || result.OK != test.wantOK || result.Code != test.wantCode {
				t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
			}
			// 刷新额度只更新额度字段，不恢复或封禁凭据。
			if writer.restored || writer.resource.RuntimeStatus != "BLOCKED" || writer.resource.QuotaStatus != test.wantStatus {
				t.Fatalf("resource=%+v restored=%v", writer.resource.Resource, writer.restored)
			}
			if writer.audit.Event != operation.ResourceQuotaRefresh || (writer.audit.ErrorCode == "") != test.wantOK {
				t.Fatalf("unexpected audit: %+v", writer.audit)
			}
			if !test.wantOK {
				if writer.resource.QuotaCheckedAt != nil || len(writer.quotas) != 0 || result.Quotas != nil {
					t.Fatalf("quota must not be recorded: %+v quotas=%+v", writer.resource.Resource, writer.quotas)
				}
				return
			}
			if result.TestedModelCode != "claude-haiku" || result.ProviderModelMappingID != 2 || len(result.Quotas) != len(test.probe.ObservedQuotas) ||
				writer.resource.QuotaCheckedAt == nil || !writer.resource.QuotaCheckedAt.Equal(checkedAt) || writer.resource.QuotaCheckedAt.Location() != time.UTC ||
				writer.resource.QuotaResetsAt == nil || !writer.resource.QuotaResetsAt.Equal(reset) || len(writer.quotas) != len(test.probe.ObservedQuotas) {
				t.Fatalf("unexpected refresh: result=%+v resource=%+v quotas=%+v", result, writer.resource.Resource, writer.quotas)
			}
		})
	}
}

func TestRefreshResourceQuotaQueriesSubscriptionAdapter(t *testing.T) {
	reset := time.Date(2026, 10, 30, 2, 45, 0, 0, time.UTC)
	used := 12.0
	quotas := []ResourceQuota{{Code: "codex.primary", Status: QuotaAvailable, UsedPercent: &used, ResetsAt: &reset}}
	tests := []struct {
		name           string
		probe          SubscriptionProbe
		err            error
		changeResource bool
		wantOK         bool
		wantCode       string
		wantCiphertext string
	}{
		{
			name:   "records quota and rotated credential",
			probe:  SubscriptionProbe{Quotas: quotas, Credential: []byte("rotated-auth-cache")},
			wantOK: true, wantCode: "OK", wantCiphertext: "rotated-auth-cache",
		},
		{
			name:     "probe without quota keeps snapshot",
			probe:    SubscriptionProbe{},
			wantCode: "UPSTREAM_QUOTA_UNAVAILABLE", wantCiphertext: "imported-auth-cache",
		},
		{
			name:     "failed probe still saves rotated credential",
			err:      &rotatedProbeError{credential: []byte("rotated-auth-cache")},
			wantCode: "SUBSCRIPTION_UNAVAILABLE", wantCiphertext: "rotated-auth-cache",
		},
		{
			name:  "resource changed during refresh",
			probe: SubscriptionProbe{Quotas: quotas}, changeResource: true,
			wantCode: "RESOURCE_CHANGED", wantCiphertext: "imported-auth-cache",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := subscriptionRefreshWriter()
			writer.resource.RuntimeStatus = "BLOCKED"
			events := []string{}
			adapter := failingSubscriptionAdapterStub{
				subscriptionAdapterStub: subscriptionAdapterStub{
					probe: test.probe,
					onProbe: func(*catalog.OutboundProxy) {
						events = append(events, "probe")
						if test.changeResource {
							writer.resource.Version++
						}
					},
				},
				err: test.err,
			}
			var subscription SubscriptionAdapter = adapter.subscriptionAdapterStub
			if test.err != nil {
				subscription = adapter
			}
			checkedAt := time.Date(2026, 10, 24, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
			service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, nil,
				WithSubscriptionAdapter(subscription), WithClock(func() time.Time { return checkedAt }))

			result, err := service.RefreshResourceQuota(context.Background(), admin.Identity{DisplayName: "Member Key #7"}, 48, appsec.RequestMeta{})
			if err != nil || result.OK != test.wantOK || result.Code != test.wantCode {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if got := strings.Join(events, ","); !strings.Contains(got, "lock,read,probe") || !strings.HasSuffix(got, "unlock") {
				t.Fatalf("refresh must hold the subscription lock around probe and write: %s", got)
			}
			if got := string(writer.resource.Sealed.Ciphertext); got != test.wantCiphertext {
				t.Fatalf("stored credential=%q; want %q", got, test.wantCiphertext)
			}
			// 刷新额度不恢复或封禁凭据。
			if writer.restored || writer.resource.RuntimeStatus != "BLOCKED" || writer.audit.Event != operation.ResourceQuotaRefresh {
				t.Fatalf("resource=%+v restored=%v audit=%+v", writer.resource.Resource, writer.restored, writer.audit)
			}
			if !test.wantOK {
				if writer.resource.QuotaCheckedAt != nil || len(writer.quotas) != 0 || writer.audit.ErrorCode != test.wantCode {
					t.Fatalf("quota must not be recorded: %+v quotas=%+v", writer.resource.Resource, writer.quotas)
				}
				return
			}
			if writer.resource.QuotaCheckedAt == nil || !writer.resource.QuotaCheckedAt.Equal(checkedAt) || writer.resource.QuotaCheckedAt.Location() != time.UTC ||
				writer.resource.QuotaStatus != QuotaAvailable || len(writer.quotas) != 1 || len(result.Quotas) != 1 {
				t.Fatalf("unexpected refresh: resource=%+v quotas=%+v", writer.resource.Resource, writer.quotas)
			}
		})
	}
}

// actorRecordingStore 记录每次会话使用的操作者身份。
type actorRecordingStore struct {
	lockingResourceStore
	actors *[]admin.Identity
}

func (s actorRecordingStore) Read(ctx context.Context, actor admin.Identity, fn func(Reader) error) error {
	*s.actors = append(*s.actors, actor)
	return s.lockingResourceStore.Read(ctx, actor, fn)
}
func (s actorRecordingStore) Write(ctx context.Context, actor admin.Identity, fn func(Writer) error) error {
	*s.actors = append(*s.actors, actor)
	return s.lockingResourceStore.Write(ctx, actor, fn)
}

func TestRefreshSubscriptionQuotaRunsAsSystemActor(t *testing.T) {
	writer := subscriptionRefreshWriter()
	events, actors := []string{}, []admin.Identity{}
	reset := time.Date(2026, 10, 30, 2, 45, 0, 0, time.UTC)
	adapter := subscriptionAdapterStub{probe: SubscriptionProbe{Quotas: []ResourceQuota{{Code: "codex.primary", Status: QuotaAvailable, ResetsAt: &reset}}}}
	service := New(actorRecordingStore{lockingResourceStore{writer: writer, events: &events}, &actors}, nil, providerTestCipher{}, nil,
		WithSubscriptionAdapter(adapter))

	code, err := service.RefreshSubscriptionQuota(context.Background(), appsec.PrincipalIdentity{ID: 5, AccessKeyID: 7}, 48, appsec.RequestMeta{})
	if err != nil || code != "OK" || len(actors) == 0 {
		t.Fatalf("code=%s err=%v actors=%+v", code, err, actors)
	}
	for _, actor := range actors {
		if !actor.System || actor.ID != 0 || actor.DisplayName != "Member Key #7" {
			t.Fatalf("refresh must run as the system actor: %+v", actor)
		}
	}
	if _, err := service.RefreshSubscriptionQuota(context.Background(), appsec.PrincipalIdentity{ID: 5}, 48, appsec.RequestMeta{}); !errors.Is(err, appsec.ErrUnauthenticated) {
		t.Fatalf("missing access key must be rejected, got %v", err)
	}
}
