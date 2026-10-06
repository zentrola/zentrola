package postgres

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres/dbgen"
)

func memberView(r dbgen.Principal) mgmt.Member {
	return mgmt.Member{ID: r.ID, Name: r.Name, Remark: r.Remark, Status: r.Status, CreatedAt: r.CreatedAt.Time.UTC()}
}

func groupView(r dbgen.PrincipalGroup) mgmt.Group {
	return mgmt.Group{ID: r.ID, Code: r.GroupCode, Name: r.GroupName, Remark: r.Remark, Status: r.Status, CreatedAt: r.CreatedAt.Time.UTC()}
}

func modelView(id int64, code, name, status string, inputJSON, outputJSON []byte, remark string, publisherProviderID *int64, publisherProviderName *string, createdAt, updatedAt time.Time) mgmt.Model {
	// 数组格式由数据库 CHECK 保证；响应只暴露业务字段。
	input, output := []string{}, []string{}
	_ = json.Unmarshal(inputJSON, &input)
	_ = json.Unmarshal(outputJSON, &output)
	return mgmt.Model{
		ID: id, Code: code, Name: name, Status: status,
		InputModalities: input, OutputModalities: output, Remark: remark,
		PublisherProviderID: publisherProviderID, PublisherProviderName: publisherProviderName,
		CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC(),
	}
}

func providerView(r dbgen.Provider) mgmt.Provider {
	names := []string{}
	_ = json.Unmarshal(r.ProxyHeaderNames, &names)
	headers := make([]mgmt.ProviderProxyHeader, 0, len(names))
	for _, name := range names {
		headers = append(headers, mgmt.ProviderProxyHeader{Key: name, Configured: true})
	}
	provider := mgmt.Provider{
		ID: r.ID, Code: r.ProviderCode, Name: r.ProviderName, Type: r.ProviderType,
		Website: r.OfficialWebsite, Endpoints: []mgmt.ProviderEndpoint{},
		ProxyEnabled: r.ProxyEnabled, ProxyURL: r.ProxyUrlDisplay, ProxyHeaders: headers,
		Status: r.Status, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.ProxyUrlKeyVersion != nil {
		provider.ProxyURLSealed = catalog.SealedCredential{Ciphertext: r.ProxyUrlCiphertext, Nonce: r.ProxyUrlNonce, KeyVersion: *r.ProxyUrlKeyVersion}
	}
	if r.ProxyHeadersKeyVersion != nil {
		provider.ProxyHeadersSealed = catalog.SealedCredential{Ciphertext: r.ProxyHeadersCiphertext, Nonce: r.ProxyHeadersNonce, KeyVersion: *r.ProxyHeadersKeyVersion}
	}
	return provider
}

func providerHeaderNames(p mgmt.Provider) ([]byte, error) {
	names := make([]string, 0, len(p.ProxyHeaders))
	for _, header := range p.ProxyHeaders {
		names = append(names, header.Key)
	}
	return json.Marshal(names)
}

func sealedVersion(sealed catalog.SealedCredential) *int32 {
	if sealed.KeyVersion <= 0 {
		return nil
	}
	version := sealed.KeyVersion
	return &version
}

func providerMappingView(r dbgen.ProviderModel) mgmt.ProviderMapping {
	return mgmt.ProviderMapping{ID: r.ID, ProviderID: r.ProviderID, ModelID: r.ModelID, UpstreamModelCode: r.UpstreamModelCode, Priority: r.Priority, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC()}
}

func endpointView(r dbgen.ProviderEndpoint) mgmt.ProviderEndpoint {
	return mgmt.ProviderEndpoint{ProtocolType: r.ProtocolType, BaseURL: r.BaseUrl, NetworkScope: r.NetworkScope}
}

func resourceView(r dbgen.ManageResourcesRow) mgmt.Resource {
	resource := mgmt.Resource{
		ID: r.ID, ProviderID: r.ProviderID, Name: r.ResourceName,
		AuthType: r.AuthType, AuthAdapter: r.AuthAdapter, SubscriptionType: r.SubscriptionType,
		PlanCode: r.PlanCode, ExternalAccountRef: r.ExternalAccountRef, Priority: r.Priority,
		EffectiveAt: timePointer(r.EffectiveAt), ExpiresAt: timePointer(r.ExpiresAt),
		QuotaStatus: r.QuotaStatus, QuotaCheckedAt: timePointer(r.QuotaCheckedAt), QuotaResetsAt: timePointer(r.QuotaResetsAt),
		CredentialRefreshedAt: timePointer(r.CredentialRefreshedAt), CredentialExpiresAt: timePointer(r.CredentialExpiresAt),
		RuntimeStatus: r.RuntimeStatus, BlockedReason: r.BlockedReason,
		BlockedAt: timePointer(r.BlockedAt), LastErrorAt: timePointer(r.LastErrorAt),
		LastHTTPStatus: r.LastHttpStatus, LastErrorCode: r.LastErrorCode,
		CredentialConfigured: true, Version: r.Version,
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
	if r.SubscriptionPriceCurrency != "" && r.SubscriptionPeriodAmount.Valid &&
		r.SubscriptionBillingPeriod != "" && r.SubscriptionPriceEffectiveAt.Valid {
		resource.SubscriptionPrice = &mgmt.SubscriptionPriceSummary{
			Currency: r.SubscriptionPriceCurrency, PeriodAmount: priceNumber(r.SubscriptionPeriodAmount),
			BillingPeriod: r.SubscriptionBillingPeriod,
			EffectiveAt:   r.SubscriptionPriceEffectiveAt.Time.UTC(),
		}
	}
	return resource
}

func keyView(r dbgen.ManageKeysRow) mgmt.Key {
	return mgmt.Key{ID: r.ID, Name: r.Name, MaskedKey: r.MaskedKey, Status: r.Status, ExpiresAt: timePointer(r.ExpiresAt), RevokedAt: timePointer(r.RevokedAt), CreatedAt: r.CreatedAt.Time.UTC()}
}

func operationView(r dbgen.ManageOperationsRow) mgmt.Operation {
	return mgmt.Operation{ID: r.ID, OperatorName: r.OperatorName, Type: r.OperationType, TargetType: r.TargetType, TargetID: r.TargetID, TargetName: r.TargetName, RequestID: r.RequestID, Result: r.Result, ErrorCode: r.ErrorCode, Before: r.BeforeData, After: r.AfterData, CreatedAt: r.CreatedAt.Time.UTC()}
}

func managementPageLimit(p mgmt.Page) int32 {
	if p.ProbeNext {
		return p.Limit + 1
	}
	return p.Limit
}

func pgNumeric(value *string) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	var numeric pgtype.Numeric
	if numeric.Scan(*value) != nil {
		return pgtype.Numeric{}
	}
	return numeric
}

func numericString(value pgtype.Numeric) *string {
	if !value.Valid {
		return nil
	}
	raw, err := value.Value()
	if err != nil {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return nil
	}
	return &text
}

func pgFloat(value *float64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	text := strconv.FormatFloat(*value, 'f', -1, 64)
	return pgNumeric(&text)
}

func numericFloat(value pgtype.Numeric) *float64 {
	text := numericString(value)
	if text == nil {
		return nil
	}
	parsed, err := strconv.ParseFloat(*text, 64)
	if err != nil {
		return nil
	}
	return &parsed
}
