package management

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

var errInvalidProviderProxy = errors.New("invalid provider proxy")

const defaultProviderMappingPriority int32 = 100

var providerProtocols = [...]string{"OPENAI", "ANTHROPIC"}

func (input *ProviderInput) Normalize() {
	input.Name = strings.TrimSpace(input.Name)
	input.Website = strings.TrimSpace(input.Website)
	input.ProxyURL = strings.TrimSpace(input.ProxyURL)
	for index := range input.Endpoints {
		input.Endpoints[index].ProtocolType = strings.TrimSpace(input.Endpoints[index].ProtocolType)
		input.Endpoints[index].BaseURL = strings.TrimSpace(input.Endpoints[index].BaseURL)
	}
	for index := range input.ProxyHeaders {
		input.ProxyHeaders[index].Key = strings.TrimSpace(input.ProxyHeaders[index].Key)
		input.ProxyHeaders[index].Value = strings.TrimSpace(input.ProxyHeaders[index].Value)
	}
	for index := range input.Mappings {
		input.Mappings[index].UpstreamModelCode = strings.TrimSpace(input.Mappings[index].UpstreamModelCode)
	}
}

func (input ProviderInput) Valid() bool {
	provider, ok := providerFromInput(Provider{}, input)
	if !ok || !validProviderMappings(provider, input.Mappings) {
		return false
	}
	if !input.ProxyEnabled {
		return true
	}
	if _, _, ok := normalizeProxyURL(input.ProxyURL); !ok || len(input.ProxyHeaders) > 32 {
		return false
	}
	seen := make(map[string]struct{}, len(input.ProxyHeaders))
	for _, header := range input.ProxyHeaders {
		key := http.CanonicalHeaderKey(header.Key)
		if !validHeaderName(key) {
			return false
		}
		key = strings.ToLower(key)
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		// 空值表示编辑时保留已有密文，是否存在由用例结合当前数据判断。
		if header.Value != "" && !validHeaderValue(header.Value) {
			return false
		}
	}
	return true
}

func optionalURL(raw string, website bool) (*string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	if !validText(raw, 2048) {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, false
	}
	if website {
		if u.Scheme != "https" && u.Scheme != "http" {
			return nil, false
		}
	} else if u.Scheme != "https" {
		return nil, false
	}
	normalized := strings.TrimSuffix(raw, "/")
	return &normalized, true
}

func providerFromInput(current Provider, input ProviderInput) (Provider, bool) {
	if !validText(input.Name, 128) {
		return Provider{}, false
	}
	website, websiteOK := optionalURL(input.Website, true)
	if !websiteOK || len(input.Endpoints) == 0 || len(input.Endpoints) > len(providerProtocols) {
		return Provider{}, false
	}
	seen := make(map[string]struct{}, len(input.Endpoints))
	endpoints := make([]ProviderEndpoint, 0, len(input.Endpoints))
	for _, endpoint := range input.Endpoints {
		baseURL, ok := optionalURL(endpoint.BaseURL, false)
		if !ok || baseURL == nil || !validProviderProtocol(endpoint.ProtocolType) {
			return Provider{}, false
		}
		if _, duplicate := seen[endpoint.ProtocolType]; duplicate {
			return Provider{}, false
		}
		seen[endpoint.ProtocolType] = struct{}{}
		endpoints = append(endpoints, ProviderEndpoint{ProtocolType: endpoint.ProtocolType, BaseURL: *baseURL})
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].ProtocolType < endpoints[j].ProtocolType })
	current.Name = input.Name
	current.Website = website
	current.Endpoints = endpoints
	return current, true
}

func validProviderProtocol(protocol string) bool {
	for _, candidate := range providerProtocols {
		if protocol == candidate {
			return true
		}
	}
	return false
}

func validHeaderName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	const separators = "()<>@,;:\\\"/[]?={} \t"
	for _, ch := range name {
		if ch < 33 || ch > 126 || strings.ContainsRune(separators, ch) {
			return false
		}
	}
	switch strings.ToLower(name) {
	case "host", "connection", "content-length", "proxy-connection", "transfer-encoding", "upgrade":
		return false
	}
	return true
}

func validHeaderValue(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) &&
		!strings.ContainsAny(value, "\r\n\x00")
}

func normalizeProxyURL(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if !validText(raw, 2048) {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.RawQuery != "" || u.Fragment != "" || (u.EscapedPath() != "" && u.EscapedPath() != "/") {
		return "", "", false
	}
	normalized := strings.TrimSuffix(raw, "/")
	displayURL, err := url.Parse(normalized)
	if err != nil {
		return "", "", false
	}
	if displayURL.User != nil {
		if _, hasPassword := displayURL.User.Password(); hasPassword {
			displayURL.User = url.UserPassword("******", "******")
		} else {
			displayURL.User = url.User("******")
		}
	}
	return normalized, displayURL.String(), true
}

func proxyOwner(providerID int64, field string) catalog.ProviderProxyOwner {
	return catalog.ProviderProxyOwner{ProviderID: providerID, Field: field}
}

func configuredProxyHeaders(headers map[string]string) []ProviderProxyHeader {
	names := make([]string, 0, len(headers))
	for key := range headers {
		names = append(names, key)
	}
	sort.Strings(names)
	result := make([]ProviderProxyHeader, 0, len(names))
	for _, key := range names {
		result = append(result, ProviderProxyHeader{Key: key, Configured: true})
	}
	return result
}

func (s *Service) applyProviderProxy(current Provider, input ProviderInput) (Provider, error) {
	if !input.ProxyEnabled {
		current.ProxyEnabled = false
		current.ProxyURL = nil
		current.ProxyHeaders = []ProviderProxyHeader{}
		current.ProxyURLSealed = catalog.SealedCredential{}
		current.ProxyHeadersSealed = catalog.SealedCredential{}
		return current, nil
	}

	proxyURL := strings.TrimSpace(input.ProxyURL)
	if len(input.ProxyHeaders) > 32 {
		return Provider{}, errInvalidProviderProxy
	}
	if current.ProxyEnabled && current.ProxyURL != nil && proxyURL == *current.ProxyURL && current.ProxyURLSealed.KeyVersion > 0 {
		// 管理端回传脱敏展示值表示 URL 未修改，保留原密文。
	} else {
		normalized, display, ok := normalizeProxyURL(proxyURL)
		if !ok {
			return Provider{}, errInvalidProviderProxy
		}
		plain := []byte(normalized)
		sealed, err := s.cipher.EncryptProviderProxy(plain, proxyOwner(current.ID, "url"))
		clear(plain)
		if err != nil {
			return Provider{}, err
		}
		current.ProxyURL = &display
		current.ProxyURLSealed = sealed
	}

	existing := map[string]string{}
	if current.ProxyHeadersSealed.KeyVersion > 0 {
		plain, err := s.cipher.DecryptProviderProxy(current.ProxyHeadersSealed, proxyOwner(current.ID, "headers"))
		if err != nil {
			return Provider{}, err
		}
		err = json.Unmarshal(plain, &existing)
		clear(plain)
		if err != nil {
			return Provider{}, err
		}
	}
	headers := make(map[string]string, len(input.ProxyHeaders))
	for _, item := range input.ProxyHeaders {
		key := http.CanonicalHeaderKey(strings.TrimSpace(item.Key))
		if !validHeaderName(key) {
			return Provider{}, errInvalidProviderProxy
		}
		lowerKey := strings.ToLower(key)
		for existingKey := range headers {
			if strings.ToLower(existingKey) == lowerKey {
				return Provider{}, errInvalidProviderProxy
			}
		}
		value := item.Value
		if value == "" {
			var found bool
			for existingKey, existingValue := range existing {
				if strings.EqualFold(existingKey, key) {
					value, found = existingValue, true
					break
				}
			}
			if !found {
				return Provider{}, errInvalidProviderProxy
			}
		}
		if !validHeaderValue(value) {
			return Provider{}, errInvalidProviderProxy
		}
		headers[key] = value
	}
	current.ProxyEnabled = true
	current.ProxyHeaders = configuredProxyHeaders(headers)
	current.ProxyHeadersSealed = catalog.SealedCredential{}
	if len(headers) > 0 {
		plain, err := json.Marshal(headers)
		if err != nil {
			return Provider{}, err
		}
		sealed, err := s.cipher.EncryptProviderProxy(plain, proxyOwner(current.ID, "headers"))
		clear(plain)
		if err != nil {
			return Provider{}, err
		}
		current.ProxyHeadersSealed = sealed
	}
	return current, nil
}

func validProviderMappings(_ Provider, mappings []ProviderMappingInput) bool {
	seen := make(map[int64]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping.ModelID <= 0 ||
			(mapping.UpstreamModelCode != "" && !validText(mapping.UpstreamModelCode, 128)) ||
			mapping.Priority < 0 || mapping.Priority > 10000 {
			return false
		}
		if _, duplicate := seen[mapping.ModelID]; duplicate {
			return false
		}
		seen[mapping.ModelID] = struct{}{}
	}
	return true
}

func (s *Service) replaceProviderMappings(ctx context.Context, w Writer, provider Provider, inputs []ProviderMappingInput) ([]ProviderMapping, error) {
	current, err := w.ProviderMappings(ctx, provider.ID)
	if err != nil {
		return nil, err
	}
	currentByModel := make(map[int64]ProviderMapping, len(current))
	for _, mapping := range current {
		currentByModel[mapping.ModelID] = mapping
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	desired := make([]ProviderMapping, 0, len(inputs))
	retained := make(map[int64]struct{}, len(inputs))
	validatedModels := make(map[int64]struct{}, len(inputs))
	for _, input := range inputs {
		if _, validated := validatedModels[input.ModelID]; !validated {
			if _, err := w.Model(ctx, input.ModelID); err != nil {
				return nil, err
			}
			validatedModels[input.ModelID] = struct{}{}
		}
		mapping, exists := currentByModel[input.ModelID]
		if !exists {
			id, err := s.next(ctx)
			if err != nil {
				return nil, err
			}
			mapping = ProviderMapping{ID: id, ProviderID: provider.ID, ModelID: input.ModelID, CreatedAt: now}
		}
		mapping.UpstreamModelCode = input.UpstreamModelCode
		mapping.Priority = input.Priority
		if mapping.Priority == 0 {
			mapping.Priority = defaultProviderMappingPriority
		}
		mapping.UpdatedAt = now
		desired = append(desired, mapping)
		retained[mapping.ID] = struct{}{}
	}

	// 先停用并逻辑删除被移除的映射，避免后续创建映射时触发唯一键冲突。
	for _, mapping := range current {
		if _, keep := retained[mapping.ID]; !keep {
			if err := w.DeleteProviderMapping(ctx, provider.ID, mapping.ID, now); err != nil {
				return nil, err
			}
		}
	}
	for _, mapping := range desired {
		if _, exists := currentByModel[mapping.ModelID]; exists {
			if err := w.UpdateProviderMapping(ctx, mapping); err != nil {
				return nil, err
			}
		} else if err := w.CreateProviderMapping(ctx, mapping); err != nil {
			return nil, err
		}
	}
	sort.Slice(desired, func(i, j int) bool { return desired[i].ID < desired[j].ID })
	return desired, nil
}

func (s *Service) CreateProvider(ctx context.Context, actor admin.Identity, input ProviderInput, meta appsec.RequestMeta) (Provider, error) {
	input.Normalize()
	if !input.Valid() {
		return Provider{}, appsec.ErrInvalidArgument
	}
	for _, header := range input.ProxyHeaders {
		if input.ProxyEnabled && header.Value == "" {
			return Provider{}, appsec.ErrInvalidArgument
		}
	}
	id, err := s.next(ctx)
	if err != nil {
		return Provider{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	provider, ok := providerFromInput(Provider{ID: id, Code: "provider-" + strconv.FormatInt(id, 10), Type: "CUSTOM", Status: "DISABLED", CreatedAt: now, UpdatedAt: now}, input)
	if !ok || !validProviderMappings(provider, input.Mappings) {
		return Provider{}, appsec.ErrInvalidArgument
	}
	provider, err = s.applyProviderProxy(provider, input)
	if errors.Is(err, errInvalidProviderProxy) {
		return Provider{}, appsec.ErrInvalidArgument
	}
	if err != nil {
		return Provider{}, appsec.ErrUnavailable
	}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		if err := w.CreateProvider(ctx, provider); err != nil {
			return err
		}
		mappings, err := s.replaceProviderMappings(ctx, w, provider, input.Mappings)
		if err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ProviderCreate, Target: "PROVIDER", ID: id, Name: provider.Name, After: ProviderDetail{Provider: provider, Mappings: mappings}}, meta)
	})
	return s.withProviderCapabilities(provider), err
}

func (s *Service) UpdateProvider(ctx context.Context, actor admin.Identity, id int64, input ProviderInput, meta appsec.RequestMeta) (Provider, error) {
	input.Normalize()
	if id <= 0 || !input.Valid() {
		return Provider{}, appsec.ErrInvalidArgument
	}
	var updated Provider
	err := s.store.Write(ctx, actor, func(w Writer) error {
		current, err := w.Provider(ctx, id)
		if err != nil {
			return err
		}
		var ok bool
		updated, ok = providerFromInput(current, input)
		if !ok || !validProviderMappings(updated, input.Mappings) {
			return appsec.ErrInvalidArgument
		}
		updated, err = s.applyProviderProxy(updated, input)
		if errors.Is(err, errInvalidProviderProxy) {
			return appsec.ErrInvalidArgument
		}
		if err != nil {
			return appsec.ErrUnavailable
		}
		beforeMappings, err := w.ProviderMappings(ctx, id)
		if err != nil {
			return err
		}
		updated.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if err := w.UpdateProvider(ctx, updated); err != nil {
			return err
		}
		mappings, err := s.replaceProviderMappings(ctx, w, updated, input.Mappings)
		if err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ProviderUpdate, Target: "PROVIDER", ID: id, Name: updated.Name, Before: ProviderDetail{Provider: current, Mappings: beforeMappings}, After: ProviderDetail{Provider: updated, Mappings: mappings}}, meta)
	})
	return s.withProviderCapabilities(updated), err
}

func (s *Service) SetProviderStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		provider, err := w.Provider(ctx, id)
		if err != nil {
			return err
		}
		if provider.Status == status {
			return nil
		}
		if status == "ACTIVE" {
			mappings, err := w.ProviderMappings(ctx, id)
			if err != nil {
				return err
			}
			if len(mappings) == 0 {
				return ErrProviderModelMappingRequired
			}
			configured, err := w.ProviderCredentialConfigured(ctx, id)
			if err != nil {
				return err
			}
			if !configured {
				return ErrProviderCredentialRequired
			}
		}
		if err := w.SetProviderStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ProviderStatusChange, Target: "PROVIDER", ID: id, Name: provider.Name, Before: map[string]string{"status": provider.Status}, After: map[string]string{"status": status}}, meta)
	})
}

func (s *Service) DeleteProvider(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		provider, err := w.Provider(ctx, id)
		if err != nil {
			return err
		}
		mappings, err := w.ProviderMappings(ctx, id)
		if err != nil {
			return err
		}
		before := ProviderDetail{Provider: provider, Mappings: mappings}
		if err := w.DeleteProvider(ctx, id, time.Now().UTC().Truncate(time.Microsecond)); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ProviderDelete, Target: "PROVIDER", ID: id, Name: provider.Name, Before: before, After: map[string]bool{"deleted": true}}, meta)
	})
}
