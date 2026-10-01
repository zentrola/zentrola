package management

import (
	"context"
	"slices"
	"strings"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

// ModelService 聚合逻辑模型用例，作为 Service 的嵌入式子服务保留现有调用 API。
// 它只持有模型写入需要的依赖，为继续拆分 management facade 建立稳定边界。
type ModelService struct {
	store ModelStore
	ids   shared.IDGenerator
}

type ModelLookup interface {
	Model(context.Context, int64) (Model, error)
}

type ProviderLookup interface {
	Provider(context.Context, int64) (Provider, error)
}

type ModelSession interface {
	ModelLookup
	ProviderLookup
	CreateModel(context.Context, Model) error
	UpdateModel(context.Context, Model) error
	DeleteModel(context.Context, int64, time.Time) error
	SetModelStatus(context.Context, int64, string) error
	Audit(context.Context, Audit, appsec.RequestMeta) error
}

type ModelStore interface {
	ReadModel(context.Context, admin.Identity, func(ModelLookup) error) error
	WriteModel(context.Context, admin.Identity, func(ModelSession) error) error
}

// modelStoreAdapter 将共享事务会话收窄为模型用例实际需要的端口。
// PostgreSQL 适配器无需为每个子服务复制事务管理代码。
type modelStoreAdapter struct{ Store }

func (s modelStoreAdapter) ReadModel(ctx context.Context, actor admin.Identity, fn func(ModelLookup) error) error {
	return s.Read(ctx, actor, func(reader Reader) error { return fn(reader) })
}

func (s modelStoreAdapter) WriteModel(ctx context.Context, actor admin.Identity, fn func(ModelSession) error) error {
	return s.Write(ctx, actor, func(writer Writer) error { return fn(writer) })
}

func (s *ModelService) next(ctx context.Context) (int64, error) {
	return nextID(ctx, s.ids)
}

func (input *ModelInput) Normalize() {
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Remark = strings.TrimSpace(input.Remark)
	for index := range input.InputModalities {
		input.InputModalities[index] = strings.TrimSpace(input.InputModalities[index])
	}
	for index := range input.OutputModalities {
		input.OutputModalities[index] = strings.TrimSpace(input.OutputModalities[index])
	}
}

func (input ModelInput) Valid() bool { return validModelInput(input) }

func validModelInput(input ModelInput) bool {
	return validText(input.Code, 128) && validText(input.Name, 128) &&
		(input.PublisherProviderID == nil || *input.PublisherProviderID > 0) &&
		validRemark(input.Remark) &&
		catalog.ValidModalities(input.InputModalities) && catalog.ValidModalities(input.OutputModalities)
}

func applyModelInput(m Model, input ModelInput) Model {
	m.Code, m.Name, m.Remark = input.Code, input.Name, input.Remark
	m.InputModalities, m.OutputModalities = slices.Clone(input.InputModalities), slices.Clone(input.OutputModalities)
	m.PublisherProviderID, m.PublisherProviderName = nil, nil
	if input.PublisherProviderID != nil {
		publisherID := *input.PublisherProviderID
		m.PublisherProviderID = &publisherID
	}
	return m
}

func hydrateModelPublisher(ctx context.Context, w ProviderLookup, model *Model) error {
	if model.PublisherProviderID == nil {
		return nil
	}
	provider, err := w.Provider(ctx, *model.PublisherProviderID)
	if err != nil {
		return err
	}
	publisherName := provider.Name
	model.PublisherProviderName = &publisherName
	return nil
}

func (s *ModelService) CreateModel(ctx context.Context, actor admin.Identity, input ModelInput, meta appsec.RequestMeta) (Model, error) {
	input.Normalize()
	if !input.Valid() {
		return Model{}, appsec.ErrInvalidArgument
	}
	id, err := s.next(ctx)
	if err != nil {
		return Model{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	model := applyModelInput(Model{ID: id, Status: "DISABLED", CreatedAt: now, UpdatedAt: now}, input)
	err = s.store.WriteModel(ctx, actor, func(w ModelSession) error {
		if err := hydrateModelPublisher(ctx, w, &model); err != nil {
			return err
		}
		if err := w.CreateModel(ctx, model); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ModelCreate, Target: "MODEL", ID: id, Name: model.Name, After: model}, meta)
	})
	return model, err
}

func (s *ModelService) UpdateModel(ctx context.Context, actor admin.Identity, id int64, input ModelInput, meta appsec.RequestMeta) (Model, error) {
	input.Normalize()
	if id <= 0 || !input.Valid() {
		return Model{}, appsec.ErrInvalidArgument
	}
	var model Model
	err := s.store.WriteModel(ctx, actor, func(w ModelSession) error {
		before, err := w.Model(ctx, id)
		if err != nil {
			return err
		}
		model = applyModelInput(before, input)
		model.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if err := hydrateModelPublisher(ctx, w, &model); err != nil {
			return err
		}
		if err := w.UpdateModel(ctx, model); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ModelUpdate, Target: "MODEL", ID: id, Name: model.Name, Before: before, After: model}, meta)
	})
	return model, err
}

func (s *ModelService) DeleteModel(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteModel(ctx, actor, func(w ModelSession) error {
		model, err := w.Model(ctx, id)
		if err != nil {
			return err
		}
		if err := w.DeleteModel(ctx, id, time.Now().UTC().Truncate(time.Microsecond)); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ModelDelete, Target: "MODEL", ID: id, Name: model.Name, Before: model, After: map[string]bool{"deleted": true}}, meta)
	})
}

func (s *ModelService) Model(ctx context.Context, actor admin.Identity, id int64) (Model, error) {
	if id <= 0 {
		return Model{}, appsec.ErrInvalidArgument
	}
	var model Model
	err := s.store.ReadModel(ctx, actor, func(r ModelLookup) error {
		var err error
		model, err = r.Model(ctx, id)
		return err
	})
	return model, err
}

func (s *ModelService) SetModelStatus(ctx context.Context, actor admin.Identity, id int64, status string, meta appsec.RequestMeta) error {
	if id <= 0 || !validStatus(status) {
		return appsec.ErrInvalidArgument
	}
	return s.store.WriteModel(ctx, actor, func(w ModelSession) error {
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
	return catalog.CredentialOwner{ProviderID: r.ProviderID, ResourceID: r.ID}
}
