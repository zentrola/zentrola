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
)

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

func hydrateModelPublisher(ctx context.Context, w Writer, model *Model) error {
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

func (s *Service) CreateModel(ctx context.Context, actor admin.Identity, input ModelInput, meta appsec.RequestMeta) (Model, error) {
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
	err = s.store.Write(ctx, actor, func(w Writer) error {
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

func (s *Service) UpdateModel(ctx context.Context, actor admin.Identity, id int64, input ModelInput, meta appsec.RequestMeta) (Model, error) {
	input.Normalize()
	if id <= 0 || !input.Valid() {
		return Model{}, appsec.ErrInvalidArgument
	}
	var model Model
	err := s.store.Write(ctx, actor, func(w Writer) error {
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

func (s *Service) DeleteModel(ctx context.Context, actor admin.Identity, id int64, meta appsec.RequestMeta) error {
	if id <= 0 {
		return appsec.ErrInvalidArgument
	}
	return s.store.Write(ctx, actor, func(w Writer) error {
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

func (s *Service) Model(ctx context.Context, actor admin.Identity, id int64) (Model, error) {
	if id <= 0 {
		return Model{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, actor, func(r Reader) (Model, error) { return r.Model(ctx, id) })
}
