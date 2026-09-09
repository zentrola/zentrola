package management

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type Service struct {
	store      Store
	ids        shared.IDGenerator
	cipher     Cipher
	tester     ConnectionTester
	discoverer ModelDiscoverer
}

type Option func(*Service)

func WithModelDiscoverer(discoverer ModelDiscoverer) Option {
	return func(service *Service) { service.discoverer = discoverer }
}

func New(store Store, ids shared.IDGenerator, cipher Cipher, tester ConnectionTester, options ...Option) *Service {
	service := &Service{store: store, ids: ids, cipher: cipher, tester: tester}
	for _, option := range options {
		option(service)
	}
	return service
}

func validText(s string, max int) bool {
	return utf8.ValidString(s) && s == strings.TrimSpace(s) && s != "" && len(s) <= max && !strings.ContainsRune(s, 0)
}
func validRemark(s string) bool { return s == "" || validText(s, 2000) }
func remark(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func validStatus(s string) bool { return s == "ACTIVE" || s == "DISABLED" }
func validCredential(s string) bool {
	if len(s) == 0 || len(s) > 4096 {
		return false
	}
	for _, ch := range s {
		if ch < 33 || ch > 126 {
			return false
		}
	}
	return true
}
func (s *Service) next(ctx context.Context) (int64, error) {
	id, err := s.ids.NextID(ctx)
	if err != nil {
		return 0, appsec.ErrUnavailable
	}
	return id, nil
}

func (s *Service) CreateMember(ctx context.Context, actor admin.Identity, name, note string, meta appsec.RequestMeta) (Member, error) {
	return s.CreateMemberWithGroups(ctx, actor, name, note, nil, meta)
}
func (s *Service) CreateMemberWithGroups(ctx context.Context, actor admin.Identity, name, note string, groupIDs []int64, meta appsec.RequestMeta) (Member, error) {
	if !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(groupIDs) {
		return Member{}, appsec.ErrInvalidArgument
	}
	id, err := s.next(ctx)
	if err != nil {
		return Member{}, err
	}
	m := Member{ID: id, Name: name, Remark: remark(note), Status: "DISABLED", CreatedAt: time.Now().UTC()}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		groups := make(map[int64]Group, len(groupIDs))
		for _, groupID := range groupIDs {
			group, err := w.Group(ctx, groupID)
			if err != nil {
				return err
			}
			if group.Status != "ACTIVE" {
				return ErrConflict
			}
			groups[groupID] = group
		}
		if err := w.CreateMember(ctx, m); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.MemberCreate, Target: "PRINCIPAL", ID: id, Name: name, After: m}, meta); err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			changed, err := w.SetGroupMember(ctx, groupID, id, true)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			group := groups[groupID]
			if err := w.Audit(ctx, Audit{Event: operation.GroupMemberAdd, Target: "GROUP", ID: groupID, Name: group.Name, Before: map[string]any{"memberId": idString(id), "included": false}, After: map[string]any{"memberId": idString(id), "included": true}}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return m, err
}
func (s *Service) UpdateMemberWithGroups(ctx context.Context, actor admin.Identity, id int64, name, note string, groupIDs []int64, meta appsec.RequestMeta) (Member, error) {
	if id <= 0 || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(groupIDs) {
		return Member{}, appsec.ErrInvalidArgument
	}
	var updated Member
	err := s.store.Write(ctx, actor, func(w Writer) error {
		current, err := w.Member(ctx, id)
		if err != nil {
			return err
		}
		currentGroups, err := w.MemberGroups(ctx, id, Page{Limit: 1<<31 - 1})
		if err != nil {
			return err
		}
		currentIDs := make(map[int64]Group, len(currentGroups))
		for _, group := range currentGroups {
			currentIDs[group.ID] = group
		}
		selectedGroups := make(map[int64]Group, len(groupIDs))
		for _, groupID := range groupIDs {
			group, err := w.Group(ctx, groupID)
			if err != nil {
				return err
			}
			if _, alreadyIncluded := currentIDs[groupID]; !alreadyIncluded && group.Status != "ACTIVE" {
				return ErrConflict
			}
			selectedGroups[groupID] = group
		}

		updated = current
		updated.Name = name
		updated.Remark = remark(note)
		if err := w.UpdateMember(ctx, updated); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.MemberUpdate, Target: "PRINCIPAL", ID: id, Name: name, Before: current, After: updated}, meta); err != nil {
			return err
		}
		for _, group := range currentGroups {
			if _, keep := selectedGroups[group.ID]; keep {
				continue
			}
			changed, err := w.SetGroupMember(ctx, group.ID, id, false)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			if err := w.Audit(ctx, Audit{Event: operation.GroupMemberRemove, Target: "GROUP", ID: group.ID, Name: group.Name, Before: map[string]any{"memberId": idString(id), "included": true}, After: map[string]any{"memberId": idString(id), "included": false}}, meta); err != nil {
				return err
			}
		}
		for _, groupID := range groupIDs {
			if _, exists := currentIDs[groupID]; exists {
				continue
			}
			changed, err := w.SetGroupMember(ctx, groupID, id, true)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			group := selectedGroups[groupID]
			if err := w.Audit(ctx, Audit{Event: operation.GroupMemberAdd, Target: "GROUP", ID: groupID, Name: group.Name, Before: map[string]any{"memberId": idString(id), "included": false}, After: map[string]any{"memberId": idString(id), "included": true}}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}
func (s *Service) SetMemberStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		m, err := w.Member(ctx, id)
		if err != nil {
			return err
		}
		if m.Status == status {
			return nil
		}
		if status == "ACTIVE" {
			keys, err := w.Keys(ctx, id, Page{Limit: 1})
			if err != nil {
				return err
			}
			if len(keys) == 0 {
				return ErrConflict
			}
		}
		if err := w.SetMemberStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.MemberStatusChange, Target: "PRINCIPAL", ID: id, Name: m.Name, Before: map[string]string{"status": m.Status}, After: map[string]string{"status": status}}, meta)
	})
}
func (s *Service) DeleteMember(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		m, err := w.Member(ctx, id)
		if err != nil {
			return err
		}
		if err := w.DeleteMember(ctx, id); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.MemberDelete, Target: "PRINCIPAL", ID: id, Name: m.Name, Before: m, After: map[string]bool{"deleted": true}}, meta)
	})
}
func (s *Service) CreateGroup(ctx context.Context, actor admin.Identity, code, name, note string, meta appsec.RequestMeta) (Group, error) {
	return s.createGroup(ctx, actor, code, name, note, nil, meta)
}
func (s *Service) CreateGroupWithModels(ctx context.Context, actor admin.Identity, code, name, note string, modelIDs []int64, meta appsec.RequestMeta) (Group, error) {
	return s.createGroup(ctx, actor, code, name, note, modelIDs, meta)
}
func (s *Service) createGroup(ctx context.Context, actor admin.Identity, code, name, note string, modelIDs []int64, meta appsec.RequestMeta) (Group, error) {
	if (code != "" && !validText(code, 64)) || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(modelIDs) {
		return Group{}, appsec.ErrInvalidArgument
	}
	id, err := s.next(ctx)
	if err != nil {
		return Group{}, err
	}
	if code == "" {
		code = "group-" + strconv.FormatInt(id, 10)
	}
	g := Group{ID: id, Code: code, Name: name, Remark: remark(note), Status: "ACTIVE", CreatedAt: time.Now().UTC()}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		for _, modelID := range modelIDs {
			model, err := w.Model(ctx, modelID)
			if err != nil {
				return err
			}
			if model.Status != "ACTIVE" {
				return ErrConflict
			}
		}
		if err := w.CreateGroup(ctx, g); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.GroupCreate, Target: "GROUP", ID: id, Name: name, After: g}, meta); err != nil {
			return err
		}
		for _, modelID := range modelIDs {
			changed, err := w.SetGroupModel(ctx, id, modelID, true)
			if err != nil {
				return err
			}
			if !changed {
				return ErrConflict
			}
			if err := w.Audit(ctx, Audit{Event: operation.GroupModelGrant, Target: "GROUP", ID: id, Name: name, Before: map[string]any{"modelId": idString(modelID), "allowed": false}, After: map[string]any{"modelId": idString(modelID), "allowed": true}}, meta); err != nil {
				return err
			}
		}
		return nil
	})
	return g, err
}
func uniquePositiveIDs(ids []int64) bool {
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}
func (s *Service) UpdateGroupWithModels(ctx context.Context, actor admin.Identity, id int64, name, note string, modelIDs []int64, meta appsec.RequestMeta) (Group, error) {
	if id <= 0 || !validText(name, 128) || !validRemark(note) || !uniquePositiveIDs(modelIDs) {
		return Group{}, appsec.ErrInvalidArgument
	}
	var updated Group
	err := s.store.Write(ctx, actor, func(w Writer) error {
		current, err := w.Group(ctx, id)
		if err != nil {
			return err
		}
		currentModels, err := w.GroupModels(ctx, id, Page{Limit: 1<<31 - 1})
		if err != nil {
			return err
		}
		currentIDs := make(map[int64]struct{}, len(currentModels))
		for _, model := range currentModels {
			currentIDs[model.ID] = struct{}{}
		}
		selectedIDs := make(map[int64]struct{}, len(modelIDs))
		for _, modelID := range modelIDs {
			model, err := w.Model(ctx, modelID)
			if err != nil {
				return err
			}
			_, alreadyGranted := currentIDs[modelID]
			if !alreadyGranted && (model.Status != "ACTIVE" || current.Status != "ACTIVE") {
				return ErrConflict
			}
			selectedIDs[modelID] = struct{}{}
		}

		updated = current
		updated.Name = name
		updated.Remark = remark(note)
		if err := w.UpdateGroup(ctx, updated); err != nil {
			return err
		}
		if err := w.Audit(ctx, Audit{Event: operation.GroupUpdate, Target: "GROUP", ID: id, Name: name, Before: current, After: updated}, meta); err != nil {
			return err
		}
		for _, model := range currentModels {
			if _, keep := selectedIDs[model.ID]; keep {
				continue
			}
			if changed, err := w.SetGroupModel(ctx, id, model.ID, false); err != nil {
				return err
			} else if changed {
				if err := w.Audit(ctx, Audit{Event: operation.GroupModelRevoke, Target: "GROUP", ID: id, Name: name, Before: map[string]any{"modelId": idString(model.ID), "allowed": true}, After: map[string]any{"modelId": idString(model.ID), "allowed": false}}, meta); err != nil {
					return err
				}
			}
		}
		for _, modelID := range modelIDs {
			if _, exists := currentIDs[modelID]; exists {
				continue
			}
			if changed, err := w.SetGroupModel(ctx, id, modelID, true); err != nil {
				return err
			} else if changed {
				if err := w.Audit(ctx, Audit{Event: operation.GroupModelGrant, Target: "GROUP", ID: id, Name: name, Before: map[string]any{"modelId": idString(modelID), "allowed": false}, After: map[string]any{"modelId": idString(modelID), "allowed": true}}, meta); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return updated, err
}
func (s *Service) SetGroupStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		group, err := w.Group(ctx, id)
		if err != nil {
			return err
		}
		if group.Status == status {
			return nil
		}
		if err := w.SetGroupStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.GroupStatusChange, Target: "GROUP", ID: id, Name: group.Name, Before: map[string]string{"status": group.Status}, After: map[string]string{"status": status}}, meta)
	})
}
func (s *Service) DeleteGroup(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		group, err := w.Group(ctx, id)
		if err != nil {
			return err
		}
		if err := w.DeleteGroup(ctx, id); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.GroupDelete, Target: "GROUP", ID: id, Name: group.Name, Before: group, After: map[string]bool{"deleted": true}}, meta)
	})
}
func (s *Service) SetGroupMember(ctx context.Context, actor admin.Identity, groupID, memberID int64, add bool, meta appsec.RequestMeta) error {
	if groupID <= 0 || memberID <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		g, err := w.Group(ctx, groupID)
		if err != nil {
			return err
		}
		if _, err := w.Member(ctx, memberID); err != nil {
			return err
		}
		if add && g.Status != "ACTIVE" {
			return ErrConflict
		}
		changed, err := w.SetGroupMember(ctx, groupID, memberID, add)
		if err != nil || !changed {
			return err
		}
		event := operation.GroupMemberRemove
		if add {
			event = operation.GroupMemberAdd
		}
		return w.Audit(ctx, Audit{Event: event, Target: "GROUP", ID: groupID, Name: g.Name, Before: map[string]any{"memberId": idString(memberID), "included": !add}, After: map[string]any{"memberId": idString(memberID), "included": add}}, meta)
	})
}
func (s *Service) SetGroupModel(ctx context.Context, actor admin.Identity, groupID, modelID int64, grant bool, meta appsec.RequestMeta) error {
	if groupID <= 0 || modelID <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		g, err := w.Group(ctx, groupID)
		if err != nil {
			return err
		}
		m, err := w.Model(ctx, modelID)
		if err != nil {
			return err
		}
		if grant && (g.Status != "ACTIVE" || m.Status != "ACTIVE") {
			return ErrConflict
		}
		changed, err := w.SetGroupModel(ctx, groupID, modelID, grant)
		if err != nil || !changed {
			return err
		}
		event := operation.GroupModelRevoke
		if grant {
			event = operation.GroupModelGrant
		}
		return w.Audit(ctx, Audit{Event: event, Target: "GROUP", ID: groupID, Name: g.Name, Before: map[string]any{"modelId": idString(modelID), "allowed": !grant}, After: map[string]any{"modelId": idString(modelID), "allowed": grant}}, meta)
	})
}
func (s *Service) SetModelStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		m, err := w.Model(ctx, id)
		if err != nil {
			return err
		}
		if m.Status == status {
			return nil
		}
		if err := w.SetModelStatus(ctx, id, status); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ModelStatusChange, Target: "MODEL", ID: id, Name: m.Name, Before: map[string]string{"status": m.Status}, After: map[string]string{"status": status}}, meta)
	})
}

func owner(actor admin.Identity, r Resource) catalog.CredentialOwner {
	return catalog.CredentialOwner{OrganizationID: actor.OrganizationID, ProviderID: r.ProviderID, ResourceID: r.ID}
}

func (s *Service) decryptedProviderProxy(provider Provider) (*catalog.OutboundProxy, error) {
	if !provider.ProxyEnabled {
		return nil, nil
	}
	urlBytes, err := s.cipher.DecryptProviderProxy(provider.ProxyURLSealed, proxyOwner(provider.ID, "url"))
	if err != nil {
		return nil, err
	}
	proxy := &catalog.OutboundProxy{URL: string(urlBytes), Headers: map[string]string{}}
	clear(urlBytes)
	if provider.ProxyHeadersSealed.KeyVersion == 0 {
		return proxy, nil
	}
	headerBytes, err := s.cipher.DecryptProviderProxy(provider.ProxyHeadersSealed, proxyOwner(provider.ID, "headers"))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(headerBytes, &proxy.Headers)
	clear(headerBytes)
	if err != nil {
		return nil, err
	}
	return proxy, nil
}
func (s *Service) CreateResource(ctx context.Context, actor admin.Identity, providerID int64, name, credential string, meta appsec.RequestMeta) (Resource, error) {
	if providerID <= 0 || !validText(name, 128) || !validCredential(credential) {
		return Resource{}, appsec.ErrInvalidArgument
	}
	id, err := s.next(ctx)
	if err != nil {
		return Resource{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	r := Resource{ID: id, ProviderID: providerID, Name: name, Status: "ACTIVE", CredentialConfigured: true, CreatedAt: now, UpdatedAt: now}
	plain := []byte(credential)
	defer clear(plain)
	sealed, err := s.cipher.Encrypt(plain, owner(actor, r))
	if err != nil {
		return Resource{}, appsec.ErrUnavailable
	}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		if _, err := w.Provider(ctx, providerID); err != nil {
			return err
		}
		if err := w.CreateResource(ctx, ResourceRecord{Resource: r, Sealed: sealed}); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ResourceCreate, Target: "RESOURCE", ID: id, Name: name, After: r}, meta)
	})
	return r, err
}
func (s *Service) UpdateCredential(ctx context.Context, actor admin.Identity, id int64, credential string, meta appsec.RequestMeta) error {
	if id <= 0 || !validCredential(credential) {
		return appsec.ErrInvalidArgument
	}
	plain := []byte(credential)
	defer clear(plain)
	return s.store.Write(ctx, actor, func(w Writer) error {
		r, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		r.Sealed, err = s.cipher.Encrypt(plain, owner(actor, r.Resource))
		if err != nil {
			return appsec.ErrUnavailable
		}
		r.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if err := w.UpdateResource(ctx, r); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ResourceCredentialUpdate, Target: "RESOURCE", ID: id, Name: r.Name, After: map[string]bool{"credentialConfigured": true}}, meta)
	})
}
func (s *Service) SetResourceStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
		r, err := w.Resource(ctx, id)
		if err != nil {
			return err
		}
		if status == "ACTIVE" {
			p, err := w.Provider(ctx, r.ProviderID)
			if err != nil {
				return err
			}
			if p.Status != "ACTIVE" {
				return ErrProvider
			}
			plain, err := s.cipher.Decrypt(r.Sealed, owner(actor, r.Resource))
			clear(plain)
			if err != nil {
				return ErrCredential
			}
		}
		if r.Status == status {
			return nil
		}
		before := r.Status
		r.Status = status
		r.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if err := w.UpdateResource(ctx, r); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ResourceStatusChange, Target: "RESOURCE", ID: id, Name: r.Name, Before: map[string]string{"status": before}, After: map[string]string{"status": status}}, meta)
	})
}
func (s *Service) TestResource(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) (ConnectionResult, error) {
	if id <= 0 {
		return ConnectionResult{}, appsec.ErrInvalidArgument
	}
	var resource ResourceRecord
	var provider Provider
	err := s.store.Read(ctx, actor, func(r Reader) error {
		var err error
		resource, err = r.Resource(ctx, id)
		if err != nil {
			return err
		}
		provider, err = r.Provider(ctx, resource.ProviderID)
		return err
	})
	if err != nil {
		return ConnectionResult{}, err
	}
	result := ConnectionResult{Code: "PROVIDER_UNAVAILABLE"}
	protocol, baseURL := preferredProviderEndpoint(provider)
	if baseURL != "" {
		plain, err := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
		if err != nil {
			result.Code = "CREDENTIAL_UNRECOVERABLE"
		} else {
			defer clear(plain)
			proxy, proxyErr := s.decryptedProviderProxy(provider)
			if proxyErr != nil {
				result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
			} else {
				result = s.tester.Test(ctx, protocol, baseURL, plain, proxy)
			}
		}
	}
	// 网络调用不占有数据库事务；即使调用方取消，仍尽力保存有界审计记录。
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = s.store.Write(auditCtx, actor, func(w Writer) error {
		current, err := w.Resource(auditCtx, id)
		if err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(resource.UpdatedAt) {
			result.OK = false
			result.Code = "RESOURCE_CHANGED"
		}
		code := ""
		if !result.OK {
			code = result.Code
		}
		return w.Audit(auditCtx, Audit{Event: operation.ResourceConnectionTest, Target: "RESOURCE", ID: id, Name: resource.Name, After: map[string]any{"test": result, "testedUpdatedAt": resource.UpdatedAt}, ErrorCode: code}, meta)
	})
	return result, err
}
