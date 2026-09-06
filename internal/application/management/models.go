package management

import (
	"context"
	"slices"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/operation"
)

func validModelInput(input ModelInput) bool {
	return validText(input.Code, 128) && validText(input.Name, 128) &&
		validRemark(input.Remark) &&
		catalog.ValidModalities(input.InputModalities) && catalog.ValidModalities(input.OutputModalities)
}

func applyModelInput(m Model, input ModelInput) Model {
	m.Code, m.Name, m.Remark = input.Code, input.Name, input.Remark
	m.InputModalities, m.OutputModalities = slices.Clone(input.InputModalities), slices.Clone(input.OutputModalities)
	return m
}

func (s *Service) CreateModel(ctx context.Context, actor admin.Identity, input ModelInput, meta appsec.RequestMeta) (Model, error) {
	if !validModelInput(input) {
		return Model{}, appsec.ErrInvalidArgument
	}
	id, err := s.next()
	if err != nil {
		return Model{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	model := applyModelInput(Model{ID: id, Status: "DISABLED", CreatedAt: now, UpdatedAt: now}, input)
	err = s.store.Write(ctx, actor, func(w Writer) error {
		if err := w.CreateModel(ctx, model); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ModelCreate, Target: "MODEL", ID: id, Name: model.Name, After: model}, meta)
	})
	return model, err
}

func (s *Service) UpdateModel(ctx context.Context, actor admin.Identity, id int64, input ModelInput, meta appsec.RequestMeta) (Model, error) {
	if id <= 0 || !validModelInput(input) {
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
		if err := w.UpdateModel(ctx, model); err != nil {
			return err
		}
		return w.Audit(ctx, Audit{Event: operation.ModelUpdate, Target: "MODEL", ID: id, Name: model.Name, Before: before, After: model}, meta)
	})
	return model, err
}

func (s *Service) Model(ctx context.Context, actor admin.Identity, id int64) (Model, error) {
	if id <= 0 {
		return Model{}, appsec.ErrInvalidArgument
	}
	return read(ctx, s, actor, func(r Reader) (Model, error) { return r.Model(ctx, id) })
}
