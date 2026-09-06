package management

import (
	"context"
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
	store  Store
	ids    shared.IDGenerator
	cipher Cipher
	tester ConnectionTester
}

func New(store Store, ids shared.IDGenerator, cipher Cipher, tester ConnectionTester) *Service {
	return &Service{store: store, ids: ids, cipher: cipher, tester: tester}
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
func (s *Service) next() (int64, error) {
	id, err := s.ids.NextID()
	if err != nil {
		return 0, appsec.ErrUnavailable
	}
	return id, nil
}

func (s *Service) CreateMember(ctx context.Context, actor admin.Identity, name, note string, meta appsec.RequestMeta) (Member, error) {
	if !validText(name, 128) || !validRemark(note) {
		return Member{}, appsec.ErrInvalidArgument
	}
	id, err := s.next()
	if err != nil {
		return Member{}, err
	}
	m := Member{ID: id, Name: name, Remark: remark(note), Status: "ACTIVE", CreatedAt: time.Now().UTC()}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		if err := w.CreateMember(ctx, m); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.MemberCreate, Target: "PRINCIPAL", ID: id, Name: name, After: m}, meta)
	})
	return m, err
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
	if !validText(code, 64) || !validText(name, 128) || !validRemark(note) {
		return Group{}, appsec.ErrInvalidArgument
	}
	id, err := s.next()
	if err != nil {
		return Group{}, err
	}
	g := Group{ID: id, Code: code, Name: name, Remark: remark(note), Status: "ACTIVE", CreatedAt: time.Now().UTC()}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		if err := w.CreateGroup(ctx, g); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.GroupCreate, Target: "GROUP", ID: id, Name: name, After: g}, meta)
	})
	return g, err
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
		m, err := w.Member(ctx, memberID)
		if err != nil {
			return err
		}
		if add && (g.Status != "ACTIVE" || m.Status != "ACTIVE") {
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
func (s *Service) CreateResource(ctx context.Context, actor admin.Identity, providerID int64, name, credential string, meta appsec.RequestMeta) (Resource, error) {
	if providerID <= 0 || !validText(name, 128) || !validCredential(credential) {
		return Resource{}, appsec.ErrInvalidArgument
	}
	id, err := s.next()
	if err != nil {
		return Resource{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	r := Resource{ID: id, ProviderID: providerID, Name: name, Status: "DISABLED", CredentialConfigured: true, CreatedAt: now, UpdatedAt: now}
	plain := []byte(credential)
	defer clear(plain)
	sealed, err := s.cipher.Encrypt(plain, owner(actor, r))
	if err != nil {
		return Resource{}, appsec.ErrUnavailable
	}
	err = s.store.Write(ctx, actor, func(w Writer) error {
		p, err := w.Provider(ctx, providerID)
		if err != nil {
			return err
		}
		if p.Status != "ACTIVE" {
			return ErrProvider
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
	if provider.Status == "ACTIVE" {
		plain, err := s.cipher.Decrypt(resource.Sealed, owner(actor, resource.Resource))
		if err != nil {
			result.Code = "CREDENTIAL_UNRECOVERABLE"
		} else {
			defer clear(plain)
			result = s.tester.Test(ctx, provider.BaseURL, plain)
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
