package management

import (
	"context"
	"errors"
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
		{
			name: "Codex subscription resource", mappings: mappings,
			resource: Resource{ID: 48, ProviderID: 40, Version: 3, AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterOpenAICodex},
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
